package telegram

import (
	"encoding/json"
	"testing"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/domain"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func TestMessageFromTelegramMapsAnimation(t *testing.T) {
	msg := mustTelegramMessage(t, `{
		"message_id": 1,
		"chat": {"id": 10},
		"animation": {
			"file_id": "animation-file",
			"file_unique_id": "animation-unique",
			"file_name": "sample.mp4",
			"mime_type": "video/mp4",
			"file_size": 1234,
			"duration": 2,
			"width": 320,
			"height": 240
		},
		"document": {
			"file_id": "document-duplicate",
			"file_unique_id": "document-unique"
		}
	}`)

	content := MessageFromTelegram(msg).Content
	if content == nil {
		t.Fatal("content is nil")
	}
	if content.Type != domain.MediaAnimation {
		t.Fatalf("content.Type = %q, want %q", content.Type, domain.MediaAnimation)
	}
	if content.FileID != "animation-file" {
		t.Fatalf("content.FileID = %q, want animation file", content.FileID)
	}
}

func TestMessageFromTelegramMapsAnimatedSticker(t *testing.T) {
	msg := mustTelegramMessage(t, `{
		"message_id": 1,
		"chat": {"id": 10},
		"sticker": {
			"file_id": "animated-sticker-file",
			"file_unique_id": "animated-sticker-unique",
			"width": 512,
			"height": 512,
			"is_animated": true
		}
	}`)

	content := MessageFromTelegram(msg).Content
	if content == nil {
		t.Fatal("content is nil")
	}
	if content.Type != domain.MediaStickerAnimated {
		t.Fatalf("content.Type = %q, want %q", content.Type, domain.MediaStickerAnimated)
	}
}

func TestMessageFromTelegramMapsNonAnimatedStickerByFileTypeLater(t *testing.T) {
	msg := mustTelegramMessage(t, `{
		"message_id": 1,
		"chat": {"id": 10},
		"sticker": {
			"file_id": "sticker-file",
			"file_unique_id": "sticker-unique",
			"width": 512,
			"height": 512,
			"is_animated": false,
			"is_video": true,
			"file_size": 257643
		}
	}`)

	content := MessageFromTelegram(msg).Content
	if content == nil {
		t.Fatal("content is nil")
	}
	if content.Type != domain.MediaStickerStatic {
		t.Fatalf("content.Type = %q, want %q", content.Type, domain.MediaStickerStatic)
	}
}

func TestMessageFromTelegramIgnoresVideo(t *testing.T) {
	msg := mustTelegramMessage(t, `{
		"message_id": 1,
		"chat": {"id": 10},
		"video": {
			"file_id": "video-file",
			"file_unique_id": "video-unique",
			"mime_type": "video/mp4",
			"file_size": 123456,
			"duration": 60,
			"width": 1280,
			"height": 720
		}
	}`)

	content := MessageFromTelegram(msg).Content
	if content != nil {
		t.Fatalf("content = %#v, want nil", content)
	}
}

func TestMessageFromTelegramParsesBotCommand(t *testing.T) {
	msg := mustTelegramMessage(t, `{
		"message_id": 1,
		"chat": {"id": 10},
		"text": "/ban@bastyle_bot",
		"entities": [{"offset": 0, "length": 16, "type": "bot_command"}]
	}`)

	message := MessageFromTelegram(msg)
	if message.Command != "ban" {
		t.Fatalf("message.Command = %q, want ban", message.Command)
	}
}

func mustTelegramMessage(t *testing.T, data string) *tgbotapi.Message {
	t.Helper()

	var msg tgbotapi.Message
	if err := json.Unmarshal([]byte(data), &msg); err != nil {
		t.Fatal(err)
	}

	return &msg
}
