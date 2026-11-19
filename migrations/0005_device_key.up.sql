-- Stable client-generated device key. Device identity must not depend on the
-- user-editable display name, otherwise any password holder can impersonate an
-- already-approved device by choosing the same name.
ALTER TABLE devices ADD COLUMN client_key TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_devices_client_key ON devices(user_id, client_key);
