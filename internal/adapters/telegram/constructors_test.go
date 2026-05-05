package telegram

import "testing"

func TestConstructorsRejectNilBot(t *testing.T) {
	if _, err := NewBotActions(nil); err == nil {
		t.Fatal("expected nil bot actions bot to be rejected")
	}

	if _, err := NewAdminChecker(nil); err == nil {
		t.Fatal("expected nil admin checker bot to be rejected")
	}

	if _, err := NewFileDownloader(nil); err == nil {
		t.Fatal("expected nil file downloader bot to be rejected")
	}
}
