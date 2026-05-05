package videolike

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/app/ports"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/domain"
)

type matcher struct {
	downloader ports.MediaDownloader
}

func NewMatcher(downloader ports.MediaDownloader) (ports.ContentMatcher, error) {
	if downloader == nil {
		return nil, errors.New("videolike matcher downloader is not configured")
	}

	return &matcher{
		downloader: downloader,
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
		return true, nil
	case domain.MediaStickerStatic:
		media, err := m.downloader.Download(ctx, content)
		if err != nil {
			return false, err
		}

		return isWebMFile(media.FilePath), nil
	default:
		return false, nil
	}
}

func isWebMFile(filePath string) bool {
	return strings.EqualFold(filepath.Ext(filePath), ".webm")
}
