package telegram

import (
	"errors"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/app/ports"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type adminChecker struct {
	bot *tgbotapi.BotAPI
}

func NewAdminChecker(bot *tgbotapi.BotAPI) (ports.AdminChecker, error) {
	if bot == nil {
		return nil, errors.New("telegram admin checker bot is not configured")
	}

	return &adminChecker{
		bot: bot,
	}, nil
}

func (a *adminChecker) IsAdmin(chatID int64, userID int64) (bool, error) {
	cfg := tgbotapi.GetChatMemberConfig{
		ChatConfigWithUser: tgbotapi.ChatConfigWithUser{
			ChatID: chatID,
			UserID: userID,
		},
	}

	member, err := a.bot.GetChatMember(cfg)
	if err != nil {
		return false, err
	}

	return member.Status == "administrator" || member.Status == "creator", nil
}
