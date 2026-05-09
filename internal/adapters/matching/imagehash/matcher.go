package imagehash

import (
	"context"
	"image"
	"path/filepath"
	"strings"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/app/ports"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/domain"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/pkg/apperrors"
	"github.com/corona10/goimagehash"
	"github.com/disintegration/imaging"
)

type matcher struct {
	downloader ports.MediaDownloader
	extractor  ports.MediaExtractor
	threshold  int

	index *LinearIndex
	store *sqliteStore
}

func NewMatcher(downloader ports.MediaDownloader, extractor ports.MediaExtractor, threshold int, buffer int) (*matcher, error) {
	const methodCtx = "imagehash/NewMatcher"

	if err := validateMatcherConfig(downloader, extractor, threshold, buffer); err != nil {
		return nil, apperrors.Wrap(methodCtx, err)
	}

	return &matcher{
		downloader: downloader,
		extractor:  extractor,
		threshold:  threshold,
		index:      NewLinearIndex(buffer),
	}, nil
}

func NewSQLiteMatcher(ctx context.Context, downloader ports.MediaDownloader, extractor ports.MediaExtractor, threshold int, buffer int, dbPath string) (*matcher, error) {
	const methodCtx = "imagehash/NewSQLiteMatcher"

	if dbPath == "" {
		return nil, apperrors.New(methodCtx, "путь к БД imagehash-матчера не настроен")
	}
	if err := validateMatcherConfig(downloader, extractor, threshold, buffer); err != nil {
		return nil, apperrors.Wrap(methodCtx, err)
	}

	store, err := OpenSQLiteStore(ctx, dbPath)
	if err != nil {
		return nil, apperrors.Wrap(methodCtx, err)
	}

	storedHashes, err := store.load(ctx)
	if err != nil {
		_ = store.close()
		return nil, apperrors.Wrap(methodCtx, err)
	}

	matcher, err := NewMatcher(downloader, extractor, threshold, buffer+len(storedHashes))
	if err != nil {
		_ = store.close()
		return nil, apperrors.Wrap(methodCtx, err)
	}
	matcher.store = store
	matcher.index.AddMany(storedHashes)

	return matcher, nil
}

func validateMatcherConfig(downloader ports.MediaDownloader, extractor ports.MediaExtractor, threshold int, buffer int) error {
	const methodCtx = "imagehash/validateMatcherConfig"

	if downloader == nil {
		return apperrors.New(methodCtx, "загрузчик imagehash-матчера не настроен")
	}
	if extractor == nil {
		return apperrors.New(methodCtx, "извлекатель imagehash-матчера не настроен")
	}
	if threshold < 0 {
		return apperrors.New(methodCtx, "порог imagehash-матчера не должен быть отрицательным")
	}
	if buffer < 0 {
		return apperrors.New(methodCtx, "буфер imagehash-матчера не должен быть отрицательным")
	}

	return nil
}

func (m *matcher) Close() error {
	const methodCtx = "imagehash/matcher.Close"

	if m.store == nil {
		return nil
	}

	return apperrors.Wrap(methodCtx, m.store.close())
}

func (m *matcher) IsBlocked(ctx context.Context, chatID int64, content domain.Content) (bool, error) {
	const methodCtx = "imagehash/matcher.IsBlocked"

	if !m.supports(content) {
		return false, nil
	}

	hashes, err := m.hashContent(ctx, content)
	if err != nil {
		return false, apperrors.Wrap(methodCtx, err)
	}
	if len(hashes) == 0 {
		return false, nil
	}

	return m.index.Search(chatID, hashes, m.threshold), nil
}

func (m *matcher) Block(ctx context.Context, chatID int64, content domain.Content) error {
	const methodCtx = "imagehash/matcher.Block"

	if !m.supports(content) {
		return nil
	}
	if m.store == nil {
		return apperrors.New(methodCtx, "хранилище imagehash не настроено")
	}

	hashes, err := m.hashContent(ctx, content)
	if err != nil {
		return apperrors.Wrap(methodCtx, err)
	}
	if len(hashes) == 0 {
		return nil
	}

	storedHash := StoredImageHash{
		ChatID:       chatID,
		FileUniqueID: content.FileUniqueID,
		MediaType:    content.Type,
		Hashes:       hashes,
	}

	id, err := m.store.insert(ctx, storedHash)
	if err != nil {
		return apperrors.Wrap(methodCtx, err)
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
	const methodCtx = "imagehash/matcher.hashContent"

	media, err := m.downloader.Download(ctx, content)
	if err != nil {
		return nil, apperrors.Wrap(methodCtx, err)
	}
	if isVideoFile(media.FilePath) {
		return nil, nil
	}

	extracted, err := m.extractor.Extract(ctx, media, domain.MediaExtractionPlan{
		MaxFrames: 1,
	})
	if err != nil {
		return nil, apperrors.Wrap(methodCtx, err)
	}

	if len(extracted.Frames) == 0 {
		return nil, apperrors.New(methodCtx, "извлекатель медиа не вернул кадров")
	}

	hashes, err := perceptionHashVariants(extracted.Frames[0].Image)
	if err != nil {
		return nil, apperrors.Wrap(methodCtx, err)
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
	const methodCtx = "imagehash/perceptionHashVariants"

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
			return nil, apperrors.Wrap(methodCtx, err)
		}

		hashes = append(hashes, hash.GetHash())
	}

	return hashes, nil
}
