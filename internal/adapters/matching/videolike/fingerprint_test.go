package videolike

import (
	"image"
	"image/color"
	"testing"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/domain"
)

func TestFingerprintVideoLikeStoresFrameHashesWithMetadata(t *testing.T) {
	fingerprint, err := fingerprintVideoLike(domain.Content{
		FileUniqueID: "animation-unique",
		Type:         domain.MediaAnimation,
		DurationSec:  3,
	}, domain.ExtractedMedia{
		Frames: []domain.ExtractedFrame{
			{Index: 0, PositionMillis: 0, Image: testFrameImage(color.RGBA{R: 255, A: 255})},
			{Index: 1, PositionMillis: 1000, Image: testFrameImage(color.RGBA{G: 255, A: 255})},
			{Index: 2, PositionMillis: 2000, Image: testFrameImage(color.RGBA{B: 255, A: 255})},
		},
	})
	if err != nil {
		t.Fatalf("fingerprint video like: %v", err)
	}

	if fingerprint.FileUniqueID != "animation-unique" {
		t.Fatalf("file unique id = %q, want animation-unique", fingerprint.FileUniqueID)
	}
	if fingerprint.SourceType != domain.MediaAnimation {
		t.Fatalf("source type = %q, want %q", fingerprint.SourceType, domain.MediaAnimation)
	}
	if fingerprint.DurationSec != 3 {
		t.Fatalf("duration = %d, want 3", fingerprint.DurationSec)
	}
	if fingerprint.HashVersion != videoLikeHashVersion {
		t.Fatalf("hash version = %q, want %q", fingerprint.HashVersion, videoLikeHashVersion)
	}
	if len(fingerprint.Frames) != 3 {
		t.Fatalf("frames = %d, want 3", len(fingerprint.Frames))
	}
	for i, frame := range fingerprint.Frames {
		if frame.FrameIndex != i {
			t.Fatalf("frame index = %d, want %d", frame.FrameIndex, i)
		}
		if frame.PositionMillis != i*1000 {
			t.Fatalf("position millis = %d, want %d", frame.PositionMillis, i*1000)
		}
	}
}

func TestMatchVideoLikeFingerprintRequiresMoreThanOneMatchedFrame(t *testing.T) {
	query := StoredVideoLikeHash{
		HashVersion: videoLikeHashVersion,
		Frames: []StoredVideoLikeFrameHash{
			{FrameIndex: 0, PositionMillis: 0, Hash: 0x1111},
			{FrameIndex: 1, PositionMillis: 1000, Hash: 0x2222},
			{FrameIndex: 2, PositionMillis: 2000, Hash: 0x3333},
			{FrameIndex: 3, PositionMillis: 3000, Hash: 0x4444},
		},
	}
	stored := StoredVideoLikeHash{
		HashVersion: videoLikeHashVersion,
		Frames: []StoredVideoLikeFrameHash{
			{FrameIndex: 0, PositionMillis: 0, Hash: 0x1111},
			{FrameIndex: 1, PositionMillis: 1000, Hash: 0xaaaa},
			{FrameIndex: 2, PositionMillis: 2000, Hash: 0xbbbb},
			{FrameIndex: 3, PositionMillis: 3000, Hash: 0xcccc},
		},
	}

	if matchVideoLikeFingerprint(query, stored, 0) {
		t.Fatal("expected one matched frame to be rejected")
	}
}

func TestMatchVideoLikeFingerprintAcceptsTwoFramesWithRequiredRatio(t *testing.T) {
	query := StoredVideoLikeHash{
		HashVersion: videoLikeHashVersion,
		Frames: []StoredVideoLikeFrameHash{
			{FrameIndex: 0, PositionMillis: 0, Hash: 0x1111},
			{FrameIndex: 1, PositionMillis: 1000, Hash: 0x2222},
			{FrameIndex: 2, PositionMillis: 2000, Hash: 0x3333},
			{FrameIndex: 3, PositionMillis: 3000, Hash: 0x4444},
		},
	}
	stored := StoredVideoLikeHash{
		HashVersion: videoLikeHashVersion,
		Frames: []StoredVideoLikeFrameHash{
			{FrameIndex: 0, PositionMillis: 0, Hash: 0x1111},
			{FrameIndex: 1, PositionMillis: 1000, Hash: 0x2222},
			{FrameIndex: 2, PositionMillis: 2000, Hash: 0xbbbb},
			{FrameIndex: 3, PositionMillis: 3000, Hash: 0xcccc},
		},
	}

	if !matchVideoLikeFingerprint(query, stored, 0) {
		t.Fatal("expected two matched frames to be accepted")
	}
}

func TestMatchVideoLikeFingerprintRejectsHashVersionMismatch(t *testing.T) {
	query := StoredVideoLikeHash{
		HashVersion: videoLikeHashVersion,
		Frames: []StoredVideoLikeFrameHash{
			{FrameIndex: 0, PositionMillis: 0, Hash: 0x1111},
			{FrameIndex: 1, PositionMillis: 1000, Hash: 0x2222},
		},
	}
	stored := StoredVideoLikeHash{
		HashVersion: "other-version",
		Frames: []StoredVideoLikeFrameHash{
			{FrameIndex: 0, PositionMillis: 0, Hash: 0x1111},
			{FrameIndex: 1, PositionMillis: 1000, Hash: 0x2222},
		},
	}

	if matchVideoLikeFingerprint(query, stored, 0) {
		t.Fatal("expected hash version mismatch to be rejected")
	}
}

func testFrameImage(fill color.Color) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < img.Bounds().Dy(); y++ {
		for x := 0; x < img.Bounds().Dx(); x++ {
			img.Set(x, y, fill)
		}
	}

	return img
}
