package telegram

import (
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/domain"
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

func (d *FileDownloader) Download(ctx context.Context, content domain.Content) (domain.MediaFile, error) {
	url, err := d.bot.GetFileDirectURL(content.FileID)
	if err != nil {
		return domain.MediaFile{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return domain.MediaFile{}, err
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return domain.MediaFile{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return domain.MediaFile{}, errors.New("download file: unexpected status")
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return domain.MediaFile{}, err
	}

	return domain.MediaFile{
		Content: content,
		Data:    data,
	}, nil
}
