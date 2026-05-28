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
)

func TestNewPoolConfigAppliesDatabaseSettings(t *testing.T) {
	cfg := config.Database{
		DSN:               "postgres://user:pass@localhost:5432/app?sslmode=disable",
		MaxConns:          12,
		MinConns:          2,
		MaxConnLifetime:   config.Duration(2 * time.Hour),
		MaxConnIdleTime:   config.Duration(20 * time.Minute),
		HealthCheckPeriod: config.Duration(45 * time.Second),
		ConnectTimeout:    config.Duration(6 * time.Second),
		StatementTimeout:  config.Duration(11 * time.Second),
	}

	poolConfig, err := newPoolConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}

	if poolConfig.MaxConns != 12 {
		t.Fatalf("неожиданное значение max conns: %d", poolConfig.MaxConns)
	}
	if poolConfig.MinConns != 2 {
		t.Fatalf("неожиданное значение min conns: %d", poolConfig.MinConns)
	}
	if poolConfig.MaxConnLifetime != 2*time.Hour {
		t.Fatalf("неожиданное значение max conn lifetime: %s", poolConfig.MaxConnLifetime)
	}
	if poolConfig.MaxConnIdleTime != 20*time.Minute {
		t.Fatalf("неожиданное значение max conn idle time: %s", poolConfig.MaxConnIdleTime)
	}
	if poolConfig.HealthCheckPeriod != 45*time.Second {
		t.Fatalf("неожиданное значение health check period: %s", poolConfig.HealthCheckPeriod)
	}
	if poolConfig.ConnConfig.ConnectTimeout != 6*time.Second {
		t.Fatalf("неожиданное значение connect timeout: %s", poolConfig.ConnConfig.ConnectTimeout)
	}
	if poolConfig.AfterConnect == nil {
		t.Fatal("ожидалось, что statement_timeout будет настроен через AfterConnect")
	}
}

func TestNewPoolConfigRejectsInvalidDSN(t *testing.T) {
	_, err := newPoolConfig(config.Database{
		DSN: "://broken",
	})
	if err == nil {
		t.Fatal("ожидалась ошибка")
	}
}

func TestPoolWithTxCommitsAndRollsBack(t *testing.T) {
	ctx := context.Background()

	container, err := tcpostgres.Run(
		ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("bastyle"),
		tcpostgres.WithUsername("bastyle"),
		tcpostgres.WithPassword("bastyle"),
		tcpostgres.BasicWaitStrategies(),
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
