package aivector

import (
	"context"
	"errors"
	"image"
	"testing"
	"time"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/domain"
)

func TestMatcherBlockSendsFramesToClient(t *testing.T) {
	client := &fakeClient{}
	matcher := newTestMatcher(t, client)
	content := testPhotoContent()

	if err := matcher.Block(context.Background(), 10, content); err != nil {
		t.Fatal(err)
	}

	if len(client.banRequests) != 1 {
		t.Fatalf("запросы бана = %d, ожидалось 1", len(client.banRequests))
	}
	request := client.banRequests[0]
	if request.ChatID != 10 {
		t.Fatalf("chat_id = %d, ожидалось 10", request.ChatID)
	}
	if request.FileUniqueID != content.FileUniqueID {
		t.Fatalf("file_unique_id = %q, ожидалось %q", request.FileUniqueID, content.FileUniqueID)
	}
	if request.MediaType != string(content.Type) {
		t.Fatalf("тип медиа = %q, ожидалось %q", request.MediaType, content.Type)
	}
	if len(request.Frames) != 1 {
		t.Fatalf("кадры = %d, ожидалось 1", len(request.Frames))
	}
	if len(request.Frames[0].Data) == 0 {
		t.Fatal("ожидалось: закодированные данные кадра")
	}
}

func TestMatcherIsBlockedReturnsStaticMatch(t *testing.T) {
	client := &fakeClient{
		searchResponse: SearchResponse{
			Frames: []FrameSearchResponse{
				{FrameIndex: 0, Hits: []VectorSearchHit{{BanID: 10, Score: 0.95}}},
			},
		},
	}
	matcher := newTestMatcher(t, client)

	blocked, err := matcher.IsBlocked(context.Background(), 10, testPhotoContent())
	if err != nil {
		t.Fatal(err)
	}

	if !blocked {
		t.Fatal("ожидалось: совпадение статичного медиа")
	}
}

func TestMatcherIsBlockedAppliesVideoLikeRule(t *testing.T) {
	client := &fakeClient{
		searchResponse: SearchResponse{
			Frames: []FrameSearchResponse{
				{FrameIndex: 0, Hits: []VectorSearchHit{{BanID: 10, Score: 0.95}}},
				{FrameIndex: 1, Hits: []VectorSearchHit{{BanID: 10, Score: 0.94}}},
				{FrameIndex: 2, Hits: []VectorSearchHit{{BanID: 11, Score: 0.96}}},
			},
		},
	}
	matcher := newTestMatcher(t, client)
	matcher.videoExtractor = fakeExtractor{frames: 3}

	blocked, err := matcher.IsBlocked(context.Background(), 10, domain.Content{
		FileID:       "animation-file-id",
		FileUniqueID: "animation-file-unique-id",
		Type:         domain.MediaAnimation,
		SizeBytes:    1024,
		DurationSec:  3,
	})
	if err != nil {
		t.Fatal(err)
	}

	if !blocked {
		t.Fatal("ожидалось: video-like совпадение")
	}
}

func TestMatcherSkipsUnsupportedContent(t *testing.T) {
	client := &fakeClient{}
	matcher := newTestMatcher(t, client)

	blocked, err := matcher.IsBlocked(context.Background(), 10, domain.Content{
		FileID:       "file-id",
		FileUniqueID: "file-unique-id",
		Type:         domain.MediaStickerAnimated,
	})
	if err != nil {
		t.Fatal(err)
	}

	if blocked {
		t.Fatal("ожидалось: неподдерживаемый контент должен быть пропущен")
	}
	if len(client.searchRequests) != 0 {
		t.Fatalf("поисковые запросы = %d, ожидалось 0", len(client.searchRequests))
	}
}

func TestMatcherReturnsClientError(t *testing.T) {
	expectedErr := errors.New("ошибка клиента")
	client := &fakeClient{searchErr: expectedErr}
	matcher := newTestMatcher(t, client)

	_, err := matcher.IsBlocked(context.Background(), 10, testPhotoContent())
	if !errors.Is(err, expectedErr) {
		t.Fatalf("ошибка = %v, ожидалось %v", err, expectedErr)
	}
}

func newTestMatcher(t *testing.T, client *fakeClient) *Matcher {
	t.Helper()

	matcher, err := NewMatcher(Options{
		Downloader:     fakeDownloader{},
		ImageExtractor: fakeExtractor{frames: 1},
		VideoExtractor: fakeExtractor{frames: 2},
		Client:         client,
		Threshold:      0.92,
		TopK:           5,
		Plan: domain.MediaExtractionPlan{
			MaxFrames:    10,
			TargetWidth:  320,
			TargetHeight: 320,
		},
		Limits: Limits{
			MaxAnimationDuration:    10 * time.Second,
			MaxVideoStickerDuration: 3 * time.Second,
			MaxAnimationSize:        20 << 20,
			MaxVideoStickerSize:     256 << 10,
		},
		Rule: MatchRule{
			MinMatchedFrames: 2,
			MinMatchedRatio:  0.4,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	return matcher
}

func testPhotoContent() domain.Content {
	return domain.Content{
		FileID:       "file-id",
		FileUniqueID: "file-unique-id",
		Type:         domain.MediaPhoto,
	}
}

type fakeClient struct {
	banRequests    []BanRequest
	searchRequests []SearchRequest
	searchResponse SearchResponse
	banErr         error
	searchErr      error
}

func (c *fakeClient) Ban(_ context.Context, request BanRequest) (BanResponse, error) {
	c.banRequests = append(c.banRequests, request)
	if c.banErr != nil {
		return BanResponse{}, c.banErr
	}

	return BanResponse{BanID: 1}, nil
}

func (c *fakeClient) Search(_ context.Context, request SearchRequest) (SearchResponse, error) {
	c.searchRequests = append(c.searchRequests, request)
	if c.searchErr != nil {
		return SearchResponse{}, c.searchErr
	}

	return c.searchResponse, nil
}

type fakeDownloader struct{}

func (fakeDownloader) Download(_ context.Context, content domain.Content) (domain.MediaFile, error) {
	return domain.MediaFile{
		Content:  content,
		FilePath: "media.png",
		Data:     []byte("media"),
	}, nil
}

type fakeExtractor struct {
	frames int
}

func (e fakeExtractor) Extract(_ context.Context, media domain.MediaFile, _ domain.MediaExtractionPlan) (domain.ExtractedMedia, error) {
	frames := make([]domain.ExtractedFrame, 0, e.frames)
	for i := 0; i < e.frames; i++ {
		frames = append(frames, domain.ExtractedFrame{
			Index:          i,
			PositionMillis: i * 1000,
			Image:          image.NewRGBA(image.Rect(0, 0, 8, 8)),
		})
	}

	return domain.ExtractedMedia{Frames: frames}, nil
}
