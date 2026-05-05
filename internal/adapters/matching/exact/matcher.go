package exact

import (
	"context"
	"errors"
	"sync"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/app/ports"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/domain"
)

type matcher struct {
	mu      sync.RWMutex
	blocked map[string]struct{}
}

func NewMatcher(buffer int) (ports.ContentMatcher, error) {
	if buffer < 0 {
		return nil, errors.New("exact matcher buffer must be non-negative")
	}

	return &matcher{
		blocked: make(map[string]struct{}, buffer),
	}, nil
}

func (m *matcher) IsBlocked(_ context.Context, content domain.Content) (bool, error) {
	if content.FileUniqueID == "" {
		return false, nil
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	_, ok := m.blocked[content.FileUniqueID]
	return ok, nil
}

func (m *matcher) Block(_ context.Context, content domain.Content) error {
	if content.FileUniqueID == "" {
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.blocked[content.FileUniqueID] = struct{}{}
	return nil
}
