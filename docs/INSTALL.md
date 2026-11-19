# 安装指南（Docker Compose / NAS）

Private Drop 是**单容器**应用：Go 后端提供 REST API + WebSocket，并把构建好的 React 前端嵌入同一个二进制。容器内监听 **8080**，Compose 默认映射为宿主机 **8088**。除 `./data` 外不需要任何外部数据库或缓存。

## 1. 前置条件

| 项目 | 要求 |
|---|---|
| NAS | 支持 Docker Engine 29+ 与 Docker Compose v5+（群晖 Container Manager、QNAP Container Station、威联通/铁威马等均可）；x86_64 或 arm64 |
| 内存 | 建议 ≥ 2 GiB 可用（构建镜像时前端 + Go 编译会短暂占用更多，内存紧张可先在 PC 上构建再导入） |
| 磁盘 | 为 `./data` 预留足够空间；单文件上限在管理界面配置 |
| 网络 | 局域网可直连 NAS；公网访问另见 [PUBLIC_DEPLOY.md](PUBLIC_DEPLOY.md) |

> 本机开发提示：Docker 守护进程不常驻时用 `docker-up` 启动，用完 `docker-down` 释放内存。

## 2. 目录布局

推荐在 NAS 上建立一个独立目录（下面以 `/volume1/docker/private-drop` 为例，请按实际路径替换）：

```
private-drop/
├── compose.yaml        # 部署定义
├── .env                # 由 .env.example 复制，只含 PORT / DATA_DIR / TZ
├── data/               # 唯一持久化目录，必须独立映射！
│   ├── db.sqlite       # 数据库（消息、设置、设备、会话、上传任务等）
│   ├── db.sqlite-wal   # WAL 日志（数据库为 WAL 模式）
│   ├── db.sqlite-shm   # WAL 共享内存文件
│   ├── files/          # 实体附件（UUID 命名，SHA-256 去重）
│   └── tmp/            # 分片上传的临时分片
└── docker-compose 之外：NAS 自身的快照/备份任务
```

**重要**：容器内进程以 **UID 10001 / GID 10001** 运行。宿主机 `data/` 必须对该 UID 可写，否则容器启动后会无法建库/写文件。

## 3. 部署步骤

```bash
# 3.1 进入部署目录
mkdir -p /volume1/docker/private-drop && cd /volume1/docker/private-drop

# 3.2 放置配置（从项目仓库获取，或手工复制文件内容）
curl -fLO https://raw.githubusercontent.com/OWNER/private-drop/main/compose.yaml
curl -fLO https://raw.githubusercontent.com/OWNER/private-drop/main/.env.example
cp .env.example .env

# 3.3 创建并授权数据目录（关键步骤）
mkdir -p data
sudo chown -R 10001:10001 data        # 无 sudo 的 NAS 用文件管理器修改所有者
# 若 NAS 不支持按数字 UID 授权，可临时用 777（仅示例，生产建议收紧）：
# chmod -R 777 data

# 3.4 首次部署
docker compose up -d --build
# 如使用已发布镜像，在 .env 中设置 IMAGE=ghcr.io/<owner>/private-drop:latest 后：
# docker compose pull && docker compose up -d

# 3.5 检查状态
docker compose ps
docker compose logs -f private-drop
# 健康检查来自镜像内 HEALTHCHECK，访问 /api/auth/status
```

> `OWNER/private-drop` 请替换为实际的 GitHub 仓库路径。离线环境可在本机执行 `docker build -t private-drop:local .` 后 `docker save`/`docker load` 导入 NAS。

## 4. 首次初始化（仅局域网）

1. 在**局域网内**的设备上打开 `http://<NAS 的局域网 IP>:8088`。
2. 首次进入初始化向导：创建唯一管理员账号与密码，填写设备名称，按需配置初始安全策略。
3. 初始化完成后，**初始化入口永久关闭**，不能再创建第二个账号，也不允许公网访客抢先注册。
4. 强烈建议初始化后立即到「管理 → 安全」启用 TOTP 或 Passkey，并配置管理界面二次验证。

> 初始化页面仅允许局域网访问。若你打算通过 Cloudflare Tunnel 对外发布，请**先在局域网完成初始化**，并确认应用不会把经代理转发的公网请求误判为局域网（详见 [PUBLIC_DEPLOY.md](PUBLIC_DEPLOY.md) 与 [SECURITY.md](SECURITY.md)）。

常用地址：

| 场景 | 地址 |
|---|---|
| 局域网 | `http://192.168.31.64:8088`（示例 IP，按实际替换） |
| 公网（配置完成后） | `https://drop.example.com` |

## 5. 基本配置

Compose / `.env` 只需关心三项：

| 变量 | 默认值 | 说明 |
|---|---|---|
| `PORT` | `8088` | 宿主机对外端口，容器内始终为 8080 |
| `DATA_DIR` | `/data` | 容器内数据目录，对应宿主机 `./data` |
| `TZ` | `Asia/Shanghai` | 时区，影响日志时间显示 |

其余全部在 Web「管理界面」中设置：保存期限（默认 30 天）、回收站期限、单文件上传上限与并发数、设备授权策略、链接解析开关、语言、通知、空间警戒等。

修改 `.env` 后需 `docker compose up -d` 重建容器才生效；Web 管理设置即时生效。

## 6. 常用运维命令

```bash
docker compose up -d            # 启动 / 应用配置变更
docker compose down             # 停止并移除容器（不会删除 ./data）
docker compose stop / start     # 仅停止/启动，做一致性备份时使用
docker compose logs -f          # 查看日志
docker compose exec private-drop /usr/local/bin/private-drop-resetpwd --help
```

容器内置密码重置工具（忘记密码时使用）：

```bash
# 交互方式最安全（密码不进入 shell 历史）：
docker compose exec -it private-drop /usr/local/bin/private-drop-resetpwd --data-dir /data

# 具体参数以实际版本的 --help 输出为准；工具会修改 data/db.sqlite 中的管理员密码。
```

详见 [BACKUP_RESTORE.md](BACKUP_RESTORE.md)（备份/恢复）、[UPGRADE_ROLLBACK.md](UPGRADE_ROLLBACK.md)（升级/回滚）、[KNOWN_LIMITATIONS.md](KNOWN_LIMITATIONS.md)（限制）。
