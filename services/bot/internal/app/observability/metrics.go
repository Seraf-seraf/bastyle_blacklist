package observability

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/app/ports"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/config"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/pkg/apperrors"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/pkg/logging"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type MetricsDependencies struct {
	DBPool         *pgxpool.Pool
	Outbox         ports.OutboxStore
	Checkpoints    ports.IndexCheckpointStore
	ConsumerID     string
	IndexNames     []string
	ReplicaID      string
	QueryDurations map[string]time.Duration
	QueryErrors    map[string]int64
}

type appMetrics struct {
	deps                        MetricsDependencies
	buildInfo                   *prometheus.GaugeVec
	dbPoolAcquired              prometheus.Gauge
	dbPoolIdle                  prometheus.Gauge
	dbQueryDurationSeconds      *prometheus.GaugeVec
	dbErrorsTotal               *prometheus.GaugeVec
	outboxUnpublishedTotal      prometheus.Gauge
	outboxUnpublishedMaxAge     prometheus.Gauge
	indexStale                  *prometheus.GaugeVec
	indexRebuildDurationSeconds *prometheus.GaugeVec
}

func StartMetricsServer(
	cfg config.Metrics,
	deps MetricsDependencies,
) (*http.Server, error) {
	const methodCtx = "observability/StartMetricsServer"

	if deps.Outbox == nil {
		return nil, apperrors.New(methodCtx, "outbox store не настроен")
	}

	registry := prometheus.NewRegistry()
	metrics := newAppMetrics(registry, deps)
	handler := promhttp.HandlerFor(registry, promhttp.HandlerOpts{
		ErrorHandling: promhttp.ContinueOnError,
	})

	mux := http.NewServeMux()
	mux.HandleFunc(cfg.Path, func(w http.ResponseWriter, r *http.Request) {
		if err := metrics.update(r.Context()); err != nil {
			logging.Error(methodCtx, err)
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		handler.ServeHTTP(w, r)
	})

	listener, err := net.Listen("tcp", cfg.Address())
	if err != nil {
		return nil, apperrors.Wrap(methodCtx, err)
	}

	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 2 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       30 * time.Second,
		MaxHeaderBytes:    8 << 10,
	}

	go func() {
		log.Printf("Metrics-сервер слушает %s%s", cfg.Address(), cfg.Path)
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logging.Error(methodCtx, err)
		}
	}()

	return server, nil
}

func newAppMetrics(registry prometheus.Registerer, deps MetricsDependencies) *appMetrics {
	factory := promauto.With(registry)
	return &appMetrics{
		deps: deps,
		buildInfo: factory.NewGaugeVec(prometheus.GaugeOpts{
			Name: "bastyle_build_info",
			Help: "Информация о запущенной реплике Bastyle.",
		}, []string{"replica_id"}),
		dbPoolAcquired: factory.NewGauge(prometheus.GaugeOpts{
			Name: "bastyle_db_pool_acquired",
			Help: "Количество занятых соединений PostgreSQL pool.",
		}),
		dbPoolIdle: factory.NewGauge(prometheus.GaugeOpts{
			Name: "bastyle_db_pool_idle",
			Help: "Количество свободных соединений PostgreSQL pool.",
		}),
		dbQueryDurationSeconds: factory.NewGaugeVec(prometheus.GaugeOpts{
			Name: "bastyle_db_query_duration_seconds",
			Help: "Длительность последнего запроса к PostgreSQL в секундах.",
		}, []string{"operation"}),
		dbErrorsTotal: factory.NewGaugeVec(prometheus.GaugeOpts{
			Name: "bastyle_db_errors_total",
			Help: "Количество ошибок PostgreSQL по операциям.",
		}, []string{"operation"}),
		outboxUnpublishedTotal: factory.NewGauge(prometheus.GaugeOpts{
			Name: "bastyle_outbox_unpublished_total",
			Help: "Количество неопубликованных событий outbox.",
		}),
		outboxUnpublishedMaxAge: factory.NewGauge(prometheus.GaugeOpts{
			Name: "bastyle_outbox_unpublished_max_age_seconds",
			Help: "Максимальный возраст неопубликованного события outbox в секундах.",
		}),
		indexStale: factory.NewGaugeVec(prometheus.GaugeOpts{
			Name: "bastyle_index_stale",
			Help: "Признак устаревшего локального индекса.",
		}, []string{"consumer_id", "index_name"}),
		indexRebuildDurationSeconds: factory.NewGaugeVec(prometheus.GaugeOpts{
			Name: "bastyle_index_rebuild_duration_seconds",
			Help: "Длительность последней пересборки локального индекса в секундах.",
		}, []string{"index_name"}),
	}
}

func (m *appMetrics) update(ctx context.Context) error {
	const methodCtx = "observability/metrics.update"

	startedAt := time.Now()
	stats, err := m.deps.Outbox.Stats(ctx)
	outboxStatsDuration := time.Since(startedAt)
	if err != nil {
		return apperrors.Wrap(methodCtx, err)
	}

	m.buildInfo.WithLabelValues(m.deps.ReplicaID).Set(1)
	if m.deps.DBPool != nil {
		poolStats := m.deps.DBPool.Stat()
		m.dbPoolAcquired.Set(float64(poolStats.AcquiredConns()))
		m.dbPoolIdle.Set(float64(poolStats.IdleConns()))
	}
	m.dbQueryDurationSeconds.Reset()
	for operation, duration := range m.deps.QueryDurations {
		m.dbQueryDurationSeconds.WithLabelValues(operation).Set(duration.Seconds())
	}
	m.dbErrorsTotal.Reset()
	for operation, count := range m.deps.QueryErrors {
		m.dbErrorsTotal.WithLabelValues(operation).Set(float64(count))
	}
	m.dbQueryDurationSeconds.WithLabelValues("outbox_stats").Set(outboxStatsDuration.Seconds())
	m.outboxUnpublishedTotal.Set(float64(stats.UnpublishedCount))
	m.outboxUnpublishedMaxAge.Set(stats.UnpublishedMaxAge.Seconds())
	if m.deps.Checkpoints != nil {
		startedAt := time.Now()
		checkpointStats, err := m.deps.Checkpoints.Stats(ctx, m.deps.ConsumerID, m.deps.IndexNames)
		checkpointStatsDuration := time.Since(startedAt)
		if err != nil {
			return apperrors.Wrap(methodCtx, err)
		}
		m.dbQueryDurationSeconds.WithLabelValues("index_checkpoint_stats").Set(checkpointStatsDuration.Seconds())
		m.indexStale.Reset()
		for _, stat := range checkpointStats {
			value := 0.0
			if stat.Stale {
				value = 1
			}
			m.indexStale.WithLabelValues(stat.ConsumerID, stat.IndexName).Set(value)
		}
	}
	m.indexRebuildDurationSeconds.WithLabelValues("unknown").Set(0)
	return nil
}
