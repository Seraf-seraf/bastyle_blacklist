package main

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestStartHealthServer(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server, err := startHealthServer(ctx, address, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := server.Close(); err != nil {
			t.Fatal(err)
		}
	}()

	response, err := http.Get("http://" + address + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		t.Fatalf("неожиданный HTTP-статус: %d", response.StatusCode)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != `{"status":"ok"}` {
		t.Fatalf("неожиданное тело ответа: %s", body)
	}
}

func TestStartHealthServerChecksDependency(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ready := false
	server, err := startHealthServer(ctx, address, func(context.Context) error {
		if !ready {
			return errors.New("БД недоступна")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := server.Close(); err != nil {
			t.Fatal(err)
		}
	}()

	response, err := http.Get("http://" + address + "/health")
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("неожиданный HTTP-статус: %d", response.StatusCode)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if err := response.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if string(body) != `{"status":"unavailable"}` {
		t.Fatalf("неожиданное тело ответа: %s", body)
	}

	ready = true
	response, err = http.Get("http://" + address + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("неожиданный HTTP-статус: %d", response.StatusCode)
	}
	body, err = io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != `{"status":"ok"}` {
		t.Fatalf("неожиданное тело ответа: %s", body)
	}
}

func TestStartIndexSubscriberRunsBootstrapBeforeSubscriber(t *testing.T) {
	ctx := context.Background()
	order := make([]string, 0, 2)
	runDone := make(chan struct{})
	synchronizer := &fakeStartupSynchronizer{
		catchUp: func(context.Context) error {
			order = append(order, "catch-up")
			return nil
		},
	}
	subscriber := &fakeIndexSubscriberRunner{
		run: func(context.Context) error {
			defer close(runDone)
			order = append(order, "run")
			return nil
		},
	}

	if err := startIndexSubscriber(ctx, synchronizer, subscriber); err != nil {
		t.Fatal(err)
	}
	select {
	case <-runDone:
	case <-time.After(time.Second):
		t.Fatal("subscriber не был запущен")
	}

	if len(order) != 2 || order[0] != "catch-up" || order[1] != "run" {
		t.Fatalf("неожиданный порядок запуска: %#v", order)
	}
}

func TestStartIndexSubscriberDoesNotRunSubscriberAfterBootstrapError(t *testing.T) {
	ctx := context.Background()
	expectedErr := errors.New("catch-up failed")
	synchronizer := &fakeStartupSynchronizer{
		catchUp: func(context.Context) error {
			return expectedErr
		},
	}
	subscriber := &fakeIndexSubscriberRunner{
		run: func(context.Context) error {
			t.Fatal("subscriber не должен запускаться после ошибки bootstrap catch-up")
			return nil
		},
	}

	err := startIndexSubscriber(ctx, synchronizer, subscriber)
	if !errors.Is(err, expectedErr) {
		t.Fatalf("ожидалась ошибка bootstrap catch-up: %v", err)
	}
}

type fakeStartupSynchronizer struct {
	catchUp func(context.Context) error
}

func (s *fakeStartupSynchronizer) CatchUpAllIndexes(ctx context.Context) error {
	return s.catchUp(ctx)
}

type fakeIndexSubscriberRunner struct {
	run func(context.Context) error
}

func (r *fakeIndexSubscriberRunner) Run(ctx context.Context) error {
	return r.run(ctx)
}
