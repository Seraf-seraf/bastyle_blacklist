package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/adapters/memory"
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
	blacklist := memory.NewBlacklistStore(500)
	actions := telegram.NewBotActions(bot)
	admin := telegram.NewAdminChecker(bot)

	service := moderation.NewService(blacklist, admin, actions)

	jobs := make(chan Job, 100)
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			worker(jobs, service)
		}()
	}

	for update := range updates {
		jobs <- Job{Update: update}
	}

	close(jobs)
	wg.Wait()
	log.Println("Shutdown complete")
}

func worker(jobs <-chan Job, service *moderation.Service) {
	for job := range jobs {
		msg := job.Update.Message
		if msg == nil {
			continue
		}

		message := telegram.MessageFromTelegram(msg)
		if err := service.HandleMessage(message); err != nil {
			log.Printf("[ERROR]: %s", err)
		}
	}
}
