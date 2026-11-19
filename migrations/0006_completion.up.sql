-- Audit-driven backend completion (v2).
-- Original relative path captured at upload init so ZIP "keep structure"
-- uses the real client path instead of a fabricated message hash.
ALTER TABLE uploads ADD COLUMN original_path TEXT NOT NULL DEFAULT '';
ALTER TABLE attachments ADD COLUMN original_path TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_uploads_message ON uploads(message_id);

-- Durable WebSocket event log for gap-free reconnect catch-up. Monotonic seq
-- is provided by SQLite AUTOINCREMENT; events survive process restarts.
CREATE TABLE IF NOT EXISTS ws_events (
    seq       INTEGER PRIMARY KEY AUTOINCREMENT,
    type      TEXT NOT NULL,
    data      TEXT NOT NULL DEFAULT '{}',
    ts        TEXT NOT NULL,
    device_id TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_ws_events_ts ON ws_events(ts);
CREATE INDEX IF NOT EXISTS idx_ws_events_device ON ws_events(device_id);
