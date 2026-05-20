-- +goose Up
CREATE TABLE IF NOT EXISTS watermill_outbox_events (
    "offset" BIGSERIAL,
    uuid VARCHAR(36) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    payload BYTEA DEFAULT NULL,
    metadata JSON DEFAULT NULL,
    transaction_id xid8 NOT NULL,
    PRIMARY KEY (transaction_id, "offset")
);

CREATE UNIQUE INDEX IF NOT EXISTS watermill_outbox_events_uuid_idx
ON watermill_outbox_events(uuid);

CREATE INDEX IF NOT EXISTS watermill_outbox_events_created_at_idx
ON watermill_outbox_events(created_at);

CREATE TABLE IF NOT EXISTS watermill_offsets_outbox_events (
    consumer_group VARCHAR(255) NOT NULL,
    offset_acked BIGINT,
    last_processed_transaction_id xid8 NOT NULL,
    PRIMARY KEY (consumer_group)
);

-- +goose Down
DROP TABLE IF EXISTS watermill_offsets_outbox_events;
DROP TABLE IF EXISTS watermill_outbox_events;
DROP INDEX IF EXISTS watermill_outbox_events_created_at_idx;
