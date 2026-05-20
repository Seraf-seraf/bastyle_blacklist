package aivector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/pkg/apperrors"
)

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
