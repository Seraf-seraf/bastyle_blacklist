package moderation

import (
	"context"
	"reflect"
	"testing"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/domain"
)

type fakeMatcher struct {
	blocked []domain.Content
}

func (m *fakeMatcher) Block(_ context.Context, content domain.Content) error {
	m.blocked = append(m.blocked, content)
	return nil
}

func (m *fakeMatcher) IsBlocked(_ context.Context, _ domain.Content) (bool, error) {
	return false, nil
}

type fakeAdmins struct {
	admin bool
}

func (a fakeAdmins) IsAdmin(_ int64, _ int64) (bool, error) {
	return a.admin, nil
}

type fakeActions struct {
	sent    []sentMessage
	deleted []deletedMessage
}

type sentMessage struct {
	chatID int64
	text   string
}

type deletedMessage struct {
	chatID    int64
	messageID int
}

func (a *fakeActions) SendMessage(chatID int64, text string) error {
	a.sent = append(a.sent, sentMessage{
		chatID: chatID,
		text:   text,
	})
	return nil
}

func (a *fakeActions) DeleteMessage(chatID int64, messageID int) error {
	a.deleted = append(a.deleted, deletedMessage{
		chatID:    chatID,
		messageID: messageID,
	})
	return nil
}

func TestHandleMessageBanBlocksTargetAndDeletesTargetThenCommand(t *testing.T) {
	ctx := context.Background()
	matcher := &fakeMatcher{}
	actions := &fakeActions{}
	service, err := NewService(matcher, fakeAdmins{admin: true}, actions)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	content := domain.Content{
		FileID:       "file-id",
		FileUniqueID: "file-unique-id",
		Type:         domain.MediaPhoto,
	}

	msg := domain.Message{
		ID:       20,
		ChatID:   100,
		SenderID: 1,
		Command:  "ban",
		ReplyTo: &domain.Message{
			ID:      10,
			ChatID:  100,
			Content: &content,
		},
	}

	if err := service.HandleMessage(ctx, msg); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(matcher.blocked, []domain.Content{content}) {
		t.Fatalf("blocked content = %#v, want %#v", matcher.blocked, []domain.Content{content})
	}

	wantDeleted := []deletedMessage{
		{chatID: 100, messageID: 10},
		{chatID: 100, messageID: 20},
	}
	if !reflect.DeepEqual(actions.deleted, wantDeleted) {
		t.Fatalf("deleted messages = %#v, want %#v", actions.deleted, wantDeleted)
	}
}

func TestHandleMessageBanDoesNotBlockOrDeleteTextReply(t *testing.T) {
	ctx := context.Background()
	matcher := &fakeMatcher{}
	actions := &fakeActions{}
	service, err := NewService(matcher, fakeAdmins{admin: true}, actions)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	msg := domain.Message{
		ID:       20,
		ChatID:   100,
		SenderID: 1,
		Command:  "ban",
		ReplyTo: &domain.Message{
			ID:     10,
			ChatID: 100,
		},
	}

	if err := service.HandleMessage(ctx, msg); err != nil {
		t.Fatal(err)
	}

	if len(matcher.blocked) != 0 {
		t.Fatalf("blocked content = %#v, want empty", matcher.blocked)
	}

	if len(actions.deleted) != 0 {
		t.Fatalf("deleted messages = %#v, want empty", actions.deleted)
	}

	wantSent := []sentMessage{
		{chatID: 100, text: "Текстовые сообщения не баним"},
	}
	if !reflect.DeepEqual(actions.sent, wantSent) {
		t.Fatalf("sent messages = %#v, want %#v", actions.sent, wantSent)
	}
}

func TestNewServiceRejectsNilDependencies(t *testing.T) {
	matcher := &fakeMatcher{}
	admins := fakeAdmins{}
	actions := &fakeActions{}

	if _, err := NewService(nil, admins, actions); err == nil {
		t.Fatal("expected nil content matcher to be rejected")
	}

	if _, err := NewService(matcher, nil, actions); err == nil {
		t.Fatal("expected nil admin checker to be rejected")
	}

	if _, err := NewService(matcher, admins, nil); err == nil {
		t.Fatal("expected nil message actions to be rejected")
	}
}
