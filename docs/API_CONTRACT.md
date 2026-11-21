# API 契约（Private Drop）

前后端以此为准。所有响应 JSON（除流式下载/WS）。所有需鉴权接口在 Header 携带 `Authorization: Bearer <token>`。
所有请求（含登录/初始化）还应携带稳定的客户端设备标识 `X-Device-Key: <client-generated-uuid>`；服务端据此识别设备，**不使用可编辑的设备名称**。缺失时回退到 UA+IP 严格匹配。
设备管理接口（列表/重命名/撤销/审批/拒绝）额外要求 `X-MFA-Token`；一次性授权码审批（`/api/devices/requests/code`）除外，它是新设备 bootstrap 流程。
错误响应统一：`{ "error": { "code": "STRING_CODE", "message": "..." } }`，HTTP 状态码语义化。
时间统一 RFC3339（UTC）。分页用游标：`?cursor=<opaque>&limit=<n>`，返回 `{ items: [...], nextCursor: string|null }`。

## 认证与初始化
| 方法 | 路径 | 说明 | 请求体 | 响应 |
|---|---|---|---|---|
| GET | /api/auth/status | 是否已初始化 | - | `{ initialized: bool, lanOnly: bool }` |
| POST | /api/auth/init | 首次初始化（仅局域网），创建唯一管理员 | `{ username, password, deviceName?, policy? }` | `{ token, user, device }`；初始化后入口关闭 |
| POST | /api/auth/login | 密码登录 | `{ username, password, deviceName?, totp? }` | `{ token?, user, device, mfaRequired?: "totp"\|"passkey" }` |
| POST | /api/auth/logout | 主动退出当前会话 | - | `{}` |
| GET | /api/auth/me | 当前用户与设备 | - | `{ user, device, settings, mfaValid }`（`mfaValid` 表示请求携带的 `X-MFA-Token` 是否仍有效；前端据此决定是否显示管理界面二次验证门禁，不再仅凭本地是否有令牌判断） |
| POST | /api/auth/refresh | 刷新令牌 | - | `{ token }` |
| POST | /api/auth/totp/setup | 生成 TOTP 密钥 | - | `{ secret, otpauthUrl, qr }` |
| POST | /api/auth/totp/enable | 启用 TOTP | `{ code }` | `{}` |
| POST | /api/auth/passkey/register/begin | 注册 Passkey 开始 | - | WebAuthn creation options |
| POST | /api/auth/passkey/register/finish | 注册 Passkey 完成 | WebAuthn credential | `{}` |
| POST | /api/auth/passkey/login/begin | Passkey 登录开始 | `{ username? }` | WebAuthn request options |
| POST | /api/auth/passkey/login/finish | Passkey 登录完成 | WebAuthn assertion | `{ token, user, device }` |
| POST | /api/auth/mfa/verify | 管理界面二次验证 | `{ type: "totp"\|"passkey", code? , assertion? }` | `{ mfaToken }` |

## 设备管理
| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /api/devices | 设备列表 |
| PATCH | /api/devices/:id | 重命名 `{ name }` |
| POST | /api/devices/:id/revoke | 远程撤销/退出 |
| GET | /api/devices/requests | 待审批新设备列表 |
| POST | /api/devices/requests/:id/approve | 批准 `{ }` |
| POST | /api/devices/requests/:id/reject | 拒绝 |
| POST | /api/devices/requests/code | 用一次性码授权 `{ code }` |

## 消息
| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /api/messages | `cursor,limit,sort(newest\|oldest\|favorite\|pinned),type,favorite,tag,from,to` → `{items,nextCursor}` |
| POST | /api/messages | 创建 `{ text?, attachmentIds?: [] }` → Message |
| GET | /api/messages/:id | 详情 |
| PATCH | /api/messages/:id | 编辑 `{ text?, pinned?, favorited?, tagIds? }` → Message |
| DELETE | /api/messages/:id | 移入回收站（软删除） |
| POST | /api/messages/bulk | `{ ids:[], action:"delete"\|"favorite"\|"unfavorite"\|"pin"\|"unpin"\|"tag"\|"untag"\|"restore", tagId? }` |
| GET | /api/messages/expiring | 即将过期列表 |

Message 结构：
```
{ id, text, pinned, favorited, createdAt, updatedAt, expiresAt, deletedAt,
  device: { id, name, type },
  attachments: [ AttachmentRef ],
  tags: [ Tag ],
  links: [ LinkPreview ] }
```

## 附件 / 文件
| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /api/files | `cursor,limit,type,tag,favorite,from,to,sort` → 文件列表（Attachment） |
| GET | /api/files/:id | 附件详情 |
| PATCH | /api/files/:id | `{ favorited?, name? }` |
| DELETE | /api/files/:id | 解除引用并移入回收站 |
| GET | /api/files/:id/download | 鉴权下载（单文件） |
| POST | /api/files/zip | `{ ids:[], keepStructure:bool }` → 流式 ZIP |
| GET | /api/files/:id/thumbnail | 缩略图（图片/视频） |
| GET | /api/files/:id/preview | 在线预览（PDF/TXT/MD/代码等） |

Attachment 结构：
```
{ id, name, mime, size, sha256, width?, height?, favorited, uploadedAt,
  type:"image"\|"video"\|"audio"\|"pdf"\|"office"\|"text"\|"code"\|"other",
  messageIds: [] }
```

## 上传（分片 + 断点续传）
| 方法 | 路径 | 说明 |
|---|---|---|
| POST | /api/uploads/init | `{ name, size, mime, sha256?, totalChunks, chunkSize }` → `{ uploadId, uploadedChunks:[int], reusedAttachmentId? }` |
| PUT | /api/uploads/:id/chunk/:index | Body=二进制分片；返回 `{ received:true }` |
| POST | /api/uploads/:id/complete | 合并、去重、建引用 → `{ attachmentId }` |
| GET | /api/uploads/:id/status | `{ uploadId, uploadedChunks:[], status }` |
| GET | /api/uploads | 上传任务列表（跨设备） |
| DELETE | /api/uploads/:id | 取消上传 + 清理临时分片 |
| POST | /api/uploads/:id/retry | 重试失败任务 |

- 服务端持久化 `uploaded_chunks`，刷新/关闭浏览器后可查询恢复。
- `sha256` 命中已有实体时返回 `reusedAttachmentId`，前端可直接用于发送消息。
- 上传进度事件通过 WS `upload.progress` 广播。

## 搜索
| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /api/search | `q,type,ext,minSize,maxSize,from,to,tag,favorite,pinned,device,cursor,limit` → `{items,nextCursor}`（混合消息与文件结果） |

## 标签
| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /api/tags | 全部标签（系统 + 用户） |
| POST | /api/tags | `{ name, color?, icon?, sort? }` |
| PATCH | /api/tags/:id | 更新 |
| DELETE | /api/tags/:id | 删除（仅用户标签） |

## 回收站
| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /api/recycle | 列表 |
| POST | /api/recycle/:id/restore | `{ mode:"reset"\|"original" }` |
| DELETE | /api/recycle/:id | 彻底删除 |
| DELETE | /api/recycle | 清空回收站 |

## 通知
| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /api/notifications | 列表 + 未读数 `{items, unread}` |
| POST | /api/notifications/read-all | 全部已读 |
| DELETE | /api/notifications/:id | 清除 |

## 设置（管理界面，需 mfaToken）
| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /api/settings | 全部设置（敏感项脱敏） |
| PATCH | /api/settings | `{ key:value, ... }` |
| GET | /api/settings/security | 安全设置 |
| POST | /api/settings/password | `{ oldPassword, newPassword }` |

## 日志 / 审计 / 存储 / 诊断
| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /api/logs | `type(app\|upload\|clean\|error\|security)` |
| GET | /api/audit | 审计日志 |
| GET | /api/storage/stats | 空间统计、大文件、重复、即将过期、回收站/收藏占用 |
| GET | /api/system/info | 端口、版本、数据目录、运行状态 |
| GET | /api/system/diagnose | 诊断结果 |

## 链接解析
| 方法 | 路径 | 说明 |
|---|---|---|
| POST | /api/link-preview | `{ url }` → `{ title?, description?, imageUrl? }`；失败返回空不阻断 |

## 补全契约（审计后新增，v2）
- `POST /api/uploads/init` 增加可选 `messageId`：上传任务绑定消息，消息删除/过期时服务端取消任务并清理分片。
- 上传状态增加可选 `bytesUploaded`、`bytesRemaining`、`speed`（字节/秒），状态可为 `needs_source`（原文件不可访问，需用户重新选择）。
- `POST /api/files/bulk`：`{ ids:[], action:"favorite"|"unfavorite"|"delete"|"restore"|"tag"|"untag", tagId? }`，附件批量操作。
- `PATCH /api/files/:id` 增加 `tagIds?: string[]`（附件独立标签）。
- `POST /api/messages/bulk` 的 `restore` 动作支持 `mode:"reset"|"original"`。
- `GET /api/storage/stats` 的 `warnings` 为实际磁盘空间检查结果，`suggestions` 给出清理建议。
- 链接解析：`POST /api/messages` 时后端按设置自动解析文本中的 URL，持久化到 `link_previews`/`message_links`，并随消息返回 `links`；`imageUrl` 需经同一 SSRF 策略校验。
- 新增 WS 事件：`file.updated`、`file.deleted`、`file.restored`、`upload.needs_source`；`upload.progress` 携带上述字节字段。

## WebSocket
连接：`WS /api/ws?token=<token>`（或首包鉴权）。
服务端推送事件帧：`{ "type": "...", "data": {...}, "ts": "..." }`
事件类型：`message.created` / `message.updated` / `message.deleted` / `upload.progress` / `upload.completed` / `notification` / `device.new` / `device.revoked` / `settings.changed`。
客户端发送：`{ "type":"subscribe"|"ping", ... }`。重连后客户端带 `?since=<ts或游标>` 请求补齐。
