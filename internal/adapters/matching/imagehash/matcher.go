package imagehash

import (
	"context"
	"errors"
	"sync"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/app/ports"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/domain"
	"github.com/corona10/goimagehash"
)

type Matcher struct {
	downloader ports.MediaDownloader
	extractor  ports.MediaExtractor
	threshold  int

	mu     sync.RWMutex
	hashes []*goimagehash.ImageHash
}

func NewMatcher(downloader ports.MediaDownloader, extractor ports.MediaExtractor, threshold int, buffer int) *Matcher {
	return &Matcher{
		downloader: downloader,
		extractor:  extractor,
		threshold:  threshold,
		hashes:     make([]*goimagehash.ImageHash, 0, buffer),
	}
}

func (m *Matcher) IsBlocked(ctx context.Context, content domain.Content) (bool, error) {
	if !m.supports(content) {
		return false, nil
	}

	hash, err := m.hashContent(ctx, content)
	if err != nil {
		return false, err
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, blockedHash := range m.hashes {
		distance, err := hash.Distance(blockedHash)
		if err != nil {
			return false, err
		}

		if distance <= m.threshold {
			return true, nil
		}
	}

	return false, nil
}

func (m *Matcher) Block(ctx context.Context, content domain.Content) error {
	if !m.supports(content) {
		return nil
	}

	hash, err := m.hashContent(ctx, content)
	if err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.hashes = append(m.hashes, hash)
	return nil
}

func (m *Matcher) supports(content domain.Content) bool {
	return (content.Type == domain.MediaPhoto ||
		content.Type == domain.MediaStickerStatic) &&
		content.CanDownload()
}

func (m *Matcher) hashContent(ctx context.Context, content domain.Content) (*goimagehash.ImageHash, error) {
	media, err := m.downloader.Download(ctx, content)
	if err != nil {
		return nil, err
	}

	extracted, err := m.extractor.Extract(ctx, media, domain.MediaExtractionPlan{
		MaxFrames: 1,
	})
	if err != nil {
		return nil, err
	}

	if len(extracted.Frames) == 0 {
		return nil, errors.New("media extractor returned no frames")
	}

	hash, err := goimagehash.PerceptionHash(extracted.Frames[0].Image)
	if err != nil {
		return nil, err
	}

	return hash, nil
}
