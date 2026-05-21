package postgres

import (
	"context"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/app/ports"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/pkg/apperrors"
	"github.com/jackc/pgx/v5/pgxpool"
)

type store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) (*store, error) {
	const methodCtx = "indexcheckpoint/postgres/NewStore"

	if pool == nil {
		return nil, apperrors.New(methodCtx, "PostgreSQL pool не настроен")
	}

	return &store{pool: pool}, nil
}

func (s *store) GetOrCreate(ctx context.Context, consumerID string, indexName string) (ports.IndexCheckpoint, error) {
	const methodCtx = "indexcheckpoint/postgres/Store.GetOrCreate"

	if err := validateCheckpointKey(consumerID, indexName); err != nil {
		return ports.IndexCheckpoint{}, apperrors.Wrap(methodCtx, err)
	}
	if s.pool == nil {
		return ports.IndexCheckpoint{}, apperrors.New(methodCtx, "PostgreSQL pool не настроен")
	}

	if _, err := s.pool.Exec(ctx, `
INSERT INTO index_checkpoints (consumer_id, index_name)
VALUES ($1, $2)
ON CONFLICT (consumer_id, index_name) DO NOTHING
`, consumerID, indexName); err != nil {
		return ports.IndexCheckpoint{}, apperrors.Wrap(methodCtx, err)
	}

	var checkpoint ports.IndexCheckpoint
	if err := s.pool.QueryRow(ctx, `
SELECT consumer_id, index_name, last_applied_transaction_id::text, last_applied_offset, updated_at, stale, COALESCE(stale_reason, '')
FROM index_checkpoints
WHERE consumer_id = $1 AND index_name = $2
`, consumerID, indexName).Scan(
		&checkpoint.ConsumerID,
		&checkpoint.IndexName,
		&checkpoint.LastAppliedTransactionID,
		&checkpoint.LastAppliedOffset,
		&checkpoint.UpdatedAt,
		&checkpoint.Stale,
		&checkpoint.StaleReason,
	); err != nil {
		return ports.IndexCheckpoint{}, apperrors.Wrap(methodCtx, err)
	}

	return checkpoint, nil
}

func (s *store) Update(ctx context.Context, consumerID string, indexName string, transactionID string, offset int64) error {
	const methodCtx = "indexcheckpoint/postgres/Store.Update"

	if err := validateCheckpointKey(consumerID, indexName); err != nil {
		return apperrors.Wrap(methodCtx, err)
	}
	if transactionID == "" {
		return apperrors.New(methodCtx, "transaction_id события обязателен")
	}
	if offset < 0 {
		return apperrors.New(methodCtx, "offset события не должен быть отрицательным")
	}
	if s.pool == nil {
		return apperrors.New(methodCtx, "PostgreSQL pool не настроен")
	}

	_, err := s.pool.Exec(ctx, updateCheckpointSQL(), consumerID, indexName, transactionID, offset)
	return apperrors.Wrap(methodCtx, err)
}

func (s *store) MarkStale(ctx context.Context, consumerID string, indexName string, reason string) error {
	const methodCtx = "indexcheckpoint/postgres/Store.MarkStale"

	if err := validateCheckpointKey(consumerID, indexName); err != nil {
		return apperrors.Wrap(methodCtx, err)
	}
	if reason == "" {
		return apperrors.New(methodCtx, "причина stale обязательна")
	}
	if s.pool == nil {
		return apperrors.New(methodCtx, "PostgreSQL pool не настроен")
	}

	_, err := s.pool.Exec(ctx, `
UPDATE index_checkpoints
SET stale = TRUE,
    stale_reason = $3,
    updated_at = now()
WHERE consumer_id = $1
  AND index_name = $2
`, consumerID, indexName, reason)
	return apperrors.Wrap(methodCtx, err)
}

func (s *store) CheckFresh(ctx context.Context, consumerID string, indexNames []string) error {
	const methodCtx = "indexcheckpoint/postgres/Store.CheckFresh"

	if consumerID == "" {
		return apperrors.New(methodCtx, "consumer_id обязателен")
	}
	if len(indexNames) == 0 {
		return nil
	}
	if s.pool == nil {
		return apperrors.New(methodCtx, "PostgreSQL pool не настроен")
	}

	var staleCount int
	if err := s.pool.QueryRow(ctx, checkFreshSQL(len(indexNames)), consumerID, indexNames).Scan(&staleCount); err != nil {
		return apperrors.Wrap(methodCtx, err)
	}
	if staleCount > 0 {
		return apperrors.New(methodCtx, "найдены stale index checkpoints")
	}
	return nil
}

func updateCheckpointSQL() string {
	return `
UPDATE index_checkpoints
SET last_applied_transaction_id = $3::xid8,
    last_applied_offset = $4,
    updated_at = now(),
    stale = FALSE,
    stale_reason = NULL
WHERE consumer_id = $1
  AND index_name = $2
  AND (
        last_applied_transaction_id < $3::xid8
        OR (last_applied_transaction_id = $3::xid8 AND last_applied_offset <= $4)
      )
`
}

func checkFreshSQL(_ int) string {
	return `
SELECT count(*)
FROM index_checkpoints
WHERE consumer_id = $1
  AND index_name = ANY($2)
  AND stale = TRUE
`
}

func validateCheckpointKey(consumerID string, indexName string) error {
	const methodCtx = "indexcheckpoint/postgres/validateCheckpointKey"

	if consumerID == "" {
		return apperrors.New(methodCtx, "consumer_id обязателен")
	}
	if indexName == "" {
		return apperrors.New(methodCtx, "index_name обязателен")
	}
	return nil
}

var _ ports.IndexCheckpointStore = (*store)(nil)
