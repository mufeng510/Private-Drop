-- Private Drop initial schema. All timestamps are RFC3339 UTC strings.

CREATE TABLE IF NOT EXISTS users (
    id            TEXT PRIMARY KEY,
    username      TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    totp_secret   TEXT,
    webauthn_id   BLOB,
    created_at    TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS devices (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL,
    name        TEXT NOT NULL,
    type        TEXT NOT NULL,
    ip          TEXT NOT NULL DEFAULT '',
    user_agent  TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL,
    last_active TEXT NOT NULL,
    authorized  INTEGER NOT NULL DEFAULT 1,
    revoked_at  TEXT
);
CREATE INDEX IF NOT EXISTS idx_devices_user ON devices(user_id);

CREATE TABLE IF NOT EXISTS sessions (
    id         TEXT PRIMARY KEY,           -- SHA-256 of the bearer token
    device_id  TEXT NOT NULL,
    user_id    TEXT NOT NULL,
    expires_at TEXT,                       -- NULL means "forever"
    created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sessions_device ON sessions(device_id);

CREATE TABLE IF NOT EXISTS device_requests (
    id          TEXT PRIMARY KEY,
    device_id   TEXT NOT NULL,
    name        TEXT NOT NULL,
    type        TEXT NOT NULL,
    ip          TEXT NOT NULL DEFAULT '',
    device_info TEXT NOT NULL DEFAULT '',
    code        TEXT NOT NULL UNIQUE,
    status      TEXT NOT NULL DEFAULT 'pending',
    created_at  TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS messages (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL,
    device_id   TEXT NOT NULL,
    text        TEXT NOT NULL DEFAULT '',
    pinned      INTEGER NOT NULL DEFAULT 0,
    favorited   INTEGER NOT NULL DEFAULT 0,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL,
    expires_at  TEXT,                     -- NULL means "never expires" (favorited)
    deleted_at  TEXT,                     -- non-NULL means in recycle bin
    restore_mode TEXT
);
CREATE INDEX IF NOT EXISTS idx_messages_created ON messages(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_messages_deleted ON messages(deleted_at);
CREATE INDEX IF NOT EXISTS idx_messages_expires ON messages(expires_at);

CREATE TABLE IF NOT EXISTS entity_files (
    id         TEXT PRIMARY KEY,
    sha256     TEXT NOT NULL UNIQUE,
    path       TEXT NOT NULL,
    size       INTEGER NOT NULL,
    mime       TEXT NOT NULL,
    ref_count  INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS attachments (
    id          TEXT PRIMARY KEY,
    entity_id   TEXT NOT NULL,
    name        TEXT NOT NULL,
    mime        TEXT NOT NULL,
    size        INTEGER NOT NULL,
    sha256      TEXT NOT NULL,
    width       INTEGER NOT NULL DEFAULT 0,
    height      INTEGER NOT NULL DEFAULT 0,
    favorited   INTEGER NOT NULL DEFAULT 0,
    uploaded_at TEXT NOT NULL,
    deleted_at  TEXT
);
CREATE INDEX IF NOT EXISTS idx_attachments_entity ON attachments(entity_id);

CREATE TABLE IF NOT EXISTS message_attachments (
    message_id    TEXT NOT NULL,
    attachment_id TEXT NOT NULL,
    PRIMARY KEY (message_id, attachment_id)
);
CREATE INDEX IF NOT EXISTS idx_ma_attachment ON message_attachments(attachment_id);

CREATE TABLE IF NOT EXISTS tags (
    id        TEXT PRIMARY KEY,
    name      TEXT NOT NULL UNIQUE,
    color     TEXT NOT NULL DEFAULT '',
    icon      TEXT NOT NULL DEFAULT '',
    sort      INTEGER NOT NULL DEFAULT 0,
    is_system INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS message_tags (
    message_id TEXT NOT NULL,
    tag_id     TEXT NOT NULL,
    PRIMARY KEY (message_id, tag_id)
);

CREATE TABLE IF NOT EXISTS attachment_tags (
    attachment_id TEXT NOT NULL,
    tag_id        TEXT NOT NULL,
    PRIMARY KEY (attachment_id, tag_id)
);

CREATE TABLE IF NOT EXISTS uploads (
    id              TEXT PRIMARY KEY,
    device_id       TEXT NOT NULL,
    message_id      TEXT,
    filename        TEXT NOT NULL,
    size            INTEGER NOT NULL,
    mime            TEXT NOT NULL,
    sha256          TEXT NOT NULL DEFAULT '',
    total_chunks    INTEGER NOT NULL,
    chunk_size      INTEGER NOT NULL,
    uploaded_chunks TEXT NOT NULL DEFAULT '[]',
    status          TEXT NOT NULL DEFAULT 'uploading',
    attachment_id   TEXT,
    error           TEXT NOT NULL DEFAULT '',
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_uploads_status ON uploads(status);

CREATE TABLE IF NOT EXISTS notifications (
    id         TEXT PRIMARY KEY,
    type       TEXT NOT NULL,
    payload    TEXT NOT NULL DEFAULT '{}',
    read       INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS audit_logs (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    action     TEXT NOT NULL,
    detail     TEXT NOT NULL DEFAULT '',
    ip         TEXT NOT NULL DEFAULT '',
    device_id  TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS logs (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    type       TEXT NOT NULL,
    message    TEXT NOT NULL,
    created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_logs_type ON logs(type);

CREATE TABLE IF NOT EXISTS link_previews (
    id          TEXT PRIMARY KEY,
    url         TEXT NOT NULL UNIQUE,
    title       TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    image_url   TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS mfa_tokens (
    token_hash TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL,
    device_id  TEXT NOT NULL DEFAULT '',
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL
);

-- Default settings.
INSERT OR IGNORE INTO settings(key, value) VALUES
    ('retention_days', '30'),
    ('recycle_days', '30'),
    ('max_upload_size', '104857600'),
    ('upload_concurrency', '3'),
    ('chunk_size', '1048576'),
    ('storage_warning_percent', '90'),
    ('device_policy', 'open'),
    ('link_preview_enabled', 'true'),
    ('language', 'zh'),
    ('session_ttl_days', '30'),
    ('initialized', 'false');

-- System tags derived from attachment/message type.
INSERT OR IGNORE INTO tags(id, name, color, icon, sort, is_system) VALUES
    ('sys-text',  '文本',  '#64748b', 'text',  0, 1),
    ('sys-image', '图片',  '#0ea5e9', 'image', 1, 1),
    ('sys-video', '视频',  '#8b5cf6', 'video', 2, 1),
    ('sys-audio', '音频',  '#f59e0b', 'audio', 3, 1),
    ('sys-file',  '文件',  '#10b981', 'file',  4, 1);
