package imagehash

import (
	"context"
	"database/sql"
	"math"
	"path/filepath"
	"testing"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/domain"
)

func TestSQLiteStorePersistsImageHashes(t *testing.T) {
	ctx := context.Background()
	store, err := OpenSQLiteStore(ctx, filepath.Join(t.TempDir(), "imagehash.sqlite"))
	if err != nil {
		t.Fatalf("открытие SQLite-хранилища: %v", err)
	}
	defer func() {
		if err := store.close(); err != nil {
			t.Fatalf("закрытие SQLite-хранилища: %v", err)
		}
	}()

	id, err := store.insert(ctx, StoredImageHash{
		FileUniqueID: "file-unique-id",
		MediaType:    domain.MediaPhoto,
		Hashes: []uint64{
			0,
			1,
			math.MaxUint64 - 1,
			math.MaxUint64,
		},
	})
	if err != nil {
		t.Fatalf("вставка image-hash: %v", err)
	}

	hashes, err := store.load(ctx)
	if err != nil {
		t.Fatalf("загрузка image-hash: %v", err)
	}

	if len(hashes) != 1 {
		t.Fatalf("ожидалось: 1 сохраненный image-hash, получено %d", len(hashes))
	}

	if hashes[0].ID != id {
		t.Fatalf("ожидалось: id %d, получено %d", id, hashes[0].ID)
	}

	if hashes[0].FileUniqueID != "file-unique-id" {
		t.Fatalf("ожидалось: file_unique_id должен сохраниться без изменений, получено %q", hashes[0].FileUniqueID)
	}

	if hashes[0].MediaType != domain.MediaPhoto {
		t.Fatalf("ожидалось: тип медиа %q, получено %q", domain.MediaPhoto, hashes[0].MediaType)
	}

	expectedHashes := []uint64{0, 1, math.MaxUint64 - 1, math.MaxUint64}
	for i, expected := range expectedHashes {
		if hashes[0].Hashes[i] != expected {
			t.Fatalf("ожидалось: hash[%d] %d, получено %d", i, expected, hashes[0].Hashes[i])
		}
	}
}

func TestSQLiteStoreDeduplicatesImageHashes(t *testing.T) {
	ctx := context.Background()
	store, err := OpenSQLiteStore(ctx, filepath.Join(t.TempDir(), "imagehash.sqlite"))
	if err != nil {
		t.Fatalf("открытие SQLite-хранилища: %v", err)
	}
	defer func() {
		if err := store.close(); err != nil {
			t.Fatalf("закрытие SQLite-хранилища: %v", err)
		}
	}()

	hash := StoredImageHash{
		FileUniqueID: "file-unique-id",
		MediaType:    domain.MediaPhoto,
		Hashes:       []uint64{10, 20, 30, 40},
	}

	firstID, err := store.insert(ctx, hash)
	if err != nil {
		t.Fatalf("вставка image-hash: %v", err)
	}

	secondID, err := store.insert(ctx, hash)
	if err != nil {
		t.Fatalf("вставка дубликата image-hash: %v", err)
	}

	if secondID != firstID {
		t.Fatalf("ожидалось: вставка дубликата должна вернуть id %d, получено %d", firstID, secondID)
	}

	hashes, err := store.load(ctx)
	if err != nil {
		t.Fatalf("загрузка image-hash: %v", err)
	}

	if len(hashes) != 1 {
		t.Fatalf("ожидалось: вставка дубликата должна оставить 1 сохраненный image-hash, получено %d", len(hashes))
	}
}

func TestSQLiteStoreKeepsSameImageHashInDifferentChats(t *testing.T) {
	ctx := context.Background()
	store, err := OpenSQLiteStore(ctx, filepath.Join(t.TempDir(), "imagehash.sqlite"))
	if err != nil {
		t.Fatalf("открытие SQLite-хранилища: %v", err)
	}
	defer func() {
		if err := store.close(); err != nil {
			t.Fatalf("закрытие SQLite-хранилища: %v", err)
		}
	}()

	hash := StoredImageHash{
		FileUniqueID: "file-unique-id",
		MediaType:    domain.MediaPhoto,
		Hashes:       []uint64{10, 20, 30, 40},
	}
	first := hash
	first.ChatID = 10
	second := hash
	second.ChatID = 20

	firstID, err := store.insert(ctx, first)
	if err != nil {
		t.Fatalf("вставка image-hash первого чата: %v", err)
	}
	secondID, err := store.insert(ctx, second)
	if err != nil {
		t.Fatalf("вставка image-hash второго чата: %v", err)
	}
	if secondID == firstID {
		t.Fatalf("ожидались разные строки для разных чатов, получен id %d", secondID)
	}

	hashes, err := store.load(ctx)
	if err != nil {
		t.Fatalf("загрузка image-hash: %v", err)
	}
	if len(hashes) != 2 {
		t.Fatalf("ожидалось: 2 сохраненных image-hash, получено %d", len(hashes))
	}
	if hashes[0].ChatID != 10 || hashes[1].ChatID != 20 {
		t.Fatalf("chat_id сохраненных хешей = [%d %d], ожидалось [10 20]", hashes[0].ChatID, hashes[1].ChatID)
	}
}

func TestSQLiteStoreResetsLegacySchemaThroughMigration(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "imagehash.sqlite")
	createLegacyImageHashSchema(t, ctx, path)

	store, err := OpenSQLiteStore(ctx, path)
	if err != nil {
		t.Fatalf("открытие SQLite-хранилища со старой схемой: %v", err)
	}
	defer func() {
		if err := store.close(); err != nil {
			t.Fatalf("закрытие SQLite-хранилища: %v", err)
		}
	}()

	id, err := store.insert(ctx, StoredImageHash{
		ChatID:       10,
		FileUniqueID: "file-unique-id",
		MediaType:    domain.MediaPhoto,
		Hashes:       []uint64{10, 20, 30, 40},
	})
	if err != nil {
		t.Fatalf("вставка image-hash после миграции схемы: %v", err)
	}
	if id == 0 {
		t.Fatal("ожидалось: id вставленного image-hash больше 0")
	}

	var version int
	if err := store.db.QueryRowContext(ctx, `
SELECT version_id
FROM imagehash_schema_migrations
WHERE is_applied = 1
ORDER BY id DESC
LIMIT 1
`).Scan(&version); err != nil {
		t.Fatalf("проверка версии миграции: %v", err)
	}
	if version != 1 {
		t.Fatalf("версия миграции = %d, ожидалось 1", version)
	}
}

func createLegacyImageHashSchema(t *testing.T, ctx context.Context, path string) {
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
CREATE TABLE blocked_image (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	file_unique_id TEXT NOT NULL,
	media_type TEXT NOT NULL,
	hash_version TEXT NOT NULL,
	hash_signature TEXT NOT NULL,
	created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE blocked_image_hash (
	image_id INTEGER NOT NULL,
	variant INTEGER NOT NULL,
	hash_uint64 TEXT NOT NULL,
	PRIMARY KEY (image_id, variant),
	FOREIGN KEY (image_id) REFERENCES blocked_image(id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX blocked_image_hash_signature_idx
ON blocked_image(hash_version, hash_signature)
WHERE hash_signature <> '';
`)
	if err != nil {
		t.Fatalf("создание старой схемы imagehash: %v", err)
	}
}
