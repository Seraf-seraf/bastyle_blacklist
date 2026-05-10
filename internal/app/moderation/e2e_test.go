package moderation

import (
	"context"
	"image"
	"image/color"
	"image/draw"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/adapters/matching/composite"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/adapters/matching/exact"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/adapters/matching/imagehash"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/domain"
)

func TestAppE2EWithoutTelegramBanDeletesRepeatedAndSimilarPhoto(t *testing.T) {
	ctx := context.Background()
	actions := &fakeActions{}
	media := fakeE2EMedia{
		"original-file": e2EPatternImage(color.RGBA{R: 220, G: 40, B: 40, A: 255}),
		"variant-file":  e2EVariantImage(color.RGBA{R: 220, G: 40, B: 40, A: 255}),
	}
	service, closeMatcher := newE2EModerationService(t, media, fakeAdmins{admin: true}, actions)
	defer closeMatcher()

	originalContent := domain.Content{
		FileID:       "original-file",
		FileUniqueID: "original-unique",
		Type:         domain.MediaPhoto,
	}
	banCommand := domain.Message{
		ID:       20,
		ChatID:   100,
		SenderID: 1,
		Command:  "ban",
		ReplyTo: &domain.Message{
			ID:      10,
			ChatID:  100,
			Content: &originalContent,
		},
	}

	if err := service.HandleMessage(ctx, banCommand); err != nil {
		t.Fatal(err)
	}

	repeatedContent := domain.Content{
		FileID:       "repeated-file",
		FileUniqueID: "original-unique",
		Type:         domain.MediaPhoto,
	}
	repeatedMessage := domain.Message{
		ID:      30,
		ChatID:  100,
		Content: &repeatedContent,
	}
	if err := service.HandleMessage(ctx, repeatedMessage); err != nil {
		t.Fatal(err)
	}

	similarContent := domain.Content{
		FileID:       "variant-file",
		FileUniqueID: "variant-unique",
		Type:         domain.MediaPhoto,
	}
	similarMessage := domain.Message{
		ID:      40,
		ChatID:  100,
		Content: &similarContent,
	}
	if err := service.HandleMessage(ctx, similarMessage); err != nil {
		t.Fatal(err)
	}

	wantDeleted := []deletedMessage{
		{chatID: 100, messageID: 10},
		{chatID: 100, messageID: 20},
		{chatID: 100, messageID: 30},
		{chatID: 100, messageID: 40},
	}
	if !reflect.DeepEqual(actions.deleted, wantDeleted) {
		t.Fatalf("удаленные сообщения = %#v, ожидалось %#v", actions.deleted, wantDeleted)
	}
	if len(actions.sent) != 0 {
		t.Fatalf("отправленные сообщения = %#v, ожидался пустой список", actions.sent)
	}
}

func TestAppE2EWithoutTelegramNonAdminBanDoesNotBlockPhoto(t *testing.T) {
	ctx := context.Background()
	actions := &fakeActions{}
	media := fakeE2EMedia{
		"original-file": e2EPatternImage(color.RGBA{R: 40, G: 90, B: 220, A: 255}),
	}
	service, closeMatcher := newE2EModerationService(t, media, fakeAdmins{admin: false}, actions)
	defer closeMatcher()

	content := domain.Content{
		FileID:       "original-file",
		FileUniqueID: "original-unique",
		Type:         domain.MediaPhoto,
	}
	banCommand := domain.Message{
		ID:       20,
		ChatID:   100,
		SenderID: 1,
		Command:  "ban",
		ReplyTo: &domain.Message{
			ID:      10,
			ChatID:  100,
			Content: &content,
		},
	}

	if err := service.HandleMessage(ctx, banCommand); err != nil {
		t.Fatal(err)
	}
	if err := service.HandleMessage(ctx, domain.Message{
		ID:      30,
		ChatID:  100,
		Content: &content,
	}); err != nil {
		t.Fatal(err)
	}

	if len(actions.deleted) != 0 {
		t.Fatalf("удаленные сообщения = %#v, ожидался пустой список", actions.deleted)
	}

	wantSent := []sentMessage{
		{chatID: 100, text: "Команда доступна только админам"},
	}
	if !reflect.DeepEqual(actions.sent, wantSent) {
		t.Fatalf("отправленные сообщения = %#v, ожидалось %#v", actions.sent, wantSent)
	}
}

func newE2EModerationService(
	t *testing.T,
	media fakeE2EMedia,
	admins fakeAdmins,
	actions *fakeActions,
) (*service, func()) {
	t.Helper()

	exactMatcher, err := exact.NewSQLiteMatcher(context.Background(), 10, filepath.Join(t.TempDir(), "exact.sqlite"))
	if err != nil {
		t.Fatalf("создание exact-матчера: %v", err)
	}
	imageHashMatcher, err := imagehash.NewSQLiteMatcher(
		context.Background(),
		media,
		media,
		12,
		10,
		t.TempDir()+"/imagehash.sqlite",
	)
	if err != nil {
		t.Fatalf("создание imagehash-матчера: %v", err)
	}
	contentMatcher, err := composite.NewMatcher(exactMatcher, imageHashMatcher)
	if err != nil {
		t.Fatalf("создание composite-матчера: %v", err)
	}
	service, err := NewService(contentMatcher, admins, actions)
	if err != nil {
		t.Fatalf("создание сервиса модерации: %v", err)
	}

	return service, func() {
		if err := exactMatcher.Close(); err != nil {
			t.Fatalf("закрытие exact-матчера: %v", err)
		}
		if err := imageHashMatcher.Close(); err != nil {
			t.Fatalf("закрытие imagehash-матчера: %v", err)
		}
	}
}

type fakeE2EMedia map[string]image.Image

func (m fakeE2EMedia) Download(_ context.Context, content domain.Content) (domain.MediaFile, error) {
	return domain.MediaFile{
		Content:  content,
		FilePath: content.FileID + ".jpg",
		Data:     []byte(content.FileID),
	}, nil
}

func (m fakeE2EMedia) Extract(_ context.Context, media domain.MediaFile, _ domain.MediaExtractionPlan) (domain.ExtractedMedia, error) {
	img, ok := m[media.Content.FileID]
	if !ok {
		img = m["original-file"]
	}

	return domain.ExtractedMedia{
		Frames: []domain.ExtractedFrame{
			{
				Index: 0,
				Image: img,
			},
		},
	}, nil
}

func e2EPatternImage(base color.RGBA) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 96, 96))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: color.RGBA{R: 245, G: 245, B: 245, A: 255}}, image.Point{}, draw.Src)

	draw.Draw(img, image.Rect(8, 8, 64, 64), &image.Uniform{C: base}, image.Point{}, draw.Src)
	draw.Draw(img, image.Rect(30, 30, 88, 88), &image.Uniform{C: color.RGBA{R: 30, G: 30, B: 30, A: 255}}, image.Point{}, draw.Src)
	draw.Draw(img, image.Rect(36, 36, 82, 82), &image.Uniform{C: base}, image.Point{}, draw.Src)

	return img
}

func e2EVariantImage(base color.RGBA) image.Image {
	img := e2EPatternImage(base).(*image.RGBA)
	draw.Draw(img, image.Rect(10, 10, 20, 20), &image.Uniform{C: color.RGBA{R: 230, G: 230, B: 230, A: 255}}, image.Point{}, draw.Src)
	return img
}
