-- +goose Up
CREATE TABLE IF NOT EXISTS blocked_exact (
    ban_uid UUID PRIMARY KEY REFERENCES media_ban(ban_uid) ON DELETE CASCADE,
    chat_id BIGINT NOT NULL,
    file_unique_id TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(chat_id, file_unique_id)
);

-- +goose Down
DROP TABLE IF EXISTS blocked_exact;
