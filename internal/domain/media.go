package domain

import "image"

type MediaFile struct {
	Content  Content
	FilePath string
	Data     []byte
}

type MediaExtractionPlan struct {
	MaxFrames    int
	TargetWidth  int
	TargetHeight int
}

type ExtractedMedia struct {
	Frames []ExtractedFrame
}

type ExtractedFrame struct {
	Image image.Image
}
