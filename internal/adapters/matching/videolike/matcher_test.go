package videolike

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/domain"
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

func TestSupportsAcceptsAnimationWithoutDownload(t *testing.T) {
	downloader := &fakeDownloader{}
	matcher := newTestMatcher(t, downloader)

	supported, err := matcher.supports(context.Background(), domain.Content{
		FileID:       "animation-file",
		FileUniqueID: "animation-unique",
		Type:         domain.MediaAnimation,
	})
	if err != nil {
		t.Fatalf("supports animation: %v", err)
	}
	if !supported {
		t.Fatal("expected animation to be supported")
	}
	if downloader.calls != 0 {
		t.Fatalf("download calls = %d, want 0", downloader.calls)
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
		t.Fatalf("supports webm sticker: %v", err)
	}
	if !supported {
		t.Fatal("expected webm sticker to be supported")
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
		t.Fatalf("supports static sticker: %v", err)
	}
	if supported {
		t.Fatal("expected static webp sticker to be rejected")
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
		t.Fatalf("supports static sticker: %v", err)
	}
	if supported {
		t.Fatal("expected static webp sticker to be rejected")
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
		t.Fatalf("supports animated sticker: %v", err)
	}
	if supported {
		t.Fatal("expected animated sticker to be rejected")
	}
	if downloader.calls != 0 {
		t.Fatalf("download calls = %d, want 0", downloader.calls)
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
			t.Fatalf("supports %s: %v", mediaType, err)
		}
		if supported {
			t.Fatalf("expected %s to be rejected", mediaType)
		}
	}

	if downloader.calls != 0 {
		t.Fatalf("download calls = %d, want 0", downloader.calls)
	}
}

func TestSupportsReturnsDownloaderErrorForStickerScopeDetection(t *testing.T) {
	downloadErr := errors.New("download failed")
	matcher := newTestMatcher(t, &fakeDownloader{err: downloadErr})

	_, err := matcher.supports(context.Background(), domain.Content{
		FileID:       "sticker-file",
		FileUniqueID: "sticker-unique",
		Type:         domain.MediaStickerStatic,
	})
	if !errors.Is(err, downloadErr) {
		t.Fatalf("supports error = %v, want %v", err, downloadErr)
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
		t.Fatal("expected animation duration limit error")
	}
	if downloader.calls != 0 {
		t.Fatalf("download calls = %d, want 0", downloader.calls)
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
		t.Fatal("expected animation size limit error")
	}
	if downloader.calls != 0 {
		t.Fatalf("download calls = %d, want 0", downloader.calls)
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
		t.Fatal("expected video sticker duration limit error")
	}
	if downloader.calls != 0 {
		t.Fatalf("download calls = %d, want 0", downloader.calls)
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
		t.Fatal("expected video sticker size limit error")
	}
	if downloader.calls != 1 {
		t.Fatalf("download calls = %d, want 1", downloader.calls)
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
		t.Fatal("expected downloaded size limit error")
	}
	if downloader.calls != 1 {
		t.Fatalf("download calls = %d, want 1", downloader.calls)
	}
}

func TestNewMatcherRejectsNilDownloader(t *testing.T) {
	_, err := NewMatcher(nil, defaultTestLimits())
	if err == nil {
		t.Fatal("expected nil downloader to be rejected")
	}
}

func TestNewMatcherRejectsInvalidLimits(t *testing.T) {
	_, err := NewMatcher(&fakeDownloader{}, Limits{})
	if err == nil {
		t.Fatal("expected invalid limits to be rejected")
	}
}

func newTestMatcher(t *testing.T, downloader *fakeDownloader) *matcher {
	t.Helper()

	return newTestMatcherWithLimits(t, downloader, defaultTestLimits())
}

func newTestMatcherWithLimits(t *testing.T, downloader *fakeDownloader, limits Limits) *matcher {
	t.Helper()

	contentMatcher, err := NewMatcher(downloader, limits)
	if err != nil {
		t.Fatalf("new matcher: %v", err)
	}

	matcher, ok := contentMatcher.(*matcher)
	if !ok {
		t.Fatalf("matcher type = %T, want *matcher", contentMatcher)
	}

	return matcher
}

func defaultTestLimits() Limits {
	return Limits{
		MaxAnimationDuration:    10 * time.Second,
		MaxVideoStickerDuration: 3 * time.Second,
		MaxAnimationSize:        20 << 20,
		MaxVideoStickerSize:     256 << 10,
		FFmpegTimeout:           10 * time.Second,
	}
}
