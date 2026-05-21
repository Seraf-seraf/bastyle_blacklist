package postgres

import (
	"context"
	"testing"
)

func TestStoreRequiresPool(t *testing.T) {
	if _, err := NewStore(nil); err == nil {
		t.Fatal("ожидалась ошибка для nil pool")
	}
}

func TestStoreSQLUsesMonotonicCheckpointUpdate(t *testing.T) {
	query := updateCheckpointSQL()
	if !containsAll(query, "last_applied_transaction_id = $3::xid8", "last_applied_offset = $4", "last_applied_transaction_id < $3::xid8", "last_applied_offset <= $4", "stale = FALSE", "stale_reason = NULL") {
		t.Fatalf("update checkpoint должен быть monotonic и сбрасывать stale, SQL: %s", query)
	}
}

func TestStoreRejectsInvalidArgumentsBeforeDatabaseQuery(t *testing.T) {
	store := &store{}
	ctx := context.Background()

	if _, err := store.GetOrCreate(ctx, "", "exact"); err == nil {
		t.Fatal("ожидалась ошибка для пустого consumer_id")
	}
	if err := store.Update(ctx, "replica-1", "exact", "1", -1); err == nil {
		t.Fatal("ожидалась ошибка для отрицательного event id")
	}
	if err := store.MarkStale(ctx, "replica-1", "exact", ""); err == nil {
		t.Fatal("ожидалась ошибка для пустой причины stale")
	}
	if err := store.CheckFresh(ctx, "replica-1", []string{"exact"}); err == nil {
		t.Fatal("ожидалась ошибка для store без PostgreSQL pool")
	}
}

func TestStoreCheckFreshSQLDetectsStaleIndexes(t *testing.T) {
	query := checkFreshSQL(2)
	if !containsAll(query, "FROM index_checkpoints", "stale = TRUE", "consumer_id = $1", "index_name = ANY($2)") {
		t.Fatalf("readiness SQL должен проверять stale checkpoints, SQL: %s", query)
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
