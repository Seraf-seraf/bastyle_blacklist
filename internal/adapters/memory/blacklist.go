package memory

import "sync"

type BlacklistStore struct {
	mu  sync.RWMutex
	ban map[string]struct{}
}

func NewBlacklistStore(buffer int) *BlacklistStore {
	return &BlacklistStore{
		ban: make(map[string]struct{}, buffer),
	}
}

func (b *BlacklistStore) IsBlocked(FileUniqueID string) (bool, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if _, ok := b.ban[FileUniqueID]; ok {
		return true, nil
	}

	return false, nil
}

func (b *BlacklistStore) Block(FileUniqueID string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.ban[FileUniqueID] = struct{}{}

	return nil
}
