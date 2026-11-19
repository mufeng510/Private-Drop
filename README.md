# Private Drop

部署在个人 NAS 上的**单用户私有消息中转站**。打开网页就能在自己的多台手机、电脑、平板之间快速传递文字、图片、视频、音频和文件；数据全部留在你自己的 NAS。

[English README](README.en.md) · [产品任务书](taskbook.md) · [项目知识 Agent.md](Agent.md) · [API 契约](docs/API_CONTRACT.md)

## 功能概览

- 单用户、所有设备共享同一消息空间；支持局域网与公网访问（推荐 Cloudflare Tunnel + Access + HTTPS）
- 文字 / 图片 / 视频 / 音频 / 普通文件 / 混合附件；多文件上传、拖拽与粘贴
- 分片上传 + 断点续传，上传任务跨刷新/跨设备可查
- WebSocket 多设备实时同步，断线自动重连并按游标补齐
- 文件管理：自动分类、标签、搜索、筛选、预览、单文件与 ZIP 下载（均需鉴权）
- SHA-256 去重（同一实体只存一份）+ 引用计数，删除消息不误删共享文件
- 保存期限默认 30 天、收藏不过期、回收站、置顶、批量管理
- 密码 / TOTP / Passkey，设备管理与新设备审批，管理界面二次验证
- 中英文界面、PWA（可添加到桌面）、快捷键、通知中心、日志与审计
- 单容器部署：Go 单二进制（内嵌前端）+ SQLite，非 root 运行，数据独立挂载

## 快速开始（Docker Compose）

前置条件：NAS 支持 Docker Compose v5+；建议 ≥2 GiB 可用内存。

```bash
# 1. 准备目录（换成你的实际路径）
mkdir -p /volume1/docker/private-drop && cd /volume1/docker/private-drop

# 2. 获取配置并复制环境变量文件
curl -fLO https://raw.githubusercontent.com/OWNER/private-drop/main/compose.yaml
curl -fLO https://raw.githubusercontent.com/OWNER/private-drop/main/.env.example
cp .env.example .env

# 3. 创建数据目录并授权（容器以 UID 10001 运行）
mkdir -p data && sudo chown -R 10001:10001 data

# 4. 启动（本地构建）
docker compose up -d --build
# 或使用已发布镜像：在 .env 中设置 IMAGE=ghcr.io/OWNER/private-drop:latest 后 docker compose pull && docker compose up -d

# 5. 查看状态
docker compose ps && docker compose logs -f
```

> `OWNER/private-drop` 请替换为实际的 GitHub 仓库地址。完整步骤见 [安装指南](docs/INSTALL.md)。

## 首次初始化

1. 在**局域网内**打开 `http://<NAS 的局域网 IP>:8088`（默认端口 8088，容器内固定 8080）。
2. 按向导创建唯一管理员账号与密码，填写设备名称并配置初始安全选项。
3. 初始化完成后入口**永久关闭**，不能再创建第二个账号，公网访客也无法抢注。
4. 强烈建议立即启用 TOTP 或 Passkey，并打开管理界面二次验证。

常用地址：

| 场景 | 地址 |
|---|---|
| 局域网 | `http://<NAS-IP>:8088` |
| 公网（配置后） | `https://drop.example.com` |

## 基本配置

部署只需三个环境变量（在 `.env` 中修改后 `docker compose up -d` 生效）：

| 变量 | 默认值 | 说明 |
|---|---|---|
| `PORT` | `8088` | 宿主机对外端口；容器内固定 8080 |
| `DATA_DIR` | `/data` | 容器内数据目录，对应宿主机 `./data` |
| `TZ` | `Asia/Shanghai` | 时区，影响日志时间显示 |

保存期限、上传上限与并发数、设备策略、链接解析、语言、通知、空间警戒等**全部在 Web 管理界面设置**，Compose 保持简单。

## 文档

| 文档 | 内容 |
|---|---|
| [安装指南](docs/INSTALL.md) | NAS 部署、目录布局、首次初始化、密码重置工具 |
| [公网部署](docs/PUBLIC_DEPLOY.md) | 域名 + Tunnel + Access + HTTPS，应用鉴权独立强制 |
| [Cloudflare 配置](docs/CLOUDFLARE.md) | Tunnel/Access 逐步配置与**官方上传大小限制** |
| [备份与恢复](docs/BACKUP_RESTORE.md) | 备份 `data/` 的哪些内容、停机制作一致性备份、迁移 |
| [升级与回滚](docs/UPGRADE_ROLLBACK.md) | 镜像标签策略、数据库迁移、回滚步骤 |
| [安全说明](docs/SECURITY.md) | 安全模型、SSRF/文件安全、日志脱敏、安全审查要求 |
| [已知限制](docs/KNOWN_LIMITATIONS.md) | 浏览器能力差异、Cloudflare 限制、无内置备份等 |
| [测试指南](docs/TESTING.md) | 各层测试命令、安全清单、测试报告模板 |
| [API 契约](docs/API_CONTRACT.md) | 前后端接口约定（唯一事实来源） |

## 技术栈

Go 1.27（net/http + modernc.org/sqlite + coder/websocket，纯 Go 静态编译）/ React 19 + TypeScript + Vite 8 + Tailwind 4 + PWA / 单容器 Docker 镜像 / SQLite WAL。详见 [Agent.md](Agent.md)。

## 许可证

见 [LICENSE](LICENSE)。
