# Private Drop — 项目知识（Agent.md）

> 本文件是本项目的单一事实来源。每个开发任务完成后必须更新。
> 任务书见 `taskbook.md`。中文为第一语言，界面同时支持英文。

## 1. 项目目标
部署在个人 NAS 上的单用户私有消息中转 Web 应用（Private Drop）。所有设备共享同一消息空间；支持文字/图片/视频/音频/文件；PWA；30 天默认保存；收藏不过期；回收站；完善鉴权与设备管理。个人自用优先，避免过度工程化。

## 2. 架构
- **单容器**：Go 后端提供 REST API + WebSocket + 托管前端静态资源，单个二进制。
- **数据**：SQLite（WAL 模式）存元数据；附件以实体文件存于独立挂载目录；SHA-256 去重 + 引用计数。
- **实时**：WebSocket 广播事件（`message.created/updated/deleted`、`upload.*`、`notification`、`device.*`），客户端断线重连并按游标补齐。
- **前端**：React SPA（Vite 构建），构建产物由 Go 嵌入或同容器托管；PWA Service Worker。

## 3. 技术选型（2026-11-19 核实最新稳定版）
| 层 | 技术 | 版本 | 理由 |
|---|---|---|---|
| 后端 | Go | 1.27 | 单静态二进制、低内存、并发好、适合 NAS |
| HTTP | net/http ServeMux | 标准库 | 无额外路由依赖，Go 1.22+ 路径参数 |
| DB | modernc.org/sqlite | v1.62 | 纯 Go 无 CGO，WAL，易静态编译 |
| WebSocket | github.com/coder/websocket | v1.8 | 现代、维护活跃 |
| Passkey | go-webauthn/webauthn | v0.18 | 标准 WebAuthn 实现 |
| TOTP | pquerna/otp | v1.5 | RFC6238 |
| 密码哈希 | golang.org/x/crypto (argon2id) | v0.59 | 抗 GPU 破解 |
| UUID | google/uuid | v1.6 | 实体标识 |
| 前端 | React + TypeScript | 19 / 5.9 | 生态成熟、AI 易维护 |
| 构建 | Vite + plugin-react | 8 / 6 | 快、现代 |
| PWA | vite-plugin-pwa | 2 | Workbox 自动生成 SW |
| 样式 | Tailwind CSS | 4 | 快速、统一 |
| 状态 | Zustand | 5 | 轻量 |
| 路由 | react-router-dom | 7 | 成熟 |
| i18n | i18next + react-i18next | 27 / 18 | 成熟 |
| 测试 | Go testing/httptest + Vitest + Playwright | - | 官方/主流 |

**否决方案**：Redis/MQ/外部 DB（个人自用无必要）、微服务（违反简洁原则）、Next.js/SSR（增加 Node 运行时，破坏单二进制简洁性）、CGO SQLite（静态编译复杂）。

## 4. 目录结构
```
/
├── cmd/server/main.go          # 服务入口
├── cmd/resetpwd/main.go        # CLI 密码重置工具
├── internal/
│   ├── config/                 # 配置加载
│   ├── db/                     # 迁移 + 数据访问
│   ├── auth/                   # 密码/TOTP/Passkey/会话/设备审批
│   ├── api/                    # HTTP handler + 路由 + 中间件
│   ├── ws/                     # WebSocket hub
│   ├── message/                # 消息业务
│   ├── upload/                 # 分片上传任务
│   ├── file/                   # 文件/去重/预览/下载/ZIP
│   ├── search/                 # 搜索
│   ├── notify/                 # 通知
│   ├── clean/                  # 过期/回收站清理
│   ├── linkpreview/            # 链接解析 + SSRF 防护
│   └── audit/                  # 审计/日志
├── web/                        # React 前端源码
│   └── dist/                   # 构建产物（Go embed）
├── migrations/                 # SQL 迁移
├── docs/                       # 文档（见下）
├── compose.yaml
├── .env.example
├── Dockerfile
└── .github/workflows/ci.yml
```

## 5. 数据模型（核心表）
- `users`：id, username, password_hash, totp_secret, passkey_creds, created_at。
- `settings`：key, value（集中配置：保存期限、回收站期限、上传限制、并发数、空间警戒、设备策略、语言、链接解析开关等）。
- `devices`：id, user_id, name, type, ip, created_at, last_active, authorized, revoked_at。
- `sessions`：id(token hash), device_id, user_id, expires_at, created_at。
- `device_requests`：id, device_info, code, status, created_at（新设备审批）。
- `messages`：id, user_id, device_id, text, pinned, favorited, created_at, updated_at, expires_at, deleted_at(回收站), restore_mode。
- `attachments`：id, entity_id(实体文件), name, mime, size, sha256, width, height, favorited, uploaded_at。
- `entity_files`：id, sha256, path, size, mime, ref_count, created_at（去重实体）。
- `message_attachments`：message_id, attachment_id（多对多引用）。
- `tags`：id, name, color, icon, sort, is_system。
- `message_tags`：message_id, tag_id。
- `attachment_tags`：attachment_id, tag_id（可选，附件独立分类）。
- `uploads`：id, filename, size, mime, sha256, total_chunks, chunk_size, uploaded_chunks(JSON), status, message_id, created_at。
- `notifications`：id, type, payload, read, created_at。
- `audit_logs`：id, action, detail, ip, device_id, created_at。
- `link_previews`：id, url, title, description, image_url, created_at。

**引用一致性规则**：实体文件仅在 `ref_count == 0` 且无有效上传任务占用时才可物理删除；删除消息只减引用；回收站/收藏引用仍有效。

## 6. 业务规则
- 保存默认 30 天，从 `created_at` 计；编辑不改创建时间与期限；收藏（消息/附件）不过期。
- 取消收藏后恢复原期限，已过期则进入清理。
- 回收站默认 30 天；恢复时可选“重算期限”或“沿用原始时间”。
- 置顶独立展示，不影响期限。
- 去重：上传前按 SHA-256 查实体，命中则复用并增加引用；并发上传同文件需加锁避免多份实体。
- 分片上传：服务端记录任务与已完成分片，支持跨刷新/跨设备恢复；原文件不可访问时明确提示。
- 空间警戒仅提醒，不自动删除、不自动禁止上传。

## 7. 安全规则
- 初始化向导仅局域网可访问；完成后永久关闭，禁止公网抢注。
- 所有 API（除初始化状态/登录）需鉴权；所有文件预览/下载/链接复制需鉴权且无公开永久链接。
- WebRTC/WS 建连需校验会话令牌。
- 登录速率限制；管理界面二次验证（Passkey 或 TOTP）。
- 严格 SSRF 防护（禁私网/环回/元数据/非法协议/DNS 重绑定/重定向绕过）。
- 路径遍历防护：实体文件用 UUID 命名，不接受用户路径。
- 日志不得泄露密码/令牌/TOTP 密钥/Passkey 私钥/文件内容。

## 8. 开发规范
- Go：`gofmt`、标准 error 包装、`context` 透传；不改生成文件。
- TS：严格模式；组件函数式；API 调用集中在 `web/src/api`。
- 每次改动后运行测试（见第 10 节）；改 TS 跑 `tsc --noEmit`；Go 跑 `go vet ./...`。

## 9. API 契约
见 `docs/API_CONTRACT.md`。前后端以该文件为准，任何变更需同步更新。

## 10. 命令
- 后端测试：`go test ./...`
- 前端类型检查：`cd web && npm run typecheck`
- 前端测试：`cd web && npm test`
- 构建：`docker build -t private-drop .`
- 本地运行：`docker compose up -d`

## 11. 部署
- 单容器；内部端口 8080；compose 映射如 `8088:8080`（外部可配）。
- 数据目录 `/data`（含 `db.sqlite` 与 `files/`、`tmp/`），必须独立映射以便 NAS 备份。
- `docker-entrypoint.sh` 以 root 启动，仅修复绑定挂载目录 `/data` 的属主，随后 `su-exec` 降到 UID 10001；容器主进程始终为非 root。
- 公网推荐 Cloudflare Tunnel + Access + HTTPS；应用自身鉴权独立有效。
- 公网限制已按官方文档（2026）核实：Free/Pro 单请求 100 MB、Business 200 MB、Enterprise 自助最高 5 GB；分片上传仅绕过应用层限制，代理层限制与超时仍在（见 `docs/CLOUDFLARE.md`）。

## 12. 当前进度（审计后补全完成，2026-11-19 第二轮）
- 首轮 0～6 阶段已交付；针对任务书逐节审计发现的缺口已完成第二轮补全，详见 `docs/TEST_REPORT.md`。
- 第二轮新增：上传与消息绑定及删除/过期取消、附件 WS 同步、链接预览持久化、服务端上传并发限制、附件标签与文件批量接口、真实磁盘警戒与建议、keyset 分页、编辑保留期限、僵死上传清理与 needs_source、持久化 WS 事件日志、ZIP 原始目录、批量恢复模式、通知开关生效、resetpwd 审计、上传字节字段；前端搜索/文件/回收站批量操作、PWA 文件分享、标签图标排序、自动定位最新、并发设置恢复、Esc 退出、登录有效期、Playwright E2E、ESLint。
- GitHub Actions 首次真实运行（run 44508779998）：Backend / Frontend / Security scan / Docker build / Publish to GHCR 全部成功；GHCR 已发布 `latest` 与 `sha-859896f`。
- 任务列表已清空并重建为 4 条补全任务（plan 95d48f9e…）。
- 计划与 7 条阶段任务已录入 OpenChamber 项目知识（plan + todos）。
- 已验证（本机实测，非推断）：
  - `gofmt -l .`（无输出）、`go vet ./...`、`go build ./...`、`go test -count=1 ./...`（全部通过）。
  - `npm run typecheck`（0 错误）、`npm test -- --run`（4 文件 / 23 测试通过）、`npm run build`（PWA 产物生成）。
  - `docker build -t private-drop:verify .`（三阶段镜像构建成功）；`docker compose up -d` 容器 healthy。
  - 真实 E2E（本地二进制 + Docker 容器两轮）：初始化/防重复初始化、登录、MFA 门禁、设置读写、消息 CRUD/编辑/收藏/置顶、分片上传+合并+SHA-256 去重复用、鉴权下载（字节一致，未登录 401）、回收站与恢复、SPA 回退、11 个读接口 200、未登录访问 401。
  - 真实浏览器（Playwright + 系统 Chrome，连接 Docker 服务）验证：登录页、消息流、管理界面、保存期限值 45、通知开关状态、存储统计真实数字、系统诊断、0 条 console error。

## 13. 已知问题 / 未验证项
- 已按只读安全审查修复：设备身份改用客户端生成的 `X-Device-Key`（名称不再作为身份，防止审批绕过）；撤销设备立即断开其 WebSocket；MFA 令牌按设备绑定并在登出/注册二次因子时撤销；设备管理接口加 MFA 门禁（一次性码审批除外）；初始化在事务内二次检查，消除并发抢注 TOCTOU。
- 第二轮仍未验证：Passkey 真实验证器、TOTP 浏览器完整启用、双浏览器实时同步、真实 Tunnel 环境 LAN 判定、链接解析对真实站点抓取与 DNS 重绑定。
- Passkey/WebAuthn 已实现并通过编译，但未使用真实验证器（硬件密钥/平台验证器）做端到端联调；TOTP 已实现但未在浏览器中走通完整启用流程。
- WS 历史补齐使用进程内环形缓冲，服务重启后缓冲丢失；限流状态亦为进程内（单实例自用场景可接受）。
- 缩略图仅支持图片；视频返回 `THUMBNAIL_UNAVAILABLE` 由前端回退；Office 文件仅下载不在线转换。
- 链接解析的 SSRF 拦截有单元测试，但未对真实外部站点抓取做集成测试。
- GitHub Actions / GHCR 发布流水线已配置但未在 GitHub 上实际运行；本地仅做了 `docker compose config` 与 `docker build --check`。
- 手机系统分享的文件 multipart 接收依赖浏览器/SW 能力，iOS/后台上传限制见 `docs/KNOWN_LIMITATIONS.md`。
- 应用内无备份功能，依赖 NAS 备份整个 `data/`（见 `docs/BACKUP_RESTORE.md`）。

## 14. 重要决策
- 采用 Go 单二进制 + SQLite（纯 Go 驱动 `modernc.org/sqlite`）以满足单容器、低资源、易备份。
- 前端构建产物通过根目录 `embed_assets.go` 嵌入 Go 二进制（go:embed 不支持 `..`，故该文件必须在仓库根）。
- **设置键适配**：后端以 snake_case 字符串存储（大小为字节），前端以 camelCase 强类型使用（大小为 MB）；在 `web/src/api/settings.ts` 做双向翻译，避免改动任一侧契约。PATCH 后重新读取设置以对齐响应（后端只返回 changed 列表）。
- **MFA 自举**：首次初始化/无二次因子时的登录会下发 24h 的 bootstrap MFA 令牌（`IssueBootstrapMFA`），否则无法进入管理界面配置 TOTP/Passkey；注册二次因子后不再下发。
- **响应形状适配**：`/api/recycle` 返回 `{messages,files}`，前端适配为 `{items}`；`/api/storage/stats` 后端补齐前端所需指标；`/api/system/diagnose` 前端将扁平检查表归一化。
- **Docker 权限**：绑定挂载常由 Docker 建为 root 所有，入口脚本在必要时修复属主后降到非 root，解决首次部署启动失败。
- **初始化 fail-closed**：带代理头（`X-Forwarded-For` / `CF-Connecting-IP` / `X-Real-IP`）且直接对端不在 `TRUSTED_PROXY_CIDRS` 的初始化请求一律 403，防止公网经 Tunnel 被误判为局域网而抢先初始化；仅有受信任代理时才用 XFF 判定真实 IP。
- 业务包通过 `events.Sink` 接口发事件，避免 ws/api 循环依赖。
