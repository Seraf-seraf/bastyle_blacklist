package exact

import (
	"context"
	"testing"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/app/ports"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func TestNewMatcherRejectsNegativeBuffer(t *testing.T) {
	_, err := newMatcher(-1)
	if err == nil {
		t.Fatal("ожидалось, что отрицательный буфер будет отклонен")
	}
}

func TestMatcherScopesBlockedFilesByChat(t *testing.T) {
	ctx := context.Background()

	matcher, err := newMatcher(1)
	if err != nil {
		t.Fatalf("создание matcher: %v", err)
	}
	matcher.store = newMemoryExactStore(nil)
	defer matcher.Close()
	content := domain.Content{FileUniqueID: "file-unique-id"}
	matcher.blocked[exactKey{chatID: 10, fileUniqueID: content.FileUniqueID}] = struct{}{}

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
	content := domain.Content{FileUniqueID: "persist-me"}
	store := newMemoryExactStore([]exactRecord{
		{ChatID: 77, FileUniqueID: content.FileUniqueID},
	})

	reloaded, err := newMatcher(1)
	if err != nil {
		t.Fatalf("повторное создание matcher: %v", err)
	}
	reloaded.store = store
	stored, err := store.load(ctx)
	if err != nil {
		t.Fatalf("загрузка store: %v", err)
	}
	for _, item := range stored {
		reloaded.blocked[exactKey{chatID: item.ChatID, fileUniqueID: item.FileUniqueID}] = struct{}{}
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

type memoryExactStore struct {
	records []exactRecord
}

func newMemoryExactStore(records []exactRecord) *memoryExactStore {
	return &memoryExactStore{records: records}
}

func (s *memoryExactStore) load(context.Context) ([]exactRecord, error) {
	return append([]exactRecord(nil), s.records...), nil
}

func (s *memoryExactStore) LoadByBanUID(context.Context, uuid.UUID) (exactRecord, error) {
	return exactRecord{}, errExactArtifactNotFound
}

func (s *memoryExactStore) Insert(_ context.Context, _ pgx.Tx, _ uuid.UUID, chatID int64, fileUniqueID string) (ports.PersistBlockResult, error) {
	for _, record := range s.records {
		if record.ChatID == chatID && record.FileUniqueID == fileUniqueID {
			return ports.PersistBlockResult{}, nil
		}
	}

	s.records = append(s.records, exactRecord{ChatID: chatID, FileUniqueID: fileUniqueID})
	return ports.PersistBlockResult{Created: true}, nil
}

func (s *memoryExactStore) close() error {
	return nil
}
