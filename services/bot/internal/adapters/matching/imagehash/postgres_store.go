package imagehash

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

func (s *postgresStore) Insert(ctx context.Context, tx pgx.Tx, banUID uuid.UUID, hash StoredImageHash) (imageHashInsertResult, error) {
	const methodCtx = "imagehash/postgresStore.Insert"

	signature := hashSignature(hash.Hashes)
	var id int64
	var storedBanUID uuid.UUID
	err := tx.QueryRow(ctx, `
WITH inserted AS (
    INSERT INTO blocked_image (ban_uid, chat_id, file_unique_id, media_type, hash_version, hash_signature)
    VALUES ($1, $2, $3, $4, $5, $6)
    ON CONFLICT (chat_id, hash_version, hash_signature) DO NOTHING
    RETURNING id, ban_uid
)
SELECT id, ban_uid FROM inserted
UNION ALL
SELECT id, ban_uid FROM blocked_image
WHERE chat_id = $2 AND hash_version = $5 AND hash_signature = $6
LIMIT 1
`, banUID, hash.ChatID, hash.FileUniqueID, string(hash.MediaType), hashVersion, signature).Scan(&id, &storedBanUID)
	if err != nil {
		return imageHashInsertResult{}, apperrors.Wrap(methodCtx, err)
	}
	created := storedBanUID == banUID

	variants := make([]int32, 0, len(hash.Hashes))
	values := make([]string, 0, len(hash.Hashes))
	for variant, value := range hash.Hashes {
		variants = append(variants, int32(variant))
		values = append(values, strconv.FormatUint(value, 10))
	}

	_, err = tx.Exec(ctx, `
INSERT INTO blocked_image_hash (image_id, variant, hash_uint64)
SELECT $1, variant, hash_uint64
FROM unnest($2::int[], $3::text[]) AS h(variant, hash_uint64)
ON CONFLICT (image_id, variant) DO NOTHING
`, id, variants, values)
	if err != nil {
		return imageHashInsertResult{}, apperrors.Wrap(methodCtx, err)
	}

	return imageHashInsertResult{ID: id, Created: created}, nil
}

func (s *postgresStore) LoadByBanUID(ctx context.Context, banUID uuid.UUID) (StoredImageHash, error) {
	const methodCtx = "imagehash/postgresStore.LoadByBanUID"

	rows, err := s.pool.Query(ctx, `
SELECT bi.id, bi.chat_id, bi.file_unique_id, bi.media_type, bih.hash_uint64
FROM blocked_image bi
JOIN media_ban mb ON mb.ban_uid = bi.ban_uid
JOIN blocked_image_hash bih ON bih.image_id = bi.id
WHERE bi.ban_uid = $1 AND mb.active = TRUE AND bi.hash_version = $2
ORDER BY bih.variant
`, banUID, hashVersion)
	if err != nil {
		return StoredImageHash{}, apperrors.Wrap(methodCtx, err)
	}
	defer rows.Close()

	var record StoredImageHash
	found := false
	for rows.Next() {
		var mediaType string
		var hashText string
		if err := rows.Scan(&record.ID, &record.ChatID, &record.FileUniqueID, &mediaType, &hashText); err != nil {
			return StoredImageHash{}, apperrors.Wrap(methodCtx, err)
		}
		record.MediaType = domain.MediaType(mediaType)
		value, err := strconv.ParseUint(hashText, 10, 64)
		if err != nil {
			return StoredImageHash{}, apperrors.Wrap(methodCtx, err)
		}
		record.Hashes = append(record.Hashes, value)
		found = true
	}
	if err := rows.Err(); err != nil {
		return StoredImageHash{}, apperrors.Wrap(methodCtx, err)
	}
	if !found {
		return StoredImageHash{}, errImageHashArtifactNotFound
	}
	return record, nil
}

func (s *postgresStore) Deactivate(ctx context.Context, tx pgx.Tx, banUID uuid.UUID) error {
	const methodCtx = "imagehash/postgresStore.Deactivate"

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

func (s *postgresStore) load(ctx context.Context) ([]StoredImageHash, error) {
	const methodCtx = "imagehash/postgresStore.load"

	rows, err := s.pool.Query(ctx, `
SELECT bi.id, bi.chat_id, bi.file_unique_id, bi.media_type, bih.hash_uint64
FROM blocked_image bi
JOIN media_ban mb ON mb.ban_uid = bi.ban_uid
JOIN blocked_image_hash bih ON bih.image_id = bi.id
WHERE mb.active = TRUE AND bi.hash_version = $1
ORDER BY bi.id, bih.variant
`, hashVersion)
	if err != nil {
		return nil, apperrors.Wrap(methodCtx, err)
	}
	defer rows.Close()

	byID := make(map[int64]*StoredImageHash)
	order := make([]int64, 0)
	for rows.Next() {
		var id int64
		var chatID int64
		var fileUniqueID string
		var mediaType string
		var hashText string
		if err := rows.Scan(&id, &chatID, &fileUniqueID, &mediaType, &hashText); err != nil {
			return nil, apperrors.Wrap(methodCtx, err)
		}

		value, err := strconv.ParseUint(hashText, 10, 64)
		if err != nil {
			return nil, apperrors.Wrap(methodCtx, err)
		}

		stored, ok := byID[id]
		if !ok {
			stored = &StoredImageHash{
				ID:           id,
				ChatID:       chatID,
				FileUniqueID: fileUniqueID,
				MediaType:    domain.MediaType(mediaType),
				Hashes:       make([]uint64, 0, 4),
			}
			byID[id] = stored
			order = append(order, id)
		}
		stored.Hashes = append(stored.Hashes, value)
	}
	if err := rows.Err(); err != nil {
		return nil, apperrors.Wrap(methodCtx, err)
	}

	hashes := make([]StoredImageHash, 0, len(order))
	for _, id := range order {
		hashes = append(hashes, *byID[id])
	}
	return hashes, nil
}
