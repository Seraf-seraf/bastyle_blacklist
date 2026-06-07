BOT_DIR := services/bot
AIMATCHER_DIR := services/aimatcher

.PHONY: help fmt fmt-check vet test build clean db-up db-down db-status migrate ci docker-build-bot docker-build-aimatcher docker-build-migrations bootstrap-vault install-app helm-lint helm-template

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
	@echo "  make migrate DB_DSN=... - применить PostgreSQL migrations"
	@echo "  make ci        - полный прогон: vet + test + build"
	@echo "  make docker-build-bot - собрать локальный образ Go-бота"
	@echo "  make docker-build-aimatcher - собрать локальный образ AI matcher"
	@echo "  make docker-build-migrations - собрать образ миграций"
	@echo "  make bootstrap-vault - настроить Vault engines, policy и Kubernetes auth"
	@echo "  make install-app - установить Helm chart приложения в Kubernetes"
	@echo "  make helm-lint - проверить Helm chart приложения"

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

migrate: db-up

docker-build-bot:
	docker build -f docker/Dockerfile.bot -t blacklist:local .

docker-build-aimatcher:
	docker build -f docker/Dockerfile.aimatcher -t blacklist-aimatcher:local .

docker-build-migrations:
	docker build -f docker/Dockerfile.migrations -t blacklist-migrations:local .

bootstrap-vault:
	scripts/bootstrap-vault.sh

install-app:
	scripts/install-app.sh

helm-lint:
	helm lint charts/blacklist

helm-template:
	helm template blacklist charts/blacklist --namespace bastyle

ci:
	$(MAKE) -C $(BOT_DIR) ci
	$(MAKE) -C $(AIMATCHER_DIR) test
	$(MAKE) helm-lint
