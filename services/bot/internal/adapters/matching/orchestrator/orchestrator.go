package orchestrator

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/adapters/database/postgres"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/app/ports"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/domain"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/pkg/apperrors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type BlockOrchestrator struct {
	db       *postgres.Pool
	outbox   ports.OutboxWriter
	handlers []ports.ContentBlockMatcher
}

type banCreatedPayload struct {
	BanUID       string `json:"ban_uid"`
	ChatID       int64  `json:"chat_id"`
	MediaType    string `json:"media_type"`
	FileUniqueID string `json:"file_unique_id"`
}

type preparedHandlerBlock struct {
	handler ports.ContentBlockMatcher
	block   ports.PreparedBlock
}

func NewBlockOrchestrator(db *postgres.Pool, outbox ports.OutboxWriter, handlers ...ports.ContentBlockMatcher) (ports.ContentBlocker, error) {
	const methodCtx = "orchestrator/NewBlockOrchestrator"

	if db == nil {
		return nil, apperrors.New(methodCtx, "PostgreSQL pool не настроен")
	}
	if outbox == nil {
		return nil, apperrors.New(methodCtx, "outbox writer не настроен")
	}
	if len(handlers) == 0 {
		return nil, apperrors.New(methodCtx, "требуется хотя бы один матчер")
	}

	for _, handler := range handlers {
		if handler == nil {
			return nil, apperrors.New(methodCtx, "найден nil-матчер")
		}
	}

	return &BlockOrchestrator{
		db:       db,
		outbox:   outbox,
		handlers: handlers,
	}, nil
}

func (o *BlockOrchestrator) Block(ctx context.Context, chatID int64, content domain.Content) error {
	const methodCtx = "orchestrator/BlockOrchestrator.Block"

	prepared := make([]preparedHandlerBlock, 0, len(o.handlers))
	for _, handler := range o.handlers {
		block, err := handler.PrepareBlock(ctx, chatID, content)
		if err != nil {
			if errors.Is(err, ports.ErrUnsupportedContent) {
				continue
			}
			return apperrors.Wrap(methodCtx, err)
		}
		prepared = append(prepared, preparedHandlerBlock{
			handler: handler,
			block:   block,
		})
	}
	if len(prepared) == 0 {
		return apperrors.Wrap(methodCtx, ports.ErrUnsupportedContent)
	}

	banUID := uuid.New()
	err := o.db.WithTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := insertMediaBan(ctx, tx, banUID, chatID, content); err != nil {
			return err
		}

		created := false
		for _, item := range prepared {
			result, err := item.handler.PersistBlock(ctx, tx, banUID, item.block)
			if err != nil {
				return err
			}
			created = created || result.Created
		}

		if !created {
			return deleteEmptyMediaBan(ctx, tx, banUID)
		}
		if err := o.saveBanCreatedEvent(ctx, tx, banUID, banCreatedPayload{
			BanUID:       banUID.String(),
			ChatID:       chatID,
			MediaType:    string(content.Type),
			FileUniqueID: content.FileUniqueID,
		}); err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return apperrors.Wrap(methodCtx, err)
	}

	for _, item := range prepared {
		if err := item.handler.ApplyBlock(ctx, item.block); err != nil {
			return apperrors.Wrap(methodCtx, err)
		}
	}

	return nil
}

func (o *BlockOrchestrator) IsBlocked(ctx context.Context, chatID int64, content domain.Content) (bool, error) {
	const methodCtx = "orchestrator/BlockOrchestrator.IsBlocked"

	for _, handler := range o.handlers {
		blocked, err := handler.IsBlocked(ctx, chatID, content)
		if err != nil {
			return false, apperrors.Wrap(methodCtx, err)
		}

		if blocked {
			return true, nil
		}
	}

	return false, nil
}

func insertMediaBan(ctx context.Context, tx pgx.Tx, banUID uuid.UUID, chatID int64, content domain.Content) error {
	const methodCtx = "orchestrator/insertMediaBan"

	_, err := tx.Exec(ctx, `
INSERT INTO media_ban (ban_uid, chat_id, media_type, file_unique_id)
VALUES ($1, $2, $3, $4)
ON CONFLICT (ban_uid) DO NOTHING
`, banUID, chatID, string(content.Type), content.FileUniqueID)
	return apperrors.Wrap(methodCtx, err)
}

func deleteEmptyMediaBan(ctx context.Context, tx pgx.Tx, banUID uuid.UUID) error {
	const methodCtx = "orchestrator/deleteEmptyMediaBan"

	_, err := tx.Exec(ctx, `
DELETE FROM media_ban mb
WHERE mb.ban_uid = $1
  AND NOT EXISTS (SELECT 1 FROM blocked_exact be WHERE be.ban_uid = mb.ban_uid)
  AND NOT EXISTS (SELECT 1 FROM blocked_image bi WHERE bi.ban_uid = mb.ban_uid)
  AND NOT EXISTS (SELECT 1 FROM blocked_video_like bvl WHERE bvl.ban_uid = mb.ban_uid)
  AND NOT EXISTS (SELECT 1 FROM ai_vector_ban avb WHERE avb.ban_uid = mb.ban_uid)
`, banUID)
	return apperrors.Wrap(methodCtx, err)
}

func (o *BlockOrchestrator) saveBanCreatedEvent(ctx context.Context, tx pgx.Tx, banUID uuid.UUID, payload banCreatedPayload) error {
	const methodCtx = "orchestrator/BlockOrchestrator.saveBanCreatedEvent"

	rawPayload, err := json.Marshal(payload)
	if err != nil {
		return apperrors.Wrap(methodCtx, err)
	}

	return o.outbox.Save(ctx, tx, ports.NewOutboxEvent{
		EventUID:      uuid.New(),
		EventType:     "media.ban.created.v1",
		AggregateType: "media_ban",
		AggregateUID:  banUID,
		Payload:       rawPayload,
	})
}

var _ ports.ContentBlocker = (*BlockOrchestrator)(nil)
