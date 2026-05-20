package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/app/ports"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/config"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/pkg/apperrors"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/pkg/logging"
)

func startMetricsServer(
	ctx context.Context,
	cfg config.Metrics,
	store ports.OutboxStore,
) (*http.Server, error) {
	const methodCtx = "cmd/startMetricsServer"

	if store == nil {
		return nil, apperrors.New(methodCtx, "outbox store не настроен")
	}

	mux := http.NewServeMux()
	mux.HandleFunc(cfg.Path, func(w http.ResponseWriter, _ *http.Request) {
		writeMetrics(ctx, w, store)
	})

	listener, err := net.Listen("tcp", cfg.Address())
	if err != nil {
		return nil, apperrors.Wrap(methodCtx, err)
	}

	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 2 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logging.Error(methodCtx, err)
		}
	}()

	go func() {
		log.Printf("Metrics-сервер слушает %s%s", cfg.Address(), cfg.Path)
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logging.Error(methodCtx, err)
		}
	}()

	return server, nil
}

func writeMetrics(
	ctx context.Context,
	w http.ResponseWriter,
	store ports.OutboxStore,
) {
	const methodCtx = "cmd/writeMetrics"

	stats, err := store.Stats(ctx)
	if err != nil {
		logging.Error(methodCtx, err)
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	_, _ = fmt.Fprintf(w, "# TYPE bastyle_outbox_unpublished_total gauge\n")
	_, _ = fmt.Fprintf(w, "bastyle_outbox_unpublished_total %d\n", stats.UnpublishedCount)
	_, _ = fmt.Fprintf(w, "# TYPE bastyle_outbox_unpublished_max_age_seconds gauge\n")
	_, _ = fmt.Fprintf(w, "bastyle_outbox_unpublished_max_age_seconds %.0f\n", stats.UnpublishedMaxAge.Seconds())
}
