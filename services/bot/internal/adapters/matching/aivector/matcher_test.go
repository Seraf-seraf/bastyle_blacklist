package aivector

import (
	"context"
	"errors"
	"image"
	"testing"
	"time"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/domain"
)

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
	matcher.contentFrames.videoFrameExtractor = fakeExtractor{frames: 3}

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

func newTestMatcher(t *testing.T, client *fakeClient) *matcher {
	t.Helper()

	matcher, err := NewMatcher(Options{
		Downloader:          fakeDownloader{},
		ImageFrameExtractor: fakeExtractor{frames: 1},
		VideoFrameExtractor: fakeExtractor{frames: 2},
		Client:              client,
		ModelName:           "test-model",
		ModelRevision:       "test-revision",
		Threshold:           0.92,
		TopK:                5,
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
	embedRequests  []EmbedRequest
	searchRequests []SearchRequest
	embedResponse  EmbedResponse
	searchResponse SearchResponse
	embedErr       error
	searchErr      error
}

func (c *fakeClient) Embed(_ context.Context, request EmbedRequest) (EmbedResponse, error) {
	c.embedRequests = append(c.embedRequests, request)
	if c.embedErr != nil {
		return EmbedResponse{}, c.embedErr
	}
	if c.embedResponse.Dimension > 0 {
		return c.embedResponse, nil
	}

	return EmbedResponse{
		ModelName: "test-model",
		Dimension: 3,
		Frames: []FrameEmbedding{
			{FrameIndex: 0, Vector: []float32{1, 0, 0}},
		},
	}, nil
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
