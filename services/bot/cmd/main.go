package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/adapters/database/postgres"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/adapters/httpclient"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/adapters/matching/aivector"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/adapters/matching/composite"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/adapters/matching/exact"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/adapters/matching/imagehash"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/adapters/matching/videolike"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/adapters/media"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/adapters/telegram"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/app/moderation"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/app/ports"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/config"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/domain"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/pkg/apperrors"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Job struct {
	Update tgbotapi.Update
}

func main() {
	const methodCtx = "cmd/main"

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	configPath := flag.String("config", "infra/config/config.yaml", "путь к YAML-конфигу")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		panicWithContext(methodCtx, err)
	}
	dbPool, err := postgres.NewPool(ctx, cfg.Database)
	if err != nil {
		panicWithContext(methodCtx, err)
	}
	defer func() {
		dbPool.Close()
		log.Println("PostgreSQL pool закрыт")
	}()

	bot, err := newTelegramBot(cfg)
	if err != nil {
		panicWithContext(methodCtx, err)
	}

	log.Printf("Авторизован как %s", bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = cfg.Telegram.UpdateTimeoutSeconds

	updates := bot.GetUpdatesChan(u)
	go func() {
		<-ctx.Done()
		log.Println("Получен сигнал завершения")
		bot.StopReceivingUpdates()
	}()

	exactMatcher, err := exact.NewSQLiteMatcher(ctx, cfg.Matching.Exact.Buffer, cfg.Database.DSN)
	if err != nil {
		panicWithContext(methodCtx, err)
	}
	mediaDownloader, err := telegram.NewFileDownloader(bot)
	if err != nil {
		panicWithContext(methodCtx, err)
	}
	mediaExtractor := media.NewExtractor()
	imageHashMatcher, err := imagehash.NewSQLiteMatcher(
		ctx,
		mediaDownloader,
		mediaExtractor,
		cfg.Matching.ImageHash.Threshold,
		cfg.Matching.ImageHash.Buffer,
		cfg.Database.DSN,
	)
	if err != nil {
		panicWithContext(methodCtx, err)
	}
	defer func() {
		if err := imageHashMatcher.Close(); err != nil {
			logError(methodCtx, err)
		}
	}()

	defer func() {
		if err := exactMatcher.Close(); err != nil {
			logError(methodCtx, err)
		}
	}()

	matchers := []ports.ContentMatcher{exactMatcher, imageHashMatcher}
	videoLikeExtractor, err := media.NewFFmpegFrameExtractor(
		cfg.MediaConfig.FFmpegBinary,
		cfg.MediaConfig.FFmpegTimeout.Value(),
	)
	if err != nil {
		panicWithContext(methodCtx, err)
	}
	videoLikeMatcher, err := videolike.NewSQLiteMatcher(
		ctx,
		mediaDownloader,
		videoLikeExtractor,
		cfg.Matching.VideoLike.Threshold,
		cfg.Matching.VideoLike.Buffer,
		cfg.Database.DSN,
		domain.MediaExtractionPlan{
			MaxFrames:    cfg.MediaConfig.MaxFrames,
			TargetWidth:  cfg.MediaConfig.TargetWidth,
			TargetHeight: cfg.MediaConfig.TargetHeight,
		},
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
		panicWithContext(methodCtx, err)
	}
	defer func() {
		if err := videoLikeMatcher.Close(); err != nil {
			logError(methodCtx, err)
		}
	}()
	matchers = append(matchers, videoLikeMatcher)

	if cfg.Matching.AIVector.Enabled {
		aiVectorClient, err := aivector.NewHTTPClient(
			cfg.Matching.AIVector.Service.URL(),
			cfg.Matching.AIVector.RequestTimeout.Value(),
		)
		if err != nil {
			panicWithContext(methodCtx, err)
		}
		aiVectorExtractor, err := media.NewFFmpegFrameExtractor(
			cfg.MediaConfig.FFmpegBinary,
			cfg.MediaConfig.FFmpegTimeout.Value(),
		)
		if err != nil {
			panicWithContext(methodCtx, err)
		}
		aiVectorMatcher, err := aivector.NewMatcher(aivector.Options{
			Downloader:     mediaDownloader,
			ImageExtractor: mediaExtractor,
			VideoExtractor: aiVectorExtractor,
			Client:         aiVectorClient,
			Threshold:      cfg.Matching.AIVector.Threshold,
			TopK:           cfg.Matching.AIVector.TopK,
			Plan: domain.MediaExtractionPlan{
				MaxFrames:    cfg.MediaConfig.MaxFrames,
				TargetWidth:  cfg.MediaConfig.TargetWidth,
				TargetHeight: cfg.MediaConfig.TargetHeight,
			},
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
		if err != nil {
			panicWithContext(methodCtx, err)
		}
		matchers = append(matchers, aiVectorMatcher)
	}

	contentMatcher, err := composite.NewMatcher(matchers...)
	if err != nil {
		panicWithContext(methodCtx, err)
	}
	actions, err := telegram.NewBotActions(bot)
	if err != nil {
		panicWithContext(methodCtx, err)
	}
	admin, err := telegram.NewAdminChecker(bot)
	if err != nil {
		panicWithContext(methodCtx, err)
	}

	service, err := moderation.NewService(contentMatcher, admin, actions)
	if err != nil {
		panicWithContext(methodCtx, err)
	}

	if cfg.Health.Enabled {
		healthServer, err := startHealthServer(ctx, cfg.Health.Address(), dbPool.Ping)
		if err != nil {
			panicWithContext(methodCtx, err)
		}
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := healthServer.Shutdown(shutdownCtx); err != nil {
				logError(methodCtx, err)
			}
		}()
	}

	jobs := make(chan Job, cfg.JobsBuffer)
	var wg sync.WaitGroup

	for i := 0; i < cfg.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			worker(ctx, jobs, service)
		}()
	}

	for update := range updates {
		select {
		case <-ctx.Done():
			log.Println("Остановка приема обновлений")
			close(jobs)
			wg.Wait()
			log.Println("Завершение работы выполнено")
			return
		case jobs <- Job{Update: update}:
		}
	}

	close(jobs)
	wg.Wait()
	log.Println("Завершение работы выполнено")
}

type readinessCheck func(context.Context) error

func startHealthServer(ctx context.Context, address string, readiness readinessCheck) (*http.Server, error) {
	const methodCtx = "cmd/startHealthServer"

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		writeReadinessStatus(ctx, w, readiness)
	})

	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, apperrors.Wrap(methodCtx, err)
	}

	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 2 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logError(methodCtx, err)
		}
	}()

	go func() {
		log.Printf("Health-сервер слушает %s", address)
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logError(methodCtx, err)
		}
	}()

	return server, nil
}

func writeReadinessStatus(ctx context.Context, w http.ResponseWriter, readiness readinessCheck) {
	const methodCtx = "cmd/writeReadinessStatus"

	if readiness != nil {
		checkCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		if err := readiness(checkCtx); err != nil {
			logError(methodCtx, err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"unavailable"}`))
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func newTelegramBot(cfg config.Config) (*tgbotapi.BotAPI, error) {
	const methodCtx = "cmd/newTelegramBot"

	if !cfg.Telegram.HTTPClient.Enabled {
		bot, err := tgbotapi.NewBotAPI(cfg.Telegram.Token)
		return bot, apperrors.Wrap(methodCtx, err)
	}

	client, err := httpclient.New(httpclient.Options{
		ProxyURL: cfg.Telegram.HTTPClient.ProxyURL,
	})
	if err != nil {
		return nil, apperrors.Wrap(methodCtx, err)
	}

	bot, err := tgbotapi.NewBotAPIWithClient(cfg.Telegram.Token, tgbotapi.APIEndpoint, client)
	return bot, apperrors.Wrap(methodCtx, err)
}

type moderationService interface {
	HandleMessage(ctx context.Context, msg domain.Message) error
}

func worker(ctx context.Context, jobs <-chan Job, service moderationService) {
	const methodCtx = "cmd/worker"

	for {
		select {
		case <-ctx.Done():
			return
		case job, ok := <-jobs:
			if !ok {
				return
			}

			msg := job.Update.Message
			if msg == nil {
				continue
			}

			message := telegram.MessageFromTelegram(msg)
			if err := service.HandleMessage(ctx, message); err != nil {
				logError(methodCtx, err)
			}
		}
	}
}

func logError(methodCtx string, err error) {
	log.Printf("[ERROR]: %s: %s", methodCtx, err)
}

func panicWithContext(methodCtx string, err error) {
	log.Panicf("[ERROR]: %s: %s", methodCtx, err)
}
