package media

import (
	"bytes"
	"context"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/domain"
	_ "golang.org/x/image/webp"
)

type Extractor struct{}

func NewExtractor() *Extractor {
	return &Extractor{}
}

func (e *Extractor) Extract(ctx context.Context, media domain.MediaFile, plan domain.MediaExtractionPlan) (domain.ExtractedMedia, error) {
	if err := ctx.Err(); err != nil {
		return domain.ExtractedMedia{}, err
	}

	img, _, err := image.Decode(bytes.NewReader(media.Data))
	if err != nil {
		return domain.ExtractedMedia{}, err
	}

	return domain.ExtractedMedia{
		Frames: []domain.ExtractedFrame{
			{Image: img},
		},
	}, nil
}
