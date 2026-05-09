-- +goose Up
DROP TABLE IF EXISTS blocked_image_hash;
DROP TABLE IF EXISTS blocked_image;

CREATE TABLE blocked_image (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	chat_id INTEGER NOT NULL DEFAULT 0,
	file_unique_id TEXT NOT NULL,
	media_type TEXT NOT NULL,
	hash_version TEXT NOT NULL,
	hash_signature TEXT NOT NULL,
	created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE blocked_image_hash (
	image_id INTEGER NOT NULL,
	variant INTEGER NOT NULL,
	hash_uint64 TEXT NOT NULL,
	PRIMARY KEY (image_id, variant),
	FOREIGN KEY (image_id) REFERENCES blocked_image(id) ON DELETE CASCADE
);

CREATE INDEX blocked_image_hash_version_idx
ON blocked_image(hash_version);

CREATE UNIQUE INDEX blocked_image_chat_hash_signature_idx
ON blocked_image(chat_id, hash_version, hash_signature)
WHERE hash_signature <> '';

-- +goose Down
DROP TABLE IF EXISTS blocked_image_hash;
DROP TABLE IF EXISTS blocked_image;
