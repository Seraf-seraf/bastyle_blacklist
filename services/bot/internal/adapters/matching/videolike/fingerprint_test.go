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
		t.Fatalf("создание video-like fingerprint: %v", err)
	}

	if fingerprint.FileUniqueID != "animation-unique" {
		t.Fatalf("file_unique_id = %q, ожидалось animation-unique", fingerprint.FileUniqueID)
	}
	if fingerprint.SourceType != domain.MediaAnimation {
		t.Fatalf("тип источника = %q, ожидалось %q", fingerprint.SourceType, domain.MediaAnimation)
	}
	if fingerprint.DurationSec != 3 {
		t.Fatalf("длительность = %d, ожидалось 3", fingerprint.DurationSec)
	}
	if fingerprint.HashVersion != videoLikeHashVersion {
		t.Fatalf("версия хеша = %q, ожидалось %q", fingerprint.HashVersion, videoLikeHashVersion)
	}
	if len(fingerprint.Frames) != 3 {
		t.Fatalf("кадры = %d, ожидалось 3", len(fingerprint.Frames))
	}
	for i, frame := range fingerprint.Frames {
		if frame.FrameIndex != i {
			t.Fatalf("индекс кадра = %d, ожидалось %d", frame.FrameIndex, i)
		}
		if frame.PositionMillis != i*1000 {
			t.Fatalf("позиция в миллисекундах = %d, ожидалось %d", frame.PositionMillis, i*1000)
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
		t.Fatal("ожидалось: один совпавший кадр должен быть отклонен")
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
		t.Fatal("ожидалось: два совпавших кадра должны быть приняты")
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
		t.Fatal("ожидалась ошибка при несовпадении версии хеша")
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
