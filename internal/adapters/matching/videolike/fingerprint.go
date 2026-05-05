package videolike

import (
	"errors"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/domain"
	"github.com/corona10/goimagehash"
)

const (
	videoLikeHashVersion = "goimagehash-phash-64-video-frames-v1"
	minMatchedFrames     = 2
	minMatchedFrameRatio = 0.4
)

type StoredVideoLikeHash struct {
	ID           int64
	FileUniqueID string
	SourceType   domain.MediaType
	DurationSec  int
	HashVersion  string
	Frames       []StoredVideoLikeFrameHash
}

type StoredVideoLikeFrameHash struct {
	FrameIndex     int
	PositionMillis int
	Hash           uint64
}

func fingerprintVideoLike(content domain.Content, extracted domain.ExtractedMedia) (StoredVideoLikeHash, error) {
	if len(extracted.Frames) == 0 {
		return StoredVideoLikeHash{}, errors.New("videolike fingerprint: extracted media has no frames")
	}

	frames := make([]StoredVideoLikeFrameHash, 0, len(extracted.Frames))
	for _, frame := range extracted.Frames {
		if frame.Image == nil {
			return StoredVideoLikeHash{}, errors.New("videolike fingerprint: extracted frame image is empty")
		}

		hash, err := goimagehash.PerceptionHash(frame.Image)
		if err != nil {
			return StoredVideoLikeHash{}, err
		}

		frames = append(frames, StoredVideoLikeFrameHash{
			FrameIndex:     frame.Index,
			PositionMillis: frame.PositionMillis,
			Hash:           hash.GetHash(),
		})
	}

	return StoredVideoLikeHash{
		FileUniqueID: content.FileUniqueID,
		SourceType:   content.Type,
		DurationSec:  content.DurationSec,
		HashVersion:  videoLikeHashVersion,
		Frames:       frames,
	}, nil
}

func matchVideoLikeFingerprint(query StoredVideoLikeHash, stored StoredVideoLikeHash, threshold int) bool {
	_, matched := matchStoredVideoLikeFingerprint(query, stored, threshold, DefaultMatchRule())
	return matched
}
