package telegram

import (
	"context"
	"fmt"
	"io"
	"net/http"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type FileDownloader struct {
	bot        *tgbotapi.BotAPI
	httpClient *http.Client
}

func NewFileDownloader(bot *tgbotapi.BotAPI) *FileDownloader {
	return &FileDownloader{
		bot:        bot,
		httpClient: http.DefaultClient,
	}
}

func (d *FileDownloader) Download(ctx context.Context, fileID string) ([]byte, error) {
	url, err := d.bot.GetFileDirectURL(fileID)
	if err != nil {
		return nil, fmt.Errorf("get file url: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create download request: %w", err)
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("download file: unexpected status %s", resp.Status)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read downloaded file: %w", err)
	}

	return data, nil
}
