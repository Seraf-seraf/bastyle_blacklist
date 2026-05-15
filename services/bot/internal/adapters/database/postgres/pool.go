package postgres

import (
	"context"
	"fmt"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/config"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/pkg/apperrors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Pool struct {
	pool *pgxpool.Pool
}

func NewPool(ctx context.Context, cfg config.Database) (*Pool, error) {
	const methodCtx = "postgres/NewPool"

	poolConfig, err := newPoolConfig(cfg)
	if err != nil {
		return nil, apperrors.Wrap(methodCtx, err)
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, apperrors.Wrap(methodCtx, err)
	}

	client := &Pool{pool: pool}
	if err := client.Ping(ctx); err != nil {
		pool.Close()
		return nil, apperrors.Wrap(methodCtx, err)
	}

	return client, nil
}

func newPoolConfig(cfg config.Database) (*pgxpool.Config, error) {
	const methodCtx = "postgres/newPoolConfig"

	poolConfig, err := pgxpool.ParseConfig(cfg.DSN)
	if err != nil {
		return nil, apperrors.Wrap(methodCtx, err)
	}

	poolConfig.MaxConns = int32(cfg.MaxConns)
	poolConfig.MinConns = int32(cfg.MinConns)
	poolConfig.MaxConnLifetime = cfg.MaxConnLifetime.Value()
	poolConfig.MaxConnIdleTime = cfg.MaxConnIdleTime.Value()
	poolConfig.HealthCheckPeriod = cfg.HealthCheckPeriod.Value()
	poolConfig.ConnConfig.ConnectTimeout = cfg.ConnectTimeout.Value()
	poolConfig.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, fmt.Sprintf("SET statement_timeout = %d", cfg.StatementTimeout.Value().Milliseconds()))
		return err
	}

	return poolConfig, nil
}

func (p *Pool) Raw() *pgxpool.Pool {
	return p.pool
}

func (p *Pool) Ping(ctx context.Context) error {
	const methodCtx = "postgres/Pool.Ping"

	if err := p.pool.Ping(ctx); err != nil {
		return apperrors.Wrap(methodCtx, err)
	}

	return nil
}

func (p *Pool) Close() {
	p.pool.Close()
}

func (p *Pool) WithTx(ctx context.Context, fn func(ctx context.Context, tx pgx.Tx) error) error {
	const methodCtx = "postgres/Pool.WithTx"

	tx, err := p.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return apperrors.Wrap(methodCtx, err)
	}

	if err := fn(ctx, tx); err != nil {
		if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
			return apperrors.Wrap(methodCtx, fmt.Errorf("%w; rollback: %w", err, rollbackErr))
		}
		return apperrors.Wrap(methodCtx, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return apperrors.Wrap(methodCtx, err)
	}

	return nil
}
