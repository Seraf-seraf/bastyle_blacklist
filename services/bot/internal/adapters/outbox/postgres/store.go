package postgres

import (
	"context"
	"time"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/app/ports"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/pkg/apperrors"
	watermillsql "github.com/ThreeDotsLabs/watermill-sql/v4/pkg/sql"
	"github.com/ThreeDotsLabs/watermill/components/forwarder"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	ForwarderTopic         = "watermill_outbox_events"
	ForwarderConsumerGroup = "outbox-publisher"
	MessagesTableName      = "watermill_outbox_events"
	OffsetsTableName       = "watermill_offsets_outbox_events"
)

type store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) (*store, error) {
	const methodCtx = "outbox/postgres/NewStore"

	if pool == nil {
		return nil, apperrors.New(methodCtx, "PostgreSQL pool не настроен")
	}

	return &store{pool: pool}, nil
}

func NewWatermillSchema(subscribeBatchSize int) watermillsql.DefaultPostgreSQLSchema {
	return watermillsql.DefaultPostgreSQLSchema{
		GenerateMessagesTableName: func(string) string {
			return quotedIdentifier(MessagesTableName)
		},
		GeneratePayloadType: func(string) string {
			return "BYTEA"
		},
		SubscribeBatchSize: subscribeBatchSize,
	}
}

func NewWatermillOffsetsAdapter() watermillsql.DefaultPostgreSQLOffsetsAdapter {
	return watermillsql.DefaultPostgreSQLOffsetsAdapter{
		GenerateMessagesOffsetsTableName: func(string) string {
			return quotedIdentifier(OffsetsTableName)
		},
	}
}

func (s *store) Save(ctx context.Context, tx pgx.Tx, event ports.NewOutboxEvent) error {
	const methodCtx = "outbox/postgres/store.Save"

	if tx == nil {
		return apperrors.New(methodCtx, "PostgreSQL transaction не настроена")
	}
	if event.EventType == "" {
		return apperrors.New(methodCtx, "тип outbox-события обязателен")
	}
	if event.AggregateType == "" {
		return apperrors.New(methodCtx, "тип агрегата outbox-события обязателен")
	}
	if event.AggregateUID == uuid.Nil {
		return apperrors.New(methodCtx, "UID агрегата outbox-события обязателен")
	}
	if len(event.Payload) == 0 {
		return apperrors.New(methodCtx, "payload outbox-события обязателен")
	}
	if event.EventUID == uuid.Nil {
		event.EventUID = uuid.New()
	}

	sqlPublisher, err := watermillsql.NewPublisher(
		watermillsql.TxFromPgx(tx),
		watermillsql.PublisherConfig{
			SchemaAdapter: NewWatermillSchema(0),
		},
		nil,
	)
	if err != nil {
		return apperrors.Wrap(methodCtx, err)
	}
	defer func() {
		_ = sqlPublisher.Close()
	}()

	publisher := forwarder.NewPublisher(sqlPublisher, forwarder.PublisherConfig{
		ForwarderTopic: ForwarderTopic,
	})

	msg := message.NewMessage(event.EventUID.String(), []byte(event.Payload))
	msg.SetContext(ctx)
	msg.Metadata = message.Metadata{
		"event_uid":      event.EventUID.String(),
		"event_type":     event.EventType,
		"aggregate_type": event.AggregateType,
		"aggregate_uid":  event.AggregateUID.String(),
	}

	return apperrors.Wrap(methodCtx, publisher.Publish(event.EventType, msg))
}

func (s *store) Stats(ctx context.Context) (ports.OutboxStats, error) {
	const methodCtx = "outbox/postgres/store.Stats"

	var stats ports.OutboxStats
	var maxAgeSeconds *float64
	if err := s.pool.QueryRow(ctx, `
WITH last_processed AS (
    SELECT
        COALESCE(
            (SELECT offset_acked
             FROM watermill_offsets_outbox_events
             WHERE consumer_group = $1),
            0
        ) AS offset_acked,
        COALESCE(
            (SELECT last_processed_transaction_id
             FROM watermill_offsets_outbox_events
             WHERE consumer_group = $1),
            '0'::xid8
        ) AS last_processed_transaction_id
)
SELECT count(*),
       extract(epoch FROM max(now() - created_at))
FROM watermill_outbox_events, last_processed
WHERE (
        transaction_id = last_processed_transaction_id
        AND "offset" > offset_acked
      )
   OR transaction_id > last_processed_transaction_id
`, ForwarderConsumerGroup).Scan(
		&stats.UnpublishedCount,
		&maxAgeSeconds,
	); err != nil {
		return ports.OutboxStats{}, apperrors.Wrap(methodCtx, err)
	}
	if maxAgeSeconds != nil {
		stats.UnpublishedMaxAge = time.Duration(*maxAgeSeconds * float64(time.Second))
	}

	return stats, nil
}

func quotedIdentifier(identifier string) string {
	return `"` + identifier + `"`
}

var _ ports.OutboxWriter = (*store)(nil)
var _ ports.OutboxStore = (*store)(nil)
