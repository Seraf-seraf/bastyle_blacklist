package imagehash

import (
	"context"
	"errors"
	"image"
	"image/color"
	"math"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/app/ports"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/domain"
	"github.com/corona10/goimagehash"
	"github.com/disintegration/imaging"
)

type Matcher struct {
	downloader ports.MediaDownloader
	extractor  ports.MediaExtractor
	threshold  int

	index *LinearIndex
	store *SQLiteStore
}

func NewMatcher(downloader ports.MediaDownloader, extractor ports.MediaExtractor, threshold int, buffer int) *Matcher {
	return &Matcher{
		downloader: downloader,
		extractor:  extractor,
		threshold:  threshold,
		index:      NewLinearIndex(buffer),
	}
}

func NewSQLiteMatcher(ctx context.Context, downloader ports.MediaDownloader, extractor ports.MediaExtractor, threshold int, buffer int, dbPath string) (*Matcher, error) {
	store, err := OpenSQLiteStore(ctx, dbPath)
	if err != nil {
		return nil, err
	}

	storedHashes, err := store.Load(ctx)
	if err != nil {
		_ = store.Close()
		return nil, err
	}

	matcher := NewMatcher(downloader, extractor, threshold, buffer+len(storedHashes))
	matcher.store = store
	matcher.index.AddMany(storedHashes)

	return matcher, nil
}

func (m *Matcher) Close() error {
	if m.store == nil {
		return nil
	}

	return m.store.Close()
}

func (m *Matcher) IsBlocked(ctx context.Context, content domain.Content) (bool, error) {
	if !m.supports(content) {
		return false, nil
	}

	hashes, err := m.hashContent(ctx, content)
	if err != nil {
		return false, err
	}
	if len(hashes) == 0 {
		return false, nil
	}

	return m.index.Search(hashes, m.threshold), nil
}

func (m *Matcher) Block(ctx context.Context, content domain.Content) error {
	if !m.supports(content) {
		return nil
	}
	if m.store == nil {
		return errors.New("imagehash store is not configured")
	}

	hashes, err := m.hashContent(ctx, content)
	if err != nil {
		return err
	}
	if len(hashes) == 0 {
		return nil
	}

	storedHash := StoredImageHash{
		FileUniqueID: content.FileUniqueID,
		MediaType:    content.Type,
		Hashes:       hashes,
	}

	id, err := m.store.Insert(ctx, storedHash)
	if err != nil {
		return err
	}
	storedHash.ID = id

	m.index.Add(storedHash)
	return nil
}

func (m *Matcher) supports(content domain.Content) bool {
	return (content.Type == domain.MediaPhoto ||
		content.Type == domain.MediaStickerStatic) &&
		content.CanDownload()
}

func (m *Matcher) hashContent(ctx context.Context, content domain.Content) ([]uint64, error) {
	media, err := m.downloader.Download(ctx, content)
	if err != nil {
		return nil, err
	}

	extracted, err := m.extractor.Extract(ctx, media, domain.MediaExtractionPlan{
		MaxFrames: 1,
	})
	if err != nil {
		return nil, err
	}

	if len(extracted.Frames) == 0 {
		return nil, errors.New("media extractor returned no frames")
	}

	if !isInformativeImage(extracted.Frames[0].Image) {
		return nil, nil
	}

	hashes, err := perceptionHashVariants(extracted.Frames[0].Image)
	if err != nil {
		return nil, err
	}

	return hashes, nil
}

func isInformativeImage(img image.Image) bool {
	const (
		minVariance     = 120.0
		minOccupiedBins = 3
		bins            = 16
	)

	bounds := img.Bounds()
	if bounds.Empty() {
		return false
	}

	stepX := max(1, bounds.Dx()/64)
	stepY := max(1, bounds.Dy()/64)

	var count int
	var sum float64
	var sumSquares float64
	occupied := [bins]bool{}

	for y := bounds.Min.Y; y < bounds.Max.Y; y += stepY {
		for x := bounds.Min.X; x < bounds.Max.X; x += stepX {
			luma := luminance(img.At(x, y))
			sum += luma
			sumSquares += luma * luma
			occupied[int(luma)/16] = true
			count++
		}
	}

	if count == 0 {
		return false
	}

	mean := sum / float64(count)
	variance := sumSquares/float64(count) - mean*mean
	if variance < minVariance {
		return false
	}

	occupiedCount := 0
	for _, ok := range occupied {
		if ok {
			occupiedCount++
		}
	}

	return occupiedCount >= minOccupiedBins
}

func luminance(c color.Color) float64 {
	r, g, b, _ := c.RGBA()
	return math.Round(0.299*float64(r>>8) + 0.587*float64(g>>8) + 0.114*float64(b>>8))
}

func perceptionHashVariants(img image.Image) ([]uint64, error) {
	images := []image.Image{
		img,
		imaging.Rotate90(img),
		imaging.Rotate180(img),
		imaging.Rotate270(img),
	}
	hashes := make([]uint64, 0, len(images))

	for _, img := range images {
		hash, err := goimagehash.PerceptionHash(img)
		if err != nil {
			return nil, err
		}

		hashes = append(hashes, hash.GetHash())
	}

	return hashes, nil
}
