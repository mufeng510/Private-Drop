# 测试报告（Private Drop）

> 本报告只记录**实际执行过**的验证。未执行的项目在“未执行/未验证”中列出，不计为通过。
> 测试日期：2026-11-19。环境：Debian 13，Go 1.27.4，Node 24.22.0，npm 11.19.1，Docker 29.9.1 / Compose v5.6.0。

## 1. 后端（Go）

| 命令 | 结果 |
|---|---|
| `gofmt -l .` | 无输出（格式合规） |
| `go vet ./...` | 通过，无输出 |
| `go build ./...` | 通过，无输出（含 server 与 resetpwd） |
| `go test -count=1 ./...` | 通过：`internal/api`、`internal/auth`、`internal/clean`、`internal/file`、`internal/linkpreview`、`internal/message`、`internal/upload` 全部 ok；其余包无测试用例 |

测试覆盖：消息创建/编辑/收藏/置顶/删除/恢复、保存期限计算、去重与引用计数、设备授权、SSRF 拦截、初始化一次性、未鉴权访问拦截、下载鉴权等（均为实际 Go 测试用例）。

## 2. 前端（React / TypeScript）

| 命令 | 结果 |
|---|---|
| `npm run typecheck` (`tsc --noEmit`) | 通过，0 错误 |
| `npm test -- --run` | 4 个测试文件 / 23 个测试全部通过 |
| `npm run build` | 通过：123 modules；`dist/assets/index-*.js` 约 481 kB（gzip 145 kB）；PWA precache 13 entries，生成 `sw.js` |

## 3. 容器与部署

| 命令 | 结果 |
|---|---|
| `docker compose config` | 通过，确认 `8088:8080`、`./data:/data` |
| `docker build --check .` | Check complete, no warnings found |
| `docker build -t private-drop:verify .` | 成功生成镜像（三阶段：Node 构建 → Go 静态编译 → Alpine 运行时） |
| `docker compose up -d` | 容器启动并达到 healthy（初始曾因绑定挂载 root 属主失败，已通过 `docker-entrypoint.sh` 修复并复测通过） |

## 4. 端到端接口冒烟（真实服务，非 mock）

对**本地二进制**与 **Docker 容器（8088）**各执行一轮，均通过：

- 初始化前 `initialized=false`；初始化后 `true`；重复初始化返回 409；初始化时下发 bootstrap MFA 令牌。
- 登录 `/api/auth/me` 返回正确用户与设备；`/api/settings` 无 MFA 令牌返回 403。
- 设置读取与修改生效（含 `retention_days=45`、通知开关）。
- 消息创建、查询、编辑（收藏/置顶）正常，保存期限自动计算为 30 天后。
- 分片上传（2 片）→ 合并 → SHA-256 去重复用（`reusedAttachmentId` 命中）。
- 鉴权下载字节与原文件一致；未登录下载返回 401。
- 删除进入回收站 → 恢复（`mode=reset`）成功。
- SPA 回退：`/messages` 返回 `index.html`。
- 11 个读接口（`messages/files/search/tags/uploads/notifications/logs/audit/storage/system/info/diagnose`）均返回 200。
- 未登录访问 `/api/messages` 返回 401。

## 5. 真实浏览器验证（Playwright + 系统 Chrome，连接 Docker 服务）

- 登录页渲染、输入账号密码登录成功。
- 消息流显示真实消息（置顶、来源设备、剩余期限标签）。
- 管理界面可进入（bootstrap 授权生效）；保存期限输入框读到 45、通知开关状态与 API 修改一致。
- 存储统计显示真实数字（已用空间 23 B、文件 1、消息 1、各类型占用、大文件）。
- 系统诊断显示“所有检查通过”。
- 浏览器 console error：0 条。
- 安全审查后复测（新镜像）：初始化（策略 open）→ 登录（新浏览器作为新设备被正确识别）→ 浏览器发送消息 → 管理界面与设备管理加载成功 → `X-Device-Key` 与 bootstrap MFA 均生效 → console error 0 条。
- 修复过程中发现并修正的集成问题：`settings` 键/单位转换、回收站/存储统计/诊断/设备/标签响应形状、MFA 自举、Docker 绑定挂载属主、初始化代理头 fail-closed、WS 测试注册竞态。
- 截图：`/tmp/opencode/pd-login.png`、`/tmp/opencode/pd-ui-messages.png`、`/tmp/opencode/pd-ui-admin.png`、`/tmp/opencode/pd-ui-storage.png`、`/tmp/opencode/pd-ui-final.png`。

## 6. 审计后补全轮次（2026-11-19 第二轮）

按三路只读审计逐项补全后复测：

- **Go**：`gofmt -l .` 无输出；`go vet ./...`、`go build ./...` 通过；`go test -count=1 ./...` 全部包 ok（api / auth / clean / db / file / linkpreview / message / upload / ws），关键时序包 `go test -count=2` 稳定。
- **新增 Go 测试**：`internal/api/completion_test.go`（HTTP 登录/上传全生命周期与续传、预览、缩略图、ZIP 与 Zip-Slip、标签、文件批量、无令牌 401、存储建议）、`internal/api/ws_completion_test.go`（双客户端广播与 `since` 重放去重）、`internal/ws/store_test.go`（持久化事件序号/时间游标）、`internal/message/regression_test.go`（分页并列、编辑保留期限、批量恢复模式、标签）、`internal/file/events_test.go`、`internal/upload/completion_test.go`、`internal/message/links_test.go`、`internal/db/migration_test.go`。
- **前端**：`npm run typecheck` 0 错误；`npm run lint` 0 error / 0 warning；`npm test -- --run` 4 文件 / 23 测试通过；`npm run build` 成功（PWA `injectManifest`，自定义 `sw.js`）。
- **Playwright E2E（新增）**：`npm run test:e2e` — 10 个用例全部通过（system Chrome）：导航、消息流、搜索（含大小筛选与选择）、上传队列、图片画廊、文件管理与批量、回收站批量恢复/删除、管理界面、390px 移动端、PWA POST 分享目标与自定义 SW。
- **功能补全**：上传绑定消息并在删除/过期时取消与清理；附件状态 WS 同步；链接预览持久化；服务端上传并发限制；附件标签与批量接口；真实磁盘警戒与建议；keyset 分页；编辑不改期限；僵死上传清理与 `needs_source`；持久化 WS 事件日志；ZIP 原始目录；批量恢复模式；通知开关生效；resetpwd 审计；上传字节字段。
- **前端补全**：搜索结果操作与批量；PWA 文件分享；回收站批量；消息/文件批量工具栏；搜索大小/标签/设备筛选；标签图标与排序；自动定位最新；并发设置恢复；Esc 退出操作；登录有效期选项。

### 6.1 第三轮：此前「未验证」项的自动化补测

- **虚拟 Passkey 端到端**：`web/e2e/real/passkey.spec.ts`，用 CDP 虚拟验证器在浏览器中注册 Passkey，随后清除会话并用「通行密钥登录」成功 → **通过**。
- **完整 TOTP 流程**：`web/e2e/real/totp.spec.ts`，从界面读取密钥、用 RFC6238 生成验证码启用 TOTP，再走「密码 → TOTP」两步登录成功 → **通过**。
- **双设备实时同步**：`web/e2e/real/realtime.spec.ts`，两个独立浏览器上下文（不同设备标识）登录，A 发送消息后 B 不经刷新即收到 → **通过**。
- **SSRF 重定向复验**：`internal/linkpreview/redirect_internal_test.go`，注入解析器模拟「公开域名 → 私网重定向」，验证跳转目标被拒绝 → **通过**。
- 运行方式：本地 `bash` 启动后端后 `npm run test:e2e:real`；CI 新增 `e2e-real` Job，发布 GHCR 前必须通过。真实运行记录：Actions run `44516962959`，六个 Job（含 `E2E against real backend`）全部成功。

## 7. 未执行 / 未验证（不得视为通过）

- GitHub Actions 与 GHCR 发布：已真实运行并通过（最新 run `44516962959`，https://github.com/mufeng510/Private-Drop/actions/runs/44516962959）；Backend / Frontend / Security scan / Docker build / E2E against real backend / Publish to GHCR 六个 Job 全部成功；GHCR 包 `private-drop` 已发布 `latest` 与 `sha-4d58cfd`。SemVer 标签需推送 `v*` 标签才会产生，未测试。
- Passkey / TOTP / 双设备实时同步：已由真实后端浏览器 E2E 覆盖（见 6.1），不再列为未验证。
- 链接解析对真实外部站点抓取：`TestRealExternalPreview` 在本沙箱被跳过——DNS 把公网域名解析到 `198.18.0.0/15` 基准测试保留段，SSRF 策略**正确地**拒绝了该地址，因此无法在此环境验证真实外网抓取；DNS 重绑定攻击（解析在校验与连接间变化）未单独模拟。重定向复验已通过。
- 真实 Cloudflare Tunnel 环境：未验证（见下条）。
- 手机系统分享的文件接收已实现（POST multipart + 自定义 SW），但受 iOS 无 Share Target / 后台上传限制，见 `docs/KNOWN_LIMITATIONS.md`。
- 经 Cloudflare Tunnel 的公网请求不会被视为局域网：代码层 fail-closed 已实现，未在真实 Tunnel 环境复测。
