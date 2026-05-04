package imagehash

import (
	"context"
	"image"
	"image/color"
	"path/filepath"
	"testing"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/domain"
	"github.com/disintegration/imaging"
)

type fakeDownloader struct{}

func (fakeDownloader) Download(_ context.Context, content domain.Content) (domain.MediaFile, error) {
	return domain.MediaFile{
		Content: content,
	}, nil
}

type fakeExtractor struct {
	images map[string]image.Image
}

func (e fakeExtractor) Extract(_ context.Context, media domain.MediaFile, _ domain.MediaExtractionPlan) (domain.ExtractedMedia, error) {
	return domain.ExtractedMedia{
		Frames: []domain.ExtractedFrame{
			{Image: e.images[media.Content.FileID]},
		},
	}, nil
}

func TestMatcherBlocksSameWhiteImageBackgroundWithDifferentFileID(t *testing.T) {
	ctx := context.Background()
	blockedContent := domain.Content{
		FileID:       "blocked-white",
		FileUniqueID: "blocked-white-unique",
		Type:         domain.MediaPhoto,
	}
	candidateContent := domain.Content{
		FileID:       "candidate-white",
		FileUniqueID: "candidate-white-unique",
		Type:         domain.MediaPhoto,
	}

	matcher := newTestSQLiteMatcher(t, ctx, fakeDownloader{}, fakeExtractor{
		images: map[string]image.Image{
			blockedContent.FileID:   solidImage(128, 128, color.White),
			candidateContent.FileID: solidImage(320, 240, color.White),
		},
	}, 8, 2)

	if err := matcher.Block(ctx, blockedContent); err != nil {
		t.Fatalf("block white image: %v", err)
	}

	blocked, err := matcher.IsBlocked(ctx, candidateContent)
	if err != nil {
		t.Fatalf("check candidate white image: %v", err)
	}

	if !blocked {
		t.Fatal("expected white image with another file id to be blocked by perceptual hash")
	}
}

func TestMatcherBlocksRotatedImageWithPerceptionHashVariants(t *testing.T) {
	ctx := context.Background()
	blockedContent := domain.Content{
		FileID:       "blocked-pattern",
		FileUniqueID: "blocked-pattern-unique",
		Type:         domain.MediaPhoto,
	}
	candidateContent := domain.Content{
		FileID:       "candidate-pattern",
		FileUniqueID: "candidate-pattern-unique",
		Type:         domain.MediaPhoto,
	}

	blockedImage := patternImage()
	candidateImage := imaging.Rotate180(blockedImage)

	matcher := newTestSQLiteMatcher(t, ctx, fakeDownloader{}, fakeExtractor{
		images: map[string]image.Image{
			blockedContent.FileID:   blockedImage,
			candidateContent.FileID: candidateImage,
		},
	}, 8, 2)

	if err := matcher.Block(ctx, blockedContent); err != nil {
		t.Fatalf("block image: %v", err)
	}

	blocked, err := matcher.IsBlocked(ctx, candidateContent)
	if err != nil {
		t.Fatalf("check rotated candidate image: %v", err)
	}

	if !blocked {
		t.Fatal("expected rotated image to be blocked by perception hash variants")
	}
}

func newTestSQLiteMatcher(t *testing.T, ctx context.Context, downloader fakeDownloader, extractor fakeExtractor, threshold int, buffer int) *Matcher {
	t.Helper()

	matcher, err := NewSQLiteMatcher(ctx, downloader, extractor, threshold, buffer, filepath.Join(t.TempDir(), "imagehash.sqlite"))
	if err != nil {
		t.Fatalf("create sqlite matcher: %v", err)
	}

	t.Cleanup(func() {
		if err := matcher.Close(); err != nil {
			t.Fatalf("close sqlite matcher: %v", err)
		}
	})

	return matcher
}

func solidImage(width int, height int, c color.Color) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, c)
		}
	}

	return img
}

func patternImage() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 128, 96))

	for y := 0; y < 96; y++ {
		for x := 0; x < 128; x++ {
			img.Set(x, y, color.White)
		}
	}

	for y := 8; y < 80; y++ {
		for x := 12; x < 24; x++ {
			img.Set(x, y, color.Black)
		}
	}

	for y := 64; y < 78; y++ {
		for x := 12; x < 96; x++ {
			img.Set(x, y, color.Black)
		}
	}

	for y := 20; y < 42; y++ {
		for x := 72; x < 108; x++ {
			img.Set(x, y, color.RGBA{R: 180, A: 255})
		}
	}

	return img
}
