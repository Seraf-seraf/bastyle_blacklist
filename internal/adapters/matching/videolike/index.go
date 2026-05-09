package videolike

import (
	"math"
	"math/bits"
	"strconv"
	"sync"
)

type MatchRule struct {
	MinMatchedFrames int
	MinMatchedRatio  float64
}

type SearchResult struct {
	Stored        StoredVideoLikeHash
	MatchedFrames int
	MatchedRatio  float64
}

type LinearIndex struct {
	mu     sync.RWMutex
	hashes []StoredVideoLikeHash
	rule   MatchRule
}

func DefaultMatchRule() MatchRule {
	return MatchRule{
		MinMatchedFrames: minMatchedFrames,
		MinMatchedRatio:  minMatchedFrameRatio,
	}
}

func NewLinearIndex(buffer int, rule MatchRule) *LinearIndex {
	if rule == (MatchRule{}) {
		rule = DefaultMatchRule()
	}

	return &LinearIndex{
		hashes: make([]StoredVideoLikeHash, 0, buffer),
		rule:   rule,
	}
}

func (i *LinearIndex) Add(hash StoredVideoLikeHash) {
	i.mu.Lock()
	defer i.mu.Unlock()

	signature := videoLikeIndexSignature(hash)
	for _, existing := range i.hashes {
		if videoLikeIndexSignature(existing) == signature {
			return
		}
	}

	i.hashes = append(i.hashes, hash)
}

func (i *LinearIndex) AddMany(hashes []StoredVideoLikeHash) {
	i.mu.Lock()
	defer i.mu.Unlock()

	seen := make(map[string]struct{}, len(i.hashes)+len(hashes))
	for _, existing := range i.hashes {
		seen[videoLikeIndexSignature(existing)] = struct{}{}
	}

	for _, hash := range hashes {
		signature := videoLikeIndexSignature(hash)
		if _, ok := seen[signature]; ok {
			continue
		}

		seen[signature] = struct{}{}
		i.hashes = append(i.hashes, hash)
	}
}

func (i *LinearIndex) Search(chatID int64, query StoredVideoLikeHash, threshold int) (SearchResult, bool) {
	i.mu.RLock()
	defer i.mu.RUnlock()

	for _, stored := range i.hashes {
		if stored.ChatID != chatID {
			continue
		}
		result, matched := matchStoredVideoLikeFingerprint(query, stored, threshold, i.rule)
		if matched {
			return result, true
		}
	}

	return SearchResult{}, false
}

func matchStoredVideoLikeFingerprint(query StoredVideoLikeHash, stored StoredVideoLikeHash, threshold int, rule MatchRule) (SearchResult, bool) {
	if threshold < 0 || query.HashVersion != stored.HashVersion || len(query.Frames) == 0 || len(stored.Frames) == 0 {
		return SearchResult{}, false
	}

	requiredFrames := requiredMatchedFrames(len(query.Frames), len(stored.Frames), rule)
	if requiredFrames <= 0 || len(query.Frames) < requiredFrames || len(stored.Frames) < requiredFrames {
		return SearchResult{}, false
	}

	matchedFrames := 0
	for idx, queryFrame := range query.Frames {
		if hasMatchingFrame(queryFrame, stored.Frames, threshold) {
			matchedFrames++
			if matchedFrames >= requiredFrames {
				return SearchResult{
					Stored:        stored,
					MatchedFrames: matchedFrames,
					MatchedRatio:  matchedFrameRatio(matchedFrames, len(query.Frames), len(stored.Frames)),
				}, true
			}
		}

		remainingFrames := len(query.Frames) - idx - 1
		if matchedFrames+remainingFrames < requiredFrames {
			return SearchResult{}, false
		}
	}

	return SearchResult{}, false
}

func requiredMatchedFrames(queryFrames int, storedFrames int, rule MatchRule) int {
	minByRatio := int(math.Ceil(rule.MinMatchedRatio * math.Min(float64(queryFrames), float64(storedFrames))))
	if rule.MinMatchedFrames > minByRatio {
		return rule.MinMatchedFrames
	}

	return minByRatio
}

func matchedFrameRatio(matchedFrames int, queryFrames int, storedFrames int) float64 {
	return float64(matchedFrames) / math.Min(float64(queryFrames), float64(storedFrames))
}

func hasMatchingFrame(query StoredVideoLikeFrameHash, stored []StoredVideoLikeFrameHash, threshold int) bool {
	for _, storedFrame := range stored {
		if bits.OnesCount64(query.Hash^storedFrame.Hash) <= threshold {
			return true
		}
	}

	return false
}

func videoLikeIndexSignature(hash StoredVideoLikeHash) string {
	return strconv.FormatInt(hash.ChatID, 10) + "|" + hash.HashVersion + "|" + videoLikeHashSignature(hash.Frames)
}
