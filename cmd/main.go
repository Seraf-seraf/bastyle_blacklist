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

	"github.com/Seraf-seraf/bastyle_blacklist/internal/adapters/httpclient"
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
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Job struct {
	Update tgbotapi.Update
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	configPath := flag.String("config", "configs/config.yaml", "path to yaml config")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Panic(err)
	}

	bot, err := newTelegramBot(cfg)
	if err != nil {
		log.Panic(err)
	}

	log.Printf("Authorized as %s", bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = cfg.Telegram.UpdateTimeoutSeconds

	updates := bot.GetUpdatesChan(u)
	go func() {
		<-ctx.Done()
		log.Println("Shutdown signal received")
		bot.StopReceivingUpdates()
	}()

	exactMatcher, err := exact.NewMatcher(cfg.Matching.Exact.Buffer)
	if err != nil {
		log.Panic(err)
	}
	mediaDownloader, err := telegram.NewFileDownloader(bot)
	if err != nil {
		log.Panic(err)
	}
	mediaExtractor := media.NewExtractor()
	imageHashMatcher, err := imagehash.NewSQLiteMatcher(
		ctx,
		mediaDownloader,
		mediaExtractor,
		cfg.Matching.ImageHash.Threshold,
		cfg.Matching.ImageHash.Buffer,
		cfg.Matching.ImageHash.DBPath,
	)
	if err != nil {
		log.Panic(err)
	}
	defer func() {
		if err := imageHashMatcher.Close(); err != nil {
			log.Printf("[ERROR]: %s", err)
		}
	}()

	matchers := []ports.ContentMatcher{exactMatcher, imageHashMatcher}
	if cfg.Matching.VideoLike.Enabled {
		videoLikeExtractor, err := media.NewFFmpegFrameExtractor(
			cfg.Matching.VideoLike.FFmpegBinary,
			cfg.Matching.VideoLike.FFmpegTimeout.Value(),
		)
		if err != nil {
			log.Panic(err)
		}
		videoLikeMatcher, err := videolike.NewSQLiteMatcher(
			ctx,
			mediaDownloader,
			videoLikeExtractor,
			cfg.Matching.VideoLike.Threshold,
			cfg.Matching.VideoLike.Buffer,
			cfg.Matching.VideoLike.DBPath,
			domain.MediaExtractionPlan{
				MaxFrames:    cfg.Matching.VideoLike.MaxFrames,
				TargetWidth:  cfg.Matching.VideoLike.TargetWidth,
				TargetHeight: cfg.Matching.VideoLike.TargetHeight,
			},
			videolike.Limits{
				MaxAnimationDuration:    cfg.Matching.VideoLike.MaxAnimationDuration.Value(),
				MaxVideoStickerDuration: cfg.Matching.VideoLike.MaxVideoStickerDuration.Value(),
				MaxAnimationSize:        cfg.Matching.VideoLike.MaxAnimationSize.Bytes(),
				MaxVideoStickerSize:     cfg.Matching.VideoLike.MaxVideoStickerSize.Bytes(),
			},
			videolike.MatchRule{
				MinMatchedFrames: cfg.Matching.VideoLike.MinMatchedFrames,
				MinMatchedRatio:  cfg.Matching.VideoLike.MinMatchedRatio,
			},
		)
		if err != nil {
			log.Panic(err)
		}
		defer func() {
			if err := videoLikeMatcher.Close(); err != nil {
				log.Printf("[ERROR]: %s", err)
			}
		}()
		matchers = append(matchers, videoLikeMatcher)
	}

	contentMatcher, err := composite.NewMatcher(matchers...)
	if err != nil {
		log.Panic(err)
	}
	actions, err := telegram.NewBotActions(bot)
	if err != nil {
		log.Panic(err)
	}
	admin, err := telegram.NewAdminChecker(bot)
	if err != nil {
		log.Panic(err)
	}

	service, err := moderation.NewService(contentMatcher, admin, actions)
	if err != nil {
		log.Panic(err)
	}

	if cfg.Health.Enabled {
		healthServer, err := startHealthServer(ctx, cfg.Health.Address)
		if err != nil {
			log.Panic(err)
		}
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := healthServer.Shutdown(shutdownCtx); err != nil {
				log.Printf("[ERROR]: %s", err)
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
		jobs <- Job{Update: update}
	}

	close(jobs)
	wg.Wait()
	log.Println("Shutdown complete")
}

func startHealthServer(ctx context.Context, address string) (*http.Server, error) {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
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
			log.Printf("[ERROR]: %s", err)
		}
	}()

	go func() {
		log.Printf("Health server listening on %s", address)
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("[ERROR]: %s", err)
		}
	}()

	return server, nil
}

func newTelegramBot(cfg config.Config) (*tgbotapi.BotAPI, error) {
	if !cfg.Telegram.HTTPClient.Enabled {
		return tgbotapi.NewBotAPI(cfg.Telegram.Token)
	}

	client, err := httpclient.New(httpclient.Options{
		ProxyURL: cfg.Telegram.HTTPClient.ProxyURL,
	})
	if err != nil {
		return nil, err
	}

	return tgbotapi.NewBotAPIWithClient(cfg.Telegram.Token, tgbotapi.APIEndpoint, client)
}

type moderationService interface {
	HandleMessage(ctx context.Context, msg domain.Message) error
}

func worker(ctx context.Context, jobs <-chan Job, service moderationService) {
	for job := range jobs {
		msg := job.Update.Message
		if msg == nil {
			continue
		}

		message := telegram.MessageFromTelegram(msg)
		if err := service.HandleMessage(ctx, message); err != nil {
			log.Printf("[ERROR]: %s", err)
		}
	}
}
