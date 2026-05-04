package imagehash

import (
	"math/bits"
	"strconv"
	"strings"
	"sync"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/domain"
)

const hashVersion = "goimagehash-phash-64-rot4-v1"

// StoredImageHash описывает один banned image item в поисковом индексе.
//
// Hashes содержит pHash variants одного изображения. Сейчас matcher кладет
// туда четыре значения: оригинал и повороты 90, 180, 270 градусов.
type StoredImageHash struct {
	ID           int64
	FileUniqueID string
	MediaType    domain.MediaType
	Hashes       []uint64
}

// LinearIndex держит хэши заблокированных картинок в памяти и проверяет их
// простым проходом по всему списку.
//
// Такой подход подходит для текущего сценария: банов мало, проверок много, а
// размер списка ожидается в пределах десятков тысяч записей. SQLite хранит
// данные между перезапусками, а этот индекс загружается из SQLite при старте и
// используется только для быстрого поиска.
type LinearIndex struct {
	mu     sync.RWMutex
	hashes []StoredImageHash
}

// NewLinearIndex создает пустой in-memory индекс с предвыделенной емкостью.
func NewLinearIndex(buffer int) *LinearIndex {
	return &LinearIndex{
		hashes: make([]StoredImageHash, 0, buffer),
	}
}

// Add добавляет одну заблокированную картинку в индекс.
//
// Дубликатом считается запись с теми же значениями Hashes в том же порядке.
// FileUniqueID здесь не используется: одна и та же картинка может прийти с
// другим Telegram id, но с теми же perceptual hashes.
func (i *LinearIndex) Add(hash StoredImageHash) {
	i.mu.Lock()
	defer i.mu.Unlock()

	signature := hashSignature(hash.Hashes)
	for _, existing := range i.hashes {
		if hashSignature(existing.Hashes) == signature {
			return
		}
	}

	i.hashes = append(i.hashes, hash)
}

// AddMany массово добавляет hashes в индекс и пропускает дубли.
//
// Метод используется при старте, когда SQLiteStore.Load возвращает persisted
// записи. В отличие от многократного Add, он строит set один раз.
func (i *LinearIndex) AddMany(hashes []StoredImageHash) {
	i.mu.Lock()
	defer i.mu.Unlock()

	seen := make(map[string]struct{}, len(i.hashes)+len(hashes))
	for _, existing := range i.hashes {
		seen[hashSignature(existing.Hashes)] = struct{}{}
	}

	for _, hash := range hashes {
		signature := hashSignature(hash.Hashes)
		if _, ok := seen[signature]; ok {
			continue
		}

		seen[signature] = struct{}{}
		i.hashes = append(i.hashes, hash)
	}
}

// Search возвращает true, если хотя бы один query hash находится в пределах
// threshold от любого stored hash.
//
// Это exact radius search по Hamming distance. Алгоритмически это O(N * Q * S),
// где N — количество stored items, Q — количество query variants, S —
// количество stored variants. Сейчас Q и S обычно равны 4.
func (i *LinearIndex) Search(query []uint64, threshold int) bool {
	i.mu.RLock()
	defer i.mu.RUnlock()

	for _, stored := range i.hashes {
		if withinThreshold(query, stored.Hashes, threshold) {
			return true
		}
	}

	return false
}

// withinThreshold выполняет самый горячий участок поиска: XOR двух 64-bit
// hashes и popcount отличающихся битов.
func withinThreshold(query []uint64, stored []uint64, threshold int) bool {
	for _, queryHash := range query {
		for _, storedHash := range stored {
			if bits.OnesCount64(queryHash^storedHash) <= threshold {
				return true
			}
		}
	}

	return false
}

// hashSignature строит стабильный ключ набора variants.
//
// Порядок важен: разные порядки variants означают разные версии представления.
// При изменении набора variants нужно менять hashVersion.
func hashSignature(hashes []uint64) string {
	parts := make([]string, 0, len(hashes))
	for _, hash := range hashes {
		parts = append(parts, strconv.FormatUint(hash, 10))
	}

	return strings.Join(parts, "|")
}
