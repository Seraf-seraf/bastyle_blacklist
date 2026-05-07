package aivector

import (
	"bytes"
	"context"
	"errors"
	"image/png"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/app/ports"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/domain"
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
	if err := validateOptions(options); err != nil {
		return nil, err
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

func (m *Matcher) Block(ctx context.Context, content domain.Content) error {
	frames, supported, err := m.framesForContent(ctx, content)
	if err != nil {
		return err
	}
	if !supported {
		return nil
	}

	_, err = m.client.Ban(ctx, BanRequest{
		FileUniqueID: content.FileUniqueID,
		MediaType:    string(content.Type),
		Frames:       frames,
	})
	return err
}

func (m *Matcher) IsBlocked(ctx context.Context, content domain.Content) (bool, error) {
	frames, supported, err := m.framesForContent(ctx, content)
	if err != nil {
		return false, err
	}
	if !supported {
		return false, nil
	}

	response, err := m.client.Search(ctx, SearchRequest{
		TopK:   m.topK,
		Frames: frames,
	})
	if err != nil {
		return false, err
	}

	if len(frames) == 1 {
		return staticMatched(response, m.threshold), nil
	}

	return videoLikeMatched(response, len(frames), m.threshold, m.rule), nil
}

func (m *Matcher) framesForContent(ctx context.Context, content domain.Content) ([]FrameFile, bool, error) {
	if !content.CanDownload() {
		return nil, false, nil
	}

	switch content.Type {
	case domain.MediaPhoto:
		return m.imageFrames(ctx, content)
	case domain.MediaStickerStatic:
		media, err := m.downloader.Download(ctx, content)
		if err != nil {
			return nil, false, err
		}
		if isWebMFile(media.FilePath) {
			if err := m.checkVideoSticker(media); err != nil {
				return nil, false, err
			}

			frames, err := m.videoFrames(ctx, media)
			return frames, true, err
		}

		frames, err := m.extractFrames(ctx, m.imageExtractor, media, domain.MediaExtractionPlan{MaxFrames: 1})
		return frames, true, err
	case domain.MediaAnimation:
		if err := m.checkAnimationMetadata(content); err != nil {
			return nil, false, err
		}

		media, err := m.downloader.Download(ctx, content)
		if err != nil {
			return nil, false, err
		}
		if err := m.checkAnimationFile(media); err != nil {
			return nil, false, err
		}

		frames, err := m.videoFrames(ctx, media)
		return frames, true, err
	default:
		return nil, false, nil
	}
}

func (m *Matcher) imageFrames(ctx context.Context, content domain.Content) ([]FrameFile, bool, error) {
	media, err := m.downloader.Download(ctx, content)
	if err != nil {
		return nil, false, err
	}
	if isVideoFile(media.FilePath) {
		return nil, false, nil
	}

	frames, err := m.extractFrames(ctx, m.imageExtractor, media, domain.MediaExtractionPlan{MaxFrames: 1})
	return frames, true, err
}

func (m *Matcher) videoFrames(ctx context.Context, media domain.MediaFile) ([]FrameFile, error) {
	return m.extractFrames(ctx, m.videoExtractor, media, m.plan)
}

func (m *Matcher) extractFrames(
	ctx context.Context,
	extractor ports.MediaExtractor,
	media domain.MediaFile,
	plan domain.MediaExtractionPlan,
) ([]FrameFile, error) {
	extracted, err := extractor.Extract(ctx, media, plan)
	if err != nil {
		return nil, err
	}
	if len(extracted.Frames) == 0 {
		return nil, errors.New("ai vector matcher extractor returned no frames")
	}

	frames := make([]FrameFile, 0, len(extracted.Frames))
	for _, frame := range extracted.Frames {
		var buffer bytes.Buffer
		if err := png.Encode(&buffer, frame.Image); err != nil {
			return nil, err
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
	if options.Downloader == nil {
		return errors.New("ai vector matcher downloader is not configured")
	}
	if options.ImageExtractor == nil {
		return errors.New("ai vector matcher image extractor is not configured")
	}
	if options.VideoExtractor == nil {
		return errors.New("ai vector matcher video extractor is not configured")
	}
	if options.Client == nil {
		return errors.New("ai vector matcher client is not configured")
	}
	if options.Threshold < 0 || options.Threshold > 1 {
		return errors.New("ai vector matcher threshold must be between 0 and 1")
	}
	if options.TopK <= 0 {
		return errors.New("ai vector matcher top k must be positive")
	}
	if options.Plan.MaxFrames <= 0 {
		return errors.New("ai vector matcher max frames must be positive")
	}
	if options.Plan.TargetWidth <= 0 {
		return errors.New("ai vector matcher target width must be positive")
	}
	if options.Plan.TargetHeight <= 0 {
		return errors.New("ai vector matcher target height must be positive")
	}
	if err := options.Limits.validate(); err != nil {
		return err
	}
	if err := options.Rule.validate(); err != nil {
		return err
	}

	return nil
}

func (l Limits) validate() error {
	if l.MaxAnimationDuration <= 0 {
		return errors.New("ai vector matcher max animation duration must be positive")
	}
	if l.MaxVideoStickerDuration <= 0 {
		return errors.New("ai vector matcher max video sticker duration must be positive")
	}
	if l.MaxAnimationSize <= 0 {
		return errors.New("ai vector matcher max animation size must be positive")
	}
	if l.MaxVideoStickerSize <= 0 {
		return errors.New("ai vector matcher max video sticker size must be positive")
	}

	return nil
}

func (r MatchRule) validate() error {
	if r.MinMatchedFrames <= 0 {
		return errors.New("ai vector matcher min matched frames must be positive")
	}
	if r.MinMatchedRatio <= 0 || r.MinMatchedRatio > 1 {
		return errors.New("ai vector matcher min matched ratio must be between 0 and 1")
	}

	return nil
}

func (m *Matcher) checkAnimationMetadata(content domain.Content) error {
	if time.Duration(content.DurationSec)*time.Second > m.limits.MaxAnimationDuration {
		return errors.New("ai vector matcher: animation duration exceeds limit")
	}
	if content.SizeBytes > m.limits.MaxAnimationSize {
		return errors.New("ai vector matcher: animation size exceeds limit")
	}

	return nil
}

func (m *Matcher) checkAnimationFile(media domain.MediaFile) error {
	if media.Content.SizeBytes > m.limits.MaxAnimationSize {
		return errors.New("ai vector matcher: animation size exceeds limit")
	}
	if int64(len(media.Data)) > m.limits.MaxAnimationSize {
		return errors.New("ai vector matcher: animation downloaded size exceeds limit")
	}

	return nil
}

func (m *Matcher) checkVideoSticker(media domain.MediaFile) error {
	if time.Duration(media.Content.DurationSec)*time.Second > m.limits.MaxVideoStickerDuration {
		return errors.New("ai vector matcher: video sticker duration exceeds limit")
	}
	if media.Content.SizeBytes > m.limits.MaxVideoStickerSize {
		return errors.New("ai vector matcher: video sticker size exceeds limit")
	}
	if int64(len(media.Data)) > m.limits.MaxVideoStickerSize {
		return errors.New("ai vector matcher: video sticker downloaded size exceeds limit")
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
