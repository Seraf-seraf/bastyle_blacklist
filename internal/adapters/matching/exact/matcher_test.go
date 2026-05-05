package exact

import "testing"

func TestNewMatcherRejectsNegativeBuffer(t *testing.T) {
	_, err := NewMatcher(-1)
	if err == nil {
		t.Fatal("expected negative buffer to be rejected")
	}
}
