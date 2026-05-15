//go:build integration

package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/config"
	"github.com/jackc/pgx/v5"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestPoolWithTxCommitsAndRollsBack(t *testing.T) {
	ctx := context.Background()

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
	defer func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			t.Fatal(err)
		}
	}()

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}

	pool, err := NewPool(ctx, config.Database{
		DSN:               dsn,
		MaxConns:          2,
		MinConns:          1,
		MaxConnLifetime:   config.Duration(time.Hour),
		MaxConnIdleTime:   config.Duration(15 * time.Minute),
		HealthCheckPeriod: config.Duration(30 * time.Second),
		ConnectTimeout:    config.Duration(5 * time.Second),
		StatementTimeout:  config.Duration(10 * time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	if err := pool.WithTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "CREATE TABLE tx_check(id BIGINT PRIMARY KEY)")
		return err
	}); err != nil {
		t.Fatal(err)
	}

	expectedErr := errors.New("rollback")
	err = pool.WithTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "INSERT INTO tx_check(id) VALUES (1)"); err != nil {
			return err
		}
		return expectedErr
	})
	if !errors.Is(err, expectedErr) {
		t.Fatalf("ожидалась rollback ошибка, получили: %v", err)
	}

	var count int
	if err := pool.Raw().QueryRow(ctx, "SELECT count(*) FROM tx_check").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("ожидался rollback, записей: %d", count)
	}
}
