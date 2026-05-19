-- +goose Up
CREATE TABLE IF NOT EXISTS blocked_video_like (
    id BIGSERIAL PRIMARY KEY,
    ban_uid UUID NOT NULL REFERENCES media_ban(ban_uid) ON DELETE CASCADE,
    chat_id BIGINT NOT NULL,
    file_unique_id TEXT NOT NULL,
    source_type TEXT NOT NULL,
    duration_sec INTEGER NOT NULL,
    hash_version TEXT NOT NULL,
    hash_signature TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(chat_id, hash_version, hash_signature)
);

CREATE INDEX IF NOT EXISTS blocked_video_like_ban_uid_idx
ON blocked_video_like(ban_uid);

CREATE TABLE IF NOT EXISTS blocked_video_like_frame_hash (
    video_like_id BIGINT NOT NULL REFERENCES blocked_video_like(id) ON DELETE CASCADE,
    frame_index INTEGER NOT NULL,
    position_millis INTEGER NOT NULL,
    hash_uint64 TEXT NOT NULL,
    PRIMARY KEY(video_like_id, frame_index)
);

-- +goose Down
DROP TABLE IF EXISTS blocked_video_like_frame_hash;
DROP TABLE IF EXISTS blocked_video_like;
