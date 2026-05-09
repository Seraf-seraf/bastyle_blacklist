package videolike

import (
	"context"
	"database/sql"
	"math"
	"path/filepath"
	"testing"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/domain"
)

func TestSQLiteStorePersistsVideoLikeHashes(t *testing.T) {
	ctx := context.Background()
	store := newTestSQLiteStore(t, ctx)

	id, err := store.insert(ctx, StoredVideoLikeHash{
		FileUniqueID: "file-unique-id",
		SourceType:   domain.MediaAnimation,
		DurationSec:  3,
		HashVersion:  videoLikeHashVersion,
		Frames: []StoredVideoLikeFrameHash{
			{FrameIndex: 0, PositionMillis: 0, Hash: 0},
			{FrameIndex: 1, PositionMillis: 1000, Hash: 1},
			{FrameIndex: 2, PositionMillis: 2000, Hash: math.MaxUint64 - 1},
			{FrameIndex: 3, PositionMillis: 3000, Hash: math.MaxUint64},
		},
	})
	if err != nil {
		t.Fatalf("вставка video-like hash: %v", err)
	}

	hashes, err := store.load(ctx)
	if err != nil {
		t.Fatalf("загрузка video-like hash: %v", err)
	}

	if len(hashes) != 1 {
		t.Fatalf("ожидалось: 1 сохраненный video-like hash, получено %d", len(hashes))
	}
	if hashes[0].ID != id {
		t.Fatalf("ожидалось: id %d, получено %d", id, hashes[0].ID)
	}
	if hashes[0].FileUniqueID != "file-unique-id" {
		t.Fatalf("ожидалось: file_unique_id должен сохраниться без изменений, получено %q", hashes[0].FileUniqueID)
	}
	if hashes[0].SourceType != domain.MediaAnimation {
		t.Fatalf("ожидалось: тип источника %q, получено %q", domain.MediaAnimation, hashes[0].SourceType)
	}
	if hashes[0].DurationSec != 3 {
		t.Fatalf("ожидалось: длительность 3, получено %d", hashes[0].DurationSec)
	}
	if hashes[0].HashVersion != videoLikeHashVersion {
		t.Fatalf("ожидалось: версия хеша %q, получено %q", videoLikeHashVersion, hashes[0].HashVersion)
	}

	expectedFrames := []StoredVideoLikeFrameHash{
		{FrameIndex: 0, PositionMillis: 0, Hash: 0},
		{FrameIndex: 1, PositionMillis: 1000, Hash: 1},
		{FrameIndex: 2, PositionMillis: 2000, Hash: math.MaxUint64 - 1},
		{FrameIndex: 3, PositionMillis: 3000, Hash: math.MaxUint64},
	}
	for i, expected := range expectedFrames {
		if hashes[0].Frames[i] != expected {
			t.Fatalf("ожидалось: кадр[%d] %+v, получено %+v", i, expected, hashes[0].Frames[i])
		}
	}
}

func TestSQLiteStoreDeduplicatesVideoLikeHashes(t *testing.T) {
	ctx := context.Background()
	store := newTestSQLiteStore(t, ctx)

	hash := StoredVideoLikeHash{
		FileUniqueID: "file-unique-id",
		SourceType:   domain.MediaAnimation,
		DurationSec:  3,
		HashVersion:  videoLikeHashVersion,
		Frames: []StoredVideoLikeFrameHash{
			{FrameIndex: 0, PositionMillis: 0, Hash: 10},
			{FrameIndex: 1, PositionMillis: 1000, Hash: 20},
			{FrameIndex: 2, PositionMillis: 2000, Hash: 30},
		},
	}

	firstID, err := store.insert(ctx, hash)
	if err != nil {
		t.Fatalf("вставка video-like hash: %v", err)
	}

	secondID, err := store.insert(ctx, hash)
	if err != nil {
		t.Fatalf("вставка дубликата video-like hash: %v", err)
	}
	if secondID != firstID {
		t.Fatalf("ожидалось: вставка дубликата должна вернуть id %d, получено %d", firstID, secondID)
	}

	hashes, err := store.load(ctx)
	if err != nil {
		t.Fatalf("загрузка video-like hash: %v", err)
	}
	if len(hashes) != 1 {
		t.Fatalf("ожидалось: вставка дубликата должна оставить 1 сохраненный video-like hash, получено %d", len(hashes))
	}
	if len(hashes[0].Frames) != 3 {
		t.Fatalf("ожидалось: вставка дубликата должна сохранить 3 хеша кадров, получено %d", len(hashes[0].Frames))
	}
}

func TestSQLiteStoreKeepsSameVideoLikeHashInDifferentChats(t *testing.T) {
	ctx := context.Background()
	store := newTestSQLiteStore(t, ctx)

	hash := StoredVideoLikeHash{
		FileUniqueID: "file-unique-id",
		SourceType:   domain.MediaAnimation,
		DurationSec:  3,
		HashVersion:  videoLikeHashVersion,
		Frames: []StoredVideoLikeFrameHash{
			{FrameIndex: 0, PositionMillis: 0, Hash: 10},
			{FrameIndex: 1, PositionMillis: 1000, Hash: 20},
			{FrameIndex: 2, PositionMillis: 2000, Hash: 30},
		},
	}
	first := hash
	first.ChatID = 10
	second := hash
	second.ChatID = 20

	firstID, err := store.insert(ctx, first)
	if err != nil {
		t.Fatalf("вставка video-like hash первого чата: %v", err)
	}
	secondID, err := store.insert(ctx, second)
	if err != nil {
		t.Fatalf("вставка video-like hash второго чата: %v", err)
	}
	if secondID == firstID {
		t.Fatalf("ожидались разные строки для разных чатов, получен id %d", secondID)
	}

	hashes, err := store.load(ctx)
	if err != nil {
		t.Fatalf("загрузка video-like hash: %v", err)
	}
	if len(hashes) != 2 {
		t.Fatalf("ожидалось: 2 сохраненных video-like hash, получено %d", len(hashes))
	}
	if hashes[0].ChatID != 10 || hashes[1].ChatID != 20 {
		t.Fatalf("chat_id сохраненных хешей = [%d %d], ожидалось [10 20]", hashes[0].ChatID, hashes[1].ChatID)
	}
}

func TestSQLiteStoreResetsLegacySchemaThroughMigration(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "videolike.sqlite")
	createLegacyVideoLikeSchema(t, ctx, path)

	store, err := OpenSQLiteStore(ctx, path)
	if err != nil {
		t.Fatalf("открытие SQLite-хранилища со старой схемой: %v", err)
	}
	defer func() {
		if err := store.close(); err != nil {
			t.Fatalf("закрытие SQLite-хранилища: %v", err)
		}
	}()

	id, err := store.insert(ctx, StoredVideoLikeHash{
		ChatID:       10,
		FileUniqueID: "file-unique-id",
		SourceType:   domain.MediaAnimation,
		DurationSec:  3,
		HashVersion:  videoLikeHashVersion,
		Frames: []StoredVideoLikeFrameHash{
			{FrameIndex: 0, PositionMillis: 0, Hash: 10},
			{FrameIndex: 1, PositionMillis: 1000, Hash: 20},
		},
	})
	if err != nil {
		t.Fatalf("вставка video-like hash после миграции схемы: %v", err)
	}
	if id == 0 {
		t.Fatal("ожидалось: id вставленного video-like hash больше 0")
	}

	var version int
	if err := store.db.QueryRowContext(ctx, `
SELECT version
FROM schema_migrations
WHERE component = 'videolike'
`).Scan(&version); err != nil {
		t.Fatalf("проверка версии миграции: %v", err)
	}
	if version != 1 {
		t.Fatalf("версия миграции = %d, ожидалось 1", version)
	}
}

func TestSQLiteStoreLoadsMultipleVideoLikeHashes(t *testing.T) {
	ctx := context.Background()
	store := newTestSQLiteStore(t, ctx)

	hashes := []StoredVideoLikeHash{
		{
			FileUniqueID: "first-unique-id",
			SourceType:   domain.MediaAnimation,
			DurationSec:  3,
			HashVersion:  videoLikeHashVersion,
			Frames: []StoredVideoLikeFrameHash{
				{FrameIndex: 0, PositionMillis: 0, Hash: 10},
				{FrameIndex: 1, PositionMillis: 1000, Hash: 20},
			},
		},
		{
			FileUniqueID: "second-unique-id",
			SourceType:   domain.MediaStickerStatic,
			DurationSec:  2,
			HashVersion:  videoLikeHashVersion,
			Frames: []StoredVideoLikeFrameHash{
				{FrameIndex: 0, PositionMillis: 0, Hash: 30},
				{FrameIndex: 1, PositionMillis: 1000, Hash: 40},
			},
		},
	}

	for _, hash := range hashes {
		if _, err := store.insert(ctx, hash); err != nil {
			t.Fatalf("вставка video-like hash: %v", err)
		}
	}

	storedHashes, err := store.load(ctx)
	if err != nil {
		t.Fatalf("загрузка video-like hash: %v", err)
	}
	if len(storedHashes) != 2 {
		t.Fatalf("ожидалось: 2 сохраненный video-like hashes, получено %d", len(storedHashes))
	}
	for i, expected := range hashes {
		if storedHashes[i].FileUniqueID != expected.FileUniqueID {
			t.Fatalf("ожидалось: hash[%d] file_unique_id %q, получено %q", i, expected.FileUniqueID, storedHashes[i].FileUniqueID)
		}
		if len(storedHashes[i].Frames) != len(expected.Frames) {
			t.Fatalf("ожидалось: hash[%d] должен содержать %d кадров, получено %d", i, len(expected.Frames), len(storedHashes[i].Frames))
		}
	}
}

func TestSQLiteStoreAllowsSameSignatureForDifferentHashVersions(t *testing.T) {
	ctx := context.Background()
	store := newTestSQLiteStore(t, ctx)

	hash := StoredVideoLikeHash{
		FileUniqueID: "file-unique-id",
		SourceType:   domain.MediaAnimation,
		DurationSec:  3,
		HashVersion:  videoLikeHashVersion,
		Frames: []StoredVideoLikeFrameHash{
			{FrameIndex: 0, PositionMillis: 0, Hash: 10},
			{FrameIndex: 1, PositionMillis: 1000, Hash: 20},
		},
	}

	firstID, err := store.insert(ctx, hash)
	if err != nil {
		t.Fatalf("вставка video-like hash: %v", err)
	}

	hash.HashVersion = "other-version"
	secondID, err := store.insert(ctx, hash)
	if err != nil {
		t.Fatalf("вставка video-like hash другой версии: %v", err)
	}
	if secondID == firstID {
		t.Fatalf("ожидалось: другая версия хеша должна создать новую строку, получено id %d", secondID)
	}

	hashes, err := store.load(ctx)
	if err != nil {
		t.Fatalf("загрузка video-like hash: %v", err)
	}
	if len(hashes) != 1 {
		t.Fatalf("ожидалось: загрузка должна вернуть только хеши текущей версии, получено %d", len(hashes))
	}
}

func newTestSQLiteStore(t *testing.T, ctx context.Context) *sqliteStore {
	t.Helper()

	store, err := OpenSQLiteStore(ctx, filepath.Join(t.TempDir(), "videolike.sqlite"))
	if err != nil {
		t.Fatalf("открытие SQLite-хранилища: %v", err)
	}
	t.Cleanup(func() {
		if err := store.close(); err != nil {
			t.Fatalf("закрытие SQLite-хранилища: %v", err)
		}
	})

	return store
}

func createLegacyVideoLikeSchema(t *testing.T, ctx context.Context, path string) {
	t.Helper()

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("открытие SQLite для старой схемы: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Fatalf("закрытие SQLite старой схемы: %v", err)
		}
	}()

	_, err = db.ExecContext(ctx, `
CREATE TABLE blocked_video_like (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	file_unique_id TEXT NOT NULL,
	source_type TEXT NOT NULL,
	duration_sec INTEGER NOT NULL,
	hash_version TEXT NOT NULL,
	hash_signature TEXT NOT NULL,
	created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE blocked_video_like_frame_hash (
	video_like_id INTEGER NOT NULL,
	frame_index INTEGER NOT NULL,
	position_millis INTEGER NOT NULL,
	hash_uint64 TEXT NOT NULL,
	PRIMARY KEY (video_like_id, frame_index),
	FOREIGN KEY (video_like_id) REFERENCES blocked_video_like(id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX blocked_video_like_hash_signature_idx
ON blocked_video_like(hash_version, hash_signature)
WHERE hash_signature <> '';
`)
	if err != nil {
		t.Fatalf("создание старой схемы videolike: %v", err)
	}
}
