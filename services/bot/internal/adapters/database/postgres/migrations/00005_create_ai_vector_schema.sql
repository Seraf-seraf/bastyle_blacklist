-- +goose Up
CREATE TABLE IF NOT EXISTS ai_vector_ban (
    id BIGSERIAL PRIMARY KEY,
    ban_uid UUID NOT NULL REFERENCES media_ban(ban_uid) ON DELETE CASCADE,
    chat_id BIGINT NOT NULL,
    file_unique_id TEXT NOT NULL,
    media_type TEXT NOT NULL,
    model_name TEXT NOT NULL,
    model_revision TEXT NOT NULL,
    vector_dim INTEGER NOT NULL CHECK (vector_dim > 0),
    frames_count INTEGER NOT NULL CHECK (frames_count > 0),
    active BOOLEAN NOT NULL DEFAULT TRUE,
    vector_signature TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deactivated_at TIMESTAMPTZ NULL,
    UNIQUE(chat_id, model_name, model_revision, vector_signature)
);

CREATE INDEX IF NOT EXISTS ai_vector_ban_active_model_idx
ON ai_vector_ban(active, model_name, model_revision, vector_dim);

CREATE INDEX IF NOT EXISTS ai_vector_ban_chat_active_model_idx
ON ai_vector_ban(chat_id, active, model_name, model_revision, vector_dim);

CREATE TABLE IF NOT EXISTS ai_vector_frame (
    id BIGSERIAL PRIMARY KEY,
    ban_id BIGINT NOT NULL REFERENCES ai_vector_ban(id) ON DELETE CASCADE,
    frame_index INTEGER NOT NULL,
    position_millis INTEGER NOT NULL,
    vector_blob BYTEA NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(ban_id, frame_index)
);

CREATE INDEX IF NOT EXISTS ai_vector_frame_ban_idx
ON ai_vector_frame(ban_id, frame_index);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION validate_ai_vector_frame_blob_size()
RETURNS trigger AS $$
DECLARE
    expected_vector_dim INTEGER;
BEGIN
    SELECT vector_dim
    INTO expected_vector_dim
    FROM ai_vector_ban
    WHERE id = NEW.ban_id;

    IF expected_vector_dim IS NULL THEN
        RAISE EXCEPTION 'ai_vector_ban % not found', NEW.ban_id
            USING ERRCODE = 'foreign_key_violation';
    END IF;

    IF octet_length(NEW.vector_blob) != expected_vector_dim * 4 THEN
        RAISE EXCEPTION 'ai_vector_frame vector_blob size % does not match vector_dim %',
            octet_length(NEW.vector_blob), expected_vector_dim
            USING ERRCODE = 'data_exception';
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS ai_vector_frame_blob_size_check ON ai_vector_frame;
CREATE TRIGGER ai_vector_frame_blob_size_check
BEFORE INSERT OR UPDATE OF vector_blob, ban_id
ON ai_vector_frame
FOR EACH ROW
EXECUTE FUNCTION validate_ai_vector_frame_blob_size();

CREATE TABLE IF NOT EXISTS ai_vector_index_state (
    model_name TEXT NOT NULL,
    model_revision TEXT NOT NULL,
    vector_dim INTEGER NOT NULL CHECK (vector_dim > 0),
    index_type TEXT NOT NULL,
    index_path TEXT NOT NULL,
    active_vectors_count INTEGER NOT NULL CHECK (active_vectors_count >= 0),
    active_vectors_hash TEXT NOT NULL,
    index_file_sha256 TEXT NOT NULL,
    rebuilt_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY(model_name, model_revision, vector_dim, index_type)
);

-- +goose Down
DROP TABLE IF EXISTS ai_vector_index_state;
DROP TRIGGER IF EXISTS ai_vector_frame_blob_size_check ON ai_vector_frame;
DROP FUNCTION IF EXISTS validate_ai_vector_frame_blob_size();
DROP TABLE IF EXISTS ai_vector_frame;
DROP TABLE IF EXISTS ai_vector_ban;
