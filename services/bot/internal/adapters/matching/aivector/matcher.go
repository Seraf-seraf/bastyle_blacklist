package aivector

import (
	"bytes"
	"context"
	"image/png"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/app/ports"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/domain"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/pkg/apperrors"
)

type Matcher struct {
	downloader     ports.MediaDownloader
	imageExtractor ports.MediaExtractor
	videoExtractor ports.MediaExtractor
	client         Client
	threshold      float64
	topK           int
	plan           domain.MediaExtractionPlan
	limits         Limits
	rule           MatchRule
}

type Options struct {
	Downloader     ports.MediaDownloader
	ImageExtractor ports.MediaExtractor
	VideoExtractor ports.MediaExtractor
	Client         Client
	Threshold      float64
	TopK           int
	Plan           domain.MediaExtractionPlan
	Limits         Limits
	Rule           MatchRule
}

type Limits struct {
	MaxAnimationDuration    time.Duration
	MaxVideoStickerDuration time.Duration
	MaxAnimationSize        int64
	MaxVideoStickerSize     int64
}

type MatchRule struct {
	MinMatchedFrames int
	MinMatchedRatio  float64
}

func NewMatcher(options Options) (*Matcher, error) {
	const methodCtx = "aivector/NewMatcher"

	if err := validateOptions(options); err != nil {
		return nil, apperrors.Wrap(methodCtx, err)
	}

	return &Matcher{
		downloader:     options.Downloader,
		imageExtractor: options.ImageExtractor,
		videoExtractor: options.VideoExtractor,
		client:         options.Client,
		threshold:      options.Threshold,
		topK:           options.TopK,
		plan:           options.Plan,
		limits:         options.Limits,
		rule:           options.Rule,
	}, nil
}

func (m *Matcher) Block(ctx context.Context, chatID int64, content domain.Content) error {
	const methodCtx = "aivector/Matcher.Block"

	frames, supported, err := m.framesForContent(ctx, content)
	if err != nil {
		return apperrors.Wrap(methodCtx, err)
	}
	if !supported {
		return nil
	}

	_, err = m.client.Ban(ctx, BanRequest{
		ChatID:       chatID,
		FileUniqueID: content.FileUniqueID,
		MediaType:    string(content.Type),
		Frames:       frames,
	})
	return apperrors.Wrap(methodCtx, err)
}

func (m *Matcher) IsBlocked(ctx context.Context, chatID int64, content domain.Content) (bool, error) {
	const methodCtx = "aivector/Matcher.IsBlocked"

	frames, supported, err := m.framesForContent(ctx, content)
	if err != nil {
		return false, apperrors.Wrap(methodCtx, err)
	}
	if !supported {
		return false, nil
	}

	response, err := m.client.Search(ctx, SearchRequest{
		ChatID: chatID,
		TopK:   m.topK,
		Frames: frames,
	})
	if err != nil {
		return false, apperrors.Wrap(methodCtx, err)
	}

	if len(frames) == 1 {
		return staticMatched(response, m.threshold), nil
	}

	return videoLikeMatched(response, len(frames), m.threshold, m.rule), nil
}

func (m *Matcher) framesForContent(ctx context.Context, content domain.Content) ([]FrameFile, bool, error) {
	const methodCtx = "aivector/Matcher.framesForContent"

	if !content.CanDownload() {
		return nil, false, nil
	}

	switch content.Type {
	case domain.MediaPhoto:
		return m.imageFrames(ctx, content)
	case domain.MediaStickerStatic:
		media, err := m.downloader.Download(ctx, content)
		if err != nil {
			return nil, false, apperrors.Wrap(methodCtx, err)
		}
		if isWebMFile(media.FilePath) {
			if err := m.checkVideoSticker(media); err != nil {
				return nil, false, apperrors.Wrap(methodCtx, err)
			}

			frames, err := m.videoFrames(ctx, media)
			return frames, true, apperrors.Wrap(methodCtx, err)
		}

		frames, err := m.extractFrames(ctx, m.imageExtractor, media, domain.MediaExtractionPlan{MaxFrames: 1})
		return frames, true, apperrors.Wrap(methodCtx, err)
	case domain.MediaAnimation:
		if err := m.checkAnimationMetadata(content); err != nil {
			return nil, false, apperrors.Wrap(methodCtx, err)
		}

		media, err := m.downloader.Download(ctx, content)
		if err != nil {
			return nil, false, apperrors.Wrap(methodCtx, err)
		}
		if err := m.checkAnimationFile(media); err != nil {
			return nil, false, apperrors.Wrap(methodCtx, err)
		}

		frames, err := m.videoFrames(ctx, media)
		return frames, true, apperrors.Wrap(methodCtx, err)
	default:
		return nil, false, nil
	}
}

func (m *Matcher) imageFrames(ctx context.Context, content domain.Content) ([]FrameFile, bool, error) {
	const methodCtx = "aivector/Matcher.imageFrames"

	media, err := m.downloader.Download(ctx, content)
	if err != nil {
		return nil, false, apperrors.Wrap(methodCtx, err)
	}
	if isVideoFile(media.FilePath) {
		return nil, false, nil
	}

	frames, err := m.extractFrames(ctx, m.imageExtractor, media, domain.MediaExtractionPlan{MaxFrames: 1})
	return frames, true, apperrors.Wrap(methodCtx, err)
}

func (m *Matcher) videoFrames(ctx context.Context, media domain.MediaFile) ([]FrameFile, error) {
	const methodCtx = "aivector/Matcher.videoFrames"

	frames, err := m.extractFrames(ctx, m.videoExtractor, media, m.plan)
	return frames, apperrors.Wrap(methodCtx, err)
}

func (m *Matcher) extractFrames(
	ctx context.Context,
	extractor ports.MediaExtractor,
	media domain.MediaFile,
	plan domain.MediaExtractionPlan,
) ([]FrameFile, error) {
	const methodCtx = "aivector/Matcher.extractFrames"

	extracted, err := extractor.Extract(ctx, media, plan)
	if err != nil {
		return nil, apperrors.Wrap(methodCtx, err)
	}
	if len(extracted.Frames) == 0 {
		return nil, apperrors.New(methodCtx, "AI-vector матчер: извлекатель не вернул кадров")
	}

	frames := make([]FrameFile, 0, len(extracted.Frames))
	for _, frame := range extracted.Frames {
		var buffer bytes.Buffer
		if err := png.Encode(&buffer, frame.Image); err != nil {
			return nil, apperrors.Wrap(methodCtx, err)
		}

		frames = append(frames, FrameFile{
			Name: frameFileName(frame.Index),
			Data: buffer.Bytes(),
		})
	}

	return frames, nil
}

func staticMatched(response SearchResponse, threshold float64) bool {
	for _, frame := range response.Frames {
		for _, hit := range frame.Hits {
			if hit.Score >= threshold {
				return true
			}
		}
	}

	return false
}

func videoLikeMatched(response SearchResponse, checkedFrames int, threshold float64, rule MatchRule) bool {
	if checkedFrames <= 0 {
		return false
	}

	matchedByBan := make(map[int]int)
	for _, frame := range response.Frames {
		matchedInFrame := make(map[int]bool)
		for _, hit := range frame.Hits {
			if hit.Score < threshold {
				continue
			}
			matchedInFrame[hit.BanID] = true
		}
		for banID := range matchedInFrame {
			matchedByBan[banID]++
		}
	}

	for _, matchedFrames := range matchedByBan {
		if matchedFrames >= rule.MinMatchedFrames &&
			float64(matchedFrames)/float64(checkedFrames) >= rule.MinMatchedRatio {
			return true
		}
	}

	return false
}

func validateOptions(options Options) error {
	const methodCtx = "aivector/validateOptions"

	if options.Downloader == nil {
		return apperrors.New(methodCtx, "AI-vector матчер: загрузчик не настроен")
	}
	if options.ImageExtractor == nil {
		return apperrors.New(methodCtx, "AI-vector матчер: извлекатель изображений не настроен")
	}
	if options.VideoExtractor == nil {
		return apperrors.New(methodCtx, "AI-vector матчер: извлекатель видео не настроен")
	}
	if options.Client == nil {
		return apperrors.New(methodCtx, "AI-vector матчер: клиент не настроен")
	}
	if options.Threshold < 0 || options.Threshold > 1 {
		return apperrors.New(methodCtx, "AI-vector матчер: порог должен быть от 0 до 1")
	}
	if options.TopK <= 0 {
		return apperrors.New(methodCtx, "AI-vector матчер: top_k должен быть положительным")
	}
	if options.Plan.MaxFrames <= 0 {
		return apperrors.New(methodCtx, "AI-vector матчер: максимальное количество кадров должно быть положительным")
	}
	if options.Plan.TargetWidth <= 0 {
		return apperrors.New(methodCtx, "AI-vector матчер: целевая ширина должна быть положительной")
	}
	if options.Plan.TargetHeight <= 0 {
		return apperrors.New(methodCtx, "AI-vector матчер: целевая высота должна быть положительной")
	}
	if err := options.Limits.validate(); err != nil {
		return apperrors.Wrap(methodCtx, err)
	}
	if err := options.Rule.validate(); err != nil {
		return apperrors.Wrap(methodCtx, err)
	}

	return nil
}

func (l Limits) validate() error {
	const methodCtx = "aivector/Limits.validate"

	if l.MaxAnimationDuration <= 0 {
		return apperrors.New(methodCtx, "AI-vector матчер: максимальная длительность анимации должна быть положительной")
	}
	if l.MaxVideoStickerDuration <= 0 {
		return apperrors.New(methodCtx, "AI-vector матчер: максимальная длительность видеостикера должна быть положительной")
	}
	if l.MaxAnimationSize <= 0 {
		return apperrors.New(methodCtx, "AI-vector матчер: максимальный размер анимации должен быть положительным")
	}
	if l.MaxVideoStickerSize <= 0 {
		return apperrors.New(methodCtx, "AI-vector матчер: максимальный размер видеостикера должен быть положительным")
	}

	return nil
}

func (r MatchRule) validate() error {
	const methodCtx = "aivector/MatchRule.validate"

	if r.MinMatchedFrames <= 0 {
		return apperrors.New(methodCtx, "AI-vector матчер: минимальное количество совпавших кадров должно быть положительным")
	}
	if r.MinMatchedRatio <= 0 || r.MinMatchedRatio > 1 {
		return apperrors.New(methodCtx, "AI-vector матчер: минимальная доля совпавших кадров должна быть от 0 до 1")
	}

	return nil
}

func (m *Matcher) checkAnimationMetadata(content domain.Content) error {
	const methodCtx = "aivector/Matcher.checkAnimationMetadata"

	if time.Duration(content.DurationSec)*time.Second > m.limits.MaxAnimationDuration {
		return apperrors.New(methodCtx, "AI-vector матчер: длительность анимации превышает лимит")
	}
	if content.SizeBytes > m.limits.MaxAnimationSize {
		return apperrors.New(methodCtx, "AI-vector матчер: размер анимации превышает лимит")
	}

	return nil
}

func (m *Matcher) checkAnimationFile(media domain.MediaFile) error {
	const methodCtx = "aivector/Matcher.checkAnimationFile"

	if media.Content.SizeBytes > m.limits.MaxAnimationSize {
		return apperrors.New(methodCtx, "AI-vector матчер: размер анимации превышает лимит")
	}
	if int64(len(media.Data)) > m.limits.MaxAnimationSize {
		return apperrors.New(methodCtx, "AI-vector матчер: размер загруженной анимации превышает лимит")
	}

	return nil
}

func (m *Matcher) checkVideoSticker(media domain.MediaFile) error {
	const methodCtx = "aivector/Matcher.checkVideoSticker"

	if time.Duration(media.Content.DurationSec)*time.Second > m.limits.MaxVideoStickerDuration {
		return apperrors.New(methodCtx, "AI-vector матчер: длительность видеостикера превышает лимит")
	}
	if media.Content.SizeBytes > m.limits.MaxVideoStickerSize {
		return apperrors.New(methodCtx, "AI-vector матчер: размер видеостикера превышает лимит")
	}
	if int64(len(media.Data)) > m.limits.MaxVideoStickerSize {
		return apperrors.New(methodCtx, "AI-vector матчер: размер загруженного видеостикера превышает лимит")
	}

	return nil
}

func frameFileName(index int) string {
	return "frame-" + strings.Repeat("0", max(0, 3-len(strconv.Itoa(index)))) + strconv.Itoa(index) + ".png"
}

func isVideoFile(filePath string) bool {
	switch strings.ToLower(filepath.Ext(filePath)) {
	case ".mp4", ".webm", ".mov", ".mkv":
		return true
	default:
		return false
	}
}

func isWebMFile(filePath string) bool {
	return strings.EqualFold(filepath.Ext(filePath), ".webm")
}
