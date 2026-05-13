package exact

import (
	"context"
	"database/sql"
	"embed"
	"io/fs"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/pkg/apperrors"
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var exactMigrations embed.FS

type exactRecord struct {
	ChatID       int64
	FileUniqueID string
}

type sqliteStore struct{ db *sql.DB }

func OpenSQLiteStore(ctx context.Context, path string) (*sqliteStore, error) {
	const methodCtx = "exact/OpenSQLiteStore"
	if path == "" {
		return nil, apperrors.New(methodCtx, "путь к SQLite пустой")
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, apperrors.Wrap(methodCtx, err)
	}
	db.SetMaxOpenConns(1)
	store := &sqliteStore{db: db}
	if err := store.ensureSchema(ctx); err != nil {
		_ = db.Close()
		return nil, apperrors.Wrap(methodCtx, err)
	}
	return store, nil
}

func (s *sqliteStore) close() error { return apperrors.Wrap("exact/sqliteStore.close", s.db.Close()) }

func (s *sqliteStore) load(ctx context.Context) ([]exactRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT chat_id, file_unique_id FROM blocked_exact`)
	if err != nil {
		return nil, apperrors.Wrap("exact/sqliteStore.load", err)
	}
	defer rows.Close()
	records := make([]exactRecord, 0)
	for rows.Next() {
		var r exactRecord
		if err := rows.Scan(&r.ChatID, &r.FileUniqueID); err != nil {
			return nil, apperrors.Wrap("exact/sqliteStore.load", err)
		}
		records = append(records, r)
	}
	return records, apperrors.Wrap("exact/sqliteStore.load", rows.Err())
}

func (s *sqliteStore) insert(ctx context.Context, chatID int64, fileUniqueID string) error {
	_, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO blocked_exact (chat_id, file_unique_id) VALUES (?, ?)`, chatID, fileUniqueID)
	return apperrors.Wrap("exact/sqliteStore.insert", err)
}

func (s *sqliteStore) ensureSchema(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON; PRAGMA busy_timeout=5000;`)
	if err != nil {
		return apperrors.Wrap("exact/sqliteStore.ensureSchema", err)
	}
	migrations, err := fs.Sub(exactMigrations, "migrations")
	if err != nil {
		return apperrors.Wrap("exact/sqliteStore.ensureSchema", err)
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, s.db, migrations,
		goose.WithTableName("exact_schema_migrations"),
		goose.WithDisableGlobalRegistry(true),
		goose.WithLogger(goose.NopLogger()),
	)
	if err != nil {
		return apperrors.Wrap("exact/sqliteStore.ensureSchema", err)
	}
	_, err = provider.Up(ctx)
	return apperrors.Wrap("exact/sqliteStore.ensureSchema", err)
}
