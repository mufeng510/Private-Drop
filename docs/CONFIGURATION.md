# 配置说明（Private Drop）

配置分两类：**部署环境变量**（Docker Compose / `.env`，只放部署必需项）与**业务设置**（登录管理界面修改，保存在 `data/db.sqlite` 的 `settings` 表）。任务书要求业务配置集中在管理界面，Compose 保持简单。

## 1. 部署环境变量（`.env` / `compose.yaml`）

| 变量 | 默认值 | 说明 |
|---|---|---|
| `PORT` | `8088` | 宿主机对外端口；容器内固定 `8080`，映射为 `${PORT}:8080`。 |
| `DATA_DIR` | `/data` | 容器内数据目录；Compose 固定把宿主机 `./data` 挂载至此，通常无需修改。 |
| `TZ` | `Asia/Shanghai` | 时区，影响日志时间显示。 |
| `IMAGE` | `private-drop:local` | 镜像名；使用 GHCR 发布版时设为 `ghcr.io/<owner>/private-drop:<tag>`。 |
| `VERSION` | `dev` | 本地构建写入镜像的版本标签（发布流水线自动传入）。 |
| `TRUSTED_PROXY_CIDRS` | 空 | **公网经 Cloudflare Tunnel/反向代理时必须配置**：受信任代理的源 IP 段（逗号分隔），例如 `127.0.0.0/8,172.16.0.0/12`。应用仅在直接对端命中这些网段时才信任 `X-Forwarded-For` / `CF-Connecting-IP`；未配置且带代理头的初始化请求会被拒绝（fail closed）。 |

### 高级环境变量（一般无需修改）
| 变量 | 默认值 | 说明 |
|---|---|---|
| `SESSION_TTL` | `720h`（30 天） | 会话默认有效期，支持 Go duration 或裸天数。 |
| `MAX_SESSION_TTL` | `87600h`（10 年） | “长期保持”的上限；长期会话仍可在设备管理页远程撤销。 |
| `LOG_LEVEL` | `info` | 应用日志级别。 |
| `WEBAUTHN_RP_ID` | 自动 | Passkey 的 Relying Party ID；为空时从请求 Host 推导，便于局域网 IP 与公网域名共用。 |
| `WEBAUTHN_ORIGINS` | 自动 | 允许的 WebAuthn Origin 列表（逗号分隔）；生产环境建议在公网域名下显式设置。 |

## 2. 业务设置（管理界面 → 对应分区）

后端以 snake_case 字符串存储；前端在 API 层转换为 camelCase（大小为 MB），并在 PATCH 后重新读取。

| 设置键 | 默认 | 管理界面位置 | 说明 |
|---|---|---|---|
| `retention_days` | 30 | 保存期限 | 普通消息/附件保存天数；收藏不自动过期。 |
| `recycle_days` | 30 | 保存期限 | 回收站保留天数。 |
| `max_upload_size` | 104857600（100 MB） | 上传限制 | 单文件上限（字节）；另受反向代理/Cloudflare 限制，见 `CLOUDFLARE.md`。 |
| `upload_concurrency` | 3 | 上传限制 | 上传并发数（服务端与客户端均执行）。 |
| `chunk_size` | 1048576（1 MB） | 上传限制 | 分片大小。 |
| `storage_warning_percent` | 90 | 存储空间警戒 | 磁盘占用警戒百分比；仅提醒，不自动删除、不禁止上传。 |
| `device_policy` | `open` | 登录策略 | `open` / `whitelist` / `approval`。 |
| `session_ttl_days` | 30 | 登录策略 | 默认会话天数；登录页也可按次选择。 |
| `login_attempt_limit` | 5 | 登录策略 | 登录失败限流阈值。 |
| `link_preview_enabled` | true | 链接解析 | 是否在消息创建时自动解析链接（含严格 SSRF 防护）。 |
| `language` | zh | 语言与主题 | 服务端默认语言；浏览器端可切换中英文。 |
| `notify_new_device` / `notify_upload_done` / `notify_upload_failed` / `notify_low_storage` / `notify_expiring` | true | 通知设置 | 各类通知开关。 |

修改方式：进入管理界面（需二次验证）→ 对应分区 → 保存。所有设置实时通过 WebSocket `settings.changed` 广播到其他设备。

## 3. 数据目录布局

```
./data/
├── db.sqlite            # 数据库（含设置、消息、附件元数据、上传任务）
├── db.sqlite-wal/...    # WAL 模式下的预写日志与共享内存文件
├── files/               # 实体附件（UUID 命名，SHA-256 去重）
└── tmp/                 # 上传临时分片（按任务 ID 分目录）
```

备份必须包含整个 `./data/`，详见 `BACKUP_RESTORE.md`。
