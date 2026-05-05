package imagehash

import (
	"context"
	"errors"
	"image"
	"path/filepath"
	"strings"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/app/ports"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/domain"
	"github.com/corona10/goimagehash"
	"github.com/disintegration/imaging"
)

type matcher struct {
	downloader ports.MediaDownloader
	extractor  ports.MediaExtractor
	threshold  int

	index *LinearIndex
	store *SQLiteStore
}

func NewMatcher(downloader ports.MediaDownloader, extractor ports.MediaExtractor, threshold int, buffer int) (*matcher, error) {
	if err := validateMatcherConfig(downloader, extractor, threshold, buffer); err != nil {
		return nil, err
	}

	return &matcher{
		downloader: downloader,
		extractor:  extractor,
		threshold:  threshold,
		index:      NewLinearIndex(buffer),
	}, nil
}

func NewSQLiteMatcher(ctx context.Context, downloader ports.MediaDownloader, extractor ports.MediaExtractor, threshold int, buffer int, dbPath string) (*matcher, error) {
	if dbPath == "" {
		return nil, errors.New("imagehash matcher db path is not configured")
	}
	if err := validateMatcherConfig(downloader, extractor, threshold, buffer); err != nil {
		return nil, err
	}

	store, err := OpenSQLiteStore(ctx, dbPath)
	if err != nil {
		return nil, err
	}

	storedHashes, err := store.Load(ctx)
	if err != nil {
		_ = store.Close()
		return nil, err
	}

	matcher, err := NewMatcher(downloader, extractor, threshold, buffer+len(storedHashes))
	if err != nil {
		_ = store.Close()
		return nil, err
	}
	matcher.store = store
	matcher.index.AddMany(storedHashes)

	return matcher, nil
}

func validateMatcherConfig(downloader ports.MediaDownloader, extractor ports.MediaExtractor, threshold int, buffer int) error {
	if downloader == nil {
		return errors.New("imagehash matcher downloader is not configured")
	}
	if extractor == nil {
		return errors.New("imagehash matcher extractor is not configured")
	}
	if threshold < 0 {
		return errors.New("imagehash matcher threshold must be non-negative")
	}
	if buffer < 0 {
		return errors.New("imagehash matcher buffer must be non-negative")
	}

	return nil
}

func (m *matcher) Close() error {
	if m.store == nil {
		return nil
	}

	return m.store.Close()
}

func (m *matcher) IsBlocked(ctx context.Context, content domain.Content) (bool, error) {
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

func (m *matcher) Block(ctx context.Context, content domain.Content) error {
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

func (m *matcher) supports(content domain.Content) bool {
	return (content.Type == domain.MediaPhoto ||
		content.Type == domain.MediaStickerStatic) &&
		content.CanDownload()
}

func (m *matcher) hashContent(ctx context.Context, content domain.Content) ([]uint64, error) {
	media, err := m.downloader.Download(ctx, content)
	if err != nil {
		return nil, err
	}
	if isVideoFile(media.FilePath) {
		return nil, nil
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

	hashes, err := perceptionHashVariants(extracted.Frames[0].Image)
	if err != nil {
		return nil, err
	}

	return hashes, nil
}

func isVideoFile(filePath string) bool {
	switch strings.ToLower(filepath.Ext(filePath)) {
	case ".mp4", ".webm", ".mov", ".mkv":
		return true
	default:
		return false
	}
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
