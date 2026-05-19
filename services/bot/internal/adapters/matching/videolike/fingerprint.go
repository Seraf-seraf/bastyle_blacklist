package videolike

import (
	"strconv"
	"strings"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/domain"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/pkg/apperrors"
	"github.com/corona10/goimagehash"
)

const (
	videoLikeHashVersion = "goimagehash-phash-64-video-frames-v1"
	minMatchedFrames     = 2
	minMatchedFrameRatio = 0.4
)

type StoredVideoLikeHash struct {
	ID           int64
	ChatID       int64
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
	const methodCtx = "videolike/fingerprintVideoLike"

	if len(extracted.Frames) == 0 {
		return StoredVideoLikeHash{}, apperrors.New(methodCtx, "videolike fingerprint: извлеченное медиа не содержит кадров")
	}

	frames := make([]StoredVideoLikeFrameHash, 0, len(extracted.Frames))
	for _, frame := range extracted.Frames {
		if frame.Image == nil {
			return StoredVideoLikeHash{}, apperrors.New(methodCtx, "videolike fingerprint: изображение извлеченного кадра пустое")
		}

		hash, err := goimagehash.PerceptionHash(frame.Image)
		if err != nil {
			return StoredVideoLikeHash{}, apperrors.Wrap(methodCtx, err)
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

func videoLikeHashSignature(frames []StoredVideoLikeFrameHash) string {
	parts := make([]string, 0, len(frames))
	for _, frame := range frames {
		parts = append(parts, strings.Join([]string{
			strconv.Itoa(frame.FrameIndex),
			strconv.Itoa(frame.PositionMillis),
			strconv.FormatUint(frame.Hash, 10),
		}, ":"))
	}

	return strings.Join(parts, "|")
}
