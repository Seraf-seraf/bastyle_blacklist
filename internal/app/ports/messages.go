package ports

type MessageActions interface {
	SendMessage(chatID int64, text string) error
	DeleteMessage(chatID int64, messageID int) error
}
