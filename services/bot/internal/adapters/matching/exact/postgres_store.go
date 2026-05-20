package exact

import (
	"context"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/pkg/apperrors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type postgresStore struct {
	pool *pgxpool.Pool
}

func NewPostgresStore(pool *pgxpool.Pool) *postgresStore {
	return &postgresStore{pool: pool}
}

func (s *postgresStore) Insert(ctx context.Context, tx pgx.Tx, banUID uuid.UUID, chatID int64, fileUniqueID string) (bool, error) {
	const methodCtx = "exact/postgresStore.Insert"

	tag, err := tx.Exec(ctx, `
INSERT INTO blocked_exact (ban_uid, chat_id, file_unique_id)
VALUES ($1, $2, $3)
ON CONFLICT (chat_id, file_unique_id) DO NOTHING
`, banUID, chatID, fileUniqueID)
	if err != nil {
		return false, apperrors.Wrap(methodCtx, err)
	}

	return tag.RowsAffected() > 0, nil
}

func (s *postgresStore) Deactivate(ctx context.Context, tx pgx.Tx, banUID uuid.UUID) error {
	const methodCtx = "exact/postgresStore.Deactivate"

	_, err := tx.Exec(ctx, `
UPDATE media_ban
SET active = FALSE, updated_at = now(), deactivated_at = COALESCE(deactivated_at, now())
WHERE ban_uid = $1 AND active = TRUE
`, banUID)
	return apperrors.Wrap(methodCtx, err)
}

func (s *postgresStore) close() error {
	return nil
}

func (s *postgresStore) load(ctx context.Context) ([]exactRecord, error) {
	const methodCtx = "exact/postgresStore.load"

	rows, err := s.pool.Query(ctx, `
SELECT be.chat_id, be.file_unique_id
FROM blocked_exact be
JOIN media_ban mb ON mb.ban_uid = be.ban_uid
WHERE mb.active = TRUE
ORDER BY be.chat_id, be.file_unique_id
`)
	if err != nil {
		return nil, apperrors.Wrap(methodCtx, err)
	}
	defer rows.Close()

	records := make([]exactRecord, 0)
	for rows.Next() {
		var record exactRecord
		if err := rows.Scan(&record.ChatID, &record.FileUniqueID); err != nil {
			return nil, apperrors.Wrap(methodCtx, err)
		}
		records = append(records, record)
	}

	return records, apperrors.Wrap(methodCtx, rows.Err())
}
