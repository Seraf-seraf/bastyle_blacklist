package imagehash

import (
	"context"
	"database/sql"
	"strconv"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/domain"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/pkg/apperrors"
	_ "modernc.org/sqlite"
)

type sqliteStore struct {
	db *sql.DB
}

func OpenSQLiteStore(ctx context.Context, path string) (*sqliteStore, error) {
	const methodCtx = "imagehash/OpenSQLiteStore"

	if path == "" {
		return nil, apperrors.New(methodCtx, "путь к SQLite пустой")
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, apperrors.Wrap(methodCtx, err)
	}
	db.SetMaxOpenConns(1)

	store := &sqliteStore{
		db: db,
	}

	if err := store.ensureSchema(ctx); err != nil {
		_ = db.Close()
		return nil, apperrors.Wrap(methodCtx, err)
	}

	return store, nil
}

func (s *sqliteStore) close() error {
	const methodCtx = "imagehash/sqliteStore.close"

	return apperrors.Wrap(methodCtx, s.db.Close())
}

func (s *sqliteStore) load(ctx context.Context) ([]StoredImageHash, error) {
	const methodCtx = "imagehash/sqliteStore.load"

	rows, err := s.db.QueryContext(ctx, `
SELECT bi.id, bi.chat_id, bi.file_unique_id, bi.media_type, bih.hash_uint64
FROM blocked_image bi
JOIN blocked_image_hash bih ON bih.image_id = bi.id
WHERE bi.hash_version = ?
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

		hash, err := strconv.ParseUint(hashText, 10, 64)
		if err != nil {
			return nil, apperrors.Wrap(methodCtx, err)
		}

		storedHash, ok := byID[id]
		if !ok {
			storedHash = &StoredImageHash{
				ID:           id,
				ChatID:       chatID,
				FileUniqueID: fileUniqueID,
				MediaType:    domain.MediaType(mediaType),
				Hashes:       make([]uint64, 0, 4),
			}
			byID[id] = storedHash
			order = append(order, id)
		}

		storedHash.Hashes = append(storedHash.Hashes, hash)
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

func (s *sqliteStore) insert(ctx context.Context, hash StoredImageHash) (int64, error) {
	const methodCtx = "imagehash/sqliteStore.insert"

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, apperrors.Wrap(methodCtx, err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	signature := hashSignature(hash.Hashes)
	_, err = tx.ExecContext(ctx, `
INSERT OR IGNORE INTO blocked_image (chat_id, file_unique_id, media_type, hash_version, hash_signature)
VALUES (?, ?, ?, ?, ?)
`, hash.ChatID, hash.FileUniqueID, string(hash.MediaType), hashVersion, signature)
	if err != nil {
		return 0, apperrors.Wrap(methodCtx, err)
	}

	var id int64
	err = tx.QueryRowContext(ctx, `
SELECT id
FROM blocked_image
WHERE hash_version = ? AND hash_signature = ?
  AND chat_id = ?
`, hashVersion, signature, hash.ChatID).Scan(&id)
	if err != nil {
		return 0, apperrors.Wrap(methodCtx, err)
	}

	for variant, value := range hash.Hashes {
		_, err = tx.ExecContext(ctx, `
INSERT OR IGNORE INTO blocked_image_hash (image_id, variant, hash_uint64)
VALUES (?, ?, ?)
`, id, variant, strconv.FormatUint(value, 10))
		if err != nil {
			return 0, apperrors.Wrap(methodCtx, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, apperrors.Wrap(methodCtx, err)
	}

	return id, nil
}

func (s *sqliteStore) ensureSchema(ctx context.Context) error {
	const methodCtx = "imagehash/sqliteStore.ensureSchema"

	_, err := s.db.ExecContext(ctx, `
PRAGMA journal_mode = WAL;
PRAGMA foreign_keys = ON;
PRAGMA busy_timeout = 5000;

CREATE TABLE IF NOT EXISTS blocked_image (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	chat_id INTEGER NOT NULL DEFAULT 0,
	file_unique_id TEXT NOT NULL,
	media_type TEXT NOT NULL,
	hash_version TEXT NOT NULL,
	hash_signature TEXT NOT NULL,
	created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS blocked_image_hash (
	image_id INTEGER NOT NULL,
	variant INTEGER NOT NULL,
	hash_uint64 TEXT NOT NULL,
	PRIMARY KEY (image_id, variant),
	FOREIGN KEY (image_id) REFERENCES blocked_image(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS blocked_image_hash_version_idx
ON blocked_image(hash_version);
`)
	if err != nil {
		return apperrors.Wrap(methodCtx, err)
	}

	_, err = s.db.ExecContext(ctx, `
	CREATE UNIQUE INDEX IF NOT EXISTS blocked_image_chat_hash_signature_idx
	ON blocked_image(chat_id, hash_version, hash_signature)
	WHERE hash_signature <> '';
	`)
	return apperrors.Wrap(methodCtx, err)
}
