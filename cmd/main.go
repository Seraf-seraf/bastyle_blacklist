package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/adapters/matching/composite"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/adapters/matching/exact"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/adapters/matching/imagehash"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/adapters/media"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/adapters/telegram"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/app/moderation"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/config"
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

	bot, err := tgbotapi.NewBotAPI(cfg.Telegram.Token)
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

	exactMatcher := exact.NewMatcher(cfg.Matching.Exact.Buffer)
	mediaDownloader := telegram.NewFileDownloader(bot)
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
	contentMatcher := composite.NewMatcher(exactMatcher, imageHashMatcher)
	actions := telegram.NewBotActions(bot)
	admin := telegram.NewAdminChecker(bot)

	service := moderation.NewService(contentMatcher, admin, actions)

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

func worker(ctx context.Context, jobs <-chan Job, service *moderation.Service) {
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
