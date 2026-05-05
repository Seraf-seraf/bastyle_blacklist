package videolike

import (
	"context"
	"errors"
	"testing"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/domain"
)

type fakeDownloader struct {
	filePaths map[string]string
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

func TestNewMatcherRejectsNilDownloader(t *testing.T) {
	_, err := NewMatcher(nil)
	if err == nil {
		t.Fatal("expected nil downloader to be rejected")
	}
}

func newTestMatcher(t *testing.T, downloader *fakeDownloader) *matcher {
	t.Helper()

	contentMatcher, err := NewMatcher(downloader)
	if err != nil {
		t.Fatalf("new matcher: %v", err)
	}

	matcher, ok := contentMatcher.(*matcher)
	if !ok {
		t.Fatalf("matcher type = %T, want *matcher", contentMatcher)
	}

	return matcher
}
