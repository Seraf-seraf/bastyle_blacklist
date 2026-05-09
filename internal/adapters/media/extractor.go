package media

import (
	"bytes"
	"context"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/domain"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/pkg/apperrors"
	_ "golang.org/x/image/webp"
)

type Extractor struct{}

const maxImagePixels = 20_000_000

func NewExtractor() *Extractor {
	return &Extractor{}
}

func (e *Extractor) Extract(ctx context.Context, media domain.MediaFile, plan domain.MediaExtractionPlan) (domain.ExtractedMedia, error) {
	const methodCtx = "media/Extractor.Extract"

	if err := ctx.Err(); err != nil {
		return domain.ExtractedMedia{}, apperrors.Wrap(methodCtx, err)
	}

	reader := bytes.NewReader(media.Data)
	img, err := decodeBoundedImage(reader)
	if err != nil {
		return domain.ExtractedMedia{}, apperrors.Wrap(methodCtx, err)
	}

	return domain.ExtractedMedia{
		Frames: []domain.ExtractedFrame{
			{Index: 0, PositionMillis: 0, Image: img},
		},
	}, nil
}

func decodeBoundedImage(reader io.ReadSeeker) (image.Image, error) {
	const methodCtx = "media/decodeBoundedImage"

	config, _, err := image.DecodeConfig(reader)
	if err != nil {
		return nil, apperrors.Wrap(methodCtx, err)
	}
	if config.Width <= 0 || config.Height <= 0 {
		return nil, apperrors.New(methodCtx, "извлечение медиа: некорректные размеры изображения")
	}
	if config.Width > maxImagePixels/config.Height {
		return nil, apperrors.New(methodCtx, "извлечение медиа: размеры изображения слишком большие")
	}

	if _, err := reader.Seek(0, 0); err != nil {
		return nil, apperrors.Wrap(methodCtx, err)
	}

	img, _, err := image.Decode(reader)
	if err != nil {
		return nil, apperrors.Wrap(methodCtx, err)
	}

	return img, nil
}
