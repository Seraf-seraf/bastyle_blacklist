package videolike

import (
	"context"
	"errors"
	"image"
	"image/color"
	"os/exec"
	"testing"
	"time"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/adapters/media"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type fakeDownloader struct {
	filePaths map[string]string
	data      map[string][]byte
	err       error
	calls     int
}

func (d *fakeDownloader) Download(_ context.Context, content domain.Content) (domain.MediaFile, error) {
	d.calls++
	if d.err != nil {
		return domain.MediaFile{}, d.err
	}

	return domain.MediaFile{
		Content:  content,
		FilePath: d.filePaths[content.FileID],
		Data:     d.data[content.FileID],
	}, nil
}

type fakeExtractor struct {
	extracted domain.ExtractedMedia
	err       error
	calls     int
	plans     []domain.MediaExtractionPlan
}

func (e *fakeExtractor) Extract(_ context.Context, _ domain.MediaFile, plan domain.MediaExtractionPlan) (domain.ExtractedMedia, error) {
	e.calls++
	e.plans = append(e.plans, plan)
	if e.err != nil {
		return domain.ExtractedMedia{}, e.err
	}

	return e.extracted, nil
}

func TestSupportsAcceptsAnimation(t *testing.T) {
	downloader := &fakeDownloader{
		filePaths: map[string]string{
			"animation-file": "animations/file.mp4",
		},
		data: map[string][]byte{
			"animation-file": []byte("animation"),
		},
	}
	matcher := newTestMatcher(t, downloader)

	supported, err := matcher.supports(context.Background(), domain.Content{
		FileID:       "animation-file",
		FileUniqueID: "animation-unique",
		Type:         domain.MediaAnimation,
	})
	if err != nil {
		t.Fatalf("проверка поддержки анимации: %v", err)
	}
	if !supported {
		t.Fatal("ожидалось: анимация должна поддерживаться")
	}
	if downloader.calls != 1 {
		t.Fatalf("вызовы загрузки = %d, ожидалось 1", downloader.calls)
	}
}

func TestSupportsAcceptsWebMStickerAfterDownload(t *testing.T) {
	downloader := &fakeDownloader{
		filePaths: map[string]string{
			"sticker-file": "stickers/video_sticker.webm",
		},
	}
	matcher := newTestMatcher(t, downloader)

	supported, err := matcher.supports(context.Background(), domain.Content{
		FileID:       "sticker-file",
		FileUniqueID: "sticker-unique",
		Type:         domain.MediaStickerStatic,
	})
	if err != nil {
		t.Fatalf("проверка поддержки webm-стикера: %v", err)
	}
	if !supported {
		t.Fatal("ожидалось: webm-стикер должен поддерживаться")
	}
}

func TestSupportsRejectsStaticWebPSticker(t *testing.T) {
	downloader := &fakeDownloader{
		filePaths: map[string]string{
			"sticker-file": "stickers/static_sticker.webp",
		},
	}
	matcher := newTestMatcher(t, downloader)

	supported, err := matcher.supports(context.Background(), domain.Content{
		FileID:       "sticker-file",
		FileUniqueID: "sticker-unique",
		Type:         domain.MediaStickerStatic,
	})
	if err != nil {
		t.Fatalf("проверка поддержки статического стикера: %v", err)
	}
	if supported {
		t.Fatal("ожидалось: статический webp-стикер должен быть отклонен")
	}
}

func TestSupportsRejectsStaticWebPStickerWithoutApplyingVideoStickerSizeLimit(t *testing.T) {
	downloader := &fakeDownloader{
		filePaths: map[string]string{
			"sticker-file": "stickers/static_sticker.webp",
		},
	}
	matcher := newTestMatcher(t, downloader)

	supported, err := matcher.supports(context.Background(), domain.Content{
		FileID:       "sticker-file",
		FileUniqueID: "sticker-unique",
		Type:         domain.MediaStickerStatic,
		SizeBytes:    defaultTestLimits().MaxVideoStickerSize + 1,
	})
	if err != nil {
		t.Fatalf("проверка поддержки статического стикера: %v", err)
	}
	if supported {
		t.Fatal("ожидалось: статический webp-стикер должен быть отклонен")
	}
}

func TestSupportsRejectsAnimatedTGSStickerWithoutDownload(t *testing.T) {
	downloader := &fakeDownloader{}
	matcher := newTestMatcher(t, downloader)

	supported, err := matcher.supports(context.Background(), domain.Content{
		FileID:       "animated-sticker-file",
		FileUniqueID: "animated-sticker-unique",
		Type:         domain.MediaStickerAnimated,
	})
	if err != nil {
		t.Fatalf("проверка поддержки анимированного стикера: %v", err)
	}
	if supported {
		t.Fatal("ожидалось: анимированный стикер должен быть отклонен")
	}
	if downloader.calls != 0 {
		t.Fatalf("вызовы загрузки = %d, ожидалось 0", downloader.calls)
	}
}

func TestSupportsRejectsOrdinaryVideoAndDocumentTypes(t *testing.T) {
	downloader := &fakeDownloader{}
	matcher := newTestMatcher(t, downloader)

	for _, mediaType := range []domain.MediaType{"video", "document"} {
		supported, err := matcher.supports(context.Background(), domain.Content{
			FileID:       string(mediaType) + "-file",
			FileUniqueID: string(mediaType) + "-unique",
			Type:         mediaType,
		})
		if err != nil {
			t.Fatalf("проверка поддержки %s: %v", mediaType, err)
		}
		if supported {
			t.Fatalf("ожидалось, что %s будет отклонен", mediaType)
		}
	}

	if downloader.calls != 0 {
		t.Fatalf("вызовы загрузки = %d, ожидалось 0", downloader.calls)
	}
}

func TestSupportsReturnsDownloaderErrorForStickerScopeDetection(t *testing.T) {
	downloadErr := errors.New("ошибка загрузки")
	matcher := newTestMatcher(t, &fakeDownloader{err: downloadErr})

	_, err := matcher.supports(context.Background(), domain.Content{
		FileID:       "sticker-file",
		FileUniqueID: "sticker-unique",
		Type:         domain.MediaStickerStatic,
	})
	if !errors.Is(err, downloadErr) {
		t.Fatalf("ошибка проверки поддержки = %v, ожидалось %v", err, downloadErr)
	}
}

func TestSupportsRejectsAnimationOverDurationLimitWithoutDownload(t *testing.T) {
	downloader := &fakeDownloader{}
	matcher := newTestMatcher(t, downloader)

	_, err := matcher.supports(context.Background(), domain.Content{
		FileID:       "animation-file",
		FileUniqueID: "animation-unique",
		Type:         domain.MediaAnimation,
		DurationSec:  int(defaultTestLimits().MaxAnimationDuration/time.Second) + 1,
	})
	if err == nil {
		t.Fatal("ожидалось: ошибка анимации лимита длительности")
	}
	if downloader.calls != 0 {
		t.Fatalf("вызовы загрузки = %d, ожидалось 0", downloader.calls)
	}
}

func TestSupportsRejectsAnimationOverSizeLimitWithoutDownload(t *testing.T) {
	downloader := &fakeDownloader{}
	matcher := newTestMatcher(t, downloader)

	_, err := matcher.supports(context.Background(), domain.Content{
		FileID:       "animation-file",
		FileUniqueID: "animation-unique",
		Type:         domain.MediaAnimation,
		SizeBytes:    defaultTestLimits().MaxAnimationSize + 1,
	})
	if err == nil {
		t.Fatal("ожидалось: ошибка лимита размера анимации")
	}
	if downloader.calls != 0 {
		t.Fatalf("вызовы загрузки = %d, ожидалось 0", downloader.calls)
	}
}

func TestSupportsRejectsVideoStickerOverDurationLimitWithoutDownload(t *testing.T) {
	downloader := &fakeDownloader{}
	matcher := newTestMatcher(t, downloader)

	_, err := matcher.supports(context.Background(), domain.Content{
		FileID:       "sticker-file",
		FileUniqueID: "sticker-unique",
		Type:         domain.MediaStickerStatic,
		DurationSec:  int(defaultTestLimits().MaxVideoStickerDuration/time.Second) + 1,
	})
	if err == nil {
		t.Fatal("ожидалось: ошибка видеостикера лимита длительности")
	}
	if downloader.calls != 0 {
		t.Fatalf("вызовы загрузки = %d, ожидалось 0", downloader.calls)
	}
}

func TestSupportsRejectsVideoStickerOverMetadataSizeLimitAfterFileTypeDetection(t *testing.T) {
	downloader := &fakeDownloader{
		filePaths: map[string]string{
			"sticker-file": "stickers/video_sticker.webm",
		},
	}
	matcher := newTestMatcher(t, downloader)

	_, err := matcher.supports(context.Background(), domain.Content{
		FileID:       "sticker-file",
		FileUniqueID: "sticker-unique",
		Type:         domain.MediaStickerStatic,
		SizeBytes:    defaultTestLimits().MaxVideoStickerSize + 1,
	})
	if err == nil {
		t.Fatal("ожидалось: ошибка лимита размера видеостикера")
	}
	if downloader.calls != 1 {
		t.Fatalf("вызовы загрузки = %d, ожидалось 1", downloader.calls)
	}
}

func TestSupportsRejectsVideoStickerOverDownloadedSizeLimit(t *testing.T) {
	limits := defaultTestLimits()
	downloader := &fakeDownloader{
		filePaths: map[string]string{
			"sticker-file": "stickers/video_sticker.webm",
		},
		data: map[string][]byte{
			"sticker-file": make([]byte, int(limits.MaxVideoStickerSize+1)),
		},
	}
	matcher := newTestMatcherWithLimits(t, downloader, limits)

	_, err := matcher.supports(context.Background(), domain.Content{
		FileID:       "sticker-file",
		FileUniqueID: "sticker-unique",
		Type:         domain.MediaStickerStatic,
	})
	if err == nil {
		t.Fatal("ожидалось: ошибка лимита размера загрузки")
	}
	if downloader.calls != 1 {
		t.Fatalf("вызовы загрузки = %d, ожидалось 1", downloader.calls)
	}
}

func TestNewMatcherRejectsNilDownloader(t *testing.T) {
	_, err := NewMatcher(nil, &fakeExtractor{}, 8, 1, defaultTestPlan(), defaultTestLimits(), DefaultMatchRule())
	if err == nil {
		t.Fatal("ожидалось, что загрузчик равен nil будет отклонен")
	}
}

func TestNewMatcherRejectsInvalidLimits(t *testing.T) {
	_, err := NewMatcher(&fakeDownloader{}, &fakeExtractor{}, 8, 1, defaultTestPlan(), Limits{}, DefaultMatchRule())
	if err == nil {
		t.Fatal("ожидалось, что некорректные лимиты будет отклонен")
	}
}

func TestMatcherPersistBlockRequiresStore(t *testing.T) {
	matcher := newTestMatcher(t, &fakeDownloader{
		filePaths: map[string]string{
			"animation-file": "animations/file.mp4",
		},
		data: map[string][]byte{
			"animation-file": []byte("animation"),
		},
	})

	_, err := matcher.PersistBlock(context.Background(), nil, uuid.New(), &preparedBlock{
		fingerprint: StoredVideoLikeHash{
			Frames: []StoredVideoLikeFrameHash{{FrameIndex: 0}},
		},
	})
	if err == nil {
		t.Fatal("ожидалось: ошибка отсутствующего хранилища")
	}
}

func TestMatcherBlockStoresFingerprintAndIsBlockedFindsIt(t *testing.T) {
	ctx := context.Background()
	downloader := &fakeDownloader{
		filePaths: map[string]string{
			"blocked-file": "animations/blocked.mp4",
			"query-file":   "animations/query.mp4",
		},
		data: map[string][]byte{
			"blocked-file": []byte("blocked"),
			"query-file":   []byte("query"),
		},
	}
	extractor := &fakeExtractor{extracted: testExtractedVideoLikeMedia()}
	matcher := newTestMatcherWithStore(t, downloader, extractor, nil)

	err := applyPreparedBlock(ctx, matcher, 10, domain.Content{
		FileID:       "blocked-file",
		FileUniqueID: "blocked-unique",
		Type:         domain.MediaAnimation,
		DurationSec:  3,
		SizeBytes:    7,
	})
	if err != nil {
		t.Fatalf("блокировка анимации: %v", err)
	}

	blocked, err := matcher.IsBlocked(ctx, 10, domain.Content{
		FileID:       "query-file",
		FileUniqueID: "query-unique",
		Type:         domain.MediaAnimation,
		DurationSec:  3,
		SizeBytes:    5,
	})
	if err != nil {
		t.Fatalf("проверка блокировки анимации: %v", err)
	}
	if !blocked {
		t.Fatal("ожидалось: анимация должна блокироваться сохраненным fingerprint")
	}
}

func TestMatcherIsBlockedAllowsUnrelatedAnimation(t *testing.T) {
	ctx := context.Background()
	downloader := &fakeDownloader{
		filePaths: map[string]string{
			"blocked-file": "animations/blocked.mp4",
			"query-file":   "animations/query.mp4",
		},
		data: map[string][]byte{
			"blocked-file": []byte("blocked"),
			"query-file":   []byte("query"),
		},
	}
	extractor := &fakeExtractor{extracted: testExtractedVideoLikeMedia()}
	matcher := newTestMatcherWithStore(t, downloader, extractor, nil)

	err := applyPreparedBlock(ctx, matcher, 10, domain.Content{
		FileID:       "blocked-file",
		FileUniqueID: "blocked-unique",
		Type:         domain.MediaAnimation,
		DurationSec:  3,
		SizeBytes:    7,
	})
	if err != nil {
		t.Fatalf("блокировка анимации: %v", err)
	}

	extractor.extracted = domain.ExtractedMedia{
		Frames: []domain.ExtractedFrame{
			{Index: 0, PositionMillis: 0, Image: testPatternImage(1)},
			{Index: 1, PositionMillis: 1000, Image: testPatternImage(2)},
			{Index: 2, PositionMillis: 2000, Image: testPatternImage(3)},
		},
	}

	blocked, err := matcher.IsBlocked(ctx, 10, domain.Content{
		FileID:       "query-file",
		FileUniqueID: "query-unique",
		Type:         domain.MediaAnimation,
		DurationSec:  3,
		SizeBytes:    5,
	})
	if err != nil {
		t.Fatalf("проверка блокировки анимации: %v", err)
	}
	if blocked {
		t.Fatal("ожидалось: несвязанная анимация должна быть разрешена")
	}
}

func TestMatcherBlockStoresVideoStickerFingerprint(t *testing.T) {
	ctx := context.Background()
	downloader := &fakeDownloader{
		filePaths: map[string]string{
			"blocked-sticker": "stickers/blocked.webm",
			"query-sticker":   "stickers/query.webm",
		},
		data: map[string][]byte{
			"blocked-sticker": []byte("blocked"),
			"query-sticker":   []byte("query"),
		},
	}
	extractor := &fakeExtractor{extracted: testExtractedVideoLikeMedia()}
	matcher := newTestMatcherWithStore(t, downloader, extractor, nil)

	err := applyPreparedBlock(ctx, matcher, 10, domain.Content{
		FileID:       "blocked-sticker",
		FileUniqueID: "blocked-sticker-unique",
		Type:         domain.MediaStickerStatic,
		DurationSec:  2,
		SizeBytes:    7,
	})
	if err != nil {
		t.Fatalf("блокировка видеостикера: %v", err)
	}

	blocked, err := matcher.IsBlocked(ctx, 10, domain.Content{
		FileID:       "query-sticker",
		FileUniqueID: "query-sticker-unique",
		Type:         domain.MediaStickerStatic,
		DurationSec:  2,
		SizeBytes:    5,
	})
	if err != nil {
		t.Fatalf("проверка блокировки видеостикера: %v", err)
	}
	if !blocked {
		t.Fatal("ожидалось: видеостикер должен блокироваться сохраненным fingerprint")
	}
}

func TestMatcherLoadsStoredHashesIntoIndex(t *testing.T) {
	ctx := context.Background()
	storedFingerprint, err := fingerprintVideoLike(domain.Content{
		FileUniqueID: "stored-unique",
		Type:         domain.MediaAnimation,
		DurationSec:  3,
	}, domain.ExtractedMedia{
		Frames: []domain.ExtractedFrame{
			{Index: 0, PositionMillis: 0, Image: testSolidImage(color.RGBA{R: 255, A: 255})},
			{Index: 1, PositionMillis: 1000, Image: testSolidImage(color.RGBA{G: 255, A: 255})},
		},
	})
	if err != nil {
		t.Fatalf("создание fingerprint для сохраненного хеша: %v", err)
	}
	storedFingerprint.ChatID = 10

	downloader := &fakeDownloader{
		filePaths: map[string]string{
			"query-file": "animations/query.mp4",
		},
		data: map[string][]byte{
			"query-file": []byte("query"),
		},
	}
	extractor := &fakeExtractor{extracted: domain.ExtractedMedia{
		Frames: []domain.ExtractedFrame{
			{Index: 0, PositionMillis: 0, Image: testSolidImage(color.RGBA{R: 255, A: 255})},
			{Index: 1, PositionMillis: 1000, Image: testSolidImage(color.RGBA{G: 255, A: 255})},
		},
	}}

	matcher := newTestMatcherWithStore(
		t,
		downloader,
		extractor,
		[]StoredVideoLikeHash{storedFingerprint},
	)

	blocked, err := matcher.IsBlocked(ctx, 10, domain.Content{
		FileID:       "query-file",
		FileUniqueID: "query-unique",
		Type:         domain.MediaAnimation,
		DurationSec:  3,
		SizeBytes:    5,
	})
	if err != nil {
		t.Fatalf("проверка блокировки: %v", err)
	}
	if !blocked {
		t.Fatal("ожидалось: загруженный fingerprint должен блокировать запрос")
	}
}

func TestMatcherWithRealFFmpegRejectsDurationLimitBeforeExtraction(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg недоступен")
	}
	extractor, err := media.NewFFmpegFrameExtractor(ffmpeg, time.Second)
	if err != nil {
		t.Fatalf("создание ffmpeg-экстрактора: %v", err)
	}
	downloader := &fakeDownloader{}
	limits := defaultTestLimits()
	limits.MaxAnimationDuration = time.Second

	matcher, err := NewMatcher(
		downloader,
		extractor,
		8,
		1,
		defaultTestPlan(),
		limits,
		DefaultMatchRule(),
	)
	if err != nil {
		t.Fatalf("создание матчера: %v", err)
	}

	_, err = matcher.IsBlocked(context.Background(), 10, domain.Content{
		FileID:       "long-animation-file",
		FileUniqueID: "long-animation-unique",
		Type:         domain.MediaAnimation,
		DurationSec:  2,
	})
	if err == nil {
		t.Fatal("ожидалось: ошибка лимита длительности")
	}
	if downloader.calls != 0 {
		t.Fatalf("вызовы загрузки = %d, ожидалось 0", downloader.calls)
	}
}

func newTestMatcher(t *testing.T, downloader *fakeDownloader) *matcher {
	t.Helper()

	return newTestMatcherWithLimits(t, downloader, defaultTestLimits())
}

func newTestMatcherWithLimits(t *testing.T, downloader *fakeDownloader, limits Limits) *matcher {
	t.Helper()

	matcher, err := NewMatcher(downloader, &fakeExtractor{}, 8, 1, defaultTestPlan(), limits, DefaultMatchRule())
	if err != nil {
		t.Fatalf("создание матчера: %v", err)
	}

	return matcher
}

func newTestMatcherWithStore(t *testing.T, downloader *fakeDownloader, extractor *fakeExtractor, stored []StoredVideoLikeHash) *matcher {
	t.Helper()

	matcher, err := NewMatcher(
		downloader,
		extractor,
		0,
		1,
		defaultTestPlan(),
		defaultTestLimits(),
		MatchRule{MinMatchedFrames: 2, MinMatchedRatio: 1},
	)
	if err != nil {
		t.Fatalf("создание video-like матчера: %v", err)
	}
	matcher.store = &memoryVideoLikeStore{hashes: append([]StoredVideoLikeHash(nil), stored...)}
	matcher.index.AddMany(stored)
	t.Cleanup(func() {
		if err := matcher.Close(); err != nil {
			t.Fatalf("закрытие матчера: %v", err)
		}
	})

	return matcher
}

type memoryVideoLikeStore struct {
	hashes []StoredVideoLikeHash
	nextID int64
}

func (s *memoryVideoLikeStore) load(context.Context) ([]StoredVideoLikeHash, error) {
	return append([]StoredVideoLikeHash(nil), s.hashes...), nil
}

func (s *memoryVideoLikeStore) Insert(_ context.Context, _ pgx.Tx, _ uuid.UUID, hash StoredVideoLikeHash) (int64, bool, error) {
	signature := videoLikeIndexSignature(hash)
	for _, stored := range s.hashes {
		if videoLikeIndexSignature(stored) == signature {
			return stored.ID, false, nil
		}
	}
	s.nextID++
	hash.ID = s.nextID
	s.hashes = append(s.hashes, hash)
	return hash.ID, true, nil
}

func (s *memoryVideoLikeStore) close() error {
	return nil
}

func applyPreparedBlock(ctx context.Context, matcher *matcher, chatID int64, content domain.Content) error {
	block, err := matcher.PrepareBlock(ctx, chatID, content)
	if err != nil {
		return err
	}

	return matcher.ApplyBlock(ctx, block)
}

func defaultTestPlan() domain.MediaExtractionPlan {
	return domain.MediaExtractionPlan{
		MaxFrames:    3,
		TargetWidth:  64,
		TargetHeight: 64,
	}
}

func defaultTestLimits() Limits {
	return Limits{
		MaxAnimationDuration:    10 * time.Second,
		MaxVideoStickerDuration: 3 * time.Second,
		MaxAnimationSize:        20 << 20,
		MaxVideoStickerSize:     256 << 10,
	}
}

func testExtractedVideoLikeMedia() domain.ExtractedMedia {
	return domain.ExtractedMedia{
		Frames: []domain.ExtractedFrame{
			{Index: 0, PositionMillis: 0, Image: testSolidImage(color.RGBA{R: 255, A: 255})},
			{Index: 1, PositionMillis: 1000, Image: testSolidImage(color.RGBA{G: 255, A: 255})},
			{Index: 2, PositionMillis: 2000, Image: testSolidImage(color.RGBA{B: 255, A: 255})},
		},
	}
}

func testSolidImage(fill color.Color) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < img.Bounds().Dy(); y++ {
		for x := 0; x < img.Bounds().Dx(); x++ {
			img.Set(x, y, fill)
		}
	}

	return img
}

func testPatternImage(seed int) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < img.Bounds().Dy(); y++ {
		for x := 0; x < img.Bounds().Dx(); x++ {
			v := uint8((x*seed*17 + y*seed*31) % 255)
			img.Set(x, y, color.RGBA{R: v, G: 255 - v, B: uint8((x + y + seed) % 255), A: 255})
		}
	}

	return img
}
