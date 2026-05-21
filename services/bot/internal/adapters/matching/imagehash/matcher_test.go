package imagehash

import (
	"context"
	"image"
	"image/color"
	"testing"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/domain"
	"github.com/disintegration/imaging"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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

	matcher := newTestMatcherWithStore(t, fakeDownloader{}, fakeExtractor{
		images: map[string]image.Image{
			blockedContent.FileID:   solidImage(128, 128, color.White),
			candidateContent.FileID: solidImage(320, 240, color.White),
		},
	}, 8, 2)

	if err := applyPreparedBlock(ctx, matcher, 10, blockedContent); err != nil {
		t.Fatalf("блокировка белого изображения: %v", err)
	}

	blocked, err := matcher.IsBlocked(ctx, 10, candidateContent)
	if err != nil {
		t.Fatalf("проверка белого изображения-кандидата: %v", err)
	}

	if !blocked {
		t.Fatal("ожидалось: белое изображение с другим file_id должно быть заблокировано перцептивным хешем")
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

	matcher := newTestMatcherWithStore(t, fakeDownloader{}, fakeExtractor{
		images: map[string]image.Image{
			blockedContent.FileID:   blockedImage,
			candidateContent.FileID: candidateImage,
		},
	}, 8, 2)

	if err := applyPreparedBlock(ctx, matcher, 10, blockedContent); err != nil {
		t.Fatalf("блокировка изображения: %v", err)
	}

	blocked, err := matcher.IsBlocked(ctx, 10, candidateContent)
	if err != nil {
		t.Fatalf("проверка повернутого изображения-кандидата: %v", err)
	}

	if !blocked {
		t.Fatal("ожидалось: повернутое изображение должно быть заблокировано вариантами перцептивного хеша")
	}
}

func TestNewMatcherRejectsInvalidConfig(t *testing.T) {
	tests := []struct {
		name       string
		downloader fakeDownloader
		extractor  fakeExtractor
		threshold  int
		buffer     int
	}{
		{
			name:      "negative порог",
			extractor: fakeExtractor{},
			threshold: -1,
		},
		{
			name:      "отрицательный буфер",
			extractor: fakeExtractor{},
			buffer:    -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewMatcher(tt.downloader, tt.extractor, tt.threshold, tt.buffer)
			if err == nil {
				t.Fatal("ожидалось, что некорректная конфигурация будет отклонен")
			}
		})
	}

	_, err := NewMatcher(nil, fakeExtractor{}, 0, 0)
	if err == nil {
		t.Fatal("ожидалось, что загрузчик равен nil будет отклонен")
	}

	_, err = NewMatcher(fakeDownloader{}, nil, 0, 0)
	if err == nil {
		t.Fatal("ожидалось, что извлекатель равен nil будет отклонен")
	}
}

func newTestMatcherWithStore(t *testing.T, downloader fakeDownloader, extractor fakeExtractor, threshold int, buffer int) *matcher {
	t.Helper()

	matcher, err := NewMatcher(downloader, extractor, threshold, buffer)
	if err != nil {
		t.Fatalf("создание imagehash-матчера: %v", err)
	}
	matcher.store = &memoryImageHashStore{}

	t.Cleanup(func() {
		if err := matcher.Close(); err != nil {
			t.Fatalf("закрытие imagehash-матчера: %v", err)
		}
	})

	return matcher
}

type memoryImageHashStore struct {
	hashes []StoredImageHash
	nextID int64
}

func (s *memoryImageHashStore) load(context.Context) ([]StoredImageHash, error) {
	return append([]StoredImageHash(nil), s.hashes...), nil
}

func (s *memoryImageHashStore) LoadByBanUID(context.Context, uuid.UUID) (StoredImageHash, error) {
	return StoredImageHash{}, errImageHashArtifactNotFound
}

func (s *memoryImageHashStore) Insert(_ context.Context, _ pgx.Tx, _ uuid.UUID, hash StoredImageHash) (imageHashInsertResult, error) {
	signature := imageHashIndexSignature(hash)
	for _, stored := range s.hashes {
		if imageHashIndexSignature(stored) == signature {
			return imageHashInsertResult{ID: stored.ID}, nil
		}
	}
	s.nextID++
	hash.ID = s.nextID
	s.hashes = append(s.hashes, hash)
	return imageHashInsertResult{ID: hash.ID, Created: true}, nil
}

func (s *memoryImageHashStore) close() error {
	return nil
}

func applyPreparedBlock(ctx context.Context, matcher *matcher, chatID int64, content domain.Content) error {
	block, err := matcher.PrepareBlock(ctx, chatID, content)
	if err != nil {
		return err
	}

	return matcher.ApplyBlock(ctx, block)
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
