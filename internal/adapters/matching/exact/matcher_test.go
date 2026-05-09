package exact

import (
	"context"
	"testing"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/domain"
)

func TestNewMatcherRejectsNegativeBuffer(t *testing.T) {
	_, err := NewMatcher(-1)
	if err == nil {
		t.Fatal("ожидалось, что отрицательный буфер будет отклонен")
	}
}

func TestMatcherScopesBlockedFilesByChat(t *testing.T) {
	matcher, err := NewMatcher(1)
	if err != nil {
		t.Fatalf("создание matcher: %v", err)
	}
	content := domain.Content{FileUniqueID: "file-unique-id"}

	if err := matcher.Block(context.Background(), 10, content); err != nil {
		t.Fatalf("блокировка: %v", err)
	}

	blocked, err := matcher.IsBlocked(context.Background(), 10, content)
	if err != nil {
		t.Fatalf("проверка исходного чата: %v", err)
	}
	if !blocked {
		t.Fatal("ожидалось: файл заблокирован в исходном чате")
	}

	blocked, err = matcher.IsBlocked(context.Background(), 20, content)
	if err != nil {
		t.Fatalf("проверка другого чата: %v", err)
	}
	if blocked {
		t.Fatal("не ожидалась блокировка в другом чате")
	}
}
