package postgres

import (
	"context"
	"encoding/json"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/app/ports"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/pkg/apperrors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type eventReader struct {
	pool *pgxpool.Pool
}

func NewEventReader(pool *pgxpool.Pool) (*eventReader, error) {
	const methodCtx = "outbox/postgres/NewEventReader"

	if pool == nil {
		return nil, apperrors.New(methodCtx, "PostgreSQL pool не настроен")
	}

	return &eventReader{pool: pool}, nil
}

func (r *eventReader) LoadAfter(ctx context.Context, transactionID string, offset int64, limit int) ([]ports.OutboxEvent, error) {
	const methodCtx = "outbox/postgres/EventReader.LoadAfter"

	if transactionID == "" {
		transactionID = "0"
	}
	if offset < 0 {
		return nil, apperrors.New(methodCtx, "offset outbox-события не должен быть отрицательным")
	}
	if limit <= 0 {
		return nil, apperrors.New(methodCtx, "limit outbox-событий должен быть положительным")
	}
	if r.pool == nil {
		return nil, apperrors.New(methodCtx, "PostgreSQL pool не настроен")
	}

	rows, err := r.pool.Query(ctx, loadAfterSQL(), transactionID, offset, limit)
	if err != nil {
		return nil, apperrors.Wrap(methodCtx, err)
	}
	defer rows.Close()

	events := make([]ports.OutboxEvent, 0, limit)
	for rows.Next() {
		var transactionID string
		var offset int64
		var eventUIDText string
		var payload []byte
		var metadataRaw []byte
		var createdAt ports.OutboxEvent
		if err := rows.Scan(
			&transactionID,
			&offset,
			&eventUIDText,
			&payload,
			&metadataRaw,
			&createdAt.CreatedAt,
		); err != nil {
			return nil, apperrors.Wrap(methodCtx, err)
		}
		metadata := struct {
			EventUID      string `json:"event_uid"`
			EventType     string `json:"event_type"`
			AggregateType string `json:"aggregate_type"`
			AggregateUID  string `json:"aggregate_uid"`
		}{}
		if len(metadataRaw) > 0 {
			if err := json.Unmarshal(metadataRaw, &metadata); err != nil {
				return nil, apperrors.Wrap(methodCtx, err)
			}
		}
		if metadata.EventUID == "" {
			metadata.EventUID = eventUIDText
		}
		eventUID, err := uuid.Parse(metadata.EventUID)
		if err != nil {
			return nil, apperrors.Wrap(methodCtx, err)
		}
		aggregateUID, err := uuid.Parse(metadata.AggregateUID)
		if err != nil {
			return nil, apperrors.Wrap(methodCtx, err)
		}
		event := ports.OutboxEvent{
			TransactionID: transactionID,
			Offset:        offset,
			EventUID:      eventUID,
			EventType:     metadata.EventType,
			AggregateType: metadata.AggregateType,
			AggregateUID:  aggregateUID,
			Payload:       payload,
			CreatedAt:     createdAt.CreatedAt,
		}
		events = append(events, event)
	}

	return events, apperrors.Wrap(methodCtx, rows.Err())
}

func loadAfterSQL() string {
	return `
SELECT transaction_id::text, "offset", uuid, payload, metadata, created_at
FROM watermill_outbox_events
WHERE (transaction_id = $1::xid8 AND "offset" > $2)
   OR transaction_id > $1::xid8
ORDER BY transaction_id ASC, "offset" ASC
LIMIT $3
`
}

var _ ports.OutboxEventReader = (*eventReader)(nil)
