CREATE TABLE IF NOT EXISTS message_links (
    message_id  TEXT NOT NULL,
    preview_id  TEXT NOT NULL,
    PRIMARY KEY (message_id, preview_id)
);
