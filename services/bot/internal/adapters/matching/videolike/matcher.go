package videolike

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/app/ports"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/domain"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/pkg/apperrors"
	"github.com/jackc/pgx/v5/pgxpool"
)

type matcher struct {
	downloader ports.MediaDownloader
	extractor  ports.MediaExtractor
	threshold  int
	plan       domain.MediaExtractionPlan
	index      *LinearIndex
	store      videoLikeStore
	limits     Limits
}

type videoLikeStore interface {
	load(context.Context) ([]StoredVideoLikeHash, error)
	insert(context.Context, StoredVideoLikeHash) (int64, error)
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
		downloader: downloader,
		extractor:  extractor,
		threshold:  threshold,
		plan:       plan,
		index:      NewLinearIndex(buffer, rule),
		limits:     limits,
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
		return apperrors.New(methodCtx, "загрузчик videolike-матчер не настроен")
	}
	if extractor == nil {
		return apperrors.New(methodCtx, "извлекатель videolike-матчер не настроен")
	}
	if threshold < 0 {
		return apperrors.New(methodCtx, "порог videolike-матчер не должен быть отрицательным")
	}
	if buffer < 0 {
		return apperrors.New(methodCtx, "буфер videolike-матчер не должен быть отрицательным")
	}
	if plan.MaxFrames <= 0 {
		return apperrors.New(methodCtx, "videolike-матчер: максимальное количество кадров должно быть положительным")
	}
	if plan.TargetWidth <= 0 {
		return apperrors.New(methodCtx, "videolike-матчер: целевая ширина должна быть положительной")
	}
	if plan.TargetHeight <= 0 {
		return apperrors.New(methodCtx, "videolike-матчер: целевая высота должна быть положительной")
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

	media, supported, err := m.mediaForContent(ctx, content)
	if err != nil {
		return false, apperrors.Wrap(methodCtx, err)
	}
	if !supported {
		return false, nil
	}

	fingerprint, err := m.fingerprint(ctx, media)
	if err != nil {
		return false, apperrors.Wrap(methodCtx, err)
	}

	_, matched := m.index.Search(chatID, fingerprint, m.threshold)
	return matched, nil
}

func (m *matcher) Block(ctx context.Context, chatID int64, content domain.Content) error {
	const methodCtx = "videolike/matcher.Block"

	media, supported, err := m.mediaForContent(ctx, content)
	if err != nil {
		return apperrors.Wrap(methodCtx, err)
	}
	if !supported {
		return nil
	}
	if m.store == nil {
		return apperrors.New(methodCtx, "хранилище videolike не настроено")
	}

	fingerprint, err := m.fingerprint(ctx, media)
	if err != nil {
		return apperrors.Wrap(methodCtx, err)
	}
	fingerprint.ChatID = chatID

	id, err := m.store.insert(ctx, fingerprint)
	if err != nil {
		return apperrors.Wrap(methodCtx, err)
	}
	fingerprint.ID = id

	m.index.Add(fingerprint)
	return nil
}

func (m *matcher) supports(ctx context.Context, content domain.Content) (bool, error) {
	const methodCtx = "videolike/matcher.supports"

	_, supported, err := m.mediaForContent(ctx, content)
	return supported, apperrors.Wrap(methodCtx, err)
}

func (m *matcher) mediaForContent(ctx context.Context, content domain.Content) (domain.MediaFile, bool, error) {
	const methodCtx = "videolike/matcher.mediaForContent"

	if !content.CanDownload() {
		return domain.MediaFile{}, false, nil
	}

	switch content.Type {
	case domain.MediaAnimation:
		if err := m.checkAnimationMetadata(content); err != nil {
			return domain.MediaFile{}, false, apperrors.Wrap(methodCtx, err)
		}

		media, err := m.downloader.Download(ctx, content)
		if err != nil {
			return domain.MediaFile{}, false, apperrors.Wrap(methodCtx, err)
		}
		if err := m.checkAnimationFile(media); err != nil {
			return domain.MediaFile{}, false, apperrors.Wrap(methodCtx, err)
		}

		return media, true, nil
	case domain.MediaStickerStatic:
		if err := m.checkVideoStickerMetadata(content); err != nil {
			return domain.MediaFile{}, false, apperrors.Wrap(methodCtx, err)
		}

		media, err := m.downloader.Download(ctx, content)
		if err != nil {
			return domain.MediaFile{}, false, apperrors.Wrap(methodCtx, err)
		}
		if !isWebMFile(media.FilePath) {
			return domain.MediaFile{}, false, nil
		}

		if err := m.checkVideoStickerFile(media); err != nil {
			return domain.MediaFile{}, false, apperrors.Wrap(methodCtx, err)
		}

		return media, true, nil
	default:
		return domain.MediaFile{}, false, nil
	}
}

func (m *matcher) checkAnimationMetadata(content domain.Content) error {
	const methodCtx = "videolike/matcher.checkAnimationMetadata"

	if contentDuration(content) > m.limits.MaxAnimationDuration {
		return apperrors.New(methodCtx, "videolike-матчер: длительность анимации превышает лимит")
	}
	if content.SizeBytes > m.limits.MaxAnimationSize {
		return apperrors.New(methodCtx, "videolike-матчер: размер анимации превышает лимит")
	}

	return nil
}

func (m *matcher) checkAnimationFile(media domain.MediaFile) error {
	const methodCtx = "videolike/matcher.checkAnimationFile"

	if media.Content.SizeBytes > m.limits.MaxAnimationSize {
		return apperrors.New(methodCtx, "videolike-матчер: размер анимации превышает лимит")
	}
	if int64(len(media.Data)) > m.limits.MaxAnimationSize {
		return apperrors.New(methodCtx, "videolike-матчер: размер загруженной анимации превышает лимит")
	}

	return nil
}

func (m *matcher) checkVideoStickerMetadata(content domain.Content) error {
	const methodCtx = "videolike/matcher.checkVideoStickerMetadata"

	if contentDuration(content) > m.limits.MaxVideoStickerDuration {
		return apperrors.New(methodCtx, "videolike-матчер: длительность видеостикера превышает лимит")
	}

	return nil
}

func (m *matcher) fingerprint(ctx context.Context, media domain.MediaFile) (StoredVideoLikeHash, error) {
	const methodCtx = "videolike/matcher.fingerprint"

	extracted, err := m.extractor.Extract(ctx, media, m.plan)
	if err != nil {
		return StoredVideoLikeHash{}, apperrors.Wrap(methodCtx, err)
	}

	fingerprint, err := fingerprintVideoLike(media.Content, extracted)
	return fingerprint, apperrors.Wrap(methodCtx, err)
}

func (m *matcher) checkVideoStickerFile(media domain.MediaFile) error {
	const methodCtx = "videolike/matcher.checkVideoStickerFile"

	if media.Content.SizeBytes > m.limits.MaxVideoStickerSize {
		return apperrors.New(methodCtx, "videolike-матчер: размер видеостикера превышает лимит")
	}
	if int64(len(media.Data)) > m.limits.MaxVideoStickerSize {
		return apperrors.New(methodCtx, "videolike-матчер: размер загруженного видеостикера превышает лимит")
	}

	return nil
}

func (l Limits) validate() error {
	const methodCtx = "videolike/Limits.validate"

	if l.MaxAnimationDuration <= 0 {
		return apperrors.New(methodCtx, "videolike-матчер: максимальная длительность анимации должна быть положительной")
	}
	if l.MaxVideoStickerDuration <= 0 {
		return apperrors.New(methodCtx, "videolike-матчер: максимальная длительность видеостикера должна быть положительной")
	}
	if l.MaxAnimationSize <= 0 {
		return apperrors.New(methodCtx, "videolike-матчер: максимальный размер анимации должен быть положительным")
	}
	if l.MaxVideoStickerSize <= 0 {
		return apperrors.New(methodCtx, "videolike-матчер: максимальный размер видеостикера должен быть положительным")
	}
	return nil
}

func (r MatchRule) validate() error {
	const methodCtx = "videolike/MatchRule.validate"

	if r == (MatchRule{}) {
		return nil
	}
	if r.MinMatchedFrames <= 0 {
		return apperrors.New(methodCtx, "videolike-матчер: минимальное количество совпавших кадров должно быть положительным")
	}
	if r.MinMatchedRatio <= 0 || r.MinMatchedRatio > 1 {
		return apperrors.New(methodCtx, "videolike-матчер: минимальная доля совпавших кадров должна быть от 0 до 1")
	}

	return nil
}

func contentDuration(content domain.Content) time.Duration {
	return time.Duration(content.DurationSec) * time.Second
}

func isWebMFile(filePath string) bool {
	return strings.EqualFold(filepath.Ext(filePath), ".webm")
}
