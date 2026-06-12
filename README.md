# Komari


Komari 是一款轻量级的自托管服务器监控工具，旨在提供简单、高效的服务器性能监控解决方案。它支持通过 Web 界面查看服务器状态，并通过轻量级 Agent 收集数据。

[文档](https://komari-document.pages.dev/) | [文档(镜像站 By Geekertao)](https://www.komari.wiki) | [Telegram 群组](https://t.me/komari_monitor)

## 特性

- **轻量高效**：低资源占用，适合各种规模的服务器。
- **自托管**：完全掌控数据隐私，部署简单。
- **Web 界面**：直观的监控仪表盘，易于使用。

## 快速开始


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


## 升级

升级只替换 Komari 程序本身，你的数据——数据目录（或 Docker 数据卷），包括 `komari.db` 和所有设置——都会保留。升级前建议先备份数据以防万一。

### 一键升级脚本（systemd，推荐）

直接运行下面这条命令即可。脚本会自动识别架构、从本仓库最新 Release 拉取对应二进制、备份旧版本、替换并重启服务，**启动失败会自动回滚**：

```bash
curl -fsSL https://raw.githubusercontent.com/xshhhlol/komari/main/upgrade-komari.sh | sudo bash
```

可选环境变量（默认值已适配本 fork 与一键安装脚本的安装方式）：

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `KOMARI_REPO` | `xshhhlol/komari` | 拉取 Release 的仓库 |
| `KOMARI_INSTALL_DIR` | `/opt/komari` | 安装目录 |
| `KOMARI_SERVICE` | `komari` | systemd 服务名 |
| `KOMARI_TAG` | `latest` | 指定 Release 标签 |

例如升级到指定版本：

```bash
curl -fsSL https://raw.githubusercontent.com/xshhhlol/komari/main/upgrade-komari.sh | sudo KOMARI_TAG=1.1 bash
```

### 手动升级（二进制 + systemd）

从 [Release 页面](https://github.com/xshhhlol/komari/releases) 下载最新二进制，替换后重启（默认目录 `/opt/komari`）：

```bash
cd /opt/komari
sudo systemctl stop komari
sudo cp komari komari.bak.$(date +%F)          # 备份旧二进制以便回滚
ARCH=$(uname -m); case "$ARCH" in x86_64) ARCH=amd64;; aarch64) ARCH=arm64;; esac
sudo curl -fL -o komari \
  "https://github.com/xshhhlol/komari/releases/latest/download/komari-linux-${ARCH}"
sudo chmod +x komari
sudo systemctl start komari
systemctl status komari --no-pager
```

回滚：停止服务，恢复 `komari.bak.<日期>`，再启动服务。

### Docker

拉取最新镜像并重建容器（数据卷保留，不会丢数据）：

```bash
docker pull ghcr.io/xshhhlol/komari:latest
docker stop komari && docker rm komari
docker run -d \
  -p 25774:25774 \
  -v $(pwd)/data:/app/data \
  --name komari \
  ghcr.io/xshhhlol/komari:latest
```

使用 docker-compose 时：`docker compose pull && docker compose up -d`。镜像需在发布 Release 时由 CI 推送到 GHCR，并将该 package 设为 public 才能直接拉取。

> [!NOTE]
> 官方一键安装脚本（`install-komari.sh`）内置的“升级”功能固定从官方 `komari-monitor/komari` 拉取，会覆盖你 fork 的构建。运行本 fork 请使用上面的一键升级脚本或手动方式。

## 前端开发指南

[Komari 主题开发指南 | Komari](https://komari-document.pages.dev/dev/theme.html)

[在 Crowdin 上翻译 Komari](https://crowdin.com/project/komari/invite?h=cd051bf172c9a9f7f1360e87ffb521692507706)

## 客户端 Agent 开发指南

[Komari Agent 信息上报与事件处理文档](https://komari-document.pages.dev/dev/agent.html)


## Star History

[![Star History Chart](https://api.star-history.com/svg?repos=komari-monitor/komari&type=Date)](https://www.star-history.com/#komari-monitor/komari&Date)
