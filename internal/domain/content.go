package domain

type MediaType string

const (
	MediaPhoto           MediaType = "photo"
	MediaAnimation       MediaType = "animation"
	MediaVideo           MediaType = "video"
	MediaStickerStatic   MediaType = "sticker_static"
	MediaStickerAnimated MediaType = "sticker_animated"
	MediaStickerVideo    MediaType = "sticker_video"
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
	return c.Type == MediaAnimation ||
		c.Type == MediaVideo ||
		c.Type == MediaStickerAnimated ||
		c.Type == MediaStickerVideo
}
