package exact

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/domain"
)

func TestNewSQLiteMatcherRejectsNegativeBuffer(t *testing.T) {
	_, err := newMatcher(-1)
	if err == nil {
		t.Fatal("ожидалось, что отрицательный буфер будет отклонен")
	}
}

func TestMatcherScopesBlockedFilesByChat(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "exact.sqlite")

	matcher, err := NewSQLiteMatcher(ctx, 1, dbPath)
	if err != nil {
		t.Fatalf("создание matcher: %v", err)
	}
	defer matcher.Close()
	content := domain.Content{FileUniqueID: "file-unique-id"}

	if err := matcher.Block(ctx, 10, content); err != nil {
		t.Fatalf("блокировка: %v", err)
	}

	blocked, err := matcher.IsBlocked(ctx, 10, content)
	if err != nil {
		t.Fatalf("проверка исходного чата: %v", err)
	}
	if !blocked {
		t.Fatal("ожидалось: файл заблокирован в исходном чате")
	}

	blocked, err = matcher.IsBlocked(ctx, 20, content)
	if err != nil {
		t.Fatalf("проверка другого чата: %v", err)
	}
	if blocked {
		t.Fatal("не ожидалась блокировка в другом чате")
	}
}

func TestMatcherRestoresStateAfterRestart(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "exact.sqlite")
	content := domain.Content{FileUniqueID: "persist-me"}

	matcher, err := NewSQLiteMatcher(ctx, 1, dbPath)
	if err != nil {
		t.Fatalf("создание matcher: %v", err)
	}
	if err := matcher.Block(ctx, 77, content); err != nil {
		t.Fatalf("блокировка: %v", err)
	}
	_ = matcher.Close()

	reloaded, err := NewSQLiteMatcher(ctx, 1, dbPath)
	if err != nil {
		t.Fatalf("повторное создание matcher: %v", err)
	}
	defer reloaded.Close()

	blocked, err := reloaded.IsBlocked(ctx, 77, content)
	if err != nil {
		t.Fatalf("проверка после перезагрузки: %v", err)
	}
	if !blocked {
		t.Fatal("ожидалось восстановление состояния после перезагрузки")
	}
}
