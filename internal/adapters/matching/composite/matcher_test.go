package composite

import (
	"context"
	"testing"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/domain"
)

type fakeMatcher struct{}

func (fakeMatcher) Block(context.Context, domain.Content) error {
	return nil
}

func (fakeMatcher) IsBlocked(context.Context, domain.Content) (bool, error) {
	return false, nil
}

func TestNewMatcherRejectsEmptyList(t *testing.T) {
	_, err := NewMatcher()
	if err == nil {
		t.Fatal("expected empty matcher list to be rejected")
	}
}

func TestNewMatcherRejectsNilMatcher(t *testing.T) {
	_, err := NewMatcher(fakeMatcher{}, nil)
	if err == nil {
		t.Fatal("expected nil matcher to be rejected")
	}
}
