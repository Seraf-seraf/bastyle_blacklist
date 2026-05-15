package main

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"testing"
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
