-- +bastyle Up
CREATE TABLE IF NOT EXISTS ai_vector_ban (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    chat_id INTEGER NOT NULL DEFAULT 0,
    file_unique_id TEXT NOT NULL,
    media_type TEXT NOT NULL,
    model_name TEXT NOT NULL,
    model_revision TEXT NOT NULL,
    vector_dim INTEGER NOT NULL,
    frames_count INTEGER NOT NULL,
    active INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS ai_vector_frame (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    ban_id INTEGER NOT NULL,
    frame_index INTEGER NOT NULL,
    position_millis INTEGER NOT NULL,
    vector_blob BLOB NOT NULL,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (ban_id, frame_index),
    FOREIGN KEY (ban_id) REFERENCES ai_vector_ban(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS ai_vector_index_state (
    model_name TEXT NOT NULL,
    model_revision TEXT NOT NULL,
    vector_dim INTEGER NOT NULL,
    index_type TEXT NOT NULL,
    index_path TEXT NOT NULL,
    active_vectors_count INTEGER NOT NULL,
    active_vectors_hash TEXT NOT NULL DEFAULT '',
    index_file_sha256 TEXT NOT NULL DEFAULT '',
    rebuilt_at TEXT NOT NULL,
    PRIMARY KEY (model_name, model_revision, vector_dim, index_type)
);

CREATE INDEX IF NOT EXISTS ai_vector_ban_active_model_idx
ON ai_vector_ban(active, model_name, model_revision, vector_dim);

CREATE INDEX IF NOT EXISTS ai_vector_ban_chat_active_model_idx
ON ai_vector_ban(chat_id, active, model_name, model_revision, vector_dim);

CREATE INDEX IF NOT EXISTS ai_vector_frame_ban_idx
ON ai_vector_frame(ban_id, frame_index);

-- +bastyle Down
DROP TABLE IF EXISTS ai_vector_frame;
DROP TABLE IF EXISTS ai_vector_ban;
DROP TABLE IF EXISTS ai_vector_index_state;
