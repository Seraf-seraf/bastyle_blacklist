package telegram

import (
	"context"
	"io"
	"net/http"
	"time"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/app/ports"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/domain"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/pkg/apperrors"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const maxDownloadBytes = 20 << 20

type fileDownloader struct {
	bot        *tgbotapi.BotAPI
	httpClient *http.Client
}

func NewFileDownloader(bot *tgbotapi.BotAPI) (ports.MediaDownloader, error) {
	const methodCtx = "telegram/NewFileDownloader"

	if bot == nil {
		return nil, apperrors.New(methodCtx, "Telegram-бот для загрузки файлов не настроен")
	}

	return &fileDownloader{
		bot: bot,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

func (d *fileDownloader) Download(ctx context.Context, content domain.Content) (domain.MediaFile, error) {
	const methodCtx = "telegram/fileDownloader.Download"

	if content.SizeBytes > maxDownloadBytes {
		return domain.MediaFile{}, apperrors.New(methodCtx, "загрузка файла: медиафайл слишком большой")
	}

	file, err := d.bot.GetFile(tgbotapi.FileConfig{FileID: content.FileID})
	if err != nil {
		return domain.MediaFile{}, apperrors.Wrap(methodCtx, err)
	}
	if file.FileSize > maxDownloadBytes {
		return domain.MediaFile{}, apperrors.New(methodCtx, "загрузка файла: медиафайл слишком большой")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, file.Link(d.bot.Token), nil)
	if err != nil {
		return domain.MediaFile{}, apperrors.Wrap(methodCtx, err)
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return domain.MediaFile{}, apperrors.Wrap(methodCtx, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return domain.MediaFile{}, apperrors.New(methodCtx, "загрузка файла: неожиданный статус ответа")
	}

	if resp.ContentLength > maxDownloadBytes {
		return domain.MediaFile{}, apperrors.New(methodCtx, "загрузка файла: ответ слишком большой")
	}

	limitedReader := io.LimitReader(resp.Body, maxDownloadBytes+1)
	data, err := io.ReadAll(limitedReader)
	if err != nil {
		return domain.MediaFile{}, apperrors.Wrap(methodCtx, err)
	}
	if len(data) > maxDownloadBytes {
		return domain.MediaFile{}, apperrors.New(methodCtx, "загрузка файла: ответ превысил ограничение размера")
	}

	return domain.MediaFile{
		Content:  content,
		FilePath: file.FilePath,
		Data:     data,
	}, nil
}
