BOT_DIR := services/bot
AIMATCHER_DIR := services/aimatcher
COMPOSE := docker compose -f infra/docker-compose.yaml --project-directory .

.PHONY: help fmt fmt-check vet test build clean db-up db-down db-status db-logs db-shell migrate up stop down ps logs ci docker-build-bot docker-build-aimatcher docker-build-postgres-exporter install-platform bootstrap-vault install-app

help:
	@echo "Доступные команды:"
	@echo "  make fmt       - отформатировать Go-код бота"
	@echo "  make fmt-check - проверить форматирование Go-кода бота"
	@echo "  make vet       - запустить go vet для бота"
	@echo "  make test      - запустить все тесты сервисов"
	@echo "  make build     - собрать бинарник бота"
	@echo "  make clean     - удалить локальные build-артефакты"
	@echo "  make db-up     - применить PostgreSQL migrations через goose"
	@echo "  make db-down   - откатить последнюю PostgreSQL migration через goose"
	@echo "  make db-status - показать статус PostgreSQL migrations через goose"
	@echo "  make db-logs   - показать логи PostgreSQL контейнера"
	@echo "  make db-shell  - открыть psql в PostgreSQL контейнере"
	@echo "  make migrate   - применить PostgreSQL migrations"
	@echo "  make ci        - полный прогон: vet + test + build"
	@echo "  make up        - запуск сервисов через Docker Compose (build + detached)"
	@echo "  make stop      - остановка контейнеров без удаления"
	@echo "  make down      - остановка и удаление контейнеров"
	@echo "  make ps        - статус контейнеров в табличном виде"
	@echo "  make logs      - логи всех сервисов (follow)"
	@echo "  make docker-build-bot - собрать локальный образ Go-бота"
	@echo "  make docker-build-aimatcher - собрать локальный образ AI matcher"
	@echo "  make docker-build-postgres-exporter - собрать локальный образ PostgreSQL exporter"
	@echo "  make install-platform - установить Vault и Vault Secrets Operator в Kubernetes"
	@echo "  make bootstrap-vault - настроить Vault engines, policy и Kubernetes auth"
	@echo "  make install-app - установить Helm chart приложения в Kubernetes"

fmt:
	$(MAKE) -C $(BOT_DIR) fmt

fmt-check:
	$(MAKE) -C $(BOT_DIR) fmt-check

vet:
	$(MAKE) -C $(BOT_DIR) vet

test:
	$(MAKE) -C $(BOT_DIR) test
	$(MAKE) -C $(AIMATCHER_DIR) test

build:
	$(MAKE) -C $(BOT_DIR) build

clean:
	$(MAKE) -C $(BOT_DIR) clean

db-up:
	$(MAKE) -C $(BOT_DIR) db-up

db-down:
	$(MAKE) -C $(BOT_DIR) db-down

db-status:
	$(MAKE) -C $(BOT_DIR) db-status

db-logs:
	$(COMPOSE) logs -f bastyle-postgresql

db-shell:
	$(COMPOSE) exec bastyle-postgresql psql -U bastyle -d bastyle

migrate: db-up

up:
	$(COMPOSE) up -d --build --pull missing

stop:
	$(COMPOSE) stop

down:
	$(COMPOSE) down

ps:
	$(COMPOSE) ps

logs:
	$(COMPOSE) logs -f

docker-build-bot:
	docker build -f infra/docker/Dockerfile.bot -t bastyle-blacklist:local .

docker-build-aimatcher:
	docker build -f infra/docker/Dockerfile.aimatcher -t bastyle-aimatcher:local .

docker-build-postgres-exporter:
	docker build -f infra/docker/Dockerfile.postgres-exporter -t bastyle-postgres-exporter:local .

install-platform:
	infra/scripts/install-platform.sh

bootstrap-vault:
	infra/scripts/bootstrap-vault.sh

install-app:
	infra/scripts/install-app.sh

ci:
	$(MAKE) -C $(BOT_DIR) ci
	$(MAKE) -C $(AIMATCHER_DIR) test
