-- +goose Up
CREATE TABLE IF NOT EXISTS blocked_exact (
    chat_id INTEGER NOT NULL,
    file_unique_id TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (chat_id, file_unique_id)
);

-- +goose Down
DROP TABLE IF EXISTS blocked_exact;
