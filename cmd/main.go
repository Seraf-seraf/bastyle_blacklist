package main

import (
	"log"
	"os"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/adapters/memory"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/app/ports"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/policy"

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
	blacklistStore := memory.NewBlacklistStore(500)
	jobs := make(chan Job, 100)
	defer close(jobs)

	for i := 0; i < workers; i++ {
		go worker(jobs, bot, blacklistStore)
	}

	for update := range updates {
		jobs <- Job{Update: update}
	}
}

func worker(jobs <-chan Job, bot *tgbotapi.BotAPI, blacklistStore ports.BlacklistStore) {
	for job := range jobs {
		msg := job.Update.Message
		if msg == nil {
			continue
		}

		if msg.IsCommand() {

			switch msg.Command() {

			case "hello":
				_, err := bot.Send(tgbotapi.NewMessage(msg.Chat.ID, "Hello, World!"))
				if err != nil {
					log.Printf("[ERROR]: %s", err)
				}

			case "ban":
				if !policy.IsAdmin(bot, msg.Chat.ID, msg.From.ID) {
					_, _ = bot.Send(tgbotapi.NewMessage(msg.Chat.ID, "Команда доступна только админам"))
					continue
				}

				if msg.ReplyToMessage == nil {
					_, _ = bot.Send(tgbotapi.NewMessage(msg.Chat.ID, "Reply на сообщение обязателен"))
					continue
				}

				target := msg.ReplyToMessage

				fileID := policy.FileUniqueID(target)
				if fileID != "" {
					blacklistStore.Block(fileID)
				}

				_, err := bot.Request(
					tgbotapi.NewDeleteMessage(target.Chat.ID, target.MessageID),
				)
				if err != nil {
					log.Printf("[ERROR]: %s", err)
				}
			}

			continue
		}

		targets := [2]*tgbotapi.Message{
			msg, msg.ReplyToMessage,
		}

		for _, target := range targets {
			if target == nil {
				continue
			}

			if id := policy.FileUniqueID(target); id != "" && blacklistStore.IsBlocked(id) {
				_, err := bot.Request(
					tgbotapi.NewDeleteMessage(target.Chat.ID, target.MessageID),
				)
				if err != nil {
					log.Printf("[ERROR]: %s", err)
				}
			}
		}
	}
}
