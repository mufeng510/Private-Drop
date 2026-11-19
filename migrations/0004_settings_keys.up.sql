-- Additional configurable settings surfaced by the admin UI.
INSERT OR IGNORE INTO settings(key, value) VALUES
    ('login_attempt_limit', '5'),
    ('notify_new_device', 'true'),
    ('notify_upload_done', 'true'),
    ('notify_upload_failed', 'true'),
    ('notify_low_storage', 'true'),
    ('notify_expiring', 'true');
