package exactmatcher

import (
	"sync"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/domain"
)

type ExactMatcher struct {
	mu      sync.RWMutex
	blocked map[string]struct{}
}

func NewExactMatcher(buffer int) *ExactMatcher {
	return &ExactMatcher{
		blocked: make(map[string]struct{}, buffer),
	}
}

func (m *ExactMatcher) IsBlocked(content domain.Content) (bool, error) {
	if content.FileUniqueID == "" {
		return false, nil
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	_, ok := m.blocked[content.FileUniqueID]
	return ok, nil
}

func (m *ExactMatcher) Block(content domain.Content) error {
	if content.FileUniqueID == "" {
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.blocked[content.FileUniqueID] = struct{}{}
	return nil
}
