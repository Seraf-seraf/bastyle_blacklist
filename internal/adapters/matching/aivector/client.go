package aivector

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
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
	if baseURL == "" {
		return nil, errors.New("ai vector service url is not configured")
	}
	if timeout <= 0 {
		return nil, errors.New("ai vector request timeout must be positive")
	}

	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, errors.New("ai vector service url must include scheme and host")
	}

	return &HTTPClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}, nil
}

func (c *HTTPClient) Ban(ctx context.Context, request BanRequest) (BanResponse, error) {
	fields := map[string]string{
		"file_unique_id": request.FileUniqueID,
		"media_type":     request.MediaType,
	}

	httpRequest, err := newMultipartRequest(ctx, http.MethodPost, c.baseURL+"/ban", fields, request.Frames)
	if err != nil {
		return BanResponse{}, err
	}

	var response BanResponse
	if err := c.do(httpRequest, &response); err != nil {
		return BanResponse{}, err
	}

	return response, nil
}

func (c *HTTPClient) Search(ctx context.Context, request SearchRequest) (SearchResponse, error) {
	if request.TopK <= 0 {
		return SearchResponse{}, errors.New("ai vector top k must be positive")
	}

	endpoint := c.baseURL + "/search?top_k=" + strconv.Itoa(request.TopK)
	httpRequest, err := newMultipartRequest(ctx, http.MethodPost, endpoint, nil, request.Frames)
	if err != nil {
		return SearchResponse{}, err
	}

	var response SearchResponse
	if err := c.do(httpRequest, &response); err != nil {
		return SearchResponse{}, err
	}

	return response, nil
}

func (c *HTTPClient) do(request *http.Request, response any) error {
	httpResponse, err := c.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer httpResponse.Body.Close()

	if httpResponse.StatusCode < 200 || httpResponse.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(httpResponse.Body, 4096))
		return fmt.Errorf("ai vector service returned %s: %s", httpResponse.Status, strings.TrimSpace(string(body)))
	}

	if err := json.NewDecoder(httpResponse.Body).Decode(response); err != nil {
		return err
	}

	return nil
}

func newMultipartRequest(ctx context.Context, method string, endpoint string, fields map[string]string, frames []FrameFile) (*http.Request, error) {
	if len(frames) == 0 {
		return nil, errors.New("ai vector request requires at least one frame")
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	for name, value := range fields {
		if err := writer.WriteField(name, value); err != nil {
			return nil, err
		}
	}

	for _, frame := range frames {
		if frame.Name == "" {
			return nil, errors.New("ai vector frame name is empty")
		}
		if len(frame.Data) == 0 {
			return nil, errors.New("ai vector frame data is empty")
		}

		part, err := writer.CreateFormFile("files", frame.Name)
		if err != nil {
			return nil, err
		}
		if _, err := part.Write(frame.Data); err != nil {
			return nil, err
		}
	}

	if err := writer.Close(); err != nil {
		return nil, err
	}

	request, err := http.NewRequestWithContext(ctx, method, endpoint, &body)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())

	return request, nil
}
