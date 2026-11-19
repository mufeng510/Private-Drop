-- Rollback for the v2 completion migration.
DROP INDEX IF EXISTS idx_ws_events_device;
DROP INDEX IF EXISTS idx_ws_events_ts;
DROP TABLE IF EXISTS ws_events;
DROP INDEX IF EXISTS idx_uploads_message;
ALTER TABLE attachments DROP COLUMN original_path;
ALTER TABLE uploads DROP COLUMN original_path;
