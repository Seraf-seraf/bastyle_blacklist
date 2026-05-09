package telegram

import (
	"github.com/Seraf-seraf/bastyle_blacklist/internal/app/ports"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/pkg/apperrors"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type adminChecker struct {
	bot *tgbotapi.BotAPI
}

func NewAdminChecker(bot *tgbotapi.BotAPI) (ports.AdminChecker, error) {
	const methodCtx = "telegram/NewAdminChecker"

	if bot == nil {
		return nil, apperrors.New(methodCtx, "Telegram-бот для проверки админов не настроен")
	}

	return &adminChecker{
		bot: bot,
	}, nil
}

func (a *adminChecker) IsAdmin(chatID int64, userID int64) (bool, error) {
	const methodCtx = "telegram/adminChecker.IsAdmin"

	cfg := tgbotapi.GetChatMemberConfig{
		ChatConfigWithUser: tgbotapi.ChatConfigWithUser{
			ChatID: chatID,
			UserID: userID,
		},
	}

	member, err := a.bot.GetChatMember(cfg)
	if err != nil {
		return false, apperrors.Wrap(methodCtx, err)
	}

	return member.Status == "administrator" || member.Status == "creator", nil
}
