package ports

import (
	"context"
	"errors"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var ErrUnsupportedContent = errors.New("matcher не поддерживает этот контент")

type PreparedBlock any

type ContentBlockPreparer interface {
	// PrepareBlock вычисляет данные для ban без записи в БД и без изменения локальных индексов.
	// Если matcher не поддерживает content, метод возвращает ErrUnsupportedContent.
	PrepareBlock(ctx context.Context, chatID int64, content domain.Content) (PreparedBlock, error)
}

type ContentBlockMatcher interface {
	IsBlocked(ctx context.Context, chatID int64, content domain.Content) (bool, error)
	ContentBlockPreparer

	// PersistBlock сохраняет prepared block в PostgreSQL внутри общей транзакции /ban.
	// Возвращает true, если artifact был создан, и false, если запись уже существовала.
	PersistBlock(ctx context.Context, tx pgx.Tx, banUID uuid.UUID, block PreparedBlock) (bool, error)

	// ApplyBlock обновляет локальную производную проекцию matcher-а, например in-memory индекс.
	// Метод вызывается только после успешного commit PostgreSQL-транзакции.
	ApplyBlock(ctx context.Context, block PreparedBlock) error
}
