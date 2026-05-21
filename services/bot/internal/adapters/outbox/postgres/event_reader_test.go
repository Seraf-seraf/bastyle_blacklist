package postgres

import (
	"context"
	"testing"
)

func TestEventReaderRequiresPool(t *testing.T) {
	if _, err := NewEventReader(nil); err == nil {
		t.Fatal("ожидалась ошибка для nil pool")
	}
}

func TestEventReaderRejectsInvalidArgumentsBeforeDatabaseQuery(t *testing.T) {
	reader := &eventReader{}
	ctx := context.Background()

	if _, err := reader.LoadAfter(ctx, "0", -1, 10); err == nil {
		t.Fatal("ожидалась ошибка для отрицательного checkpoint")
	}
	if _, err := reader.LoadAfter(ctx, "0", 0, 0); err == nil {
		t.Fatal("ожидалась ошибка для неположительного limit")
	}
}

func TestEventReaderSQLOrdersByWatermillPosition(t *testing.T) {
	query := loadAfterSQL()
	if !containsAll(query, "FROM watermill_outbox_events", "transaction_id = $1::xid8", "\"offset\" > $2", "ORDER BY transaction_id ASC, \"offset\" ASC", "LIMIT $3") {
		t.Fatalf("event reader должен читать Watermill outbox по transaction_id/offset, SQL: %s", query)
	}
}

func containsAll(value string, parts ...string) bool {
	for _, part := range parts {
		if !contains(value, part) {
			return false
		}
	}
	return true
}

func contains(value string, part string) bool {
	for i := 0; i+len(part) <= len(value); i++ {
		if value[i:i+len(part)] == part {
			return true
		}
	}
	return false
}
