package videolike

import (
	"context"
	"strconv"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/domain"
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

func (s *postgresStore) LoadActive(ctx context.Context) ([]StoredVideoLikeHash, error) {
	return s.load(ctx)
}

func (s *postgresStore) Insert(ctx context.Context, tx pgx.Tx, banUID uuid.UUID, hash StoredVideoLikeHash) (int64, error) {
	const methodCtx = "videolike/postgresStore.Insert"

	if len(hash.Frames) == 0 {
		return 0, apperrors.New(methodCtx, "PostgreSQL-хранилище video-like: кадры хеша пустые")
	}

	hashVersion := hash.HashVersion
	if hashVersion == "" {
		hashVersion = videoLikeHashVersion
	}
	signature := videoLikeHashSignature(hash.Frames)

	var id int64
	var storedBanUID uuid.UUID
	err := tx.QueryRow(ctx, `
WITH inserted AS (
    INSERT INTO blocked_video_like (
        ban_uid, chat_id, file_unique_id, source_type, duration_sec, hash_version, hash_signature
    )
    VALUES ($1, $2, $3, $4, $5, $6, $7)
    ON CONFLICT (chat_id, hash_version, hash_signature) DO NOTHING
    RETURNING id, ban_uid
)
SELECT id, ban_uid FROM inserted
UNION ALL
SELECT id, ban_uid FROM blocked_video_like
WHERE chat_id = $2 AND hash_version = $6 AND hash_signature = $7
LIMIT 1
`, banUID, hash.ChatID, hash.FileUniqueID, string(hash.SourceType), hash.DurationSec, hashVersion, signature).Scan(&id, &storedBanUID)
	if err != nil {
		return 0, apperrors.Wrap(methodCtx, err)
	}
	if storedBanUID != banUID {
		if err := cleanupEmptyMediaBan(ctx, tx, banUID); err != nil {
			return 0, apperrors.Wrap(methodCtx, err)
		}
	}

	for _, frame := range hash.Frames {
		_, err = tx.Exec(ctx, `
INSERT INTO blocked_video_like_frame_hash (video_like_id, frame_index, position_millis, hash_uint64)
VALUES ($1, $2, $3, $4)
ON CONFLICT (video_like_id, frame_index) DO NOTHING
`, id, frame.FrameIndex, frame.PositionMillis, strconv.FormatUint(frame.Hash, 10))
		if err != nil {
			return 0, apperrors.Wrap(methodCtx, err)
		}
	}

	return id, nil
}

func (s *postgresStore) Deactivate(ctx context.Context, tx pgx.Tx, banUID uuid.UUID) error {
	const methodCtx = "videolike/postgresStore.Deactivate"

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

func (s *postgresStore) load(ctx context.Context) ([]StoredVideoLikeHash, error) {
	const methodCtx = "videolike/postgresStore.load"

	rows, err := s.pool.Query(ctx, `
SELECT bvl.id, bvl.chat_id, bvl.file_unique_id, bvl.source_type, bvl.duration_sec, bvl.hash_version,
       bvlfh.frame_index, bvlfh.position_millis, bvlfh.hash_uint64
FROM blocked_video_like bvl
JOIN media_ban mb ON mb.ban_uid = bvl.ban_uid
JOIN blocked_video_like_frame_hash bvlfh ON bvlfh.video_like_id = bvl.id
WHERE mb.active = TRUE AND bvl.hash_version = $1
ORDER BY bvl.id, bvlfh.frame_index
`, videoLikeHashVersion)
	if err != nil {
		return nil, apperrors.Wrap(methodCtx, err)
	}
	defer rows.Close()

	byID := make(map[int64]*StoredVideoLikeHash)
	order := make([]int64, 0)
	for rows.Next() {
		var id int64
		var chatID int64
		var fileUniqueID string
		var sourceType string
		var durationSec int
		var hashVersion string
		var frameIndex int
		var positionMillis int
		var hashText string
		if err := rows.Scan(&id, &chatID, &fileUniqueID, &sourceType, &durationSec, &hashVersion, &frameIndex, &positionMillis, &hashText); err != nil {
			return nil, apperrors.Wrap(methodCtx, err)
		}

		value, err := strconv.ParseUint(hashText, 10, 64)
		if err != nil {
			return nil, apperrors.Wrap(methodCtx, err)
		}

		stored, ok := byID[id]
		if !ok {
			stored = &StoredVideoLikeHash{
				ID:           id,
				ChatID:       chatID,
				FileUniqueID: fileUniqueID,
				SourceType:   domain.MediaType(sourceType),
				DurationSec:  durationSec,
				HashVersion:  hashVersion,
				Frames:       make([]StoredVideoLikeFrameHash, 0),
			}
			byID[id] = stored
			order = append(order, id)
		}
		stored.Frames = append(stored.Frames, StoredVideoLikeFrameHash{
			FrameIndex:     frameIndex,
			PositionMillis: positionMillis,
			Hash:           value,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, apperrors.Wrap(methodCtx, err)
	}

	hashes := make([]StoredVideoLikeHash, 0, len(order))
	for _, id := range order {
		hashes = append(hashes, *byID[id])
	}
	return hashes, nil
}

func (s *postgresStore) insert(ctx context.Context, hash StoredVideoLikeHash) (int64, error) {
	const methodCtx = "videolike/postgresStore.insert"

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return 0, apperrors.Wrap(methodCtx, err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	hashVersion := hash.HashVersion
	if hashVersion == "" {
		hashVersion = videoLikeHashVersion
	}
	signature := videoLikeHashSignature(hash.Frames)
	var existingID int64
	if err := tx.QueryRow(ctx, `
SELECT id
FROM blocked_video_like
WHERE chat_id = $1 AND hash_version = $2 AND hash_signature = $3
`, hash.ChatID, hashVersion, signature).Scan(&existingID); err == nil {
		return existingID, apperrors.Wrap(methodCtx, tx.Commit(ctx))
	} else if err != pgx.ErrNoRows {
		return 0, apperrors.Wrap(methodCtx, err)
	}

	banUID := uuid.New()
	if err := insertMediaBan(ctx, tx, banUID, hash.ChatID, hash.SourceType, hash.FileUniqueID); err != nil {
		return 0, apperrors.Wrap(methodCtx, err)
	}
	id, err := s.Insert(ctx, tx, banUID, hash)
	if err != nil {
		return 0, apperrors.Wrap(methodCtx, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, apperrors.Wrap(methodCtx, err)
	}
	return id, nil
}

func insertMediaBan(ctx context.Context, tx pgx.Tx, banUID uuid.UUID, chatID int64, mediaType domain.MediaType, fileUniqueID string) error {
	const methodCtx = "videolike/insertMediaBan"

	_, err := tx.Exec(ctx, `
INSERT INTO media_ban (ban_uid, chat_id, media_type, file_unique_id)
VALUES ($1, $2, $3, $4)
ON CONFLICT (ban_uid) DO NOTHING
`, banUID, chatID, string(mediaType), fileUniqueID)
	return apperrors.Wrap(methodCtx, err)
}

func cleanupEmptyMediaBan(ctx context.Context, tx pgx.Tx, banUID uuid.UUID) error {
	const methodCtx = "videolike/cleanupEmptyMediaBan"

	_, err := tx.Exec(ctx, `
DELETE FROM media_ban mb
WHERE mb.ban_uid = $1
  AND NOT EXISTS (SELECT 1 FROM blocked_exact be WHERE be.ban_uid = mb.ban_uid)
  AND NOT EXISTS (SELECT 1 FROM blocked_image bi WHERE bi.ban_uid = mb.ban_uid)
  AND NOT EXISTS (SELECT 1 FROM blocked_video_like bvl WHERE bvl.ban_uid = mb.ban_uid)
  AND NOT EXISTS (SELECT 1 FROM ai_vector_ban avb WHERE avb.ban_uid = mb.ban_uid)
`, banUID)
	return apperrors.Wrap(methodCtx, err)
}
