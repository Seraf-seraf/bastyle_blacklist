package imagehash

import (
	"strconv"
	"testing"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/domain"
)

func BenchmarkLinearIndexSearch(b *testing.B) {
	for _, size := range []int{10_000, 100_000} {
		b.Run(strconv.Itoa(size)+"_miss", func(b *testing.B) {
			index := benchmarkIndex(size)
			query := []uint64{
				0xffff_ffff_ffff_ffff,
				0xeeee_eeee_eeee_eeee,
				0xdddd_dddd_dddd_dddd,
				0xcccc_cccc_cccc_cccc,
			}

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				if index.Search(query, 8) {
					b.Fatal("expected no match")
				}
			}
		})

		b.Run(strconv.Itoa(size)+"_match_last", func(b *testing.B) {
			index := benchmarkIndex(size)
			query := []uint64{
				0x1111_2222_3333_4444,
				0x2222_3333_4444_5555,
				0x3333_4444_5555_6666,
				0x4444_5555_6666_7777,
			}
			index.Add(StoredImageHash{
				FileUniqueID: "matching-item",
				MediaType:    domain.MediaPhoto,
				Hashes:       append([]uint64(nil), query...),
			})

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				if !index.Search(query, 8) {
					b.Fatal("expected match")
				}
			}
		})
	}
}

func benchmarkIndex(size int) *LinearIndex {
	index := NewLinearIndex(size)
	hashes := make([]StoredImageHash, 0, size)

	for i := 0; i < size; i++ {
		base := splitmix64(uint64(i + 1))
		hashes = append(hashes, StoredImageHash{
			FileUniqueID: strconv.Itoa(i),
			MediaType:    domain.MediaPhoto,
			Hashes: []uint64{
				base,
				splitmix64(base),
				splitmix64(base + 1),
				splitmix64(base + 2),
			},
		})
	}

	index.AddMany(hashes)
	return index
}

func splitmix64(x uint64) uint64 {
	x += 0x9e3779b97f4a7c15
	x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9
	x = (x ^ (x >> 27)) * 0x94d049bb133111eb
	return x ^ (x >> 31)
}
