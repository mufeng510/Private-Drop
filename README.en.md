# Private Drop

A **single-user private message drop** for your personal NAS. Open a web page and instantly move text, photos, videos, audio and files between your phone, PC and tablet. All data stays on your own NAS.

[中文 README](README.md) · [Product spec](taskbook.md) · [Project knowledge (Agent.md)](Agent.md) · [API contract](docs/API_CONTRACT.md)

## Features

- Single user, one shared message space across all devices; LAN access and optional public access (Cloudflare Tunnel + Access + HTTPS recommended)
- Text / images / video / audio / arbitrary files / mixed attachments; multi-file upload, drag & drop and paste
- Chunked upload with resumable tasks, tracked server-side across reloads and devices
- WebSocket real-time sync across devices with auto-reconnect and catch-up by cursor
- File manager: auto categories, tags, search, filters, preview, authenticated single-file and ZIP downloads
- SHA-256 deduplication (one physical copy) with reference counting — deleting a message never removes a file still referenced elsewhere
- 30-day default retention, favorites never expire, recycle bin, pinning, bulk actions
- Password / TOTP / Passkey, device management and new-device approval, step-up verification for admin UI
- Chinese & English UI, PWA (add to home screen), keyboard shortcuts, notification center, logs and audit trail
- Single container: one Go binary (frontend embedded) + SQLite, non-root, data mounted separately

## Quick start (Docker Compose)

Prerequisites: NAS with Docker Compose v5+, ideally ≥2 GiB free RAM.

```bash
# 1. Pick a directory on your NAS (replace with your real path)
mkdir -p /volume1/docker/private-drop && cd /volume1/docker/private-drop

# 2. Get the config files
curl -fLO https://raw.githubusercontent.com/OWNER/private-drop/main/compose.yaml
curl -fLO https://raw.githubusercontent.com/OWNER/private-drop/main/.env.example
cp .env.example .env

# 3. Create the data directory and grant access (container runs as UID 10001)
mkdir -p data && sudo chown -R 10001:10001 data

# 4. Start (build locally)
docker compose up -d --build
# Or use a published image: set IMAGE=ghcr.io/OWNER/private-drop:latest in .env,
# then: docker compose pull && docker compose up -d

# 5. Check status
docker compose ps && docker compose logs -f
```

> Replace `OWNER/private-drop` with the real GitHub repository path. Full instructions: [INSTALL.md](docs/INSTALL.md).

## First-time setup

1. Open `http://<NAS-LAN-IP>:8088` **from a device on your LAN** (default port 8088; the container always listens on 8080).
2. The setup wizard creates the one and only admin account. Pick a strong password, name the device and configure the initial security options.
3. After setup the wizard is **permanently disabled** — no second account can be created, and public visitors cannot register first.
4. Strongly recommended: enable TOTP or Passkey and step-up verification for the admin UI right away.

| Scenario | URL |
|---|---|
| LAN | `http://<NAS-IP>:8088` |
| Public (after configuration) | `https://drop.example.com` |

## Basic configuration

Only three variables are needed for deployment (edit `.env`, then `docker compose up -d`):

| Variable | Default | Meaning |
|---|---|---|
| `PORT` | `8088` | Host port; container port is always 8080 |
| `DATA_DIR` | `/data` | Container data directory, mapped to host `./data` |
| `TZ` | `Asia/Shanghai` | Timezone for log timestamps |

Retention, upload limits/concurrency, device policy, link preview, language, notifications and storage warnings are all configured in the **web admin UI** — keep Compose minimal.

## Documentation

| Doc | Topic |
|---|---|
| [Installation](docs/INSTALL.md) | NAS deployment, directory layout, first setup, password-reset CLI |
| [Public deployment](docs/PUBLIC_DEPLOY.md) | Domain + Tunnel + Access + HTTPS; app auth stays mandatory |
| [Cloudflare setup](docs/CLOUDFLARE.md) | Tunnel/Access steps and **official upload size limits** |
| [Backup & restore](docs/BACKUP_RESTORE.md) | What to back up in `data/`, consistent snapshots, migration |
| [Upgrade & rollback](docs/UPGRADE_ROLLBACK.md) | Image tag strategy, schema migrations, rollback |
| [Security](docs/SECURITY.md) | Security model, SSRF/file safety, log redaction, review requirements |
| [Known limitations](docs/KNOWN_LIMITATIONS.md) | Browser differences, Cloudflare limits, no built-in backup |
| [Testing](docs/TESTING.md) | Test commands, security checklist, report template |
| [API contract](docs/API_CONTRACT.md) | Frontend/backend interface (single source of truth) |

## Tech stack

Go 1.27 (net/http + modernc.org/sqlite + coder/websocket, pure-Go static build) / React 19 + TypeScript + Vite 8 + Tailwind 4 + PWA / single Docker image / SQLite in WAL mode. See [Agent.md](Agent.md).

## License

See [LICENSE](LICENSE).
