package telegram

import (
	"github.com/Seraf-seraf/bastyle_blacklist/internal/app/ports"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/pkg/apperrors"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type botActions struct {
	bot *tgbotapi.BotAPI
}

func NewBotActions(bot *tgbotapi.BotAPI) (ports.MessageActions, error) {
	const methodCtx = "telegram/NewBotActions"

	if bot == nil {
		return nil, apperrors.New(methodCtx, "Telegram-бот для действий не настроен")
	}

	return &botActions{
		bot: bot,
	}, nil
}

func (a *botActions) SendMessage(chatID int64, message string) error {
	const methodCtx = "telegram/botActions.SendMessage"

	_, err := a.bot.Send(tgbotapi.NewMessage(chatID, message))
	return apperrors.Wrap(methodCtx, err)
}

func (a *botActions) DeleteMessage(chatID int64, messageID int) error {
	const methodCtx = "telegram/botActions.DeleteMessage"

	_, err := a.bot.Request(tgbotapi.NewDeleteMessage(chatID, messageID))
	return apperrors.Wrap(methodCtx, err)
}
