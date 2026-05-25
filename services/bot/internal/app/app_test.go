package app

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestRunHelperCancelsAndWaitsForRunner(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup

	started := make(chan struct{})
	done := make(chan struct{})
	run(ctx, &wg, "test/runner", func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		close(done)
		return ctx.Err()
	})

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("runner не запущен")
	}

	cancel()
	wg.Wait()

	select {
	case <-done:
	default:
		t.Fatal("shutdown не дождался завершения runner-а")
	}
}
