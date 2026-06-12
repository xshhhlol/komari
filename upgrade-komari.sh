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

# 停服务 -> 备份 -> 替换 -> 启动
backup="${BINARY_PATH}.bak.$(date +%Y%m%d_%H%M%S)"
info "停止服务..."
systemctl stop "${SERVICE_NAME}.service" 2>/dev/null || true
info "备份旧二进制 -> ${backup}"
cp -f "$BINARY_PATH" "$backup"
info "替换二进制..."
cp -f "$tmp" "$BINARY_PATH"
chmod +x "$BINARY_PATH"
info "启动服务..."
systemctl start "${SERVICE_NAME}.service"

sleep 2
if systemctl is-active --quiet "${SERVICE_NAME}.service"; then
  ok "✅ 升级成功，服务运行中。"
  info "如需回滚：systemctl stop ${SERVICE_NAME} && cp '${backup}' '${BINARY_PATH}' && systemctl start ${SERVICE_NAME}"
else
  err "❌ 服务启动失败，正在回滚到备份..."
  cp -f "$backup" "$BINARY_PATH"
  systemctl start "${SERVICE_NAME}.service" || true
  err "已回滚。请查看日志排查：journalctl -u ${SERVICE_NAME} -n 50 --no-pager"
  exit 1
fi
