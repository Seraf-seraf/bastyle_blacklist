package main

import (
	"context"
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

	server, err := startHealthServer(ctx, address)
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
}
