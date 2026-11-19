DROP INDEX IF EXISTS idx_devices_client_key;
ALTER TABLE devices DROP COLUMN client_key;
