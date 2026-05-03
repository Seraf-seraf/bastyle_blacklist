package main

import (
	"context"
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
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"
)

type Job struct {
	Update tgbotapi.Update
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := godotenv.Load(); err != nil {
		log.Panic("[ERROR]: failed to load .env")
	}

	bot, err := tgbotapi.NewBotAPI(os.Getenv("TOKEN"))
	if err != nil {
		log.Panic(err)
	}

	log.Printf("Authorized as %s", bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)
	go func() {
		<-ctx.Done()
		log.Println("Shutdown signal received")
		bot.StopReceivingUpdates()
	}()

	workers := 5
	exactMatcher := exact.NewMatcher(500)
	mediaDownloader := telegram.NewFileDownloader(bot)
	mediaExtractor := media.NewExtractor()
	imageHashMatcher := imagehash.NewMatcher(mediaDownloader, mediaExtractor, 8, 500)
	contentMatcher := composite.NewMatcher(exactMatcher, imageHashMatcher)
	actions := telegram.NewBotActions(bot)
	admin := telegram.NewAdminChecker(bot)

	service := moderation.NewService(contentMatcher, admin, actions)

	jobs := make(chan Job, 100)
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
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
