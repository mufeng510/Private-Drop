CREATE TABLE IF NOT EXISTS passkey_credentials (
    id               TEXT PRIMARY KEY,
    user_id          TEXT NOT NULL,
    public_key       BLOB NOT NULL,
    attestation_type TEXT NOT NULL DEFAULT '',
    sign_count       INTEGER NOT NULL DEFAULT 0,
    transports       TEXT NOT NULL DEFAULT '',
    created_at       TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_passkey_user ON passkey_credentials(user_id);
