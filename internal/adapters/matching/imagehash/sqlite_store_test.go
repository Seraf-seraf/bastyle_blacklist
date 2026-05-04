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
		t.Fatalf("open sqlite store: %v", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			t.Fatalf("close sqlite store: %v", err)
		}
	}()

	id, err := store.Insert(ctx, StoredImageHash{
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
		t.Fatalf("insert image hash: %v", err)
	}

	hashes, err := store.Load(ctx)
	if err != nil {
		t.Fatalf("load image hashes: %v", err)
	}

	if len(hashes) != 1 {
		t.Fatalf("expected 1 stored image hash, got %d", len(hashes))
	}

	if hashes[0].ID != id {
		t.Fatalf("expected id %d, got %d", id, hashes[0].ID)
	}

	if hashes[0].FileUniqueID != "file-unique-id" {
		t.Fatalf("expected file unique id to round-trip, got %q", hashes[0].FileUniqueID)
	}

	if hashes[0].MediaType != domain.MediaPhoto {
		t.Fatalf("expected media type %q, got %q", domain.MediaPhoto, hashes[0].MediaType)
	}

	expectedHashes := []uint64{0, 1, math.MaxUint64 - 1, math.MaxUint64}
	for i, expected := range expectedHashes {
		if hashes[0].Hashes[i] != expected {
			t.Fatalf("expected hash[%d] %d, got %d", i, expected, hashes[0].Hashes[i])
		}
	}
}

func TestSQLiteStoreDeduplicatesImageHashes(t *testing.T) {
	ctx := context.Background()
	store, err := OpenSQLiteStore(ctx, filepath.Join(t.TempDir(), "imagehash.sqlite"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			t.Fatalf("close sqlite store: %v", err)
		}
	}()

	hash := StoredImageHash{
		FileUniqueID: "file-unique-id",
		MediaType:    domain.MediaPhoto,
		Hashes:       []uint64{10, 20, 30, 40},
	}

	firstID, err := store.Insert(ctx, hash)
	if err != nil {
		t.Fatalf("insert image hash: %v", err)
	}

	secondID, err := store.Insert(ctx, hash)
	if err != nil {
		t.Fatalf("insert duplicate image hash: %v", err)
	}

	if secondID != firstID {
		t.Fatalf("expected duplicate insert to return id %d, got %d", firstID, secondID)
	}

	hashes, err := store.Load(ctx)
	if err != nil {
		t.Fatalf("load image hashes: %v", err)
	}

	if len(hashes) != 1 {
		t.Fatalf("expected duplicate insert to keep 1 stored image hash, got %d", len(hashes))
	}
}
