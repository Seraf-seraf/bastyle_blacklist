package aivector

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/pkg/apperrors"
)

type HTTPClient struct {
	baseURL    string
	httpClient *http.Client
}

type EmbedRequest struct {
	Frames []FrameFile
}

type EmbedResponse struct {
	ModelName string           `json:"model_name"`
	Dimension int              `json:"dimension"`
	Frames    []FrameEmbedding `json:"frames"`
}

type FrameEmbedding struct {
	FrameIndex int       `json:"frame_index"`
	Vector     []float32 `json:"vector"`
}

type SearchRequest struct {
	ChatID int64
	TopK   int
	Frames []FrameFile
}

type SearchResponse struct {
	ModelName string                `json:"model_name"`
	Dimension int                   `json:"dimension"`
	Frames    []FrameSearchResponse `json:"frames"`
}

type FrameSearchResponse struct {
	FrameIndex int               `json:"frame_index"`
	Hits       []VectorSearchHit `json:"hits"`
}

type VectorSearchHit struct {
	BanID      int     `json:"ban_id"`
	FrameIndex int     `json:"frame_index"`
	Score      float64 `json:"score"`
}

func NewHTTPClient(baseURL string, timeout time.Duration) (*HTTPClient, error) {
	const methodCtx = "aivector/NewHTTPClient"

	if baseURL == "" {
		return nil, apperrors.New(methodCtx, "URL AI-vector сервиса не настроен")
	}
	if timeout <= 0 {
		return nil, apperrors.New(methodCtx, "таймаут запроса к AI-vector должен быть положительным")
	}

	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, apperrors.Wrap(methodCtx, err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, apperrors.New(methodCtx, "URL AI-vector сервиса должен содержать схему и хост")
	}

	return &HTTPClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}, nil
}

func (c *HTTPClient) Embed(ctx context.Context, request EmbedRequest) (EmbedResponse, error) {
	const methodCtx = "aivector/HTTPClient.Embed"

	httpRequest, err := newMultipartRequest(ctx, http.MethodPost, c.baseURL+"/embed", nil, request.Frames)
	if err != nil {
		return EmbedResponse{}, apperrors.Wrap(methodCtx, err)
	}

	var response EmbedResponse
	if err := c.do(httpRequest, &response); err != nil {
		return EmbedResponse{}, apperrors.Wrap(methodCtx, err)
	}

	return response, nil
}

func (c *HTTPClient) Search(ctx context.Context, request SearchRequest) (SearchResponse, error) {
	const methodCtx = "aivector/HTTPClient.Search"

	if request.TopK <= 0 {
		return SearchResponse{}, apperrors.New(methodCtx, "AI-vector top_k должен быть положительным")
	}

	endpoint := c.baseURL + "/search?top_k=" + strconv.Itoa(request.TopK) + "&chat_id=" + strconv.FormatInt(request.ChatID, 10)
	httpRequest, err := newMultipartRequest(ctx, http.MethodPost, endpoint, nil, request.Frames)
	if err != nil {
		return SearchResponse{}, apperrors.Wrap(methodCtx, err)
	}

	var response SearchResponse
	if err := c.do(httpRequest, &response); err != nil {
		return SearchResponse{}, apperrors.Wrap(methodCtx, err)
	}

	return response, nil
}

var _ Client = (*HTTPClient)(nil)
