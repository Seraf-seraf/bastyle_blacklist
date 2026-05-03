package exact

import (
	"context"
	"sync"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/domain"
)

type Matcher struct {
	mu      sync.RWMutex
	blocked map[string]struct{}
}

func NewMatcher(buffer int) *Matcher {
	return &Matcher{
		blocked: make(map[string]struct{}, buffer),
	}
}

func (m *Matcher) IsBlocked(_ context.Context, content domain.Content) (bool, error) {
	if content.FileUniqueID == "" {
		return false, nil
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	_, ok := m.blocked[content.FileUniqueID]
	return ok, nil
}

func (m *Matcher) Block(_ context.Context, content domain.Content) error {
	if content.FileUniqueID == "" {
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.blocked[content.FileUniqueID] = struct{}{}
	return nil
}
