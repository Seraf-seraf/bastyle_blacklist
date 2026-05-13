package composite

import (
	"context"
	"testing"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/domain"
)

type fakeMatcher struct{}

func (fakeMatcher) Block(context.Context, int64, domain.Content) error {
	return nil
}

func (fakeMatcher) IsBlocked(context.Context, int64, domain.Content) (bool, error) {
	return false, nil
}

func TestNewMatcherRejectsEmptyList(t *testing.T) {
	_, err := NewMatcher()
	if err == nil {
		t.Fatal("ожидалась ошибка для пустого списка matcher-ов")
	}
}

func TestNewMatcherRejectsNilMatcher(t *testing.T) {
	_, err := NewMatcher(fakeMatcher{}, nil)
	if err == nil {
		t.Fatal("ожидалось, что матчер равен nil будет отклонен")
	}
}
