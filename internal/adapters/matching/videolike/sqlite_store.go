package videolike

import (
	"context"
	"database/sql"
	"strconv"
	"strings"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/domain"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/pkg/apperrors"
	_ "modernc.org/sqlite"
)

type sqliteStore struct {
	db *sql.DB
}

func OpenSQLiteStore(ctx context.Context, path string) (*sqliteStore, error) {
	const methodCtx = "videolike/OpenSQLiteStore"

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
	const methodCtx = "videolike/sqliteStore.close"

	return apperrors.Wrap(methodCtx, s.db.Close())
}

func (s *sqliteStore) load(ctx context.Context) ([]StoredVideoLikeHash, error) {
	const methodCtx = "videolike/sqliteStore.load"

	rows, err := s.db.QueryContext(ctx, `
SELECT bvl.id, bvl.file_unique_id, bvl.source_type, bvl.duration_sec, bvl.hash_version,
       bvlfh.frame_index, bvlfh.position_millis, bvlfh.hash_uint64
FROM blocked_video_like bvl
JOIN blocked_video_like_frame_hash bvlfh ON bvlfh.video_like_id = bvl.id
WHERE bvl.hash_version = ?
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
		var fileUniqueID string
		var sourceType string
		var durationSec int
		var hashVersion string
		var frameIndex int
		var positionMillis int
		var hashText string

		if err := rows.Scan(&id, &fileUniqueID, &sourceType, &durationSec, &hashVersion, &frameIndex, &positionMillis, &hashText); err != nil {
			return nil, apperrors.Wrap(methodCtx, err)
		}

		hash, err := strconv.ParseUint(hashText, 10, 64)
		if err != nil {
			return nil, apperrors.Wrap(methodCtx, err)
		}

		storedHash, ok := byID[id]
		if !ok {
			storedHash = &StoredVideoLikeHash{
				ID:           id,
				FileUniqueID: fileUniqueID,
				SourceType:   domain.MediaType(sourceType),
				DurationSec:  durationSec,
				HashVersion:  hashVersion,
				Frames:       make([]StoredVideoLikeFrameHash, 0),
			}
			byID[id] = storedHash
			order = append(order, id)
		}

		storedHash.Frames = append(storedHash.Frames, StoredVideoLikeFrameHash{
			FrameIndex:     frameIndex,
			PositionMillis: positionMillis,
			Hash:           hash,
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

func (s *sqliteStore) insert(ctx context.Context, hash StoredVideoLikeHash) (int64, error) {
	const methodCtx = "videolike/sqliteStore.insert"

	if len(hash.Frames) == 0 {
		return 0, apperrors.New(methodCtx, "SQLite-хранилище video-like: кадры хеша пустые")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, apperrors.Wrap(methodCtx, err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	hashVersion := hash.HashVersion
	if hashVersion == "" {
		hashVersion = videoLikeHashVersion
	}

	signature := videoLikeHashSignature(hash.Frames)
	_, err = tx.ExecContext(ctx, `
INSERT OR IGNORE INTO blocked_video_like (file_unique_id, source_type, duration_sec, hash_version, hash_signature)
VALUES (?, ?, ?, ?, ?)
`, hash.FileUniqueID, string(hash.SourceType), hash.DurationSec, hashVersion, signature)
	if err != nil {
		return 0, apperrors.Wrap(methodCtx, err)
	}

	var id int64
	err = tx.QueryRowContext(ctx, `
SELECT id
FROM blocked_video_like
WHERE hash_version = ? AND hash_signature = ?
`, hashVersion, signature).Scan(&id)
	if err != nil {
		return 0, apperrors.Wrap(methodCtx, err)
	}

	for _, frame := range hash.Frames {
		_, err = tx.ExecContext(ctx, `
INSERT OR IGNORE INTO blocked_video_like_frame_hash (video_like_id, frame_index, position_millis, hash_uint64)
VALUES (?, ?, ?, ?)
`, id, frame.FrameIndex, frame.PositionMillis, strconv.FormatUint(frame.Hash, 10))
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
	const methodCtx = "videolike/sqliteStore.ensureSchema"

	_, err := s.db.ExecContext(ctx, `
PRAGMA journal_mode = WAL;
PRAGMA foreign_keys = ON;
PRAGMA busy_timeout = 5000;

CREATE TABLE IF NOT EXISTS blocked_video_like (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	file_unique_id TEXT NOT NULL,
	source_type TEXT NOT NULL,
	duration_sec INTEGER NOT NULL,
	hash_version TEXT NOT NULL,
	hash_signature TEXT NOT NULL,
	created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS blocked_video_like_frame_hash (
	video_like_id INTEGER NOT NULL,
	frame_index INTEGER NOT NULL,
	position_millis INTEGER NOT NULL,
	hash_uint64 TEXT NOT NULL,
	PRIMARY KEY (video_like_id, frame_index),
	FOREIGN KEY (video_like_id) REFERENCES blocked_video_like(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS blocked_video_like_hash_version_idx
ON blocked_video_like(hash_version);

CREATE UNIQUE INDEX IF NOT EXISTS blocked_video_like_hash_signature_idx
ON blocked_video_like(hash_version, hash_signature)
WHERE hash_signature <> '';
`)
	return apperrors.Wrap(methodCtx, err)
}

func videoLikeHashSignature(frames []StoredVideoLikeFrameHash) string {
	parts := make([]string, 0, len(frames))
	for _, frame := range frames {
		parts = append(parts, strings.Join([]string{
			strconv.Itoa(frame.FrameIndex),
			strconv.Itoa(frame.PositionMillis),
			strconv.FormatUint(frame.Hash, 10),
		}, ":"))
	}

	return strings.Join(parts, "|")
}
