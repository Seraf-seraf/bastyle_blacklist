APP := bastyle-blacklist
MAIN := ./cmd/main.go
BUILD_DIR := build/bin
IMAGE ?= $(APP):local
CONFIG ?= configs/config.yaml
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMPOSE := docker compose -f build/docker-compose.yaml --project-directory .

.PHONY: help fmt fmt-check vet test py-test build clean up stop down ps logs ai-data-10k ai-data-50k ai-data-100k ai-hnsw-bench ai-static-quality ci

help:
	@echo "Доступные команды:"
	@echo "  make fmt       - отформатировать Go-код"
	@echo "  make fmt-check - проверить форматирование Go-кода"
	@echo "  make vet       - запустить go vet"
	@echo "  make test      - запустить все Go-тесты"
	@echo "  make py-test   - запустить тесты Python AI-компонента"
	@echo "  make build     - собрать бинарник бота"
	@echo "  make clean     - удалить локальные build-артефакты"
	@echo "  make ci        - полный прогон: vet + test + build"
	@echo "  make up        - запуск бота через Docker Compose (build + detached)"
	@echo "  make stop      - остановка контейнеров без удаления"
	@echo "  make down      - остановка и удаление контейнеров"
	@echo "  make ps        - статус контейнеров в табличном виде"
	@echo "  make logs      - логи всех сервисов (follow)"
	@echo "  make ai-data-10k    - скачать 10k COCO train2017 images для AI benchmark"
	@echo "  make ai-data-50k    - скачать 50k COCO train2017 images для AI benchmark"
	@echo "  make ai-data-100k   - скачать 100k COCO train2017 images для AI benchmark"
	@echo "  make ai-hnsw-bench  - HNSW benchmark на synthetic или VECTORS=*.npz"
	@echo "  make ai-static-quality - threshold report на MANIFEST=*.jsonl"

fmt:
	gofmt -w cmd internal

fmt-check:
	@test -z "$$(gofmt -l cmd internal)"

vet:
	go vet ./...

test:
	go test ./...

py-test:
	python3 -m pytest ai_vector_service/tests

build:
	mkdir -p $(BUILD_DIR)
	go build -trimpath -ldflags="-s -w -X main.version=$(VERSION)" -o $(BUILD_DIR)/$(APP) $(MAIN)

clean:
	rm -rf $(BUILD_DIR)

up:
	$(COMPOSE) up -d --build

stop:
	$(COMPOSE) stop

down:
	$(COMPOSE) down

ps:
	$(COMPOSE) ps

logs:
	$(COMPOSE) logs -f

ai-data-10k:
	python3 -m ai_vector_service.benchmarks.download_coco --limit 10000

ai-data-50k:
	python3 -m ai_vector_service.benchmarks.download_coco --limit 50000

ai-data-100k:
	python3 -m ai_vector_service.benchmarks.download_coco --limit 100000

ai-hnsw-bench:
	python3 -m ai_vector_service.benchmarks.hnsw_benchmark $(if $(VECTORS),--vectors $(VECTORS),)

ai-static-quality:
	@test -n "$(MANIFEST)" || (echo "MANIFEST is required"; exit 1)
	python3 -m ai_vector_service.benchmarks.static_quality --manifest $(MANIFEST)

ci: vet test build
