package exact

import (
	"context"
	"sync"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/app/ports"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/domain"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/pkg/apperrors"
)

type matcher struct {
	mu      sync.RWMutex
	blocked map[exactKey]struct{}
}

type exactKey struct {
	chatID       int64
	fileUniqueID string
}

func NewMatcher(buffer int) (ports.ContentMatcher, error) {
	const methodCtx = "exact/NewMatcher"

	if buffer < 0 {
		return nil, apperrors.New(methodCtx, "буфер exact-матчера не должен быть отрицательным")
	}

	return &matcher{
		blocked: make(map[exactKey]struct{}, buffer),
	}, nil
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

func (m *matcher) Block(_ context.Context, chatID int64, content domain.Content) error {
	if content.FileUniqueID == "" {
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.blocked[exactKey{chatID: chatID, fileUniqueID: content.FileUniqueID}] = struct{}{}
	return nil
}
