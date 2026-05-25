package indexsync

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/app/ports"
	"github.com/google/uuid"
)

func TestSynchronizerCatchesUpIndexInBatchesAndAdvancesCheckpoint(t *testing.T) {
	ctx := context.Background()
	reader := &fakeEventReader{
		events: []ports.OutboxEvent{
			newOutboxEvent("1", 1, "media.ban.created.v1"),
			newOutboxEvent("1", 2, "unrelated.event.v1"),
			newOutboxEvent("2", 1, "media.ban.created.v1"),
		},
	}
	checkpoints := newFakeCheckpointStore()
	applier := &fakeApplier{indexName: ports.IndexExact, supported: "media.ban.created.v1"}

	synchronizer, err := New(Config{
		ConsumerID:  "replica-1",
		BatchSize:   2,
		Reader:      reader,
		Checkpoints: checkpoints,
		Appliers:    []ports.IndexEventApplier{applier},
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := synchronizer.CatchUpIndex(ctx, ports.IndexExact); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(applier.applied, []string{"1/1", "2/1"}) {
		t.Fatalf("события применены не в том порядке: %#v", applier.applied)
	}
	checkpoint := checkpoints.items[checkpointKey{"replica-1", ports.IndexExact}]
	if checkpoint.LastAppliedTransactionID != "2" || checkpoint.LastAppliedOffset != 1 {
		t.Fatalf("checkpoint должен догнать 2/1, получено %s/%d", checkpoint.LastAppliedTransactionID, checkpoint.LastAppliedOffset)
	}
	if checkpoint.Stale {
		t.Fatal("успешный catch-up не должен оставлять индекс stale")
	}
}

func TestSynchronizerMarksIndexStaleOnApplyError(t *testing.T) {
	ctx := context.Background()
	applyErr := errors.New("ошибка runtime index")
	reader := &fakeEventReader{events: []ports.OutboxEvent{
		newOutboxEvent("1", 1, "media.ban.created.v1"),
		newOutboxEvent("1", 2, "media.ban.created.v1"),
	}}
	checkpoints := newFakeCheckpointStore()
	applier := &fakeApplier{indexName: ports.IndexImageHash, supported: "media.ban.created.v1", failOnPosition: "1/2", err: applyErr}

	synchronizer, err := New(Config{
		ConsumerID:  "replica-1",
		BatchSize:   10,
		Reader:      reader,
		Checkpoints: checkpoints,
		Appliers:    []ports.IndexEventApplier{applier},
	})
	if err != nil {
		t.Fatal(err)
	}

	err = synchronizer.CatchUpIndex(ctx, ports.IndexImageHash)
	if err == nil {
		t.Fatal("ожидалась ошибка применения события")
	}
	var applyIndexErr *IndexApplyError
	if !errors.As(err, &applyIndexErr) {
		t.Fatalf("ожидалась IndexApplyError, получено %T", err)
	}

	checkpoint := checkpoints.items[checkpointKey{"replica-1", ports.IndexImageHash}]
	if checkpoint.LastAppliedTransactionID != "0" || checkpoint.LastAppliedOffset != 0 {
		t.Fatalf("checkpoint не должен обновляться после частично примененного batch, получено %s/%d", checkpoint.LastAppliedTransactionID, checkpoint.LastAppliedOffset)
	}
	if !checkpoint.Stale {
		t.Fatal("ошибка применения должна пометить индекс stale")
	}
	if checkpoint.StaleReason == "" {
		t.Fatal("stale_reason должен быть заполнен")
	}
}

func TestSynchronizerCatchesUpIndexesInParallel(t *testing.T) {
	ctx := context.Background()
	reader := &fakeEventReader{events: []ports.OutboxEvent{
		newOutboxEvent("1", 1, "media.ban.created.v1"),
	}}
	checkpoints := newFakeCheckpointStore()
	first := &fakeApplier{indexName: "first", supported: "media.ban.created.v1", delay: 40 * time.Millisecond}
	second := &fakeApplier{indexName: "second", supported: "media.ban.created.v1", delay: 40 * time.Millisecond}

	synchronizer, err := New(Config{
		ConsumerID:  "replica-1",
		BatchSize:   10,
		Reader:      reader,
		Checkpoints: checkpoints,
		Appliers:    []ports.IndexEventApplier{first, second},
	})
	if err != nil {
		t.Fatal(err)
	}

	startedAt := time.Now()
	if err := synchronizer.CatchUpAllIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(startedAt); elapsed >= 75*time.Millisecond {
		t.Fatalf("индексы синхронизировались последовательно, elapsed=%s", elapsed)
	}
}

func TestSynchronizerRejectsInvalidConfig(t *testing.T) {
	_, err := New(Config{})
	if err == nil {
		t.Fatal("ожидалась ошибка для пустого config")
	}
}

type fakeEventReader struct {
	events []ports.OutboxEvent
}

func (r *fakeEventReader) LoadAfter(_ context.Context, transactionID string, offset int64, limit int) ([]ports.OutboxEvent, error) {
	result := make([]ports.OutboxEvent, 0, limit)
	for _, event := range r.events {
		if !eventPositionAfter(event.TransactionID, event.Offset, transactionID, offset) {
			continue
		}
		result = append(result, event)
		if len(result) == limit {
			break
		}
	}
	return result, nil
}

type checkpointKey struct {
	consumerID string
	indexName  string
}

type fakeCheckpointStore struct {
	mu    sync.Mutex
	items map[checkpointKey]ports.IndexCheckpoint
}

func newFakeCheckpointStore() *fakeCheckpointStore {
	return &fakeCheckpointStore{items: make(map[checkpointKey]ports.IndexCheckpoint)}
}

func (s *fakeCheckpointStore) GetOrCreate(_ context.Context, consumerID string, indexName string) (ports.IndexCheckpoint, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := checkpointKey{consumerID: consumerID, indexName: indexName}
	checkpoint, ok := s.items[key]
	if !ok {
		checkpoint = ports.IndexCheckpoint{ConsumerID: consumerID, IndexName: indexName, LastAppliedTransactionID: "0"}
		s.items[key] = checkpoint
	}
	return checkpoint, nil
}

func (s *fakeCheckpointStore) Update(_ context.Context, consumerID string, indexName string, transactionID string, offset int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := checkpointKey{consumerID: consumerID, indexName: indexName}
	checkpoint := s.items[key]
	if eventPositionAfter(transactionID, offset, checkpoint.LastAppliedTransactionID, checkpoint.LastAppliedOffset) {
		checkpoint.LastAppliedTransactionID = transactionID
		checkpoint.LastAppliedOffset = offset
	}
	checkpoint.Stale = false
	checkpoint.StaleReason = ""
	s.items[key] = checkpoint
	return nil
}

func (s *fakeCheckpointStore) MarkStale(_ context.Context, consumerID string, indexName string, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := checkpointKey{consumerID: consumerID, indexName: indexName}
	checkpoint := s.items[key]
	checkpoint.ConsumerID = consumerID
	checkpoint.IndexName = indexName
	checkpoint.Stale = true
	checkpoint.StaleReason = reason
	s.items[key] = checkpoint
	return nil
}

func (s *fakeCheckpointStore) CheckFresh(context.Context, string, []string) error {
	return nil
}

func (s *fakeCheckpointStore) Stats(_ context.Context, consumerID string, indexNames []string) ([]ports.IndexCheckpointStat, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	stats := make([]ports.IndexCheckpointStat, 0, len(indexNames))
	for _, indexName := range indexNames {
		checkpoint := s.items[checkpointKey{consumerID: consumerID, indexName: indexName}]
		stats = append(stats, ports.IndexCheckpointStat{
			ConsumerID: consumerID,
			IndexName:  indexName,
			Stale:      checkpoint.Stale,
		})
	}
	return stats, nil
}

type fakeApplier struct {
	indexName      string
	supported      string
	failOnPosition string
	err            error
	delay          time.Duration
	applied        []string
}

func (a *fakeApplier) IndexName() string {
	return a.indexName
}

func (a *fakeApplier) Supports(eventType string) bool {
	return eventType == a.supported
}

func (a *fakeApplier) ApplyEvent(_ context.Context, event ports.OutboxEvent) error {
	if a.delay > 0 {
		time.Sleep(a.delay)
	}
	position := fmt.Sprintf("%s/%d", event.TransactionID, event.Offset)
	if position == a.failOnPosition {
		return a.err
	}
	a.applied = append(a.applied, position)
	return nil
}

func newOutboxEvent(transactionID string, offset int64, eventType string) ports.OutboxEvent {
	return ports.OutboxEvent{
		TransactionID: transactionID,
		Offset:        offset,
		EventUID:      uuid.New(),
		EventType:     eventType,
		AggregateType: "media_ban",
		AggregateUID:  uuid.New(),
		Payload:       []byte(`{}`),
	}
}
