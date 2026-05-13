APP := bastyle-blacklist
BOT_DIR := services/bot
AIMATCHER_DIR := services/aimatcher
BUILD_DIR := infra/bin
IMAGE ?= $(APP):local
CONFIG ?= infra/config/config.yaml
COMPOSE := docker compose -f infra/docker-compose.yaml --project-directory .
PYTHON := .venv/bin/python
PYTHONPATH := $(AIMATCHER_DIR)

.PHONY: help fmt fmt-check vet test py-test build clean up stop down ps logs ci

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

fmt:
	gofmt -w $(BOT_DIR)/cmd $(BOT_DIR)/internal

fmt-check:
	@test -z "$$(gofmt -l $(BOT_DIR)/cmd $(BOT_DIR)/internal)"

vet:
	go -C $(BOT_DIR) vet ./...

test:
	go -C $(BOT_DIR) test ./...

py-test:
	PYTHONPATH=$(PYTHONPATH) $(PYTHON) -m pytest $(AIMATCHER_DIR)/ai_vector_service/tests

build:
	mkdir -p $(BUILD_DIR)
	go -C $(BOT_DIR) build -trimpath -ldflags="-s -w" -o ../../$(BUILD_DIR)/$(APP) ./cmd/main.go

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

ci: vet test build
