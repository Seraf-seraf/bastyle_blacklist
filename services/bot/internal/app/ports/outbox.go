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

type OutboxEvent struct {
	TransactionID string
	Offset        int64
	EventUID      uuid.UUID
	EventType     string
	AggregateType string
	AggregateUID  uuid.UUID
	Payload       json.RawMessage
	CreatedAt     time.Time
}

type IndexCheckpoint struct {
	ConsumerID               string
	IndexName                string
	LastAppliedTransactionID string
	LastAppliedOffset        int64
	Stale                    bool
	StaleReason              string
	UpdatedAt                time.Time
}

const (
	IndexExact     = "exact"
	IndexImageHash = "imagehash"
	IndexVideoLike = "videolike"
	IndexAIVector  = "ai_vector"
)

type OutboxWriter interface {
	Save(ctx context.Context, tx pgx.Tx, event NewOutboxEvent) error
}

type OutboxStore interface {
	Stats(ctx context.Context) (OutboxStats, error)
}

type OutboxEventReader interface {
	LoadAfter(ctx context.Context, transactionID string, offset int64, limit int) ([]OutboxEvent, error)
}

type IndexCheckpointStore interface {
	GetOrCreate(ctx context.Context, consumerID string, indexName string) (IndexCheckpoint, error)
	Update(ctx context.Context, consumerID string, indexName string, transactionID string, offset int64) error
	MarkStale(ctx context.Context, consumerID string, indexName string, reason string) error
	CheckFresh(ctx context.Context, consumerID string, indexNames []string) error
}

type IndexEventApplier interface {
	IndexName() string
	Supports(eventType string) bool
	ApplyEvent(ctx context.Context, event OutboxEvent) error
}
