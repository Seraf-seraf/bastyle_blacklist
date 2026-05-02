package ports

import "github.com/Seraf-seraf/bastyle_blacklist/internal/domain"

type ContentMatcher interface {
	Block(content domain.Content) error
	IsBlocked(content domain.Content) (bool, error)
}
