package postgres

import (
	"testing"
	"time"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/config"
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
