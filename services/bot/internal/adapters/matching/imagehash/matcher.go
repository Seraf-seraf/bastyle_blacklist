package imagehash

import (
	"context"
	"errors"
	"image"
	"path/filepath"
	"strings"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/app/ports"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/domain"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/pkg/apperrors"
	"github.com/corona10/goimagehash"
	"github.com/disintegration/imaging"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type matcher struct {
	hashExtractor *hashExtractor
	threshold     int

	index *LinearIndex
	store imageHashStore
}

type imageHashStore interface {
	load(context.Context) ([]StoredImageHash, error)
	LoadByBanUID(context.Context, uuid.UUID) (StoredImageHash, error)
	Insert(context.Context, pgx.Tx, uuid.UUID, StoredImageHash) (imageHashInsertResult, error)
	close() error
}

type imageHashInsertResult struct {
	ID      int64
	Created bool
}

var errImageHashArtifactNotFound = errors.New("imagehash artifact не найден")

func NewMatcher(downloader ports.MediaDownloader, extractor ports.MediaExtractor, threshold int, buffer int) (*matcher, error) {
	const methodCtx = "imagehash/NewMatcher"

	if err := validateMatcherConfig(downloader, extractor, threshold, buffer); err != nil {
		return nil, apperrors.Wrap(methodCtx, err)
	}

	return &matcher{
		hashExtractor: newHashExtractor(downloader, extractor),
		threshold:     threshold,
		index:         NewLinearIndex(buffer),
	}, nil
}

func NewPostgresMatcher(ctx context.Context, pool *pgxpool.Pool, downloader ports.MediaDownloader, extractor ports.MediaExtractor, threshold int, buffer int) (*matcher, error) {
	const methodCtx = "imagehash/NewPostgresMatcher"

	if pool == nil {
		return nil, apperrors.New(methodCtx, "PostgreSQL pool imagehash-матчера не настроен")
	}
	if err := validateMatcherConfig(downloader, extractor, threshold, buffer); err != nil {
		return nil, apperrors.Wrap(methodCtx, err)
	}

	store := NewPostgresStore(pool)

	storedHashes, err := store.load(ctx)
	if err != nil {
		return nil, apperrors.Wrap(methodCtx, err)
	}

	matcher, err := NewMatcher(downloader, extractor, threshold, buffer+len(storedHashes))
	if err != nil {
		return nil, apperrors.Wrap(methodCtx, err)
	}
	matcher.store = store
	matcher.index.AddMany(storedHashes)

	return matcher, nil
}

func validateMatcherConfig(downloader ports.MediaDownloader, extractor ports.MediaExtractor, threshold int, buffer int) error {
	const methodCtx = "imagehash/validateMatcherConfig"

	if downloader == nil {
		return apperrors.New(methodCtx, "загрузчик не настроен")
	}
	if extractor == nil {
		return apperrors.New(methodCtx, "извлекатель не настроен")
	}
	if threshold < 0 {
		return apperrors.New(methodCtx, "порог не должен быть отрицательным")
	}
	if buffer < 0 {
		return apperrors.New(methodCtx, "буфер не должен быть отрицательным")
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

	hashes, err := m.hashExtractor.Extract(ctx, content)
	if err != nil {
		if errors.Is(err, ports.ErrUnsupportedContent) {
			return false, nil
		}
		return false, apperrors.Wrap(methodCtx, err)
	}

	return m.index.Search(chatID, hashes, m.threshold), nil
}

func (m *matcher) PrepareBlock(ctx context.Context, chatID int64, content domain.Content) (ports.PreparedBlock, error) {
	const methodCtx = "imagehash/matcher.PrepareBlock"

	hashes, err := m.hashExtractor.Extract(ctx, content)
	if err != nil {
		return nil, apperrors.Wrap(methodCtx, err)
	}

	return &preparedBlock{
		hash: StoredImageHash{
			ChatID:       chatID,
			FileUniqueID: content.FileUniqueID,
			MediaType:    content.Type,
			Hashes:       hashes,
		},
	}, nil
}

type preparedBlock struct {
	hash StoredImageHash
}

func (m *matcher) PersistBlock(ctx context.Context, tx pgx.Tx, banUID uuid.UUID, block ports.PreparedBlock) (ports.PersistBlockResult, error) {
	const methodCtx = "imagehash/matcher.PersistBlock"

	prepared, ok := block.(*preparedBlock)
	if !ok {
		return ports.PersistBlockResult{}, apperrors.New(methodCtx, "неверный тип prepared block imagehash")
	}

	result, err := m.store.Insert(ctx, tx, banUID, prepared.hash)
	if err != nil {
		return ports.PersistBlockResult{}, err
	}
	prepared.hash.ID = result.ID
	return ports.PersistBlockResult{Created: result.Created}, nil
}

func (m *matcher) ApplyBlock(_ context.Context, block ports.PreparedBlock) error {
	const methodCtx = "imagehash/matcher.ApplyBlock"

	prepared, ok := block.(*preparedBlock)
	if !ok {
		return apperrors.New(methodCtx, "неверный тип prepared block imagehash")
	}

	m.index.Add(prepared.hash)
	return nil
}

var _ ports.ContentBlockMatcher = (*matcher)(nil)
var _ ports.IndexEventApplier = (*matcher)(nil)

func (m *matcher) IndexName() string {
	return ports.IndexImageHash
}

func (m *matcher) Supports(eventType string) bool {
	return eventType == "media.ban.created.v1"
}

func (m *matcher) ApplyEvent(ctx context.Context, event ports.OutboxEvent) error {
	const methodCtx = "imagehash/matcher.ApplyEvent"

	if !m.Supports(event.EventType) {
		return nil
	}
	record, err := m.store.LoadByBanUID(ctx, event.AggregateUID)
	if err != nil {
		if errors.Is(err, errImageHashArtifactNotFound) {
			return nil
		}
		return apperrors.Wrap(methodCtx, err)
	}
	m.index.Add(record)
	return nil
}

type hashExtractor struct {
	downloader ports.MediaDownloader
	extractor  ports.MediaExtractor
}

func newHashExtractor(downloader ports.MediaDownloader, extractor ports.MediaExtractor) *hashExtractor {
	return &hashExtractor{
		downloader: downloader,
		extractor:  extractor,
	}
}

func (e *hashExtractor) Extract(ctx context.Context, content domain.Content) ([]uint64, error) {
	const methodCtx = "imagehash/hashExtractor.Extract"

	if !e.supports(content) {
		return nil, apperrors.Wrap(methodCtx, ports.ErrUnsupportedContent)
	}

	media, err := e.downloader.Download(ctx, content)
	if err != nil {
		return nil, apperrors.Wrap(methodCtx, err)
	}
	if isVideoFile(media.FilePath) {
		return nil, apperrors.Wrap(methodCtx, ports.ErrUnsupportedContent)
	}

	extracted, err := e.extractor.Extract(ctx, media, domain.MediaExtractionPlan{
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

func (e *hashExtractor) supports(content domain.Content) bool {
	return (content.Type == domain.MediaPhoto ||
		content.Type == domain.MediaStickerStatic) &&
		content.CanDownload()
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
