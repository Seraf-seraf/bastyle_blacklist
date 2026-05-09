package aivector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/pkg/apperrors"
)

type Client interface {
	Ban(ctx context.Context, request BanRequest) (BanResponse, error)
	Search(ctx context.Context, request SearchRequest) (SearchResponse, error)
}

type BanRequest struct {
	FileUniqueID string
	MediaType    string
	Frames       []FrameFile
}

type BanResponse struct {
	BanID int `json:"ban_id"`
}

type SearchRequest struct {
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

type FrameFile struct {
	Name string
	Data []byte
}

type HTTPClient struct {
	baseURL    string
	httpClient *http.Client
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

func (c *HTTPClient) Ban(ctx context.Context, request BanRequest) (BanResponse, error) {
	const methodCtx = "aivector/HTTPClient.Ban"

	fields := map[string]string{
		"file_unique_id": request.FileUniqueID,
		"media_type":     request.MediaType,
	}

	httpRequest, err := newMultipartRequest(ctx, http.MethodPost, c.baseURL+"/ban", fields, request.Frames)
	if err != nil {
		return BanResponse{}, apperrors.Wrap(methodCtx, err)
	}

	var response BanResponse
	if err := c.do(httpRequest, &response); err != nil {
		return BanResponse{}, apperrors.Wrap(methodCtx, err)
	}

	return response, nil
}

func (c *HTTPClient) Search(ctx context.Context, request SearchRequest) (SearchResponse, error) {
	const methodCtx = "aivector/HTTPClient.Search"

	if request.TopK <= 0 {
		return SearchResponse{}, apperrors.New(methodCtx, "AI-vector top_k должен быть положительным")
	}

	endpoint := c.baseURL + "/search?top_k=" + strconv.Itoa(request.TopK)
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

func (c *HTTPClient) do(request *http.Request, response any) error {
	const methodCtx = "aivector/HTTPClient.do"

	httpResponse, err := c.httpClient.Do(request)
	if err != nil {
		return apperrors.Wrap(methodCtx, err)
	}
	defer httpResponse.Body.Close()

	if httpResponse.StatusCode < 200 || httpResponse.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(httpResponse.Body, 4096))
		return apperrors.New(methodCtx, fmt.Sprintf("AI-vector сервис вернул %s: %s", httpResponse.Status, strings.TrimSpace(string(body))))
	}

	if err := json.NewDecoder(httpResponse.Body).Decode(response); err != nil {
		return apperrors.Wrap(methodCtx, err)
	}

	return nil
}

func newMultipartRequest(ctx context.Context, method string, endpoint string, fields map[string]string, frames []FrameFile) (*http.Request, error) {
	const methodCtx = "aivector/newMultipartRequest"

	if len(frames) == 0 {
		return nil, apperrors.New(methodCtx, "запрос AI-vector требует хотя бы один кадр")
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	for name, value := range fields {
		if err := writer.WriteField(name, value); err != nil {
			return nil, apperrors.Wrap(methodCtx, err)
		}
	}

	for _, frame := range frames {
		if frame.Name == "" {
			return nil, apperrors.New(methodCtx, "имя кадра AI-vector пустое")
		}
		if len(frame.Data) == 0 {
			return nil, apperrors.New(methodCtx, "данные кадра AI-vector пустые")
		}

		part, err := writer.CreateFormFile("files", frame.Name)
		if err != nil {
			return nil, apperrors.Wrap(methodCtx, err)
		}
		if _, err := part.Write(frame.Data); err != nil {
			return nil, apperrors.Wrap(methodCtx, err)
		}
	}

	if err := writer.Close(); err != nil {
		return nil, apperrors.Wrap(methodCtx, err)
	}

	request, err := http.NewRequestWithContext(ctx, method, endpoint, &body)
	if err != nil {
		return nil, apperrors.Wrap(methodCtx, err)
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())

	return request, nil
}
