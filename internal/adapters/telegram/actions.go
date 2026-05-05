package telegram

import (
	"errors"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type BotActions struct {
	bot *tgbotapi.BotAPI
}

func NewBotActions(bot *tgbotapi.BotAPI) (*BotActions, error) {
	if bot == nil {
		return nil, errors.New("telegram bot actions bot is not configured")
	}

	return &BotActions{
		bot: bot,
	}, nil
}

func (a *BotActions) SendMessage(chatID int64, message string) error {
	_, err := a.bot.Send(tgbotapi.NewMessage(chatID, message))
	return err
}

func (a *BotActions) DeleteMessage(chatID int64, messageID int) error {
	_, err := a.bot.Request(tgbotapi.NewDeleteMessage(chatID, messageID))
	return err
}
