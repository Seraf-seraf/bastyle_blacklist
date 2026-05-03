package ports

import (
	"context"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/domain"
)

type ContentMatcher interface {
	Block(ctx context.Context, content domain.Content) error
	IsBlocked(ctx context.Context, content domain.Content) (bool, error)
}

type MediaDownloader interface {
	Download(ctx context.Context, fileID string) ([]byte, error)
}
