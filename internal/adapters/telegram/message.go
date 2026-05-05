package telegram

import (
	"github.com/Seraf-seraf/bastyle_blacklist/internal/domain"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func MessageFromTelegram(msg *tgbotapi.Message) domain.Message {
	content := contentFromTelegram(msg)
	message := domain.Message{
		ID:           msg.MessageID,
		ChatID:       msg.Chat.ID,
		Command:      msg.Command(),
		FileUniqueID: content.FileUniqueID,
	}

	if !content.IsZero() {
		message.Content = &content
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

func contentFromTelegram(msg *tgbotapi.Message) domain.Content {
	switch {
	case msg.Photo != nil && len(msg.Photo) > 0:
		photo := msg.Photo[len(msg.Photo)-1]
		return domain.Content{
			FileID:       photo.FileID,
			FileUniqueID: photo.FileUniqueID,
			Type:         domain.MediaPhoto,
			SizeBytes:    int64(photo.FileSize),
			Width:        photo.Width,
			Height:       photo.Height,
		}
	case msg.Animation != nil:
		return domain.Content{
			FileID:       msg.Animation.FileID,
			FileUniqueID: msg.Animation.FileUniqueID,
			Type:         domain.MediaAnimation,
			MimeType:     msg.Animation.MimeType,
			SizeBytes:    int64(msg.Animation.FileSize),
			DurationSec:  msg.Animation.Duration,
			Width:        msg.Animation.Width,
			Height:       msg.Animation.Height,
		}
	case msg.Sticker != nil:
		mediaType := domain.MediaStickerStatic
		if msg.Sticker.IsAnimated {
			mediaType = domain.MediaStickerAnimated
		}

		return domain.Content{
			FileID:       msg.Sticker.FileID,
			FileUniqueID: msg.Sticker.FileUniqueID,
			Type:         mediaType,
			SizeBytes:    int64(msg.Sticker.FileSize),
			Width:        msg.Sticker.Width,
			Height:       msg.Sticker.Height,
		}
	}

	return domain.Content{}
}
