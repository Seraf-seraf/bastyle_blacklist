package telegram

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/app/ports"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/domain"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const maxDownloadBytes = 20 << 20

type fileDownloader struct {
	bot        *tgbotapi.BotAPI
	httpClient *http.Client
}

func NewFileDownloader(bot *tgbotapi.BotAPI) (ports.MediaDownloader, error) {
	if bot == nil {
		return nil, errors.New("telegram file downloader bot is not configured")
	}

	return &fileDownloader{
		bot: bot,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

func (d *fileDownloader) Download(ctx context.Context, content domain.Content) (domain.MediaFile, error) {
	if content.SizeBytes > maxDownloadBytes {
		return domain.MediaFile{}, errors.New("download file: media is too large")
	}

	file, err := d.bot.GetFile(tgbotapi.FileConfig{FileID: content.FileID})
	if err != nil {
		return domain.MediaFile{}, err
	}
	if file.FileSize > maxDownloadBytes {
		return domain.MediaFile{}, errors.New("download file: media is too large")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, file.Link(d.bot.Token), nil)
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

	if resp.ContentLength > maxDownloadBytes {
		return domain.MediaFile{}, errors.New("download file: response is too large")
	}

	limitedReader := io.LimitReader(resp.Body, maxDownloadBytes+1)
	data, err := io.ReadAll(limitedReader)
	if err != nil {
		return domain.MediaFile{}, err
	}
	if len(data) > maxDownloadBytes {
		return domain.MediaFile{}, errors.New("download file: response exceeded size limit")
	}

	return domain.MediaFile{
		Content:  content,
		FilePath: file.FilePath,
		Data:     data,
	}, nil
}
