package telegram

import "testing"

func TestConstructorsRejectNilBot(t *testing.T) {
	if _, err := NewBotActions(nil); err == nil {
		t.Fatal("ожидалось, что действия бота равны nil будут отклонены")
	}

	if _, err := NewAdminChecker(nil); err == nil {
		t.Fatal("ожидалось, что проверка админов равна nil будет отклонена")
	}

	if _, err := NewFileDownloader(nil); err == nil {
		t.Fatal("ожидалось, что загрузчик файлов равен nil будет отклонен")
	}
}
