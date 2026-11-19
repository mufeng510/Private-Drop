# 最终交付报告（Private Drop）

> 日期：2026-11-19。本报告只陈述已完成并验证的事实；未验证项见末尾。

## 1. 已完成的功能
单用户消息中转：初始化向导（仅局域网、一次性）、密码/TOTP/Passkey 登录、会话与三种设备授权策略、设备管理/审批/远程撤销、消息 CRUD/编辑/收藏/置顶/标签/批量、WebSocket 实时同步（断线补齐）、附件自动分类/预览/画廊/鉴权下载/ZIP、分片断点续传（服务端状态、SHA-256 去重、引用计数）、回收站与恢复、30 天保存期限与自动清理、搜索与多维筛选、通知中心、存储统计与警戒提醒、链接解析（SSRF 防护）、日志与审计、中英文 i18n、桌面/移动响应式界面、PWA（安装、系统分享）、快捷键。

## 2. 技术栈及理由
- 后端：Go 1.27 单静态二进制，`net/http`，`modernc.org/sqlite`（纯 Go、WAL），`coder/websocket`，`go-webauthn/webauthn`，`pquerna/otp`，Argon2id。单容器、低内存、单文件易备份、NAS 长期运行友好。
- 前端：React 19 + TypeScript 5.9 + Vite 8 + PWA（injectManifest）+ Tailwind 4 + Zustand 5 + react-router 7 + i18next。构建产物嵌入 Go 二进制，部署只需一个镜像。
- 未引入 Redis/MQ/外部 DB/微服务，符合「个人自用优先、避免过度工程化」。

## 3. 测试通过情况
- Go：`gofmt`/`go vet`/`go build` 通过；`go test ./...` 全部包通过（含 HTTP 集成测试：登录、上传生命周期与续传、预览、ZIP 与 Zip-Slip、缩略图、标签、文件批量、无令牌 401；双客户端 WS 广播与 `since` 重放；分页并列、编辑保留期限、恢复模式回归测试）。
- 前端：TypeScript 0 错误；ESLint 0 error / 0 warning；Vitest 4 文件 / 23 测试通过；生产构建成功。
- Playwright E2E：mock 后端 10 个用例全部通过（导航、消息流、搜索、上传、画廊、文件、回收站、管理界面、390px 移动端、PWA 分享与 SW）；真实后端 3 个用例全部通过（虚拟 Passkey 注册/登录、完整 TOTP 启用与登录、双设备实时同步），已纳入 CI 的 `e2e-real` Job。
- 真实浏览器连通 Docker 后端：登录、发消息、管理界面、存储统计、设备列表，0 条 console error。
- 详见 `docs/TEST_REPORT.md`。

## 4. Docker 构建情况
三阶段构建（Node 构建前端 → Go `CGO_ENABLED=0` 编译 → Alpine 运行时）：`docker build` 本地成功；GitHub Actions Docker build Job 成功；镜像发布至 GHCR。

## 5. 部署方式
```bash
cp .env.example .env
docker compose up -d
```
单容器，内部端口 8080，对外默认 `${PORT:-8088}`；数据目录 `./data:/data`；容器以 UID 10001 运行，入口脚本仅在必要时修复绑定挂载属主。

## 6. 如何访问
局域网：`http://<NAS-IP>:8088`；公网：自有域名 + HTTPS（推荐 Cloudflare Tunnel + Access），配置 `TRUSTED_PROXY_CIDRS` 后应用才能正确识别公网客户端。所有访问（含公网）都必须应用自身登录。

## 7. 如何初始化
首次启动后仅能从局域网访问初始化向导：创建唯一管理员、设置密码与设备策略。完成后入口永久关闭，重复初始化返回 409。详见 `docs/INSTALL.md`。

## 8. 如何配置公网
见 `docs/PUBLIC_DEPLOY.md` 与 `docs/CLOUDFLARE.md`。Cloudflare 官方限制：Free/Pro 单请求 100 MB、Business 200 MB、Enterprise 自助最高 5 GB；分片上传只绕过应用层限制，代理层限制与超时仍在，不能承诺公网任意大小。

## 9. 如何备份
备份整个 `./data/`（含 `db.sqlite` + WAL/SHM、`files/`、`tmp/`），建议停容器后备份以保证一致性。见 `docs/BACKUP_RESTORE.md`。

## 10. 如何恢复
将备份的 `data/` 还原到同一路径后 `docker compose up -d`；迁移与升级/回滚见 `docs/UPGRADE_ROLLBACK.md`。

## 11. 已知问题
- Passkey（CDP 虚拟验证器）、TOTP 完整启用与登录、双浏览器实时同步已由真实后端 E2E 覆盖并通过。
- 经 Cloudflare Tunnel 的公网请求判定未在真实 Tunnel 环境复测（代码已 fail-closed）；此项需要用户自己的 Cloudflare 账户与隧道。
- 链接解析的重定向复验已自动化通过；真实外网抓取受本沙箱 DNS 拦截限制无法验证（拦截地址段被 SSRF 策略正确拒绝）；DNS 重绑定攻击未单独模拟。
- 视频缩略图与 Office 在线转换降级为下载；iOS 无 Web Share Target。
- 依赖安全扫描（govulncheck / npm audit）为非阻断报告，需人工复核。
- 详见 `docs/KNOWN_LIMITATIONS.md`、`docs/SECURITY.md`。

## 12. 尚未完成的功能
无已知未实现的需求项；上述「已知问题」属于未充分验证或平台能力限制，不计为功能缺失。
