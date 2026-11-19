# 升级与回滚

镜像标签策略（见 [CI 配置](../.github/workflows/ci.yml)）：

| 标签 | 含义 |
|---|---|
| `latest` | 主分支（main）最近一次成功构建；跟随更新但有变化风险 |
| `1.2.3` / `1.2` | 来自 `v*` 标签的 SemVer 版本，**建议生产使用** |
| `sha-<commit>` | 精确到提交的标签，用于精确回滚/复现 |

## 1. 升级前

```bash
cd /volume1/docker/private-drop

# 记录当前正在运行的镜像（精确到 ID），必要时可作为回滚目标
docker compose images
docker image inspect "$(docker compose images -q private-drop)" --format '{{.Id}} {{.Created}}'

# 必做：按 BACKUP_RESTORE.md 做一致性备份（数据库可能被迁移改写，无法自动回退）
```

阅读该版本的 Release Notes，确认是否包含需要人工干预的变更。

## 2. 升级步骤

**方案 A（推荐，锁定 SemVer）**：编辑 `.env`：

```dotenv
IMAGE=ghcr.io/OWNER/private-drop:1.2.3
```

然后：

```bash
docker compose pull       # 拉取指定标签镜像
docker compose up -d      # Compose 检测到镜像变化会重建容器
docker compose ps
docker compose logs -f
```

**方案 B（跟随 latest）**：保持 `IMAGE=...:latest`：

```bash
docker compose pull
docker compose up -d
```

## 3. 数据库迁移

- 应用启动时会自动执行仓库 `migrations/` 中的前向迁移（如版本支持）。
- 迁移通常是**单向**的：升级后数据库结构可能发生变化；**把镜像回滚到旧版本不代表数据库结构也回退**。
- 因此：
  - 跨版本升级前必须先备份 `data/`；
  - 重大版本建议先在测试机上用备份数据试跑一次；
  - 若新版本启动失败（日志出现迁移错误），停止容器，恢复备份的 `data/`，再用旧镜像启动。

## 4. 回滚步骤

```bash
# 4.1 停止新版本
docker compose down

# 4.2 把 .env 的 IMAGE 改回上一个可用标签（优先用 sha-<commit> 精确回滚）
# IMAGE=ghcr.io/OWNER/private-drop:1.2.2

# 4.3 若新版本已改过数据库结构，先恢复升级前的 data 备份：
# mv data data.after-failed-upgrade
# tar -xzf private-drop-backup-YYYYMMDD-HHMMSS.tar.gz
# sudo chown -R 10001:10001 data

# 4.4 启动旧版本
docker compose up -d
docker compose logs -f
```

只回滚镜像而不恢复数据库仅在「新版本没有改过 schema」时安全；不确定时一律连同 `data/` 一起恢复。

## 5. 升级后验证清单

- [ ] 容器状态 healthy，`GET /api/auth/status` 返回正常
- [ ] 用原有账号登录；MFA/Passkey 仍可用
- [ ] 历史消息与附件可访问、可下载
- [ ] 上传一个小文件 + 一个跨分片文件（若公网则验证 413 行为）
- [ ] WebSocket 实时同步在两台设备上正常
- [ ] 管理界面设置项未丢失
- [ ] 备份任务仍指向正确的 `./data`

## 6. 已知注意点

- `docker compose down` 不会删除 `./data`；但请勿使用 `docker compose down -v`，那会删除命名卷（本方案用 bind mount，风险较低，但仍应避免）。
- 重新构建本地镜像时 `VERSION` 来自 `.env` 或 CI 构建参数；镜像版本号可在管理界面「系统信息」查看（若该版本已实现）。
- 内部端口固定为 8080；修改对外端口只需改 `.env` 的 `PORT`（见 [INSTALL.md](INSTALL.md)）。
