# 公网部署指南

Private Drop 首先是为局域网设计的个人中转站。公网访问是**可选**能力，推荐组合：

```
浏览器 --HTTPS--> Cloudflare 边缘（Access 身份认证）--Cloudflare Tunnel--> NAS 上的 private-drop:8080
```

**核心原则：应用自身的身份验证独立且强制。** Cloudflare Access 只是边缘的第一道门，不能替代应用登录；即使 Access 被绕过、被配置为 bypass、或令牌泄露，攻击者仍必须知道应用账号密码（建议再叠加 TOTP/Passkey 与管理界面二次验证）才能读取或修改数据。[taskbook.md 3.2](../taskbook.md) 与 [SECURITY.md](SECURITY.md) 均要求这一点。

## 1. 部署顺序（强烈建议）

1. **先在局域网完成初始化**：打开 `http://<NAS-IP>:8088`，创建唯一管理员、配置认证方式。初始化入口完成后永久关闭。
2. **确认局域网功能正常**：登录、发一条消息、上传一个小文件。
3. **再配置公网**：按 [CLOUDFLARE.md](CLOUDFLARE.md) 建立 Tunnel + Access + HTTPS。
4. **公网复测**：用未登录的浏览器/无痕窗口访问域名，应依次被 Access 与应用登录拦截。

> 初始化页面仅允许局域网访问。请确认后端**不会把来自 Cloudflare Tunnel 的请求IP误判为局域网地址**（Tunnel 入口通常来自 localhost 或容器网桥）。若代理转发的公网请求被当成局域网，公网访客可能看到初始化入口——这是必须在联调时验证的安全项。

## 2. 推荐方案：Cloudflare Tunnel + Access + HTTPS

- **Tunnel**：NAS 主动向外建立加密连接，**无需在路由器开放任何入站端口**，也无需公网 IP。可把 Tunnel 跑在 NAS 主机上（指向 `127.0.0.1:8088`），或作为同一 Docker 网络里的另一个容器（`http://private-drop:8080`）。
- **Access**：在 Cloudflare Zero Trust 中给该域名加一层身份认证（邮箱 OTP / 企业 IdP）。仅允许你自己的邮箱。
- **HTTPS**：Cloudflare 边缘证书自动提供，访客到 Cloudflare 全程 TLS；Cloudflare 到 NAS 走 Tunnel 加密通道。
- 详细逐步配置、示例 `config.yml`、故障排查与**官方上传大小限制**见 [CLOUDFLARE.md](CLOUDFLARE.md)。

## 3. 其他方案（自行负责安全）

- **反向代理 + 自己的证书**：如 Caddy/Nginx/Traefik，把公网域名反代到 `127.0.0.1:8088`。必须正确设置 `X-Forwarded-For`/`X-Forwarded-Proto`，并限制初始化端点仅内网可达；同样**不能关闭应用自身鉴权**。
- **IPv6 / DDNS + 端口映射**：不推荐；直接暴露端口会引来扫描与暴力破解，除非你有成熟的防火墙与 fail2ban 策略。
- **Tailscale / WireGuard 等 VPN**：只把 VPN 网段视为“局域网”时，应用会把 VPN 客户端当作局域网设备，可直接登录；这是最省心的私有访问方式之一，但仍需应用登录。

## 4. 公网访问的固有限制

- **上传大小**：经 Cloudflare 代理的请求体有上限（Free/Pro 100 MB、Business 200 MB、Enterprise 最高 5 GB 自助配置）。应用的分片上传可把单个请求控制在限额内，但**代理层限制依然存在**，不能宣称公网可上传任意大小文件。详见 [CLOUDFLARE.md](CLOUDFLARE.md)。
- **后台上传**：移动端浏览器切到后台或锁屏后，上传可能被系统暂停；请保持页面在前台。详见 [KNOWN_LIMITATIONS.md](KNOWN_LIMITATIONS.md)。
- **WebSocket 实时同步**：Tunnel 支持 WebSocket；若 Access 或代理导致连接被重置，客户端会按游标自动重连补齐。

## 5. 上线检查清单

- [ ] 局域网初始化已完成，初始化入口确认关闭
- [ ] 应用已启用密码 + TOTP 或 Passkey，管理界面二次验证已开启
- [ ] Access 策略仅放行本人邮箱/IdP，其他人为 deny
- [ ] 未登录访客无法看到任何消息/附件；复制的站内链接仍需登录
- [ ] 上传一个接近分片大小的文件，验证 413 / 超时行为可接受
- [ ] 备份任务已覆盖 `./data`（见 [BACKUP_RESTORE.md](BACKUP_RESTORE.md)）
- [ ] 设备授权策略已按需设置，异常设备可远程撤销
