package telegram

import (
	"github.com/Seraf-seraf/bastyle_blacklist/internal/domain"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func MessageFromTelegram(msg *tgbotapi.Message) domain.Message {
	message := domain.Message{
		ID:           msg.MessageID,
		ChatID:       msg.Chat.ID,
		Command:      msg.Command(),
		FileUniqueID: fileUniqueID(msg),
	}

	if msg.From != nil {
		message.SenderID = msg.From.ID
	}

	if msg.ReplyToMessage != nil {
		reply := MessageFromTelegram(msg.ReplyToMessage)
		message.ReplyTo = &reply
	}

	return message
}

func fileUniqueID(msg *tgbotapi.Message) string {
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
