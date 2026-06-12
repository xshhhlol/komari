# Komari

![Badge](https://hitscounter.dev/api/hit?url=https%3A%2F%2Fgithub.com%2Fkomari-monitor%2Fkomari&label=&icon=github&color=%23a370f7&message=&style=flat&tz=UTC)

![komari](https://socialify.git.ci/komari-monitor/komari/image?description=1&font=Inter&forks=1&issues=1&language=1&logo=https%3A%2F%2Fraw.githubusercontent.com%2Fkomari-monitor%2Fkomari-web%2Fd54ce1288df41ead08aa19f8700186e68028a889%2Fpublic%2Ffavicon.png&name=1&owner=1&pattern=Plus&pulls=1&stargazers=1&theme=Auto)

Komari 是一款轻量级的自托管服务器监控工具，旨在提供简单、高效的服务器性能监控解决方案。它支持通过 Web 界面查看服务器状态，并通过轻量级 Agent 收集数据。

[文档](https://komari-document.pages.dev/) | [文档(镜像站 By Geekertao)](https://www.komari.wiki) | [Telegram 群组](https://t.me/komari_monitor)

## 特性

- **轻量高效**：低资源占用，适合各种规模的服务器。
- **自托管**：完全掌控数据隐私，部署简单。
- **Web 界面**：直观的监控仪表盘，易于使用。

## 快速开始

### 0. 容器云一键部署

- 雨云云应用 - CNY 4.5/月

[![](https://rainyun-apps.cn-nb1.rains3.com/materials/deploy-on-rainyun-cn.svg)](https://app.rainyun.com/apps/rca/store/6780/NzYxNzAz_)

- 1Panel 应用商店

已上架1Panel应用商店，应用商店-实用工具-Komari 即可安装

### 1. 使用一键安装脚本

适用于使用了 systemd 的发行版（Ubuntu、Debian...）。

```bash
curl -fsSL https://raw.githubusercontent.com/komari-monitor/komari/main/install-komari.sh -o install-komari.sh
chmod +x install-komari.sh
sudo ./install-komari.sh
```

### 2. Docker 部署

1. 创建数据目录：
   ```bash
   mkdir -p ./data
   ```
2. 运行 Docker 容器：
   ```bash
   docker run -d \
     -p 25774:25774 \
     -v $(pwd)/data:/app/data \
     --name komari \
     ghcr.io/komari-monitor/komari:latest
   ```
3. 查看默认账号和密码：
   ```bash
   docker logs komari
   ```
4. 在浏览器中访问 `http://<your_server_ip>:25774`。

> [!NOTE]
> 你也可以通过环境变量 `ADMIN_USERNAME` 和 `ADMIN_PASSWORD` 自定义初始用户名和密码。

### 3. 二进制文件部署

1. 访问 Komari 的 [GitHub Release 页面](https://github.com/komari-monitor/komari/releases) 下载适用于你操作系统的最新二进制文件。
2. 运行 Komari：
   ```bash
   ./komari server -l 0.0.0.0:25774
   ```
3. 在浏览器中访问 `http://<your_server_ip>:25774`，默认监听 `25774` 端口。
4. 默认账号和密码可在启动日志中查看，或通过环境变量 `ADMIN_USERNAME` 和 `ADMIN_PASSWORD` 设置。

> [!NOTE]
> 确保二进制文件具有可执行权限（`chmod +x komari`）。数据将保存在运行目录下的 `data` 文件夹中。

### 手工构建

#### 依赖

- Go 1.18+ 和 Node.js 20+（手工构建）

1. 构建前端静态文件：
   ```bash
   git clone https://github.com/komari-monitor/komari-web
   cd komari-web
   npm install
   npm run build
   ```
2. 构建后端：
   ```bash
   git clone https://github.com/komari-monitor/komari
   cd komari
   ```
   将步骤1中生成的静态文件复制到 `komari` 项目根目录下的 `/web/public/defaultTheme/dist` 文件夹，并将 `komari-theme.json` 与 `preview.png`/`perview.png` 复制到 `/web/public/defaultTheme`。
   ```bash
   go build -o komari
   ```
3. 运行：
   ```bash
   ./komari server -l 0.0.0.0:25774
   ```
   默认监听 `25774` 端口，访问 `http://localhost:25774`。

## 升级

升级只替换 Komari 程序本身，你的数据——数据目录（或 Docker 数据卷），包括 `komari.db` 和所有设置——都会保留。升级前建议先备份数据以防万一。

### 1. 一键安装脚本（systemd）

重新运行安装脚本，在菜单中选择 `2) 升级 Komari`。脚本会自动停止服务、备份当前二进制、下载最新版本并重启。

```bash
curl -fsSL https://raw.githubusercontent.com/komari-monitor/komari/main/install-komari.sh -o install-komari.sh
chmod +x install-komari.sh
sudo ./install-komari.sh
```

### 2. Docker

拉取最新镜像并重建容器。数据卷会保留，不会丢失数据。

```bash
docker pull ghcr.io/komari-monitor/komari:latest
docker stop komari && docker rm komari
docker run -d \
  -p 25774:25774 \
  -v $(pwd)/data:/app/data \
  --name komari \
  ghcr.io/komari-monitor/komari:latest
```

使用 docker-compose 时：`docker compose pull && docker compose up -d`。

### 3. 二进制

从 [GitHub Release 页面](https://github.com/komari-monitor/komari/releases) 下载最新二进制，替换正在运行的二进制后重启。以 systemd 安装（默认目录 `/opt/komari`）为例：

```bash
cd /opt/komari
sudo systemctl stop komari
sudo cp komari komari.bak.$(date +%F)          # 备份旧二进制以便回滚
ARCH=$(uname -m); case "$ARCH" in x86_64) ARCH=amd64;; aarch64) ARCH=arm64;; esac
sudo curl -fL -o komari \
  "https://github.com/komari-monitor/komari/releases/latest/download/komari-linux-${ARCH}"
sudo chmod +x komari
sudo systemctl start komari
systemctl status komari --no-pager
```

回滚：停止服务，恢复 `komari.bak.<日期>`，再启动服务。

### 升级 Fork 构建版本

安装脚本的“升级”选项以及上面的下载地址都固定指向**官方** `komari-monitor/komari` 的 Release。如果你运行的是自己的 fork，需要先在你的 fork 上发布一个 Release——其 CI 会克隆前端、编译二进制并附加到 Release——然后改用你 fork 的产物升级：

- 二进制：`https://github.com/<你的用户名>/komari/releases/download/<tag>/komari-linux-amd64`
- Docker：`ghcr.io/<你的用户名>/komari:<tag>`

**不要**对 fork 使用安装脚本内置的“升级”功能：它会用官方版本覆盖你的构建。

## 前端开发指南

[Komari 主题开发指南 | Komari](https://komari-document.pages.dev/dev/theme.html)

[在 Crowdin 上翻译 Komari](https://crowdin.com/project/komari/invite?h=cd051bf172c9a9f7f1360e87ffb521692507706)

## 客户端 Agent 开发指南

[Komari Agent 信息上报与事件处理文档](https://komari-document.pages.dev/dev/agent.html)

## 贡献

欢迎提交 Issue 或 Pull Request！

## 鸣谢

### 破碎工坊云

[破碎工坊云 - 专业云计算服务平台，提供高效、稳定、安全的高防服务器与CDN解决方案](https://www.crash.work/)

### DreamCloud

[DreamCloud - 极高性价比解锁直连亚太高防](https://as211392.com/)

### 🚀 由 SharonNetworks 赞助

[![Sharon Networks](https://raw.githubusercontent.com/komari-monitor/public/refs/heads/main/images/sharon-networks.webp)](https://sharon.io)

SharonNetworks 为您的业务起飞保驾护航！

亚太数据中心提供顶级的中国优化网络接入 · 低延时 & 高带宽 & 提供 Tbps 级本地清洗高防服务，为您的业务保驾护航，为您的客户提供极致体验。加入社区 [Telegram 群组](https://t.me/SharonNetwork) 可参与公益募捐或群内抽奖免费使用。

### 开源社区

提交 PR、制作主题的各位开发者

—— 以及：感谢我自己能这么闲

## Star History

[![Star History Chart](https://api.star-history.com/svg?repos=komari-monitor/komari&type=Date)](https://www.star-history.com/#komari-monitor/komari&Date)
