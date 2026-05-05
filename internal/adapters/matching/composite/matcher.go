package composite

import (
	"context"
	"errors"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/app/ports"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/domain"
)

type matcher struct {
	matchers []ports.ContentMatcher
}

func NewMatcher(matchers ...ports.ContentMatcher) (ports.ContentMatcher, error) {
	if len(matchers) == 0 {
		return nil, errors.New("composite matcher requires at least one matcher")
	}
	for _, matcher := range matchers {
		if matcher == nil {
			return nil, errors.New("composite matcher contains nil matcher")
		}
	}

	return &matcher{
		matchers: matchers,
	}, nil
}

func (m *matcher) Block(ctx context.Context, content domain.Content) error {
	for _, matcher := range m.matchers {
		if err := matcher.Block(ctx, content); err != nil {
			return err
		}
	}

	return nil
}

func (m *matcher) IsBlocked(ctx context.Context, content domain.Content) (bool, error) {
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
