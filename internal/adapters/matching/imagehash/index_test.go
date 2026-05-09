package imagehash

import (
	"testing"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/domain"
)

func TestLinearIndexSearchScopesHashesByChat(t *testing.T) {
	index := NewLinearIndex(1)
	query := []uint64{0x1111}
	index.Add(StoredImageHash{
		ChatID:       10,
		FileUniqueID: "file-unique-id",
		MediaType:    domain.MediaPhoto,
		Hashes:       query,
	})

	if !index.Search(10, query, 0) {
		t.Fatal("ожидалось: hash находится в исходном чате")
	}
	if index.Search(20, query, 0) {
		t.Fatal("не ожидалось: hash находится в другом чате")
	}
}
