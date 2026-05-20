package moderation

import (
	"context"
	"image"
	"image/color"
	"image/draw"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/adapters/database/postgres"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/adapters/matching/exact"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/adapters/matching/imagehash"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/adapters/matching/orchestrator"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/adapters/matching/videolike"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/app/ports"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/config"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/domain"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

type fakeMatcher struct {
	blocked []domain.Content
	checked []domain.Content
}

func (m *fakeMatcher) Block(_ context.Context, _ int64, content domain.Content) error {
	m.blocked = append(m.blocked, content)
	return nil
}

func (m *fakeMatcher) IsBlocked(_ context.Context, _ int64, content domain.Content) (bool, error) {
	m.checked = append(m.checked, content)
	return false, nil
}

type fakeAdmins struct {
	admin bool
}

func (a fakeAdmins) IsAdmin(_ int64, _ int64) (bool, error) {
	return a.admin, nil
}

type fakeActions struct {
	sent    []sentMessage
	deleted []deletedMessage
}

type sentMessage struct {
	chatID int64
	text   string
}

type deletedMessage struct {
	chatID    int64
	messageID int
}

func (a *fakeActions) SendMessage(chatID int64, text string) error {
	a.sent = append(a.sent, sentMessage{
		chatID: chatID,
		text:   text,
	})
	return nil
}

func (a *fakeActions) DeleteMessage(chatID int64, messageID int) error {
	a.deleted = append(a.deleted, deletedMessage{
		chatID:    chatID,
		messageID: messageID,
	})
	return nil
}

func TestHandleMessageBanBlocksTargetAndDeletesTargetThenCommand(t *testing.T) {
	ctx := context.Background()
	matcher := &fakeMatcher{}
	actions := &fakeActions{}
	service, err := NewService(matcher, fakeAdmins{admin: true}, actions)
	if err != nil {
		t.Fatalf("создание сервиса: %v", err)
	}
	content := domain.Content{
		FileID:       "file-id",
		FileUniqueID: "file-unique-id",
		Type:         domain.MediaPhoto,
	}

	msg := domain.Message{
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

	if err := service.HandleMessage(ctx, msg); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(matcher.blocked, []domain.Content{content}) {
		t.Fatalf("заблокированный контент = %#v, ожидалось %#v", matcher.blocked, []domain.Content{content})
	}

	wantDeleted := []deletedMessage{
		{chatID: 100, messageID: 10},
		{chatID: 100, messageID: 20},
	}
	if !reflect.DeepEqual(actions.deleted, wantDeleted) {
		t.Fatalf("удаленные сообщения = %#v, ожидалось %#v", actions.deleted, wantDeleted)
	}
}

func TestHandleMessageBanDoesNotBlockOrDeleteTextReply(t *testing.T) {
	ctx := context.Background()
	matcher := &fakeMatcher{}
	actions := &fakeActions{}
	service, err := NewService(matcher, fakeAdmins{admin: true}, actions)
	if err != nil {
		t.Fatalf("создание сервиса: %v", err)
	}

	msg := domain.Message{
		ID:       20,
		ChatID:   100,
		SenderID: 1,
		Command:  "ban",
		ReplyTo: &domain.Message{
			ID:     10,
			ChatID: 100,
		},
	}

	if err := service.HandleMessage(ctx, msg); err != nil {
		t.Fatal(err)
	}

	if len(matcher.blocked) != 0 {
		t.Fatalf("заблокированный контент = %#v, ожидался пустой список", matcher.blocked)
	}

	if len(actions.deleted) != 0 {
		t.Fatalf("удаленные сообщения = %#v, ожидался пустой список", actions.deleted)
	}

	wantSent := []sentMessage{
		{chatID: 100, text: "Текстовые сообщения не баним"},
	}
	if !reflect.DeepEqual(actions.sent, wantSent) {
		t.Fatalf("отправленные сообщения = %#v, ожидалось %#v", actions.sent, wantSent)
	}
}

func TestHandleMessagePrivateChatSendsInfoAndSkipsModeration(t *testing.T) {
	ctx := context.Background()
	matcher := &fakeMatcher{}
	actions := &fakeActions{}
	service, err := NewService(matcher, fakeAdmins{admin: true}, actions)
	if err != nil {
		t.Fatalf("создание сервиса: %v", err)
	}
	content := domain.Content{
		FileID:       "file-id",
		FileUniqueID: "file-unique-id",
		Type:         domain.MediaPhoto,
	}

	msg := domain.Message{
		ID:       20,
		ChatID:   100,
		ChatType: domain.ChatPrivate,
		SenderID: 1,
		Command:  "ban",
		Content:  &content,
		ReplyTo: &domain.Message{
			ID:      10,
			ChatID:  100,
			Content: &content,
		},
	}

	if err := service.HandleMessage(ctx, msg); err != nil {
		t.Fatal(err)
	}

	wantSent := []sentMessage{
		{chatID: 100, text: privateChatInfo},
	}
	if !reflect.DeepEqual(actions.sent, wantSent) {
		t.Fatalf("отправленные сообщения = %#v, ожидалось %#v", actions.sent, wantSent)
	}

	if len(matcher.blocked) != 0 {
		t.Fatalf("заблокированный контент = %#v, ожидался пустой список", matcher.blocked)
	}

	if len(matcher.checked) != 0 {
		t.Fatalf("проверенный контент = %#v, ожидался пустой список", matcher.checked)
	}

	if len(actions.deleted) != 0 {
		t.Fatalf("удаленные сообщения = %#v, ожидался пустой список", actions.deleted)
	}
}

func TestNewServiceRejectsNilDependencies(t *testing.T) {
	matcher := &fakeMatcher{}
	admins := fakeAdmins{}
	actions := &fakeActions{}

	if _, err := NewService(nil, admins, actions); err == nil {
		t.Fatal("ожидалось, что матчер контента nil будет отклонен")
	}

	if _, err := NewService(matcher, nil, actions); err == nil {
		t.Fatal("ожидалось, что проверка админов равна nil будет отклонена")
	}

	if _, err := NewService(matcher, admins, nil); err == nil {
		t.Fatal("ожидалось, что действия сообщений равны nil будут отклонены")
	}
}

func TestServiceFunctionalStoresBanArtifactsInPostgres(t *testing.T) {
	ctx := context.Background()
	db := newFunctionalPostgresPool(t, ctx)
	applyFunctionalPostgresMigrations(t, ctx, db.Raw())

	media := functionalMedia{
		images: map[string]image.Image{
			"photo-original": e2EPatternImage(color.RGBA{R: 220, G: 40, B: 40, A: 255}),
			"photo-variant":  e2EVariantImage(color.RGBA{R: 220, G: 40, B: 40, A: 255}),
			"video-original": functionalFrameImage(color.RGBA{B: 220, A: 255}),
			"video-query":    functionalFrameImage(color.RGBA{B: 220, A: 255}),
		},
		paths: map[string]string{
			"photo-original": "photo-original.jpg",
			"photo-variant":  "photo-variant.jpg",
			"video-original": "video-original.mp4",
			"video-query":    "video-query.mp4",
		},
	}

	actions := &fakeActions{}
	service, closeMatchers := newFunctionalService(t, ctx, db, media, actions)
	defer closeMatchers()

	photoBan := domain.Content{
		FileID:       "photo-original",
		FileUniqueID: "photo-original-unique",
		Type:         domain.MediaPhoto,
	}
	videoBan := domain.Content{
		FileID:       "video-original",
		FileUniqueID: "video-original-unique",
		Type:         domain.MediaAnimation,
		DurationSec:  2,
		SizeBytes:    8,
	}

	handleBanCommand(t, ctx, service, 100, 10, 20, photoBan)
	handleBanCommand(t, ctx, service, 100, 30, 40, videoBan)
	closeMatchers()

	reloadedActions := &fakeActions{}
	reloadedService, closeReloadedMatchers := newFunctionalService(t, ctx, db, media, reloadedActions)
	defer closeReloadedMatchers()

	if err := reloadedService.HandleMessage(ctx, domain.Message{
		ID:     50,
		ChatID: 100,
		Content: &domain.Content{
			FileID:       "photo-variant",
			FileUniqueID: "photo-variant-unique",
			Type:         domain.MediaPhoto,
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := reloadedService.HandleMessage(ctx, domain.Message{
		ID:     60,
		ChatID: 100,
		Content: &domain.Content{
			FileID:       "video-query",
			FileUniqueID: "video-query-unique",
			Type:         domain.MediaAnimation,
			DurationSec:  2,
			SizeBytes:    8,
		},
	}); err != nil {
		t.Fatal(err)
	}

	wantDeleted := []deletedMessage{
		{chatID: 100, messageID: 50},
		{chatID: 100, messageID: 60},
	}
	if !reflect.DeepEqual(reloadedActions.deleted, wantDeleted) {
		t.Fatalf("удаленные сообщения = %#v, ожидалось %#v", reloadedActions.deleted, wantDeleted)
	}
}

func newFunctionalService(
	t *testing.T,
	ctx context.Context,
	db *postgres.Pool,
	media functionalMedia,
	actions *fakeActions,
) (*service, func()) {
	t.Helper()

	exactMatcher, err := exact.NewPostgresMatcher(ctx, db.Raw(), 4)
	if err != nil {
		t.Fatal(err)
	}
	imageHashMatcher, err := imagehash.NewPostgresMatcher(ctx, db.Raw(), media, media, 8, 4)
	if err != nil {
		t.Fatal(err)
	}
	videoLikeMatcher, err := videolike.NewPostgresMatcher(
		ctx,
		db.Raw(),
		media,
		media,
		0,
		4,
		domain.MediaExtractionPlan{MaxFrames: 2, TargetWidth: 64, TargetHeight: 64},
		videolike.Limits{
			MaxAnimationDuration:    10 * time.Second,
			MaxVideoStickerDuration: 3 * time.Second,
			MaxAnimationSize:        20 << 20,
			MaxVideoStickerSize:     256 << 10,
		},
		videolike.MatchRule{MinMatchedFrames: 1, MinMatchedRatio: 0.5},
	)
	if err != nil {
		t.Fatal(err)
	}

	blocker, err := orchestrator.NewBlockOrchestrator(
		db,
		[]ports.ContentBlockMatcher{exactMatcher, imageHashMatcher, videoLikeMatcher}...,
	)
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewService(blocker, fakeAdmins{admin: true}, actions)
	if err != nil {
		t.Fatal(err)
	}

	closeMatchers := func() {
		if err := exactMatcher.Close(); err != nil {
			t.Fatal(err)
		}
		if err := imageHashMatcher.Close(); err != nil {
			t.Fatal(err)
		}
		if err := videoLikeMatcher.Close(); err != nil {
			t.Fatal(err)
		}
	}

	return service, closeMatchers
}

func handleBanCommand(
	t *testing.T,
	ctx context.Context,
	service *service,
	chatID int64,
	targetID int,
	commandID int,
	content domain.Content,
) {
	t.Helper()

	if err := service.HandleMessage(ctx, domain.Message{
		ID:       commandID,
		ChatID:   chatID,
		SenderID: 1,
		Command:  "ban",
		ReplyTo: &domain.Message{
			ID:      targetID,
			ChatID:  chatID,
			Content: &content,
		},
	}); err != nil {
		t.Fatal(err)
	}
}

type functionalMedia struct {
	images map[string]image.Image
	paths  map[string]string
}

func (m functionalMedia) Download(_ context.Context, content domain.Content) (domain.MediaFile, error) {
	return domain.MediaFile{
		Content:  content,
		FilePath: m.paths[content.FileID],
		Data:     []byte(content.FileID),
	}, nil
}

func (m functionalMedia) Extract(_ context.Context, media domain.MediaFile, _ domain.MediaExtractionPlan) (domain.ExtractedMedia, error) {
	img, ok := m.images[media.Content.FileID]
	if !ok {
		img = functionalFrameImage(color.RGBA{R: 120, A: 255})
	}

	return domain.ExtractedMedia{
		Frames: []domain.ExtractedFrame{
			{Index: 0, PositionMillis: 0, Image: img},
			{Index: 1, PositionMillis: 1000, Image: img},
		},
	}, nil
}

func functionalFrameImage(base color.RGBA) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: base}, image.Point{}, draw.Src)
	return img
}

func newFunctionalPostgresPool(t *testing.T, ctx context.Context) *postgres.Pool {
	t.Helper()

	container, err := tcpostgres.Run(
		ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("bastyle"),
		tcpostgres.WithUsername("bastyle"),
		tcpostgres.WithPassword("bastyle"),
		testcontainers.WithWaitStrategy(wait.ForListeningPort("5432/tcp").WithStartupTimeout(30*time.Second)),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			t.Fatal(err)
		}
	})

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}

	pool, err := postgres.NewPool(ctx, config.Database{
		DSN:               dsn,
		MaxConns:          4,
		MinConns:          1,
		MaxConnLifetime:   config.Duration(time.Hour),
		MaxConnIdleTime:   config.Duration(15 * time.Minute),
		HealthCheckPeriod: config.Duration(30 * time.Second),
		ConnectTimeout:    config.Duration(5 * time.Second),
		StatementTimeout:  config.Duration(10 * time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func applyFunctionalPostgresMigrations(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	botDir := filepath.Clean(filepath.Join(wd, "../../.."))
	migrationPaths, err := filepath.Glob(filepath.Join(botDir, "internal/adapters/database/postgres/migrations/*.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for _, migrationPath := range migrationPaths {
		raw, err := os.ReadFile(migrationPath)
		if err != nil {
			t.Fatal(err)
		}
		sql := functionalGooseUpSQL(string(raw))
		if sql == "" {
			continue
		}
		if _, err := pool.Exec(ctx, sql); err != nil {
			t.Fatalf("применение миграции %s: %v", migrationPath, err)
		}
	}
}

func functionalGooseUpSQL(raw string) string {
	const upMarker = "-- +goose Up"
	const downMarker = "-- +goose Down"

	upIndex := strings.Index(raw, upMarker)
	if upIndex < 0 {
		return raw
	}
	raw = raw[upIndex+len(upMarker):]
	downIndex := strings.Index(raw, downMarker)
	if downIndex >= 0 {
		raw = raw[:downIndex]
	}
	return strings.TrimSpace(raw)
}
