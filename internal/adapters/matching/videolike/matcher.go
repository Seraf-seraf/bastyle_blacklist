package videolike

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"time"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/app/ports"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/domain"
)

type matcher struct {
	downloader ports.MediaDownloader
	limits     Limits
}

type Limits struct {
	MaxAnimationDuration    time.Duration
	MaxVideoStickerDuration time.Duration
	MaxAnimationSize        int64
	MaxVideoStickerSize     int64
	FFmpegTimeout           time.Duration
}

func NewMatcher(downloader ports.MediaDownloader, limits Limits) (ports.ContentMatcher, error) {
	if downloader == nil {
		return nil, errors.New("videolike matcher downloader is not configured")
	}
	if err := limits.validate(); err != nil {
		return nil, err
	}

	return &matcher{
		downloader: downloader,
		limits:     limits,
	}, nil
}

func (m *matcher) IsBlocked(ctx context.Context, content domain.Content) (bool, error) {
	supported, err := m.supports(ctx, content)
	if err != nil {
		return false, err
	}
	if !supported {
		return false, nil
	}

	return false, nil
}

func (m *matcher) Block(ctx context.Context, content domain.Content) error {
	_, err := m.supports(ctx, content)
	return err
}

func (m *matcher) supports(ctx context.Context, content domain.Content) (bool, error) {
	if !content.CanDownload() {
		return false, nil
	}

	switch content.Type {
	case domain.MediaAnimation:
		if err := m.checkAnimationMetadata(content); err != nil {
			return false, err
		}

		return true, nil
	case domain.MediaStickerStatic:
		if err := m.checkVideoStickerMetadata(content); err != nil {
			return false, err
		}

		media, err := m.downloader.Download(ctx, content)
		if err != nil {
			return false, err
		}
		if !isWebMFile(media.FilePath) {
			return false, nil
		}

		if err := m.checkVideoStickerFile(media); err != nil {
			return false, err
		}

		return true, nil
	default:
		return false, nil
	}
}

func (m *matcher) checkAnimationMetadata(content domain.Content) error {
	if contentDuration(content) > m.limits.MaxAnimationDuration {
		return errors.New("videolike matcher: animation duration exceeds limit")
	}
	if content.SizeBytes > m.limits.MaxAnimationSize {
		return errors.New("videolike matcher: animation size exceeds limit")
	}

	return nil
}

func (m *matcher) checkVideoStickerMetadata(content domain.Content) error {
	if contentDuration(content) > m.limits.MaxVideoStickerDuration {
		return errors.New("videolike matcher: video sticker duration exceeds limit")
	}

	return nil
}

func (m *matcher) checkVideoStickerFile(media domain.MediaFile) error {
	if media.Content.SizeBytes > m.limits.MaxVideoStickerSize {
		return errors.New("videolike matcher: video sticker size exceeds limit")
	}
	if int64(len(media.Data)) > m.limits.MaxVideoStickerSize {
		return errors.New("videolike matcher: video sticker downloaded size exceeds limit")
	}

	return nil
}

func (l Limits) validate() error {
	if l.MaxAnimationDuration <= 0 {
		return errors.New("videolike matcher max animation duration must be positive")
	}
	if l.MaxVideoStickerDuration <= 0 {
		return errors.New("videolike matcher max video sticker duration must be positive")
	}
	if l.MaxAnimationSize <= 0 {
		return errors.New("videolike matcher max animation size must be positive")
	}
	if l.MaxVideoStickerSize <= 0 {
		return errors.New("videolike matcher max video sticker size must be positive")
	}
	if l.FFmpegTimeout <= 0 {
		return errors.New("videolike matcher ffmpeg timeout must be positive")
	}

	return nil
}

func contentDuration(content domain.Content) time.Duration {
	return time.Duration(content.DurationSec) * time.Second
}

func isWebMFile(filePath string) bool {
	return strings.EqualFold(filepath.Ext(filePath), ".webm")
}
