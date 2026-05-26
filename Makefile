.PHONY: help run build tidy test test-unit test-integration test-db-up test-db-reset up down migrate-up migrate-down migrate-create sqlc

help:
	@echo "make up             - start postgres + redis via docker-compose"
	@echo "make down           - stop docker services"
	@echo "make migrate-up     - apply all migrations"
	@echo "make migrate-down   - rollback last migration"
	@echo "make migrate-create name=add_xxx - create new migration"
	@echo "make sqlc           - regenerate sqlc code"
	@echo "make run            - run the API server"
	@echo "make build          - build binary into bin/api"
	@echo "make test           - run unit + integration tests"
	@echo "make test-unit      - run unit tests (no DB required)"
	@echo "make test-integration - run integration tests (requires test DB)"
	@echo "make test-db-up     - create wearwhere_test DB and run migrations"
	@echo "make test-db-reset  - drop + recreate wearwhere_test DB and migrate"

up:
	docker-compose up -d

down:
	docker-compose down

tidy:
	go mod tidy

run:
	go run ./cmd/api

build:
	go build -o bin/api ./cmd/api

test: test-unit test-integration

# Requires: go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
DB_URL ?= postgres://wearwhere:wearwhere@localhost:5432/wearwhere?sslmode=disable
MIGRATIONS_DIR=db/migrations

migrate-up:
	migrate -path $(MIGRATIONS_DIR) -database "$(DB_URL)" up

migrate-down:
	migrate -path $(MIGRATIONS_DIR) -database "$(DB_URL)" down 1

migrate-create:
	migrate create -ext sql -dir $(MIGRATIONS_DIR) -seq $(name)

# Requires: go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
sqlc:
	sqlc generate

# ── Test targets ──────────────────────────────────────────────────────────────
TEST_DB_URL ?= postgres://wearwhere:wearwhere@localhost:5432/wearwhere_test?sslmode=disable

test-db-up:
	@docker compose exec -T postgres psql -U wearwhere -d wearwhere \
	    -c "CREATE DATABASE wearwhere_test;" 2>/dev/null || true
	migrate -path $(MIGRATIONS_DIR) -database "$(TEST_DB_URL)" up

test-db-reset:
	@docker compose exec -T postgres psql -U wearwhere -d wearwhere \
	    -c "DROP DATABASE IF EXISTS wearwhere_test; CREATE DATABASE wearwhere_test;"
	migrate -path $(MIGRATIONS_DIR) -database "$(TEST_DB_URL)" up

test-unit:
	go test ./... -v -race

test-integration: test-db-up
	TEST_DATABASE_URL="$(TEST_DB_URL)" go test -p 1 ./... -tags=integration -v -race
