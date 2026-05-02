package domain

type Message struct {
	ID           int
	ChatID       int64
	SenderID     int64
	Command      string
	FileUniqueID string
	ReplyTo      *Message
}

func (m Message) IsCommand() bool {
	return m.Command != ""
}
