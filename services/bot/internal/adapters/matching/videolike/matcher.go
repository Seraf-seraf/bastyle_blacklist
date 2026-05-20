package videolike

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"time"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/app/ports"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/domain"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/pkg/apperrors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type matcher struct {
	fingerprintExtractor *fingerprintExtractor
	threshold            int
	index                *LinearIndex
	store                videoLikeStore
}

type videoLikeStore interface {
	load(context.Context) ([]StoredVideoLikeHash, error)
	Insert(context.Context, pgx.Tx, uuid.UUID, StoredVideoLikeHash) (int64, bool, error)
	close() error
}

type Limits struct {
	MaxAnimationDuration    time.Duration
	MaxVideoStickerDuration time.Duration
	MaxAnimationSize        int64
	MaxVideoStickerSize     int64
}

func NewMatcher(
	downloader ports.MediaDownloader,
	extractor ports.MediaExtractor,
	threshold int,
	buffer int,
	plan domain.MediaExtractionPlan,
	limits Limits,
	rule MatchRule,
) (*matcher, error) {
	const methodCtx = "videolike/NewMatcher"

	if err := validateMatcherConfig(downloader, extractor, threshold, buffer, plan, limits, rule); err != nil {
		return nil, apperrors.Wrap(methodCtx, err)
	}

	return &matcher{
		fingerprintExtractor: newFingerprintExtractor(downloader, extractor, plan, limits),
		threshold:            threshold,
		index:                NewLinearIndex(buffer, rule),
	}, nil
}

func NewPostgresMatcher(
	ctx context.Context,
	pool *pgxpool.Pool,
	downloader ports.MediaDownloader,
	extractor ports.MediaExtractor,
	threshold int,
	buffer int,
	plan domain.MediaExtractionPlan,
	limits Limits,
	rule MatchRule,
) (*matcher, error) {
	const methodCtx = "videolike/NewPostgresMatcher"

	if pool == nil {
		return nil, apperrors.New(methodCtx, "PostgreSQL pool videolike-матчер не настроен")
	}
	if err := validateMatcherConfig(downloader, extractor, threshold, buffer, plan, limits, rule); err != nil {
		return nil, apperrors.Wrap(methodCtx, err)
	}

	store := NewPostgresStore(pool)

	storedHashes, err := store.load(ctx)
	if err != nil {
		return nil, apperrors.Wrap(methodCtx, err)
	}

	matcher, err := NewMatcher(downloader, extractor, threshold, buffer+len(storedHashes), plan, limits, rule)
	if err != nil {
		return nil, apperrors.Wrap(methodCtx, err)
	}
	matcher.store = store
	matcher.index.AddMany(storedHashes)

	return matcher, nil
}

func validateMatcherConfig(
	downloader ports.MediaDownloader,
	extractor ports.MediaExtractor,
	threshold int,
	buffer int,
	plan domain.MediaExtractionPlan,
	limits Limits,
	rule MatchRule,
) error {
	const methodCtx = "videolike/validateMatcherConfig"

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
	if plan.MaxFrames <= 0 {
		return apperrors.New(methodCtx, "максимальное количество кадров должно быть положительным")
	}
	if plan.TargetWidth <= 0 {
		return apperrors.New(methodCtx, "целевая ширина должна быть положительной")
	}
	if plan.TargetHeight <= 0 {
		return apperrors.New(methodCtx, "целевая высота должна быть положительной")
	}
	if err := limits.validate(); err != nil {
		return apperrors.Wrap(methodCtx, err)
	}
	if err := rule.validate(); err != nil {
		return apperrors.Wrap(methodCtx, err)
	}

	return nil
}

func (m *matcher) Close() error {
	const methodCtx = "videolike/matcher.Close"

	if m.store == nil {
		return nil
	}

	return apperrors.Wrap(methodCtx, m.store.close())
}

func (m *matcher) IsBlocked(ctx context.Context, chatID int64, content domain.Content) (bool, error) {
	const methodCtx = "videolike/matcher.IsBlocked"

	fingerprint, err := m.fingerprintExtractor.Extract(ctx, content)
	if err != nil {
		if errors.Is(err, ports.ErrUnsupportedContent) {
			return false, nil
		}
		return false, apperrors.Wrap(methodCtx, err)
	}

	_, matched := m.index.Search(chatID, fingerprint, m.threshold)
	return matched, nil
}

func (m *matcher) PrepareBlock(ctx context.Context, chatID int64, content domain.Content) (ports.PreparedBlock, error) {
	const methodCtx = "videolike/matcher.PrepareBlock"

	fingerprint, err := m.fingerprintExtractor.Extract(ctx, content)
	if err != nil {
		return nil, apperrors.Wrap(methodCtx, err)
	}
	fingerprint.ChatID = chatID

	return &preparedBlock{
		fingerprint: fingerprint,
	}, nil
}

type preparedBlock struct {
	fingerprint StoredVideoLikeHash
}

func (m *matcher) PersistBlock(ctx context.Context, tx pgx.Tx, banUID uuid.UUID, block ports.PreparedBlock) (bool, error) {
	const methodCtx = "videolike/matcher.PersistBlock"

	prepared, ok := block.(*preparedBlock)
	if !ok {
		return false, apperrors.New(methodCtx, "неверный тип prepared block videolike")
	}
	if m.store == nil {
		return false, apperrors.New(methodCtx, "PostgreSQL-хранилище videolike не настроено")
	}

	id, created, err := m.store.Insert(ctx, tx, banUID, prepared.fingerprint)
	if err != nil {
		return false, err
	}
	prepared.fingerprint.ID = id
	return created, nil
}

func (m *matcher) ApplyBlock(_ context.Context, block ports.PreparedBlock) error {
	const methodCtx = "videolike/matcher.ApplyBlock"

	prepared, ok := block.(*preparedBlock)
	if !ok {
		return apperrors.New(methodCtx, "неверный тип prepared block videolike")
	}

	m.index.Add(prepared.fingerprint)
	return nil
}

func (m *matcher) supports(ctx context.Context, content domain.Content) (bool, error) {
	const methodCtx = "videolike/matcher.supports"

	_, err := m.fingerprintExtractor.mediaForContent(ctx, content)
	if err != nil {
		if errors.Is(err, ports.ErrUnsupportedContent) {
			return false, nil
		}
		return false, apperrors.Wrap(methodCtx, err)
	}
	return true, nil
}

var _ ports.ContentBlockMatcher = (*matcher)(nil)

type fingerprintExtractor struct {
	downloader ports.MediaDownloader
	extractor  ports.MediaExtractor
	plan       domain.MediaExtractionPlan
	limits     Limits
}

func newFingerprintExtractor(
	downloader ports.MediaDownloader,
	extractor ports.MediaExtractor,
	plan domain.MediaExtractionPlan,
	limits Limits,
) *fingerprintExtractor {
	return &fingerprintExtractor{
		downloader: downloader,
		extractor:  extractor,
		plan:       plan,
		limits:     limits,
	}
}

func (e *fingerprintExtractor) Extract(ctx context.Context, content domain.Content) (StoredVideoLikeHash, error) {
	const methodCtx = "videolike/fingerprintExtractor.Extract"

	media, err := e.mediaForContent(ctx, content)
	if err != nil {
		return StoredVideoLikeHash{}, apperrors.Wrap(methodCtx, err)
	}

	fingerprint, err := e.fingerprint(ctx, media)
	return fingerprint, apperrors.Wrap(methodCtx, err)
}

func (e *fingerprintExtractor) mediaForContent(ctx context.Context, content domain.Content) (domain.MediaFile, error) {
	const methodCtx = "videolike/fingerprintExtractor.mediaForContent"

	if !content.CanDownload() {
		return domain.MediaFile{}, apperrors.Wrap(methodCtx, ports.ErrUnsupportedContent)
	}

	switch content.Type {
	case domain.MediaAnimation:
		if err := e.checkAnimationMetadata(content); err != nil {
			return domain.MediaFile{}, apperrors.Wrap(methodCtx, err)
		}

		media, err := e.downloader.Download(ctx, content)
		if err != nil {
			return domain.MediaFile{}, apperrors.Wrap(methodCtx, err)
		}
		if err := e.checkAnimationFile(media); err != nil {
			return domain.MediaFile{}, apperrors.Wrap(methodCtx, err)
		}

		return media, nil
	case domain.MediaStickerStatic:
		if err := e.checkVideoStickerMetadata(content); err != nil {
			return domain.MediaFile{}, apperrors.Wrap(methodCtx, err)
		}

		media, err := e.downloader.Download(ctx, content)
		if err != nil {
			return domain.MediaFile{}, apperrors.Wrap(methodCtx, err)
		}
		if !isWebMFile(media.FilePath) {
			return domain.MediaFile{}, apperrors.Wrap(methodCtx, ports.ErrUnsupportedContent)
		}

		if err := e.checkVideoStickerFile(media); err != nil {
			return domain.MediaFile{}, apperrors.Wrap(methodCtx, err)
		}

		return media, nil
	default:
		return domain.MediaFile{}, apperrors.Wrap(methodCtx, ports.ErrUnsupportedContent)
	}
}

func (e *fingerprintExtractor) checkAnimationMetadata(content domain.Content) error {
	const methodCtx = "videolike/fingerprintExtractor.checkAnimationMetadata"

	if contentDuration(content) > e.limits.MaxAnimationDuration {
		return apperrors.New(methodCtx, "длительность анимации превышает лимит")
	}
	if content.SizeBytes > e.limits.MaxAnimationSize {
		return apperrors.New(methodCtx, "размер анимации превышает лимит")
	}

	return nil
}

func (e *fingerprintExtractor) checkAnimationFile(media domain.MediaFile) error {
	const methodCtx = "videolike/fingerprintExtractor.checkAnimationFile"

	if media.Content.SizeBytes > e.limits.MaxAnimationSize {
		return apperrors.New(methodCtx, "размер анимации превышает лимит")
	}
	if int64(len(media.Data)) > e.limits.MaxAnimationSize {
		return apperrors.New(methodCtx, "размер загруженной анимации превышает лимит")
	}

	return nil
}

func (e *fingerprintExtractor) checkVideoStickerMetadata(content domain.Content) error {
	const methodCtx = "videolike/fingerprintExtractor.checkVideoStickerMetadata"

	if contentDuration(content) > e.limits.MaxVideoStickerDuration {
		return apperrors.New(methodCtx, "длительность видеостикера превышает лимит")
	}

	return nil
}

func (e *fingerprintExtractor) fingerprint(ctx context.Context, media domain.MediaFile) (StoredVideoLikeHash, error) {
	const methodCtx = "videolike/fingerprintExtractor.fingerprint"

	extracted, err := e.extractor.Extract(ctx, media, e.plan)
	if err != nil {
		return StoredVideoLikeHash{}, apperrors.Wrap(methodCtx, err)
	}

	fingerprint, err := fingerprintVideoLike(media.Content, extracted)
	return fingerprint, apperrors.Wrap(methodCtx, err)
}

func (e *fingerprintExtractor) checkVideoStickerFile(media domain.MediaFile) error {
	const methodCtx = "videolike/fingerprintExtractor.checkVideoStickerFile"

	if media.Content.SizeBytes > e.limits.MaxVideoStickerSize {
		return apperrors.New(methodCtx, "размер видеостикера превышает лимит")
	}
	if int64(len(media.Data)) > e.limits.MaxVideoStickerSize {
		return apperrors.New(methodCtx, "размер загруженного видеостикера превышает лимит")
	}

	return nil
}

func (l Limits) validate() error {
	const methodCtx = "videolike/Limits.validate"

	if l.MaxAnimationDuration <= 0 {
		return apperrors.New(methodCtx, "максимальная длительность анимации должна быть положительной")
	}
	if l.MaxVideoStickerDuration <= 0 {
		return apperrors.New(methodCtx, "максимальная длительность видеостикера должна быть положительной")
	}
	if l.MaxAnimationSize <= 0 {
		return apperrors.New(methodCtx, "максимальный размер анимации должен быть положительным")
	}
	if l.MaxVideoStickerSize <= 0 {
		return apperrors.New(methodCtx, "максимальный размер видеостикера должен быть положительным")
	}
	return nil
}

func (r MatchRule) validate() error {
	const methodCtx = "videolike/MatchRule.validate"

	if r == (MatchRule{}) {
		return nil
	}
	if r.MinMatchedFrames <= 0 {
		return apperrors.New(methodCtx, "минимальное количество совпавших кадров должно быть положительным")
	}
	if r.MinMatchedRatio <= 0 || r.MinMatchedRatio > 1 {
		return apperrors.New(methodCtx, "минимальная доля совпавших кадров должна быть от 0 до 1")
	}

	return nil
}

func contentDuration(content domain.Content) time.Duration {
	return time.Duration(content.DurationSec) * time.Second
}

func isWebMFile(filePath string) bool {
	return strings.EqualFold(filepath.Ext(filePath), ".webm")
}
