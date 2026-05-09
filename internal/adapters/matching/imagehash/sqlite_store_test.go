package imagehash

import (
	"context"
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
