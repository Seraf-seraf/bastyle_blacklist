-- +goose Up
DROP TABLE IF EXISTS blocked_video_like_frame_hash;
DROP TABLE IF EXISTS blocked_video_like;

CREATE TABLE blocked_video_like (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	chat_id INTEGER NOT NULL DEFAULT 0,
	file_unique_id TEXT NOT NULL,
	source_type TEXT NOT NULL,
	duration_sec INTEGER NOT NULL,
	hash_version TEXT NOT NULL,
	hash_signature TEXT NOT NULL,
	created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE blocked_video_like_frame_hash (
	video_like_id INTEGER NOT NULL,
	frame_index INTEGER NOT NULL,
	position_millis INTEGER NOT NULL,
	hash_uint64 TEXT NOT NULL,
	PRIMARY KEY (video_like_id, frame_index),
	FOREIGN KEY (video_like_id) REFERENCES blocked_video_like(id) ON DELETE CASCADE
);

CREATE INDEX blocked_video_like_hash_version_idx
ON blocked_video_like(hash_version);

CREATE UNIQUE INDEX blocked_video_like_chat_hash_signature_idx
ON blocked_video_like(chat_id, hash_version, hash_signature)
WHERE hash_signature <> '';

-- +goose Down
DROP TABLE IF EXISTS blocked_video_like_frame_hash;
DROP TABLE IF EXISTS blocked_video_like;
