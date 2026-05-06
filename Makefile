APP := bastyle-blacklist
MAIN := ./cmd/main.go
BUILD_DIR := build/bin
IMAGE ?= $(APP):local
CONFIG ?= configs/config.yaml
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

.PHONY: help fmt fmt-check vet test build clean up down ci

help:
	@printf '%s\n' \
		'Targets:' \
		'  fmt           format Go code' \
		'  vet           run go vet' \
		'  test          run Go tests' \
		'  build         build local binary' \
		'  up    start bot with Docker Compose' \
		'  down  stop Docker Compose stack' \
		'  ci            run CI checks'

fmt:
	gofmt -w cmd internal

fmt-check:
	@test -z "$$(gofmt -l cmd internal)"

vet:
	go vet ./...

test:
	go test ./...

build:
	mkdir -p $(BUILD_DIR)
	go build -trimpath -ldflags="-s -w -X main.version=$(VERSION)" -o $(BUILD_DIR)/$(APP) $(MAIN)

clean:
	rm -rf $(BUILD_DIR)

up:
	docker compose up -d --build

down:
	docker compose down

ci: vet test build
