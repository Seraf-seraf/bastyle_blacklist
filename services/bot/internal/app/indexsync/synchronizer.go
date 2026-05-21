package indexsync

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/app/ports"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/pkg/apperrors"
)

type Config struct {
	ConsumerID  string
	BatchSize   int
	Reader      ports.OutboxEventReader
	Checkpoints ports.IndexCheckpointStore
	Appliers    []ports.IndexEventApplier
}

type synchronizer struct {
	consumerID  string
	batchSize   int
	reader      ports.OutboxEventReader
	checkpoints ports.IndexCheckpointStore
	appliers    map[string]ports.IndexEventApplier
	indexes     []string
}

type IndexApplyError struct {
	IndexName string
	Event     ports.OutboxEvent
	Err       error
}

func (e *IndexApplyError) Error() string {
	return fmt.Sprintf("индекс %s: ошибка применения outbox event %s/%d: %v", e.IndexName, e.Event.TransactionID, e.Event.Offset, e.Err)
}

func (e *IndexApplyError) Unwrap() error {
	return e.Err
}

func New(cfg Config) (*synchronizer, error) {
	const methodCtx = "indexsync/New"

	if cfg.ConsumerID == "" {
		return nil, apperrors.New(methodCtx, "consumer_id обязателен")
	}
	if cfg.BatchSize <= 0 {
		return nil, apperrors.New(methodCtx, "размер batch должен быть положительным")
	}
	if cfg.Reader == nil {
		return nil, apperrors.New(methodCtx, "reader outbox-событий не настроен")
	}
	if cfg.Checkpoints == nil {
		return nil, apperrors.New(methodCtx, "checkpoint store не настроен")
	}
	if len(cfg.Appliers) == 0 {
		return nil, apperrors.New(methodCtx, "требуется хотя бы один index applier")
	}

	appliers := make(map[string]ports.IndexEventApplier, len(cfg.Appliers))
	indexes := make([]string, 0, len(cfg.Appliers))
	for _, applier := range cfg.Appliers {
		if applier == nil {
			return nil, apperrors.New(methodCtx, "найден nil index applier")
		}
		indexName := applier.IndexName()
		if indexName == "" {
			return nil, apperrors.New(methodCtx, "имя index applier обязательно")
		}
		if _, exists := appliers[indexName]; exists {
			return nil, apperrors.New(methodCtx, "найден повторяющийся index applier")
		}
		appliers[indexName] = applier
		indexes = append(indexes, indexName)
	}

	return &synchronizer{
		consumerID:  cfg.ConsumerID,
		batchSize:   cfg.BatchSize,
		reader:      cfg.Reader,
		checkpoints: cfg.Checkpoints,
		appliers:    appliers,
		indexes:     indexes,
	}, nil
}

func (s *synchronizer) CatchUpAllIndexes(ctx context.Context) error {
	const methodCtx = "indexsync/Synchronizer.CatchUpAllIndexes"

	errs := make(chan error, len(s.indexes))
	var wg sync.WaitGroup
	for _, indexName := range s.indexes {
		indexName := indexName
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.CatchUpIndex(ctx, indexName); err != nil {
				errs <- apperrors.Wrap(methodCtx, err)
			}
		}()
	}
	wg.Wait()
	close(errs)

	var joined error
	for err := range errs {
		joined = errors.Join(joined, err)
	}
	return joined
}

func (s *synchronizer) CatchUpIndex(ctx context.Context, indexName string) error {
	const methodCtx = "indexsync/Synchronizer.CatchUpIndex"

	applier, ok := s.appliers[indexName]
	if !ok {
		return apperrors.New(methodCtx, "index applier не найден: "+indexName)
	}

	checkpoint, err := s.checkpoints.GetOrCreate(ctx, s.consumerID, indexName)
	if err != nil {
		return apperrors.Wrap(methodCtx, err)
	}
	lastTransactionID := checkpoint.LastAppliedTransactionID
	lastOffset := checkpoint.LastAppliedOffset

	for {
		events, err := s.reader.LoadAfter(ctx, lastTransactionID, lastOffset, s.batchSize)
		if err != nil {
			return apperrors.Wrap(methodCtx, err)
		}
		if len(events) == 0 {
			return nil
		}

		batchLastTransactionID := lastTransactionID
		batchLastOffset := lastOffset
		for _, event := range events {
			if !eventAfter(event, batchLastTransactionID, batchLastOffset) {
				continue
			}
			if applier.Supports(event.EventType) {
				if err := applier.ApplyEvent(ctx, event); err != nil {
					applyErr := &IndexApplyError{IndexName: indexName, Event: event, Err: err}
					if staleErr := s.checkpoints.MarkStale(ctx, s.consumerID, indexName, applyErr.Error()); staleErr != nil {
						return errors.Join(applyErr, staleErr)
					}
					return applyErr
				}
			}
			batchLastTransactionID = event.TransactionID
			batchLastOffset = event.Offset
		}

		if eventPositionAfter(batchLastTransactionID, batchLastOffset, lastTransactionID, lastOffset) {
			if err := s.checkpoints.Update(ctx, s.consumerID, indexName, batchLastTransactionID, batchLastOffset); err != nil {
				return apperrors.Wrap(methodCtx, err)
			}
			lastTransactionID = batchLastTransactionID
			lastOffset = batchLastOffset
		}
		if len(events) < s.batchSize {
			return nil
		}
	}
}

func eventAfter(event ports.OutboxEvent, transactionID string, offset int64) bool {
	return eventPositionAfter(event.TransactionID, event.Offset, transactionID, offset)
}

func eventPositionAfter(transactionID string, offset int64, lastTransactionID string, lastOffset int64) bool {
	current, currentErr := strconv.ParseUint(transactionID, 10, 64)
	last, lastErr := strconv.ParseUint(lastTransactionID, 10, 64)
	if currentErr != nil || lastErr != nil {
		return false
	}
	if current > last {
		return true
	}
	return current == last && offset > lastOffset
}
