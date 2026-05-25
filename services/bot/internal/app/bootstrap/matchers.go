package bootstrap

import (
	"context"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/adapters/database/postgres"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/adapters/matching/aivector"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/adapters/matching/exact"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/adapters/matching/imagehash"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/adapters/matching/videolike"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/adapters/media"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/adapters/telegram"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/app/ports"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/config"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/domain"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/pkg/apperrors"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func buildMatchers(
	ctx context.Context,
	cfg config.Config,
	dbPool *postgres.Pool,
	bot *tgbotapi.BotAPI,
) (matchers []ports.ContentBlockMatcher, err error) {
	const methodCtx = "bootstrap/buildMatchers"
	defer func() {
		if err != nil {
			_ = closeMatchers(matchers)
		}
	}()

	exactMatcher, err := exact.NewPostgresMatcher(ctx, dbPool.Raw(), cfg.Matching.Exact.Buffer)
	if err != nil {
		return nil, apperrors.Wrap(methodCtx, err)
	}
	matchers = append(matchers, exactMatcher)

	mediaDownloader, err := telegram.NewFileDownloader(bot)
	if err != nil {
		return nil, apperrors.Wrap(methodCtx, err)
	}
	mediaExtractor := media.NewExtractor()
	imageHashMatcher, err := imagehash.NewPostgresMatcher(
		ctx,
		dbPool.Raw(),
		mediaDownloader,
		mediaExtractor,
		cfg.Matching.ImageHash.Threshold,
		cfg.Matching.ImageHash.Buffer,
	)
	if err != nil {
		return nil, apperrors.Wrap(methodCtx, err)
	}
	matchers = append(matchers, imageHashMatcher)

	videoLikeExtractor, err := media.NewFFmpegFrameExtractor(
		cfg.MediaConfig.FFmpegBinary,
		cfg.MediaConfig.FFmpegTimeout.Value(),
	)
	if err != nil {
		return nil, apperrors.Wrap(methodCtx, err)
	}
	videoLikeMatcher, err := videolike.NewPostgresMatcher(
		ctx,
		dbPool.Raw(),
		mediaDownloader,
		videoLikeExtractor,
		cfg.Matching.VideoLike.Threshold,
		cfg.Matching.VideoLike.Buffer,
		mediaPlan(cfg),
		videolike.Limits{
			MaxAnimationDuration:    cfg.MediaConfig.MaxAnimationDuration.Value(),
			MaxVideoStickerDuration: cfg.MediaConfig.MaxVideoStickerDuration.Value(),
			MaxAnimationSize:        cfg.MediaConfig.MaxAnimationSize.Bytes(),
			MaxVideoStickerSize:     cfg.MediaConfig.MaxVideoStickerSize.Bytes(),
		},
		videolike.MatchRule{
			MinMatchedFrames: cfg.Matching.VideoLike.MinMatchedFrames,
			MinMatchedRatio:  cfg.Matching.VideoLike.MinMatchedRatio,
		},
	)
	if err != nil {
		return nil, apperrors.Wrap(methodCtx, err)
	}
	matchers = append(matchers, videoLikeMatcher)

	if !cfg.Matching.AIVector.Enabled {
		return matchers, nil
	}

	aiVectorMatcher, err := buildAIVectorMatcher(cfg, mediaDownloader, mediaExtractor)
	if err != nil {
		return nil, apperrors.Wrap(methodCtx, err)
	}
	return append(matchers, aiVectorMatcher), nil
}

func buildAIVectorMatcher(
	cfg config.Config,
	mediaDownloader ports.MediaDownloader,
	mediaExtractor *media.Extractor,
) (ports.ContentBlockMatcher, error) {
	const methodCtx = "bootstrap/buildAIVectorMatcher"

	aiVectorClient, err := aivector.NewHTTPClient(
		cfg.Matching.AIVector.Service.URL(),
		cfg.Matching.AIVector.RequestTimeout.Value(),
	)
	if err != nil {
		return nil, apperrors.Wrap(methodCtx, err)
	}
	aiVectorExtractor, err := media.NewFFmpegFrameExtractor(
		cfg.MediaConfig.FFmpegBinary,
		cfg.MediaConfig.FFmpegTimeout.Value(),
	)
	if err != nil {
		return nil, apperrors.Wrap(methodCtx, err)
	}
	return aivector.NewMatcher(aivector.Options{
		Downloader:          mediaDownloader,
		ImageFrameExtractor: mediaExtractor,
		VideoFrameExtractor: aiVectorExtractor,
		Client:              aiVectorClient,
		ModelName:           cfg.Matching.AIVector.ModelName,
		ModelRevision:       cfg.Matching.AIVector.ModelRevision,
		Threshold:           cfg.Matching.AIVector.Threshold,
		TopK:                cfg.Matching.AIVector.TopK,
		Plan:                mediaPlan(cfg),
		Limits: aivector.Limits{
			MaxAnimationDuration:    cfg.MediaConfig.MaxAnimationDuration.Value(),
			MaxVideoStickerDuration: cfg.MediaConfig.MaxVideoStickerDuration.Value(),
			MaxAnimationSize:        cfg.MediaConfig.MaxAnimationSize.Bytes(),
			MaxVideoStickerSize:     cfg.MediaConfig.MaxVideoStickerSize.Bytes(),
		},
		Rule: aivector.MatchRule{
			MinMatchedFrames: cfg.Matching.AIVector.MinMatchedFrames,
			MinMatchedRatio:  cfg.Matching.AIVector.MinMatchedRatio,
		},
	})
}

func mediaPlan(cfg config.Config) domain.MediaExtractionPlan {
	return domain.MediaExtractionPlan{
		MaxFrames:    cfg.MediaConfig.MaxFrames,
		TargetWidth:  cfg.MediaConfig.TargetWidth,
		TargetHeight: cfg.MediaConfig.TargetHeight,
	}
}
