package composite

import (
	"context"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/app/ports"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/domain"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/pkg/apperrors"
)

type matcher struct {
	matchers []ports.ContentMatcher
}

func NewMatcher(matchers ...ports.ContentMatcher) (ports.ContentMatcher, error) {
	const methodCtx = "composite/NewMatcher"

	if len(matchers) == 0 {
		return nil, apperrors.New(methodCtx, "composite-матчер требует хотя бы один матчер")
	}
	for _, matcher := range matchers {
		if matcher == nil {
			return nil, apperrors.New(methodCtx, "composite-матчер содержит nil-матчер")
		}
	}

	return &matcher{
		matchers: matchers,
	}, nil
}

func (m *matcher) Block(ctx context.Context, chatID int64, content domain.Content) error {
	const methodCtx = "composite/matcher.Block"

	for _, matcher := range m.matchers {
		if err := matcher.Block(ctx, chatID, content); err != nil {
			return apperrors.Wrap(methodCtx, err)
		}
	}

	return nil
}

func (m *matcher) IsBlocked(ctx context.Context, chatID int64, content domain.Content) (bool, error) {
	const methodCtx = "composite/matcher.IsBlocked"

	for _, matcher := range m.matchers {
		blocked, err := matcher.IsBlocked(ctx, chatID, content)
		if err != nil {
			return false, apperrors.Wrap(methodCtx, err)
		}

		if blocked {
			return true, nil
		}
	}

	return false, nil
}
