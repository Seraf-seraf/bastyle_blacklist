package exact

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/app/ports"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/domain"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/pkg/apperrors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type matcher struct {
	mu      sync.RWMutex
	blocked map[exactKey]struct{}
	store   exactStore
}

type exactKey struct {
	chatID       int64
	fileUniqueID string
}

type exactRecord struct {
	ChatID       int64
	FileUniqueID string
}

type exactStore interface {
	load(context.Context) ([]exactRecord, error)
	LoadByBanUID(context.Context, uuid.UUID) (exactRecord, error)
	Insert(context.Context, pgx.Tx, uuid.UUID, int64, string) (ports.PersistBlockResult, error)
	close() error
}

var errExactArtifactNotFound = errors.New("exact artifact не найден")

func newMatcher(buffer int) (*matcher, error) {
	const methodCtx = "exact/newMatcher"

	if buffer < 0 {
		return nil, apperrors.New(methodCtx, "буфер exact-матчера не должен быть отрицательным")
	}

	return &matcher{blocked: make(map[exactKey]struct{}, buffer)}, nil
}

func NewPostgresMatcher(ctx context.Context, pool *pgxpool.Pool, buffer int) (*matcher, error) {
	const methodCtx = "exact/NewPostgresMatcher"

	if pool == nil {
		return nil, apperrors.New(methodCtx, "PostgreSQL pool exact-матчера не настроен")
	}

	store := NewPostgresStore(pool)

	stored, err := store.load(ctx)
	if err != nil {
		return nil, apperrors.Wrap(methodCtx, err)
	}

	matcher, err := newMatcher(buffer + len(stored))
	if err != nil {
		return nil, apperrors.Wrap(methodCtx, err)
	}
	matcher.store = store
	for _, item := range stored {
		matcher.blocked[exactKey{chatID: item.ChatID, fileUniqueID: item.FileUniqueID}] = struct{}{}
	}

	return matcher, nil
}

func (m *matcher) Close() error {
	const methodCtx = "exact/matcher.Close"

	return apperrors.Wrap(methodCtx, m.store.close())
}

func (m *matcher) IsBlocked(_ context.Context, chatID int64, content domain.Content) (bool, error) {
	if content.FileUniqueID == "" {
		return false, nil
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	_, ok := m.blocked[exactKey{chatID: chatID, fileUniqueID: content.FileUniqueID}]
	return ok, nil
}

func (m *matcher) PrepareBlock(_ context.Context, chatID int64, content domain.Content) (ports.PreparedBlock, error) {
	const methodCtx = "exact/matcher.PrepareBlock"

	if content.FileUniqueID == "" {
		return nil, apperrors.New(methodCtx, fmt.Sprintf("%v: exact требует file_unique_id", ports.ErrUnsupportedContent))
	}

	return &preparedBlock{
		chatID:       chatID,
		fileUniqueID: content.FileUniqueID,
	}, nil
}

type preparedBlock struct {
	chatID       int64
	fileUniqueID string
}

func (m *matcher) PersistBlock(ctx context.Context, tx pgx.Tx, banUID uuid.UUID, block ports.PreparedBlock) (ports.PersistBlockResult, error) {
	const methodCtx = "exact/matcher.PersistBlock"

	prepared, ok := block.(*preparedBlock)
	if !ok {
		return ports.PersistBlockResult{}, apperrors.New(methodCtx, "неверный тип prepared block exact")
	}

	return m.store.Insert(ctx, tx, banUID, prepared.chatID, prepared.fileUniqueID)
}

func (m *matcher) ApplyBlock(_ context.Context, block ports.PreparedBlock) error {
	const methodCtx = "exact/matcher.ApplyBlock"

	prepared, ok := block.(*preparedBlock)
	if !ok {
		return apperrors.New(methodCtx, "неверный тип prepared block exact")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.blocked[exactKey{chatID: prepared.chatID, fileUniqueID: prepared.fileUniqueID}] = struct{}{}
	return nil
}

func (m *matcher) IndexName() string {
	return ports.IndexExact
}

func (m *matcher) Supports(eventType string) bool {
	return eventType == "media.ban.created.v1"
}

func (m *matcher) ApplyEvent(ctx context.Context, event ports.OutboxEvent) error {
	const methodCtx = "exact/matcher.ApplyEvent"

	if !m.Supports(event.EventType) {
		return nil
	}
	record, err := m.store.LoadByBanUID(ctx, event.AggregateUID)
	if err != nil {
		if errors.Is(err, errExactArtifactNotFound) {
			return nil
		}
		return apperrors.Wrap(methodCtx, err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.blocked[exactKey{chatID: record.ChatID, fileUniqueID: record.FileUniqueID}] = struct{}{}
	return nil
}

var _ ports.ContentBlockMatcher = (*matcher)(nil)
var _ ports.IndexEventApplier = (*matcher)(nil)
