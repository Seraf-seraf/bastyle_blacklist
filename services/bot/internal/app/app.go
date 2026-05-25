package app

import (
	"context"
	"errors"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/app/bootstrap"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/app/observability"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/app/ports"
	appruntime "github.com/Seraf-seraf/bastyle_blacklist/internal/app/runtime"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/config"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/pkg/apperrors"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/pkg/logging"
)

func Run(parentCtx context.Context, configPath string) (func(context.Context) error, error) {
	const methodCtx = "app/Run"

	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, apperrors.Wrap(methodCtx, err)
	}

	ctx, cancel := context.WithCancel(parentCtx)
	var wg sync.WaitGroup
	var shutdownOnce sync.Once
	var shutdownErr error
	var components bootstrap.Components
	var healthServer *http.Server
	var metricsServer *http.Server
	var outboxPublisher *bootstrap.OutboxPublisher

	shutdown := func(ctx context.Context) error {
		shutdownOnce.Do(func() {
			const shutdownMethodCtx = "app/shutdown"

			cancel()
			if components.Bot != nil {
				log.Println("Получен сигнал завершения")
				components.Bot.StopReceivingUpdates()
			}

			shutdownErr = errors.Join(shutdownErr, shutdownServer(ctx, metricsServer))
			shutdownErr = errors.Join(shutdownErr, shutdownServer(ctx, healthServer))

			if outboxPublisher != nil {
				shutdownErr = errors.Join(shutdownErr, outboxPublisher.Close())
			}

			wg.Wait()
			shutdownErr = errors.Join(shutdownErr, closeMatchers(components.Matchers))

			if components.DBPool != nil {
				components.DBPool.Close()
				log.Println("PostgreSQL pool закрыт")
			}

			if shutdownErr != nil {
				shutdownErr = apperrors.Wrap(shutdownMethodCtx, shutdownErr)
			}
			log.Println("Завершение работы выполнено")
		})
		return shutdownErr
	}

	fail := func(err error) (func(context.Context) error, error) {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		if err := shutdown(shutdownCtx); err != nil {
			logging.Error(methodCtx, err)
		}
		return nil, apperrors.Wrap(methodCtx, err)
	}

	components, err = bootstrap.Build(ctx, cfg)
	if err != nil {
		return fail(err)
	}

	if cfg.Health.Enabled {
		healthServer, err = observability.StartHealthServer(
			cfg.Health.Address(),
			observability.ReadinessProbe(
				components.DBPool.Ping,
				components.Checkpoints,
				components.ReplicaID,
				components.Appliers,
			),
		)
		if err != nil {
			return fail(err)
		}
	}

	outboxPublisher, err = bootstrap.NewOutboxPublisher(cfg, components.DBPool)
	if err != nil {
		return fail(err)
	}
	if outboxPublisher != nil {
		run(ctx, &wg, "bootstrap/OutboxPublisher.Run", outboxPublisher.Run)
		log.Println("Watermill outbox publisher запущен")
	}

	indexSubscriber, err := bootstrap.NewIndexSubscriber(
		ctx,
		cfg,
		components.DBPool,
		components.ReplicaID,
		components.Checkpoints,
		components.Appliers,
	)
	if err != nil {
		return fail(err)
	}
	if indexSubscriber != nil {
		run(ctx, &wg, "bootstrap/IndexSubscriber.Run", indexSubscriber.Run)
		log.Println("RabbitMQ index events consumer запущен")
	}

	if cfg.Metrics.Enabled {
		indexNames := make([]string, 0, len(components.Appliers))
		for _, applier := range components.Appliers {
			indexNames = append(indexNames, applier.IndexName())
		}
		metricsServer, err = observability.StartMetricsServer(cfg.Metrics, observability.MetricsDependencies{
			DBPool:      components.DBPool.Raw(),
			Outbox:      components.Outbox,
			Checkpoints: components.Checkpoints,
			ConsumerID:  components.ReplicaID,
			IndexNames:  indexNames,
			ReplicaID:   components.ReplicaID,
		})
		if err != nil {
			return fail(err)
		}
	}

	appruntime.StartTelegramWorkers(
		ctx,
		&wg,
		components.Updates,
		cfg.Workers,
		cfg.JobsBuffer,
		components.Service,
	)

	return shutdown, nil
}

func run(ctx context.Context, wg *sync.WaitGroup, methodCtx string, run func(context.Context) error) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			logging.Error(methodCtx, err)
		}
	}()
}

func shutdownServer(ctx context.Context, server *http.Server) error {
	if server == nil {
		return nil
	}
	return server.Shutdown(ctx)
}

type closeError interface {
	Close() error
}

func closeMatchers(matchers []ports.ContentBlockMatcher) error {
	var closeErr error
	for i := len(matchers) - 1; i >= 0; i-- {
		closer, ok := matchers[i].(closeError)
		if !ok {
			continue
		}
		closeErr = errors.Join(closeErr, closer.Close())
	}
	return closeErr
}
