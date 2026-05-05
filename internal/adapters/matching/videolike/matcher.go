package videolike

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"time"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/app/ports"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/domain"
)

type matcher struct {
	downloader ports.MediaDownloader
	extractor  ports.MediaExtractor
	threshold  int
	plan       domain.MediaExtractionPlan
	index      *LinearIndex
	store      *sqliteStore
	limits     Limits
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
	if err := validateMatcherConfig(downloader, extractor, threshold, buffer, plan, limits, rule); err != nil {
		return nil, err
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

func NewSQLiteMatcher(
	ctx context.Context,
	downloader ports.MediaDownloader,
	extractor ports.MediaExtractor,
	threshold int,
	buffer int,
	dbPath string,
	plan domain.MediaExtractionPlan,
	limits Limits,
	rule MatchRule,
) (*matcher, error) {
	if dbPath == "" {
		return nil, errors.New("videolike matcher db path is not configured")
	}
	if err := validateMatcherConfig(downloader, extractor, threshold, buffer, plan, limits, rule); err != nil {
		return nil, err
	}

	store, err := OpenSQLiteStore(ctx, dbPath)
	if err != nil {
		return nil, err
	}

	storedHashes, err := store.load(ctx)
	if err != nil {
		_ = store.close()
		return nil, err
	}

	matcher, err := NewMatcher(downloader, extractor, threshold, buffer+len(storedHashes), plan, limits, rule)
	if err != nil {
		_ = store.close()
		return nil, err
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
	if downloader == nil {
		return errors.New("videolike matcher downloader is not configured")
	}
	if extractor == nil {
		return errors.New("videolike matcher extractor is not configured")
	}
	if threshold < 0 {
		return errors.New("videolike matcher threshold must be non-negative")
	}
	if buffer < 0 {
		return errors.New("videolike matcher buffer must be non-negative")
	}
	if plan.MaxFrames <= 0 {
		return errors.New("videolike matcher max frames must be positive")
	}
	if plan.TargetWidth <= 0 {
		return errors.New("videolike matcher target width must be positive")
	}
	if plan.TargetHeight <= 0 {
		return errors.New("videolike matcher target height must be positive")
	}
	if err := limits.validate(); err != nil {
		return err
	}
	if err := rule.validate(); err != nil {
		return err
	}

	return nil
}

func (m *matcher) Close() error {
	if m.store == nil {
		return nil
	}

	return m.store.close()
}

func (m *matcher) IsBlocked(ctx context.Context, content domain.Content) (bool, error) {
	media, supported, err := m.mediaForContent(ctx, content)
	if err != nil {
		return false, err
	}
	if !supported {
		return false, nil
	}

	fingerprint, err := m.fingerprint(ctx, media)
	if err != nil {
		return false, err
	}

	_, matched := m.index.Search(fingerprint, m.threshold)
	return matched, nil
}

func (m *matcher) Block(ctx context.Context, content domain.Content) error {
	media, supported, err := m.mediaForContent(ctx, content)
	if err != nil {
		return err
	}
	if !supported {
		return nil
	}
	if m.store == nil {
		return errors.New("videolike store is not configured")
	}

	fingerprint, err := m.fingerprint(ctx, media)
	if err != nil {
		return err
	}

	id, err := m.store.insert(ctx, fingerprint)
	if err != nil {
		return err
	}
	fingerprint.ID = id

	m.index.Add(fingerprint)
	return nil
}

func (m *matcher) supports(ctx context.Context, content domain.Content) (bool, error) {
	_, supported, err := m.mediaForContent(ctx, content)
	return supported, err
}

func (m *matcher) mediaForContent(ctx context.Context, content domain.Content) (domain.MediaFile, bool, error) {
	if !content.CanDownload() {
		return domain.MediaFile{}, false, nil
	}

	switch content.Type {
	case domain.MediaAnimation:
		if err := m.checkAnimationMetadata(content); err != nil {
			return domain.MediaFile{}, false, err
		}

		media, err := m.downloader.Download(ctx, content)
		if err != nil {
			return domain.MediaFile{}, false, err
		}
		if err := m.checkAnimationFile(media); err != nil {
			return domain.MediaFile{}, false, err
		}

		return media, true, nil
	case domain.MediaStickerStatic:
		if err := m.checkVideoStickerMetadata(content); err != nil {
			return domain.MediaFile{}, false, err
		}

		media, err := m.downloader.Download(ctx, content)
		if err != nil {
			return domain.MediaFile{}, false, err
		}
		if !isWebMFile(media.FilePath) {
			return domain.MediaFile{}, false, nil
		}

		if err := m.checkVideoStickerFile(media); err != nil {
			return domain.MediaFile{}, false, err
		}

		return media, true, nil
	default:
		return domain.MediaFile{}, false, nil
	}
}

func (m *matcher) checkAnimationMetadata(content domain.Content) error {
	if contentDuration(content) > m.limits.MaxAnimationDuration {
		return errors.New("videolike matcher: animation duration exceeds limit")
	}
	if content.SizeBytes > m.limits.MaxAnimationSize {
		return errors.New("videolike matcher: animation size exceeds limit")
	}

	return nil
}

func (m *matcher) checkAnimationFile(media domain.MediaFile) error {
	if media.Content.SizeBytes > m.limits.MaxAnimationSize {
		return errors.New("videolike matcher: animation size exceeds limit")
	}
	if int64(len(media.Data)) > m.limits.MaxAnimationSize {
		return errors.New("videolike matcher: animation downloaded size exceeds limit")
	}

	return nil
}

func (m *matcher) checkVideoStickerMetadata(content domain.Content) error {
	if contentDuration(content) > m.limits.MaxVideoStickerDuration {
		return errors.New("videolike matcher: video sticker duration exceeds limit")
	}

	return nil
}

func (m *matcher) fingerprint(ctx context.Context, media domain.MediaFile) (StoredVideoLikeHash, error) {
	extracted, err := m.extractor.Extract(ctx, media, m.plan)
	if err != nil {
		return StoredVideoLikeHash{}, err
	}

	return fingerprintVideoLike(media.Content, extracted)
}

func (m *matcher) checkVideoStickerFile(media domain.MediaFile) error {
	if media.Content.SizeBytes > m.limits.MaxVideoStickerSize {
		return errors.New("videolike matcher: video sticker size exceeds limit")
	}
	if int64(len(media.Data)) > m.limits.MaxVideoStickerSize {
		return errors.New("videolike matcher: video sticker downloaded size exceeds limit")
	}

	return nil
}

func (l Limits) validate() error {
	if l.MaxAnimationDuration <= 0 {
		return errors.New("videolike matcher max animation duration must be positive")
	}
	if l.MaxVideoStickerDuration <= 0 {
		return errors.New("videolike matcher max video sticker duration must be positive")
	}
	if l.MaxAnimationSize <= 0 {
		return errors.New("videolike matcher max animation size must be positive")
	}
	if l.MaxVideoStickerSize <= 0 {
		return errors.New("videolike matcher max video sticker size must be positive")
	}
	return nil
}

func (r MatchRule) validate() error {
	if r == (MatchRule{}) {
		return nil
	}
	if r.MinMatchedFrames <= 0 {
		return errors.New("videolike matcher min matched frames must be positive")
	}
	if r.MinMatchedRatio <= 0 || r.MinMatchedRatio > 1 {
		return errors.New("videolike matcher min matched ratio must be between 0 and 1")
	}

	return nil
}

func contentDuration(content domain.Content) time.Duration {
	return time.Duration(content.DurationSec) * time.Second
}

func isWebMFile(filePath string) bool {
	return strings.EqualFold(filepath.Ext(filePath), ".webm")
}
