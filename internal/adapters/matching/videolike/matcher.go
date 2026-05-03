package videolike

import "sync"

type Matcher struct {
	mu sync.RWMutex
}
