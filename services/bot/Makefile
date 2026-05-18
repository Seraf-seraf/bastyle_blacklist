APP := bastyle-blacklist
BUILD_DIR := ../../infra/bin
MIGRATIONS_DIR := internal/adapters/database/postgres/migrations
CONFIG ?= ../../infra/config/config.yaml
DB_DSN ?= $(shell ../../infra/scripts/database-dsn.sh "$(CONFIG)" 2>/dev/null)
GOOSE_TAGS := no_mysql no_sqlite3 no_ydb no_clickhouse no_mssql no_vertica no_turso
GOOSE := GOFLAGS=-mod=mod go run -tags '$(GOOSE_TAGS)' github.com/pressly/goose/v3/cmd/goose@v3.27.0

.PHONY: help fmt fmt-check vet test build clean db-up db-down db-status _check-db-dsn ci

help:
	@echo "Доступные команды bot:"
	@echo "  make fmt       - отформатировать Go-код"
	@echo "  make fmt-check - проверить форматирование Go-кода"
	@echo "  make vet       - запустить go vet"
	@echo "  make test      - запустить все тесты бота"
	@echo "  make build     - собрать бинарник бота"
	@echo "  make clean     - удалить локальные build-артефакты"
	@echo "  make db-up     - применить PostgreSQL migrations через goose"
	@echo "  make db-down   - откатить последнюю PostgreSQL migration через goose"
	@echo "  make db-status - показать статус PostgreSQL migrations через goose"
	@echo "  make ci        - полный прогон: vet + test + build"

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
	go build -trimpath -ldflags="-s -w" -o $(BUILD_DIR)/$(APP) ./cmd/main.go

clean:
	rm -rf $(BUILD_DIR)

_check-db-dsn:
	@test -n "$(DB_DSN)" || (echo "DB_DSN is required. Set DB_DSN or provide CONFIG with database.dsn."; exit 1)

db-up: _check-db-dsn
	$(GOOSE) -dir $(MIGRATIONS_DIR) -table postgres_schema_migrations postgres "$(DB_DSN)" up

db-down: _check-db-dsn
	$(GOOSE) -dir $(MIGRATIONS_DIR) -table postgres_schema_migrations postgres "$(DB_DSN)" down

db-status: _check-db-dsn
	$(GOOSE) -dir $(MIGRATIONS_DIR) -table postgres_schema_migrations postgres "$(DB_DSN)" status

ci: vet test build
