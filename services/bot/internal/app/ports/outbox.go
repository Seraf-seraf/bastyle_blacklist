package ports

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type NewOutboxEvent struct {
	EventUID      uuid.UUID
	EventType     string
	AggregateType string
	AggregateUID  uuid.UUID
	Payload       json.RawMessage
}

type OutboxStats struct {
	UnpublishedCount       int64
	UnpublishedMaxAge      time.Duration
	UnpublishedAttemptsSum int64
	UnpublishedMaxAttempts int
}

type OutboxWriter interface {
	Save(ctx context.Context, tx pgx.Tx, event NewOutboxEvent) error
}

type OutboxStore interface {
	Stats(ctx context.Context) (OutboxStats, error)
}
