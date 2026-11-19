DELETE FROM settings WHERE key IN (
    'login_attempt_limit', 'notify_new_device', 'notify_upload_done',
    'notify_upload_failed', 'notify_low_storage', 'notify_expiring'
);
