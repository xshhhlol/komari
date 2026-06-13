#!/usr/bin/env bash
#
# Komari 一键升级脚本（适用于 systemd + 二进制安装）
#
# 功能：自动识别架构 -> 从 GitHub Release 拉取最新二进制 -> 备份旧版本
#       -> 替换 -> 重启服务；启动失败自动回滚到备份。
#
# 用法：
#   curl -fsSL https://raw.githubusercontent.com/xshhhlol/komari/main/upgrade-komari.sh | sudo bash
#   或下载后：sudo bash upgrade-komari.sh
#
# 可选环境变量：
#   KOMARI_REPO=xshhhlol/komari      拉取 Release 的仓库（默认本 fork）
#   KOMARI_INSTALL_DIR=/opt/komari   安装目录
#   KOMARI_SERVICE=komari            systemd 服务名
#   KOMARI_TAG=latest                指定 Release 标签（默认 latest）

set -uo pipefail

REPO="${KOMARI_REPO:-xshhhlol/komari}"
INSTALL_DIR="${KOMARI_INSTALL_DIR:-/opt/komari}"
SERVICE_NAME="${KOMARI_SERVICE:-komari}"
TAG="${KOMARI_TAG:-latest}"
BINARY_PATH="${INSTALL_DIR}/komari"

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[0;33m'; NC='\033[0m'
info() { echo -e "$1"; }
ok()   { echo -e "${GREEN}$1${NC}"; }
warn() { echo -e "${YELLOW}$1${NC}"; }
err()  { echo -e "${RED}$1${NC}"; }

# 需要 root
if [ "$(id -u)" -ne 0 ]; then
  err "请使用 root 权限运行（sudo bash upgrade-komari.sh）"
  exit 1
fi

# 需要 systemd
if ! command -v systemctl >/dev/null 2>&1; then
  err "未检测到 systemd，无法管理服务。"
  exit 1
fi

# 校验服务与二进制
if ! systemctl list-unit-files 2>/dev/null | grep -q "^${SERVICE_NAME}\.service"; then
  warn "未找到服务 ${SERVICE_NAME}.service；若服务名不同，请用 KOMARI_SERVICE=xxx 指定。"
fi
if [ ! -f "$BINARY_PATH" ]; then
  err "未找到二进制 ${BINARY_PATH}；若安装目录不同，请用 KOMARI_INSTALL_DIR=/path 指定。"
  exit 1
fi

# 识别架构
arch_raw="$(uname -m)"
case "$arch_raw" in
  x86_64|amd64)  arch="amd64" ;;
  aarch64|arm64) arch="arm64" ;;
  i386|i686)     arch="386" ;;
  riscv64)       arch="riscv64" ;;
  *) err "不支持的架构: ${arch_raw}"; exit 1 ;;
esac

if [ "$TAG" = "latest" ]; then
  URL="https://github.com/${REPO}/releases/latest/download/komari-linux-${arch}"
else
  URL="https://github.com/${REPO}/releases/download/${TAG}/komari-linux-${arch}"
fi

info "仓库  : ${REPO}"
info "架构  : ${arch}"
info "服务  : ${SERVICE_NAME}"
info "二进制: ${BINARY_PATH}"
info "下载  : ${URL}"

# 先下载到临时文件，确认有效再替换
tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT
info "正在下载最新二进制..."
if ! curl -fL --retry 3 -o "$tmp" "$URL"; then
  err "下载失败：${URL}"
  err "请确认该仓库已发布对应架构的 Release 资产。"
  exit 1
fi

# 体积校验，避免把错误页面（HTML）当成二进制
size="$(wc -c < "$tmp")"
if [ "$size" -lt 1000000 ]; then
  err "下载文件过小（${size} 字节），可能不是有效二进制，已中止。"
  exit 1
fi
chmod +x "$tmp"

# ---------------------------------------------------------------------------
# 关键升级区段（停服务 -> 备份 -> 替换 -> 启动 -> 健康检查 -> 失败回滚）放进
# 一个「与当前终端会话解耦」的独立进程执行。
#
# 原因：若本脚本通过 Komari 网页终端运行，重启 komari 会切断「浏览器↔komari↔
#       agent↔shell」这条中继；连接一断，本脚本（终端会话的子进程）会被一起
#       杀掉，导致「停了服务却没装上新版本」而升级失败、面板长期掉线。
# 做法：把关键区段写入临时脚本，优先用 systemd-run 放进独立 cgroup 运行；即便
#       当前连接断开，升级也能继续跑完。下载/校验仍在前台执行，便于看到错误。
# ---------------------------------------------------------------------------
runner="$(mktemp /tmp/komari-upgrade-runner.XXXXXX.sh)"
cat > "$runner" <<RUNNER
#!/usr/bin/env bash
set -uo pipefail
SERVICE_NAME='${SERVICE_NAME}'
BINARY_PATH='${BINARY_PATH}'
NEW_BIN='${tmp}'
RUNNER_SELF='${runner}'
# 稍等，让前台提示信息先送达网页终端，再开始停服务（停服务会断开连接）
sleep 1
backup="\${BINARY_PATH}.bak.\$(date +%Y%m%d_%H%M%S)"
echo "停止服务..."
systemctl stop "\${SERVICE_NAME}.service" 2>/dev/null || true
echo "备份旧二进制 -> \${backup}"
cp -f "\${BINARY_PATH}" "\${backup}"
echo "替换二进制..."
cp -f "\${NEW_BIN}" "\${BINARY_PATH}"
chmod +x "\${BINARY_PATH}"
echo "启动服务..."
systemctl start "\${SERVICE_NAME}.service"
sleep 2
if systemctl is-active --quiet "\${SERVICE_NAME}.service"; then
  echo "✅ 升级成功，服务运行中。"
  echo "如需回滚：systemctl stop \${SERVICE_NAME} && cp '\${backup}' '\${BINARY_PATH}' && systemctl start \${SERVICE_NAME}"
else
  echo "❌ 服务启动失败，正在回滚到备份..."
  cp -f "\${backup}" "\${BINARY_PATH}"
  systemctl start "\${SERVICE_NAME}.service" || true
  echo "已回滚。请查看日志排查：journalctl -u \${SERVICE_NAME} -n 50 --no-pager"
fi
rm -f "\${NEW_BIN}" "\${RUNNER_SELF}"
RUNNER
chmod +x "$runner"

# 下载已完成，临时二进制交由分离进程接管，撤销本进程的清理陷阱
trap - EXIT

if [ "${KOMARI_NO_DETACH:-0}" = "1" ]; then
  # 显式要求内联执行（普通 SSH 下想看完整输出时用）
  info "以内联模式执行升级（KOMARI_NO_DETACH=1）..."
  bash "$runner"
  exit $?
fi

if command -v systemd-run >/dev/null 2>&1; then
  systemctl reset-failed komari-upgrade >/dev/null 2>&1 || true
  if systemd-run --unit=komari-upgrade --collect bash "$runner" >/dev/null 2>&1; then
    ok "✅ 升级已在后台独立进程启动（systemd 瞬态单元 komari-upgrade）。"
    info "本终端连接随后会断开（重启服务所致），属正常现象，升级会继续完成。"
    info "查看进度：journalctl -u komari-upgrade -f"
    exit 0
  fi
  warn "systemd-run 启动失败，回退到 setsid 后台执行..."
fi

# 回退方案：脱离当前会话与控制终端
setsid bash "$runner" >/var/log/komari-upgrade.log 2>&1 </dev/null &
ok "✅ 升级已在后台运行（setsid）。"
info "本终端连接随后可能断开，属正常现象，升级会继续完成。"
info "查看进度：tail -f /var/log/komari-upgrade.log"
exit 0
