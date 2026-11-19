# 备份与恢复

Private Drop **不提供应用内备份功能**（taskbook 23）。数据完全在你自己的 NAS 上，依赖 NAS 的快照/备份任务。**必须备份整个数据目录，只备份数据库无法恢复附件。**

## 1. 需要备份什么

在 `compose.yaml` 中，宿主机 `./data` 被挂载到容器 `/data`。要把以下内容一起纳入备份：

| 路径（宿主机 `./data` 下） | 内容 | 是否必须 |
|---|---|---|
| `db.sqlite` | 消息、设置、设备、会话、上传任务等全部元数据（**配置也在库里**） | 必须 |
| `db.sqlite-wal`、`db.sqlite-shm` | WAL 模式伴随文件，一致性备份时一起复制 | 必须（与 db 同时） |
| `files/` | 实体附件（UUID 命名，SHA-256 去重后的唯一副本） | 必须 |
| `tmp/` | 未完成任务的分片临时文件 | 建议（可放弃未完成任务，但恢复后任务会失效） |
| `../compose.yaml`、`.env` | 端口、镜像版本、时区等部署配置 | 建议（便于原样重建） |

> 若备份时容器正在运行，SQLite 的 `-wal` 可能尚未 checkpoint，单独拷贝 `db.sqlite` 会丢数据。**推荐停机后备份**。

## 2. 一致性备份（推荐：先停容器）

```bash
cd /volume1/docker/private-drop          # 换成你的部署目录

# 2.1 优雅停止（让 SQLite checkpoint、上传任务安全落盘）
docker compose stop

# 2.2 打包数据 + 部署配置，文件名带日期
STAMP=$(date +%F-%H%M%S)
tar -czf "private-drop-backup-${STAMP}.tar.gz" data/ compose.yaml .env

# 2.3 记录校验和，便于日后验证
sha256sum "private-drop-backup-${STAMP}.tar.gz" > "private-drop-backup-${STAMP}.tar.gz.sha256"

# 2.4 恢复服务
docker compose start

# 2.5 把压缩包复制到 NAS 的其他盘/异地（NAS 快照、rsync、云备份由你决定）
```

把上面步骤加入 NAS 的计划任务（例如每天凌晨），并保留多份、定期做恢复演练。**不要只备份而不验证。**

> 不停机方案：若后端实现了 SQLite 在线备份（如 `VACUUM INTO`），可用其导出一致快照后再复制 `files/`。在确认实现之前，请一律按停机流程操作。

## 3. 恢复

```bash
cd /volume1/docker/private-drop

# 3.1 停容器
docker compose stop

# 3.2 校验备份完整性
sha256sum -c private-drop-backup-YYYYMMDD-HHMMSS.tar.gz.sha256

# 3.3 把现有 data 改名保留，避免混合新旧数据
mv data data.before-restore
tar -xzf private-drop-backup-YYYYMMDD-HHMMSS.tar.gz   # 解出 data/、compose.yaml、.env

# 3.4 确认权限（容器以 UID 10001 运行）
sudo chown -R 10001:10001 data

# 3.5 启动并验证
docker compose up -d
docker compose logs -f
# 登录后检查：消息是否齐全、附件能否预览/下载、设备列表是否正常
```

确认无误后再删除 `data.before-restore`。**不要在容器运行时解压覆盖数据目录。**

## 4. 迁移到新 NAS

1. 在新 NAS 上按 [INSTALL.md](INSTALL.md) 创建目录、放置 `compose.yaml` 与 `.env`，先 `docker compose up -d` 启动一次再停止（让镜像拉取完成），或直接按下面步骤。
2. 停止旧机容器并备份；停止新机容器。
3. 把备份包传到新机，解压得到 `data/`，设置所有者为 `10001:10001`。
4. 在新机 `.env` 中设置相同的 `PORT`、合适的 `TZ`；若新旧 CPU 架构不同（x86_64 ↔ arm64），不要导入本地构建的镜像，直接让 Compose 用 GHCR 多架构镜像重新拉取。
5. `docker compose up -d`，逐项验证消息与附件。
6. 公网访问需在新的 Tunnel 中更新 Service URL；初始化入口应保持关闭（数据已包含管理员）。

## 5. 注意事项

- 收藏、回收站、上传任务、设备授权、日志都可能依赖数据库里的时间戳；恢复后保持系统时间准确（NTP）。
- 保留**下载原文件**能力：去重意味着同一实体只有一份，恢复时 `files/` 缺失会导致数据库有引用但文件打不开。
- 升级/回滚前也请先备份，见 [UPGRADE_ROLLBACK.md](UPGRADE_ROLLBACK.md)。
