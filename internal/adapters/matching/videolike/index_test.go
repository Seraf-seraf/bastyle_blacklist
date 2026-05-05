package videolike

import (
	"strconv"
	"testing"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/domain"
)

func TestLinearIndexSearchReturnsMatchedFrameCountAndRatio(t *testing.T) {
	index := NewLinearIndex(1, DefaultMatchRule())
	index.Add(StoredVideoLikeHash{
		ID:          42,
		SourceType:  domain.MediaAnimation,
		HashVersion: videoLikeHashVersion,
		Frames: []StoredVideoLikeFrameHash{
			{FrameIndex: 0, PositionMillis: 0, Hash: 0x1111},
			{FrameIndex: 1, PositionMillis: 1000, Hash: 0x2222},
			{FrameIndex: 2, PositionMillis: 2000, Hash: 0xbbbb},
			{FrameIndex: 3, PositionMillis: 3000, Hash: 0xcccc},
		},
	})

	result, matched := index.Search(StoredVideoLikeHash{
		HashVersion: videoLikeHashVersion,
		Frames: []StoredVideoLikeFrameHash{
			{FrameIndex: 0, PositionMillis: 0, Hash: 0x1111},
			{FrameIndex: 1, PositionMillis: 1000, Hash: 0x2222},
			{FrameIndex: 2, PositionMillis: 2000, Hash: 0x3333},
			{FrameIndex: 3, PositionMillis: 3000, Hash: 0x4444},
		},
	}, 0)
	if !matched {
		t.Fatal("expected matching video-like fingerprint")
	}
	if result.Stored.ID != 42 {
		t.Fatalf("matched stored id = %d, want 42", result.Stored.ID)
	}
	if result.MatchedFrames != 2 {
		t.Fatalf("matched frames = %d, want 2", result.MatchedFrames)
	}
	if result.MatchedRatio != 0.5 {
		t.Fatalf("matched ratio = %f, want 0.5", result.MatchedRatio)
	}
}

func TestLinearIndexSearchRejectsOneMatchingFrame(t *testing.T) {
	index := NewLinearIndex(1, DefaultMatchRule())
	index.Add(StoredVideoLikeHash{
		HashVersion: videoLikeHashVersion,
		Frames: []StoredVideoLikeFrameHash{
			{FrameIndex: 0, PositionMillis: 0, Hash: 0x1111},
			{FrameIndex: 1, PositionMillis: 1000, Hash: 0xaaaa},
			{FrameIndex: 2, PositionMillis: 2000, Hash: 0xbbbb},
			{FrameIndex: 3, PositionMillis: 3000, Hash: 0xcccc},
		},
	})

	_, matched := index.Search(StoredVideoLikeHash{
		HashVersion: videoLikeHashVersion,
		Frames: []StoredVideoLikeFrameHash{
			{FrameIndex: 0, PositionMillis: 0, Hash: 0x1111},
			{FrameIndex: 1, PositionMillis: 1000, Hash: 0x2222},
			{FrameIndex: 2, PositionMillis: 2000, Hash: 0x3333},
			{FrameIndex: 3, PositionMillis: 3000, Hash: 0x4444},
		},
	}, 0)
	if matched {
		t.Fatal("expected one matched frame to be rejected")
	}
}

func TestLinearIndexSearchRejectsBelowRatio(t *testing.T) {
	index := NewLinearIndex(1, MatchRule{
		MinMatchedFrames: 2,
		MinMatchedRatio:  0.75,
	})
	index.Add(StoredVideoLikeHash{
		HashVersion: videoLikeHashVersion,
		Frames: []StoredVideoLikeFrameHash{
			{FrameIndex: 0, PositionMillis: 0, Hash: 0x1111},
			{FrameIndex: 1, PositionMillis: 1000, Hash: 0x2222},
			{FrameIndex: 2, PositionMillis: 2000, Hash: 0xbbbb},
			{FrameIndex: 3, PositionMillis: 3000, Hash: 0xcccc},
		},
	})

	_, matched := index.Search(StoredVideoLikeHash{
		HashVersion: videoLikeHashVersion,
		Frames: []StoredVideoLikeFrameHash{
			{FrameIndex: 0, PositionMillis: 0, Hash: 0x1111},
			{FrameIndex: 1, PositionMillis: 1000, Hash: 0x2222},
			{FrameIndex: 2, PositionMillis: 2000, Hash: 0x3333},
			{FrameIndex: 3, PositionMillis: 3000, Hash: 0x4444},
		},
	}, 0)
	if matched {
		t.Fatal("expected match below ratio to be rejected")
	}
}

func TestLinearIndexSearchRejectsHashVersionMismatch(t *testing.T) {
	index := NewLinearIndex(1, DefaultMatchRule())
	index.Add(StoredVideoLikeHash{
		HashVersion: "other-version",
		Frames: []StoredVideoLikeFrameHash{
			{FrameIndex: 0, PositionMillis: 0, Hash: 0x1111},
			{FrameIndex: 1, PositionMillis: 1000, Hash: 0x2222},
		},
	})

	_, matched := index.Search(StoredVideoLikeHash{
		HashVersion: videoLikeHashVersion,
		Frames: []StoredVideoLikeFrameHash{
			{FrameIndex: 0, PositionMillis: 0, Hash: 0x1111},
			{FrameIndex: 1, PositionMillis: 1000, Hash: 0x2222},
		},
	}, 0)
	if matched {
		t.Fatal("expected hash version mismatch to be rejected")
	}
}

func TestLinearIndexAddManyDeduplicatesHashes(t *testing.T) {
	index := NewLinearIndex(2, DefaultMatchRule())
	hash := StoredVideoLikeHash{
		ID:          1,
		HashVersion: videoLikeHashVersion,
		Frames: []StoredVideoLikeFrameHash{
			{FrameIndex: 0, PositionMillis: 0, Hash: 0x1111},
			{FrameIndex: 1, PositionMillis: 1000, Hash: 0x2222},
		},
	}

	index.AddMany([]StoredVideoLikeHash{hash, hash})

	result, matched := index.Search(hash, 0)
	if !matched {
		t.Fatal("expected deduplicated hash to be searchable")
	}
	if result.Stored.ID != 1 {
		t.Fatalf("matched stored id = %d, want 1", result.Stored.ID)
	}
}

func TestLinearIndexKeepsSameFrameSignatureForDifferentHashVersions(t *testing.T) {
	index := NewLinearIndex(2, DefaultMatchRule())
	hash := StoredVideoLikeHash{
		ID:          1,
		HashVersion: videoLikeHashVersion,
		Frames: []StoredVideoLikeFrameHash{
			{FrameIndex: 0, PositionMillis: 0, Hash: 0x1111},
			{FrameIndex: 1, PositionMillis: 1000, Hash: 0x2222},
		},
	}
	otherVersion := hash
	otherVersion.ID = 2
	otherVersion.HashVersion = "other-version"

	index.AddMany([]StoredVideoLikeHash{hash, otherVersion})

	result, matched := index.Search(otherVersion, 0)
	if !matched {
		t.Fatal("expected other version hash to be searchable")
	}
	if result.Stored.ID != 2 {
		t.Fatalf("matched stored id = %d, want 2", result.Stored.ID)
	}
}

func BenchmarkLinearIndexSearch(b *testing.B) {
	for _, size := range []int{10_000, 50_000, 100_000} {
		b.Run(strconv.Itoa(size)+"_miss", func(b *testing.B) {
			index := benchmarkVideoLikeIndex(size)
			query := benchmarkVideoLikeHash("query", 0xffff_ffff_ffff_ffff)

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				if _, matched := index.Search(query, 8); matched {
					b.Fatal("expected no match")
				}
			}
		})

		b.Run(strconv.Itoa(size)+"_match_last", func(b *testing.B) {
			index := benchmarkVideoLikeIndex(size)
			query := benchmarkVideoLikeHash("matching-item", 0x1111_2222_3333_4444)
			index.Add(query)

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				if _, matched := index.Search(query, 8); !matched {
					b.Fatal("expected match")
				}
			}
		})
	}
}

func benchmarkVideoLikeIndex(size int) *LinearIndex {
	index := NewLinearIndex(size, DefaultMatchRule())
	hashes := make([]StoredVideoLikeHash, 0, size)

	for i := 0; i < size; i++ {
		hashes = append(hashes, benchmarkVideoLikeHash(strconv.Itoa(i), splitmix64(uint64(i+1))))
	}

	index.AddMany(hashes)
	return index
}

func benchmarkVideoLikeHash(fileUniqueID string, seed uint64) StoredVideoLikeHash {
	frames := make([]StoredVideoLikeFrameHash, 0, 10)
	for i := 0; i < 10; i++ {
		frames = append(frames, StoredVideoLikeFrameHash{
			FrameIndex:     i,
			PositionMillis: i * 100,
			Hash:           splitmix64(seed + uint64(i)),
		})
	}

	return StoredVideoLikeHash{
		FileUniqueID: fileUniqueID,
		SourceType:   domain.MediaAnimation,
		DurationSec:  3,
		HashVersion:  videoLikeHashVersion,
		Frames:       frames,
	}
}

func splitmix64(x uint64) uint64 {
	x += 0x9e3779b97f4a7c15
	x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9
	x = (x ^ (x >> 27)) * 0x94d049bb133111eb
	return x ^ (x >> 31)
}
