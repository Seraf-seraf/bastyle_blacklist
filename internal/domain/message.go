package domain

type MediaType string

const (
	MediaPhoto           MediaType = "photo"
	MediaAnimation       MediaType = "animation"
	MediaStickerStatic   MediaType = "sticker_static"
	MediaStickerAnimated MediaType = "sticker_animated"
)

type Content struct {
	FileID       string
	FileUniqueID string
	Type         MediaType
	MimeType     string
	SizeBytes    int64
	DurationSec  int
	Width        int
	Height       int
}

type Message struct {
	ID           int
	ChatID       int64
	SenderID     int64
	Command      string
	FileUniqueID string
	ReplyTo      *Message
	Content      *Content
}

func (m Message) IsCommand() bool {
	return m.Command != ""
}

func (c Content) IsZero() bool {
	return c.FileID == "" && c.FileUniqueID == ""
}

func (c Content) CanDownload() bool {
	return c.FileID != ""
}

func (c Content) IsImageLike() bool {
	return c.Type == MediaPhoto || c.Type == MediaStickerStatic
}

func (c Content) IsVideoLike() bool {
	return c.Type == MediaAnimation
}
