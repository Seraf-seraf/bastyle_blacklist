package media

import (
	"bytes"
	"context"
	"errors"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/domain"
	_ "golang.org/x/image/webp"
)

type Extractor struct{}

const maxImagePixels = 20_000_000

func NewExtractor() *Extractor {
	return &Extractor{}
}

func (e *Extractor) Extract(ctx context.Context, media domain.MediaFile, plan domain.MediaExtractionPlan) (domain.ExtractedMedia, error) {
	if err := ctx.Err(); err != nil {
		return domain.ExtractedMedia{}, err
	}

	reader := bytes.NewReader(media.Data)
	config, _, err := image.DecodeConfig(reader)
	if err != nil {
		return domain.ExtractedMedia{}, err
	}
	if config.Width <= 0 || config.Height <= 0 {
		return domain.ExtractedMedia{}, errors.New("extract media: invalid image dimensions")
	}
	if config.Width > maxImagePixels/config.Height {
		return domain.ExtractedMedia{}, errors.New("extract media: image dimensions are too large")
	}

	if _, err := reader.Seek(0, 0); err != nil {
		return domain.ExtractedMedia{}, err
	}

	img, _, err := image.Decode(reader)
	if err != nil {
		return domain.ExtractedMedia{}, err
	}

	return domain.ExtractedMedia{
		Frames: []domain.ExtractedFrame{
			{Image: img},
		},
	}, nil
}
