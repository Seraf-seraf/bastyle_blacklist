package orchestrator

import (
	"context"
	"testing"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/adapters/database/postgres"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/app/ports"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type fakeBlocker struct{}
type fakeOutboxWriter struct{}

func (fakeBlocker) IsBlocked(context.Context, int64, domain.Content) (bool, error) {
	return false, nil
}

func (fakeBlocker) PrepareBlock(context.Context, int64, domain.Content) (ports.PreparedBlock, error) {
	return nil, ports.ErrUnsupportedContent
}

func (fakeBlocker) PersistBlock(context.Context, pgx.Tx, uuid.UUID, ports.PreparedBlock) (bool, error) {
	return false, nil
}

func (fakeBlocker) ApplyBlock(context.Context, ports.PreparedBlock) error {
	return nil
}

func (fakeOutboxWriter) Save(context.Context, pgx.Tx, ports.NewOutboxEvent) error {
	return nil
}

func TestNewMatcherRejectsNilDB(t *testing.T) {
	_, err := NewBlockOrchestrator(nil, fakeOutboxWriter{}, fakeBlocker{})
	if err == nil {
		t.Fatal("ожидалась ошибка для пустого PostgreSQL pool")
	}
}

func TestNewMatcherRejectsNilOutbox(t *testing.T) {
	_, err := NewBlockOrchestrator(&postgres.Pool{}, nil, fakeBlocker{})
	if err == nil {
		t.Fatal("ожидалась ошибка для пустого outbox writer")
	}
}

func TestNewMatcherRejectsEmptyList(t *testing.T) {
	_, err := NewBlockOrchestrator(&postgres.Pool{}, fakeOutboxWriter{})
	if err == nil {
		t.Fatal("ожидалась ошибка для пустого списка matcher-ов")
	}
}

func TestNewMatcherRejectsNilMatcher(t *testing.T) {
	_, err := NewBlockOrchestrator(&postgres.Pool{}, fakeOutboxWriter{}, fakeBlocker{}, nil)
	if err == nil {
		t.Fatal("ожидалось, что матчер равен nil будет отклонен")
	}
}
