package postgres

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
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

func TestEventReaderUnwrapsWatermillForwarderEnvelope(t *testing.T) {
	eventUID := uuid.New()
	aggregateUID := uuid.New()
	payload := []byte(`{"ban_uid":"` + aggregateUID.String() + `"}`)
	envelope, err := json.Marshal(forwarderEnvelope{
		DestinationTopic: "media.ban.created.v1",
		UUID:             eventUID.String(),
		Payload:          payload,
		Metadata: map[string]string{
			"event_uid":      eventUID.String(),
			"event_type":     "media.ban.created.v1",
			"aggregate_type": "media_ban",
			"aggregate_uid":  aggregateUID.String(),
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	event, err := eventFromRow("1", 2, "", envelope, nil, time.Unix(10, 0))
	if err != nil {
		t.Fatal(err)
	}

	if event.EventUID != eventUID {
		t.Fatalf("event_uid = %s, ожидалось %s", event.EventUID, eventUID)
	}
	if event.AggregateUID != aggregateUID {
		t.Fatalf("aggregate_uid = %s, ожидалось %s", event.AggregateUID, aggregateUID)
	}
	if event.EventType != "media.ban.created.v1" {
		t.Fatalf("event_type = %q", event.EventType)
	}
	if string(event.Payload) != string(payload) {
		t.Fatalf("payload = %s, ожидалось %s", event.Payload, payload)
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
