package telegram

import (
	"errors"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/app/ports"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type botActions struct {
	bot *tgbotapi.BotAPI
}

func NewBotActions(bot *tgbotapi.BotAPI) (ports.MessageActions, error) {
	if bot == nil {
		return nil, errors.New("telegram bot actions bot is not configured")
	}

	return &botActions{
		bot: bot,
	}, nil
}

func (a *botActions) SendMessage(chatID int64, message string) error {
	_, err := a.bot.Send(tgbotapi.NewMessage(chatID, message))
	return err
}

func (a *botActions) DeleteMessage(chatID int64, messageID int) error {
	_, err := a.bot.Request(tgbotapi.NewDeleteMessage(chatID, messageID))
	return err
}
