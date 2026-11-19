# 测试指南与测试报告模板

本项目使用 Go 官方测试 + Vitest + Playwright + 安全扫描的组合。命令以仓库根目录为基准（前端部分在 `web/` 下执行）。

## 1. 后端（Go）

```bash
go vet ./...                    # 静态检查（改完 Go 必跑）
go test ./...                   # 单元 + 集成测试
go test -race ./...             # 竞态检测（并发/上传/WS 相关）
go test -cover ./...            # 覆盖率
go build ./...                  # 全量构建（server + resetpwd）
```

重点覆盖（taskbook 27）：消息创建/编辑/收藏/置顶/删除/恢复/过期与回收站、引用计数与去重、设备授权与权限检查、登录与 MFA、分片上传与断点续传、下载鉴权、ZIP、WebSocket 重连补齐、自动清理、SSRF 拦截、初始化关闭。

## 2. 前端（Node / web/）

```bash
cd web
npm ci                          # 严格按 lockfile 安装
npm run typecheck               # tsc --noEmit（改完 TS 必跑）
npm test                        # Vitest 单元/组件测试
npm run build                   # Vite 生产构建（同时验证嵌入产物可生成）
```

E2E（如仓库已配置 Playwright）：

```bash
cd web && npx playwright test   # 页面导航、消息流、搜索、上传、预览、文件管理、回收站、管理界面、响应式、PWA
```

> 前端开发代理会确认 `package.json` 中实际存在的脚本名；若脚本名不同，以实际文件为准并同步更新本文档与 CI。

## 3. Docker 与部署

```bash
docker compose config            # 校验 compose 语法与变量替换（不启动容器）
docker build -t private-drop:test .   # 验证多阶段镜像可构建
docker compose up -d && docker compose ps   # 验证容器 healthy
curl -i http://127.0.0.1:8088/api/auth/status  # 验证未鉴权健康检查端点
```

## 4. 安全测试

```bash
go install golang.org/x/vuln/cmd/govulncheck@latest
govulncheck ./...                                  # Go 依赖已知漏洞
cd web && npm audit --audit-level=high             # 前端依赖漏洞
```

手工/自动化安全验证清单（taskbook 26/27/29）：

- [x] 未登录访问 `/api/messages`、`/api/files/*` 被拒绝 — `internal/api/api_test.go`、`internal/api/completion_test.go` + 真实浏览器冒烟
- [x] 未授权无法下载附件/缩略图/ZIP — `TestPreviewThumbnailZIPAndSecurityHTTP`（无令牌均 401）；复制的站内链接无令牌同样 401（所有端点统一鉴权）
- [x] 撤销设备后会话立即失效 — `internal/auth/auth_test.go`；WS 连接由 `Hub.DisconnectDevice` 主动关闭（代码路径已实现，未做双浏览器联调）
- [x] 初始化完成后无法再次创建管理员 — `TestInitOnceAndLogin` + 代理头 fail-closed 用例
- [x] 管理界面二次验证 — API 门禁测试 `TestMFAGateOnSettings` + 浏览器验证；Passkey/真实 TOTP 设备流程未端到端测试
- [ ] SSRF：localhost / 私网 / 元数据已由 `TestSSRFBlocking` 覆盖；**DNS 重绑定与重定向后的二次校验未做自动化测试**（实现已包含该防护）
- [x] 恶意文件名不能越出实体目录 — `TestSanitizeFileName` + ZIP Zip-Slip 测试（含 Windows 盘符前缀）
- [x] 删除消息不会误删其他消息引用的实体文件 — `TestDedupAndRefCount`、`internal/clean` 清理测试
- [x] 日志中不含密码/令牌/密钥/文件内容 — 只读审查确认（审计详情与请求日志仅记路径与操作名）
- [ ] 经 Cloudflare Tunnel/Access 的公网请求不会被当成局域网请求 — 代码已实现 fail-closed + `TRUSTED_PROXY_CIDRS`，但**未在真实 Tunnel 环境验证**

CI 中 govulncheck / npm audit 为**非阻断**，结果写入 Job Summary；人工审查不可省略，见 [SECURITY.md](SECURITY.md)。

## 5. 测试报告模板

> 将下表复制到 `docs/TEST_REPORT.md` 并在验收时填写真实结果。**不要预填“通过”。**

```markdown
# Private Drop 测试报告

- 日期：
- 测试人：
- 代码版本（commit SHA / 镜像 tag）：
- 环境：OS / Go / Node / Docker / NAS 型号与架构：

## 1. 自动化结果
| 项目 | 命令 | 结果(通过/失败/未运行) | 关键输出/失败原因 |
|---|---|---|---|
| Go vet | go vet ./... | | |
| Go 测试 | go test ./... | | |
| Go 竞态 | go test -race ./... | | |
| Go 覆盖率 | go test -cover ./... | | 覆盖率: |
| TS 类型检查 | npm run typecheck | | |
| 前端单测 | npm test | | |
| 前端构建 | npm run build | | |
| E2E | npx playwright test | | |
| Compose 校验 | docker compose config | | |
| 镜像构建 | docker build -t private-drop:test . | | |

## 2. 安全扫描与审查
| 项目 | 结果 | 发现的问题 | 处理方式 |
|---|---|---|---|
| govulncheck | | | |
| npm audit | | | |
| 人工安全审查 | | | |

## 3. 功能验收（48 条，逐条标注 通过/失败/不适用 + 证据）
- 1. Docker Compose 部署：
- 2. 首次初始化创建唯一管理员：
- ...（按 taskbook 29 逐条填写）

## 4. 已知失败 / 风险 / 未覆盖项

## 5. 结论与签字
```
