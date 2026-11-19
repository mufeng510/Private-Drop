# Cloudflare Tunnel / Access 配置与上传限制

本文只讲把已经部署好的 Private Drop 安全地发布到公网的做法，以及必须正视的**上传大小限制**。配置前请先在局域网完成初始化（见 [INSTALL.md](INSTALL.md)）。

## 1. 架构

```
访客浏览器 ──HTTPS──> Cloudflare 边缘 ──Access 鉴权──> Cloudflare Tunnel ──加密通道──> NAS:8088
                                                                                    │
                                                                          应用自身登录（强制）
```

Tunnel 让 NAS 主动连出，**无需公网 IP、无需开放入站端口**。Cloudflare Access 是边缘身份验证，**不能替代应用自身的登录**。

## 2. 建立 Tunnel（推荐 Remotely-managed / 仪表盘管理）

1. 登录 [Cloudflare Zero Trust 仪表盘](https://one.dash.cloudflare.com/)，进入 **Networks → Tunnels → Create a tunnel**，选择 **Cloudflared**。
2. 命名（例如 `private-drop`），复制安装命令中的 token。
3. 在 NAS 上运行 cloudflared（二选一）：
   - **主机方式**（cloudflared 与 NAS 同机，Compose 映射端口 8088）：
     ```bash
     docker run -d --name cloudflared --restart unless-stopped \
       cloudflare/cloudflared:latest tunnel --no-autoupdate run --token <你的TOKEN>
     ```
     然后 Public Hostname 的 Service URL 填 `HTTP://127.0.0.1:8088`。
   - **容器方式**（与 private-drop 在同一个 Docker Compose 项目/网络，可用服务名解析）：
     ```yaml
     # 可另建一个 compose 文件放在同一目录，加入同名默认网络
     services:
       cloudflared:
         image: cloudflare/cloudflared:latest
         restart: unless-stopped
         command: tunnel --no-autoupdate run --token <你的TOKEN>
     ```
     Public Hostname 的 Service URL 填 `HTTP://private-drop:8080`（Compose 默认网络内的服务名）。
4. 在隧道的 **Public Hostname** 页添加：
   - Subdomain：如 `drop`；Domain：选择你的域名（如 `example.com`）。
   - Service Type：`HTTP`；URL：按上述二选一填写。
   - 保存后 Cloudflare 会自动创建代理 DNS 记录（橙色云）。等待隧道状态显示 HEALTHY。
5. 可选：允许用户 POST 表单等一般无需额外设置；WebSocket 由 Tunnel 原生支持。

<details>
<summary>本地配置文件方式（locally-managed tunnel）示例 config.yml</summary>

```yaml
tunnel: <TUNNEL_UUID>
credentials-file: /etc/cloudflared/<TUNNEL_UUID>.json

ingress:
  - hostname: drop.example.com
    service: http://127.0.0.1:8088   # 或在同网络时用 http://private-drop:8080
  - service: http_status:404         # 兜底规则，必需
```

运行：`cloudflared tunnel --config config.yml run`。
</details>

## 3. 配置 Cloudflare Access

1. Zero Trust → **Access → Applications → Add an application → Self-hosted**。
2. 填应用名（Private Drop）、域名 `drop.example.com`、会话时长按需设置。
3. 添加访问策略（Policy）：
   - Action：`Allow`；规则包含你的邮箱（`Email` 等于你自己的地址）或你的企业 IdP 组。
   - 删除/不要保留 `Allow everyone` 之类的宽泛规则；确保默认拒绝。
4. 保存后用无痕窗口访问 `https://drop.example.com`：应被 Access 登录页拦截。
5. 若需要用脚本/CLI 自动调用 API，可为服务账号创建 **Service Token**，并在请求中带 `CF-Access-Client-Id` / `CF-Access-Client-Secret` 头；浏览器上传则依赖 Access Cookie，前端需保持同源请求携带凭据（fetch/XHR 的 `credentials` 设置）。
6. **Access 策略必须覆盖 API 路径**，不要让 `/api/*` 绕过 Access；同时应用自身对 `/api/*`（除初始化状态与登录）的鉴权必须保留。

## 4. 上传大小限制（官方数据，务必如实告知用户）

Cloudflare 对**经代理**的入站请求体有硬性上限，超出返回 `413 Request Entity Too Large`。Tunnel 的 Public Hostname 走代理模式（橙色云），因此**无法用 DNS-only 模式绕过该限制**。

| Cloudflare 套餐 | 单个请求体最大上传 |
|---|---|
| Free | **100 MB** |
| Pro | **100 MB** |
| Business | **200 MB** |
| Enterprise | **最高 5 GB**（可在区域 Network 页自助调整 Maximum Upload Size） |

官方来源（核实日期：2026-11-19）：

- Cloudflare Support, *Error 413*（文档更新 2026-10-29）：<https://developers.cloudflare.com/support/troubleshooting/http-status-codes/4xx-client-error/error-413/>
- Cloudflare Workers, *Limits* → Request and response limits（文档更新 2026-11-18，同为 CDN 入站请求上限）：<https://developers.cloudflare.com/workers/platform/limits/>
- Cloudflare Changelog, *Enterprise self-serve upload limits*（2026-09-04，Enterprise 可自助调到 5 GB）：<https://developers.cloudflare.com/changelog/post/2026-09-04-enterprise-self-serve-upload-limits/>

相关连接超时（同样会影响长时间上传；文档更新 2026-11-13）：

- Proxy Read Timeout：**125 秒**（仅 Enterprise 可配置）；Proxy Write Timeout：**30 秒**；Proxy Idle Timeout：900 秒；URL 上限 16 KB；请求头总计 128 KB。Tunnel 自身的 origin 参数也生效，**两层以更短者为准**。
- 来源：Cloudflare Fundamentals, *Connection limits*：<https://developers.cloudflare.com/fundamentals/reference/connection-limits/>

> 补充：CDN 缓存文件上限（Free/Pro/Business 512 MB、Enterprise 5 GB）是**缓存**能力，不是上传请求体限制，请勿混淆。

### 4.1 Private Drop 的应对方式

- 应用采用**分片上传 + 断点续传**：把每个分片请求控制在限制以下即可绕过“单个请求体”上限。建议把分片大小设为 **8–16 MB**（远低于 100 MB 且给超时留余量），可在管理界面配置分片大小/并发数（以实际版本设置项为准）。
- **分片不能解决所有问题**，必须如实说明：
  - 每个分片请求仍受上面 125 秒读取超时约束，极慢链路上单片过大仍可能 `524`；
  - 总上传时长受 Access 会话时长、浏览器前台状态影响；
  - 本地磁盘空间、应用配置的文件大小上限、浏览器内存仍然生效；
  - 如果单个分片本身就超过套餐上限，或代理层整体拒绝大请求，分片也无能为力。
- **局域网访问不受 Cloudflare 限制**，只受应用自身配置与磁盘空间约束。
- 不得因此引入第三方云存储或改变用户指定的 NAS 存储方案（taskbook 7.4 / 22）。

## 5. 故障排查

| 现象 | 可能原因 | 处理 |
|---|---|---|
| `413 Request Entity Too Large` | 分片大小 ≥ 套餐请求体上限 | 调小分片；核对区域 Network 页 Maximum Upload Size |
| `524 A timeout occurred` | 单片传输超过代理读取超时 | 调小分片、检查 NAS 上行带宽；Enterprise 可申请调高读取超时 |
| `502 / 503` 或隧道断开 | cloudflared 未运行、token 失效、Service URL 写错 | 检查容器日志、隧道 HEALTHY 状态、URL 是否为 8080/8088 |
| Access 登录循环 | Cookie 域/会话配置、时间不同步 | 校准 NAS 时间（NTP）；检查 Access 应用域名 |
| 公网能看到初始化页 | 后端把代理请求误判为局域网 | **立即停止暴露**；在 `.env` 设置 `TRUSTED_PROXY_CIDRS=<cloudflared 源 IP 段>` 使应用按 XFF 判定公网客户端。未配置时代理头初始化请求会被 fail-closed 拒绝 |
| 登录成功但无实时同步 | WebSocket 被中间层重置 | 检查 Tunnel/Access 对 WS 的支持，确认客户端重连补齐 |

## 6. 安全提醒

- Tunnel 隐藏了端口，但**不是访问控制**：务必保留应用密码 + TOTP/Passkey。
- 不要在公网暴露初始化入口或任何调试端点。
- 所有文件预览/下载/复制链接都必须鉴权，不能产生公开永久链接。
- 定期更新 cloudflared 镜像并复核 Access 策略（见 [SECURITY.md](SECURITY.md)）。
