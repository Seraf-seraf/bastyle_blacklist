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

func (s *postgresStore) Insert(ctx context.Context, tx pgx.Tx, banUID uuid.UUID, hash StoredVideoLikeHash) (videoLikeInsertResult, error) {
	const methodCtx = "videolike/postgresStore.Insert"

	if len(hash.Frames) == 0 {
		return videoLikeInsertResult{}, apperrors.New(methodCtx, "PostgreSQL-хранилище video-like: кадры хеша пустые")
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
		return videoLikeInsertResult{}, apperrors.Wrap(methodCtx, err)
	}
	created := storedBanUID == banUID

	frameIndexes := make([]int32, 0, len(hash.Frames))
	positionMillis := make([]int32, 0, len(hash.Frames))
	hashes := make([]string, 0, len(hash.Frames))
	for _, frame := range hash.Frames {
		frameIndexes = append(frameIndexes, int32(frame.FrameIndex))
		positionMillis = append(positionMillis, int32(frame.PositionMillis))
		hashes = append(hashes, strconv.FormatUint(frame.Hash, 10))
	}

	_, err = tx.Exec(ctx, `
INSERT INTO blocked_video_like_frame_hash (video_like_id, frame_index, position_millis, hash_uint64)
SELECT $1, frame_index, position_millis, hash_uint64
FROM unnest($2::int[], $3::int[], $4::text[]) AS h(frame_index, position_millis, hash_uint64)
ON CONFLICT (video_like_id, frame_index) DO NOTHING
`, id, frameIndexes, positionMillis, hashes)
	if err != nil {
		return videoLikeInsertResult{}, apperrors.Wrap(methodCtx, err)
	}

	return videoLikeInsertResult{ID: id, Created: created}, nil
}

func (s *postgresStore) LoadByBanUID(ctx context.Context, banUID uuid.UUID) (StoredVideoLikeHash, error) {
	const methodCtx = "videolike/postgresStore.LoadByBanUID"

	rows, err := s.pool.Query(ctx, `
SELECT bvl.id, bvl.chat_id, bvl.file_unique_id, bvl.source_type, bvl.duration_sec, bvl.hash_version,
       bvlfh.frame_index, bvlfh.position_millis, bvlfh.hash_uint64
FROM blocked_video_like bvl
JOIN media_ban mb ON mb.ban_uid = bvl.ban_uid
JOIN blocked_video_like_frame_hash bvlfh ON bvlfh.video_like_id = bvl.id
WHERE bvl.ban_uid = $1 AND mb.active = TRUE AND bvl.hash_version = $2
ORDER BY bvlfh.frame_index
`, banUID, videoLikeHashVersion)
	if err != nil {
		return StoredVideoLikeHash{}, apperrors.Wrap(methodCtx, err)
	}
	defer rows.Close()

	var record StoredVideoLikeHash
	found := false
	for rows.Next() {
		var sourceType string
		var hashText string
		var frame StoredVideoLikeFrameHash
		if err := rows.Scan(
			&record.ID,
			&record.ChatID,
			&record.FileUniqueID,
			&sourceType,
			&record.DurationSec,
			&record.HashVersion,
			&frame.FrameIndex,
			&frame.PositionMillis,
			&hashText,
		); err != nil {
			return StoredVideoLikeHash{}, apperrors.Wrap(methodCtx, err)
		}
		record.SourceType = domain.MediaType(sourceType)
		value, err := strconv.ParseUint(hashText, 10, 64)
		if err != nil {
			return StoredVideoLikeHash{}, apperrors.Wrap(methodCtx, err)
		}
		frame.Hash = value
		record.Frames = append(record.Frames, frame)
		found = true
	}
	if err := rows.Err(); err != nil {
		return StoredVideoLikeHash{}, apperrors.Wrap(methodCtx, err)
	}
	if !found {
		return StoredVideoLikeHash{}, errVideoLikeArtifactNotFound
	}
	return record, nil
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
