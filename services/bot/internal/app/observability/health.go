package observability

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/app/ports"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/pkg/apperrors"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/pkg/logging"
)

type ReadinessCheck func(context.Context) error

func ReadinessProbe(
	dbPing ReadinessCheck,
	checkpoints ports.IndexCheckpointStore,
	consumerID string,
	appliers []ports.IndexEventApplier,
) ReadinessCheck {
	return func(ctx context.Context) error {
		const methodCtx = "observability/readinessProbe"

		if dbPing != nil {
			if err := dbPing(ctx); err != nil {
				return apperrors.Wrap(methodCtx, err)
			}
		}
		if checkpoints == nil {
			return nil
		}

		indexNames := make([]string, 0, len(appliers))
		for _, applier := range appliers {
			indexNames = append(indexNames, applier.IndexName())
		}
		return apperrors.Wrap(methodCtx, checkpoints.CheckFresh(ctx, consumerID, indexNames))
	}
}

func StartHealthServer(address string, readiness ReadinessCheck) (*http.Server, error) {
	const methodCtx = "observability/StartHealthServer"

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		writeReadinessStatus(r.Context(), w, readiness)
	})

	listener, err := net.Listen("tcp", address)
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
		log.Printf("Health-сервер слушает %s", address)
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logging.Error(methodCtx, err)
		}
	}()

	return server, nil
}

func writeReadinessStatus(ctx context.Context, w http.ResponseWriter, readiness ReadinessCheck) {
	const methodCtx = "observability/writeReadinessStatus"

	if readiness != nil {
		checkCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		if err := readiness(checkCtx); err != nil {
			logging.Error(methodCtx, err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"unavailable"}`))
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}
