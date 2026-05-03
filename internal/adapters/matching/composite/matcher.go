package composite

import (
	"context"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/app/ports"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/domain"
)

type Matcher struct {
	matchers []ports.ContentMatcher
}

func NewMatcher(matchers ...ports.ContentMatcher) *Matcher {
	return &Matcher{
		matchers: matchers,
	}
}

func (m *Matcher) Block(ctx context.Context, content domain.Content) error {
	for _, matcher := range m.matchers {
		if err := matcher.Block(ctx, content); err != nil {
			return err
		}
	}

	return nil
}

func (m *Matcher) IsBlocked(ctx context.Context, content domain.Content) (bool, error) {
	for _, matcher := range m.matchers {
		blocked, err := matcher.IsBlocked(ctx, content)
		if err != nil {
			return false, err
		}

		if blocked {
			return true, nil
		}
	}

	return false, nil
}
