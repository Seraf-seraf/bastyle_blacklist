-- +goose Up
CREATE TABLE IF NOT EXISTS media_ban (
    ban_uid UUID PRIMARY KEY,
    chat_id BIGINT NOT NULL,
    media_type TEXT NOT NULL,
    file_unique_id TEXT NOT NULL,
    created_by_user_id BIGINT NULL,
    created_from_message_id BIGINT NULL,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    origin_replica_id TEXT NULL,
    source_event_id UUID NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deactivated_at TIMESTAMPTZ NULL
);

CREATE INDEX IF NOT EXISTS media_ban_chat_active_idx
ON media_ban(chat_id, active);

CREATE INDEX IF NOT EXISTS media_ban_active_created_idx
ON media_ban(active, created_at);

-- +goose Down
DROP TABLE IF EXISTS media_ban;
