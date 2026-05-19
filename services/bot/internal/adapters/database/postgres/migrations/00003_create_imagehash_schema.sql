-- +goose Up
CREATE TABLE IF NOT EXISTS blocked_image (
    id BIGSERIAL PRIMARY KEY,
    ban_uid UUID NOT NULL REFERENCES media_ban(ban_uid) ON DELETE CASCADE,
    chat_id BIGINT NOT NULL,
    file_unique_id TEXT NOT NULL,
    media_type TEXT NOT NULL,
    hash_version TEXT NOT NULL,
    hash_signature TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(chat_id, hash_version, hash_signature)
);

CREATE INDEX IF NOT EXISTS blocked_image_ban_uid_idx
ON blocked_image(ban_uid);

CREATE TABLE IF NOT EXISTS blocked_image_hash (
    image_id BIGINT NOT NULL REFERENCES blocked_image(id) ON DELETE CASCADE,
    variant INTEGER NOT NULL,
    hash_uint64 TEXT NOT NULL,
    PRIMARY KEY(image_id, variant)
);

-- +goose Down
DROP TABLE IF EXISTS blocked_image_hash;
DROP TABLE IF EXISTS blocked_image;
