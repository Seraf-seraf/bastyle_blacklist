-- +goose Up
CREATE TABLE IF NOT EXISTS index_checkpoints (
    consumer_id TEXT NOT NULL,
    index_name TEXT NOT NULL,
    last_applied_transaction_id xid8 NOT NULL DEFAULT '0'::xid8,
    last_applied_offset BIGINT NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    stale BOOLEAN NOT NULL DEFAULT FALSE,
    stale_reason TEXT NULL,
    PRIMARY KEY(consumer_id, index_name)
);

-- +goose Down
DROP TABLE IF EXISTS index_checkpoints;
