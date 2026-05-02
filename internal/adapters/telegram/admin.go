package telegram

import (
	"fmt"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type AdminChecker struct {
	bot *tgbotapi.BotAPI
}

func NewAdminChecker(bot *tgbotapi.BotAPI) *AdminChecker {
	return &AdminChecker{
		bot: bot,
	}
}

func (a *AdminChecker) IsAdmin(chatID int64, userID int64) (bool, error) {
	cfg := tgbotapi.GetChatMemberConfig{
		ChatConfigWithUser: tgbotapi.ChatConfigWithUser{
			ChatID: chatID,
			UserID: userID,
		},
	}

	member, err := a.bot.GetChatMember(cfg)
	if err != nil {
		return false, fmt.Errorf("[ERROR]: %w", err)
	}

	return member.Status == "administrator" || member.Status == "creator", nil
}
