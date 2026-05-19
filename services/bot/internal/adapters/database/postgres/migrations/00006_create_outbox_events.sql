-- +goose Up
CREATE TABLE IF NOT EXISTS outbox_events (
    id BIGSERIAL PRIMARY KEY,
    event_uid UUID NOT NULL UNIQUE,
    event_type TEXT NOT NULL,
    aggregate_type TEXT NOT NULL,
    aggregate_uid UUID NOT NULL,
    payload JSONB NOT NULL,
    attempts INTEGER NOT NULL DEFAULT 0,
    last_error TEXT NULL,
    next_retry_at TIMESTAMPTZ NULL,
    locked_until TIMESTAMPTZ NULL,
    locked_by TEXT NULL,
    published_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS outbox_events_unpublished_idx
ON outbox_events(published_at, next_retry_at, id)
WHERE published_at IS NULL;

CREATE INDEX IF NOT EXISTS outbox_events_aggregate_idx
ON outbox_events(aggregate_uid, id);

-- +goose Down
DROP TABLE IF EXISTS outbox_events;
