package blacklist

import "sync"

type blacklist struct {
	mu  sync.Mutex
	ban map[string]struct{}
}

var bl *blacklist

func InitInstance(buffer int) {
	bl = &blacklist{
		ban: make(map[string]struct{}, buffer),
	}
}

func GetInstance() *blacklist {
	if bl == nil {
		panic("blacklist is nil")
	}
	return bl
}
func (b *blacklist) IsBlocked(FileUniqueID string) bool {
	if _, ok := b.ban[FileUniqueID]; ok {
		return true
	}

	return false
}

func (b *blacklist) Block(FileUniqueID string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.ban[FileUniqueID] = struct{}{}
}
