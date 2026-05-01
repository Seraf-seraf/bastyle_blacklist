package policy

import tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

func IsAdmin(bot *tgbotapi.BotAPI, chatID int64, userID int64) bool {
	cfg := tgbotapi.GetChatMemberConfig{
		ChatConfigWithUser: tgbotapi.ChatConfigWithUser{
			ChatID: chatID,
			UserID: userID,
		},
	}

	member, err := bot.GetChatMember(cfg)
	if err != nil {
		return false
	}

	switch member.Status {
	case "creator", "administrator":
		return true
	default:
		return false
	}
}

func FileUniqueID(msg *tgbotapi.Message) string {
	switch {
	case msg.Photo != nil && len(msg.Photo) > 0:
		return msg.Photo[len(msg.Photo)-1].FileUniqueID
	case msg.Animation != nil:
		return msg.Animation.FileUniqueID
	case msg.Sticker != nil:
		return msg.Sticker.FileUniqueID
	case msg.Video != nil:
		return msg.Video.FileUniqueID
	}

	return ""
}
