//go:build integration

package imagehash

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestPostgresStoreInsertConflictDoesNotLeaveOrphanMediaBan(t *testing.T) {
	ctx := context.Background()
	pool := newImageHashPostgresPool(t, ctx)
	store := NewPostgresStore(pool)
	hash := StoredImageHash{
		ChatID:       10,
		FileUniqueID: "file-unique-id",
		MediaType:    domain.MediaPhoto,
		Hashes:       []uint64{1, 2, 3, 4},
	}

	if _, err := store.insert(ctx, hash); err != nil {
		t.Fatalf("первичная вставка imagehash: %v", err)
	}

	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("старт транзакции: %v", err)
	}
	duplicateBanUID := uuid.New()
	if err := insertMediaBan(ctx, tx, duplicateBanUID, hash.ChatID, hash.MediaType, "duplicate-file-unique-id"); err != nil {
		t.Fatalf("вставка duplicate media_ban: %v", err)
	}
	if _, err := store.Insert(ctx, tx, duplicateBanUID, hash); err != nil {
		t.Fatalf("конфликтная вставка imagehash: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	var orphanCount int
	if err := pool.QueryRow(ctx, `
SELECT count(*)
FROM media_ban mb
LEFT JOIN blocked_image bi ON bi.ban_uid = mb.ban_uid
WHERE bi.ban_uid IS NULL
`).Scan(&orphanCount); err != nil {
		t.Fatalf("подсчет orphan media_ban: %v", err)
	}
	if orphanCount != 0 {
		t.Fatalf("orphan media_ban = %d, ожидалось 0", orphanCount)
	}
}

func newImageHashPostgresPool(t *testing.T, ctx context.Context) *pgxpool.Pool {
	t.Helper()

	container, err := tcpostgres.Run(
		ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("bastyle"),
		tcpostgres.WithUsername("bastyle"),
		tcpostgres.WithPassword("bastyle"),
		testcontainers.WithWaitStrategy(wait.ForListeningPort("5432/tcp").WithStartupTimeout(30*time.Second)),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			t.Fatal(err)
		}
	})

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	applyPostgresMigrations(t, ctx, pool)
	return pool
}

func applyPostgresMigrations(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	botDir := filepath.Clean(filepath.Join(wd, "../../../.."))
	migrationPaths, err := filepath.Glob(filepath.Join(botDir, "internal/adapters/database/postgres/migrations/*.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for _, migrationPath := range migrationPaths {
		raw, err := os.ReadFile(migrationPath)
		if err != nil {
			t.Fatal(err)
		}
		sql := gooseUpSQL(string(raw))
		if sql == "" {
			continue
		}
		if _, err := pool.Exec(ctx, sql); err != nil {
			t.Fatalf("применение миграции %s: %v", migrationPath, err)
		}
	}
}

func gooseUpSQL(raw string) string {
	up := strings.Split(raw, "-- +goose Down")[0]
	return strings.TrimSpace(strings.ReplaceAll(up, "-- +goose Up", ""))
}
