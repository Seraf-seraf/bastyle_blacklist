package exact

import (
	"context"
	"sync"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/app/ports"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/domain"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/pkg/apperrors"
	"github.com/jackc/pgx/v5/pgxpool"
)

type matcher struct {
	mu      sync.RWMutex
	blocked map[exactKey]struct{}
	store   exactStore
}

type exactKey struct {
	chatID       int64
	fileUniqueID string
}

type exactRecord struct {
	ChatID       int64
	FileUniqueID string
}

type exactStore interface {
	load(context.Context) ([]exactRecord, error)
	insert(context.Context, int64, domain.MediaType, string) error
	close() error
}

func newMatcher(buffer int) (*matcher, error) {
	const methodCtx = "exact/newMatcher"

	if buffer < 0 {
		return nil, apperrors.New(methodCtx, "буфер exact-матчера не должен быть отрицательным")
	}

	return &matcher{blocked: make(map[exactKey]struct{}, buffer)}, nil
}

func NewPostgresMatcher(ctx context.Context, pool *pgxpool.Pool, buffer int) (*matcher, error) {
	const methodCtx = "exact/NewPostgresMatcher"

	if pool == nil {
		return nil, apperrors.New(methodCtx, "PostgreSQL pool exact-матчера не настроен")
	}

	store := NewPostgresStore(pool)

	stored, err := store.load(ctx)
	if err != nil {
		return nil, apperrors.Wrap(methodCtx, err)
	}

	matcher, err := newMatcher(buffer + len(stored))
	if err != nil {
		return nil, apperrors.Wrap(methodCtx, err)
	}
	matcher.store = store
	for _, item := range stored {
		matcher.blocked[exactKey{chatID: item.ChatID, fileUniqueID: item.FileUniqueID}] = struct{}{}
	}

	return matcher, nil
}

func (m *matcher) Close() error {
	const methodCtx = "exact/matcher.Close"

	if m.store == nil {
		return nil
	}

	return apperrors.Wrap(methodCtx, m.store.close())
}

func (m *matcher) IsBlocked(_ context.Context, chatID int64, content domain.Content) (bool, error) {
	if content.FileUniqueID == "" {
		return false, nil
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	_, ok := m.blocked[exactKey{chatID: chatID, fileUniqueID: content.FileUniqueID}]
	return ok, nil
}

func (m *matcher) Block(ctx context.Context, chatID int64, content domain.Content) error {
	const methodCtx = "exact/matcher.Block"

	if content.FileUniqueID == "" {
		return nil
	}
	if m.store == nil {
		return apperrors.New(methodCtx, "хранилище exact не настроено")
	}

	if err := m.store.insert(ctx, chatID, content.Type, content.FileUniqueID); err != nil {
		return apperrors.Wrap(methodCtx, err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.blocked[exactKey{chatID: chatID, fileUniqueID: content.FileUniqueID}] = struct{}{}
	return nil
}

var _ ports.ContentMatcher = (*matcher)(nil)
