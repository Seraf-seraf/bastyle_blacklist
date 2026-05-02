package main

import (
	"log"
	"os"

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

	workers := 5
	blacklist := memory.NewBlacklistStore(500)
	actions := telegram.NewBotActions(bot)
	admin := telegram.NewAdminChecker(bot)

	service := moderation.NewService(blacklist, admin, actions)

	jobs := make(chan Job, 100)
	defer close(jobs)

	for i := 0; i < workers; i++ {
		go worker(jobs, service)
	}

	for update := range updates {
		jobs <- Job{Update: update}
	}
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
