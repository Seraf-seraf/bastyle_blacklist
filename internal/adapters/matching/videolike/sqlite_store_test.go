package videolike

import (
	"context"
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
		t.Fatalf("insert video-like hash: %v", err)
	}

	hashes, err := store.load(ctx)
	if err != nil {
		t.Fatalf("load video-like hashes: %v", err)
	}

	if len(hashes) != 1 {
		t.Fatalf("expected 1 stored video-like hash, got %d", len(hashes))
	}
	if hashes[0].ID != id {
		t.Fatalf("expected id %d, got %d", id, hashes[0].ID)
	}
	if hashes[0].FileUniqueID != "file-unique-id" {
		t.Fatalf("expected file unique id to round-trip, got %q", hashes[0].FileUniqueID)
	}
	if hashes[0].SourceType != domain.MediaAnimation {
		t.Fatalf("expected source type %q, got %q", domain.MediaAnimation, hashes[0].SourceType)
	}
	if hashes[0].DurationSec != 3 {
		t.Fatalf("expected duration 3, got %d", hashes[0].DurationSec)
	}
	if hashes[0].HashVersion != videoLikeHashVersion {
		t.Fatalf("expected hash version %q, got %q", videoLikeHashVersion, hashes[0].HashVersion)
	}

	expectedFrames := []StoredVideoLikeFrameHash{
		{FrameIndex: 0, PositionMillis: 0, Hash: 0},
		{FrameIndex: 1, PositionMillis: 1000, Hash: 1},
		{FrameIndex: 2, PositionMillis: 2000, Hash: math.MaxUint64 - 1},
		{FrameIndex: 3, PositionMillis: 3000, Hash: math.MaxUint64},
	}
	for i, expected := range expectedFrames {
		if hashes[0].Frames[i] != expected {
			t.Fatalf("expected frame[%d] %+v, got %+v", i, expected, hashes[0].Frames[i])
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
		t.Fatalf("insert video-like hash: %v", err)
	}

	secondID, err := store.insert(ctx, hash)
	if err != nil {
		t.Fatalf("insert duplicate video-like hash: %v", err)
	}
	if secondID != firstID {
		t.Fatalf("expected duplicate insert to return id %d, got %d", firstID, secondID)
	}

	hashes, err := store.load(ctx)
	if err != nil {
		t.Fatalf("load video-like hashes: %v", err)
	}
	if len(hashes) != 1 {
		t.Fatalf("expected duplicate insert to keep 1 stored video-like hash, got %d", len(hashes))
	}
	if len(hashes[0].Frames) != 3 {
		t.Fatalf("expected duplicate insert to keep 3 frame hashes, got %d", len(hashes[0].Frames))
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
			t.Fatalf("insert video-like hash: %v", err)
		}
	}

	storedHashes, err := store.load(ctx)
	if err != nil {
		t.Fatalf("load video-like hashes: %v", err)
	}
	if len(storedHashes) != 2 {
		t.Fatalf("expected 2 stored video-like hashes, got %d", len(storedHashes))
	}
	for i, expected := range hashes {
		if storedHashes[i].FileUniqueID != expected.FileUniqueID {
			t.Fatalf("expected hash[%d] file unique id %q, got %q", i, expected.FileUniqueID, storedHashes[i].FileUniqueID)
		}
		if len(storedHashes[i].Frames) != len(expected.Frames) {
			t.Fatalf("expected hash[%d] to have %d frames, got %d", i, len(expected.Frames), len(storedHashes[i].Frames))
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
		t.Fatalf("insert video-like hash: %v", err)
	}

	hash.HashVersion = "other-version"
	secondID, err := store.insert(ctx, hash)
	if err != nil {
		t.Fatalf("insert other version video-like hash: %v", err)
	}
	if secondID == firstID {
		t.Fatalf("expected different hash version to create a new row, got id %d", secondID)
	}

	hashes, err := store.load(ctx)
	if err != nil {
		t.Fatalf("load video-like hashes: %v", err)
	}
	if len(hashes) != 1 {
		t.Fatalf("expected load to return only current version hashes, got %d", len(hashes))
	}
}

func newTestSQLiteStore(t *testing.T, ctx context.Context) *sqliteStore {
	t.Helper()

	store, err := OpenSQLiteStore(ctx, filepath.Join(t.TempDir(), "videolike.sqlite"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	t.Cleanup(func() {
		if err := store.close(); err != nil {
			t.Fatalf("close sqlite store: %v", err)
		}
	})

	return store
}
