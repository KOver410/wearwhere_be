.PHONY: help run build tidy test up down migrate-up migrate-down migrate-create sqlc

help:
	@echo "make up             - start postgres + redis via docker-compose"
	@echo "make down           - stop docker services"
	@echo "make migrate-up     - apply all migrations"
	@echo "make migrate-down   - rollback last migration"
	@echo "make migrate-create name=add_xxx - create new migration"
	@echo "make sqlc           - regenerate sqlc code"
	@echo "make run            - run the API server"
	@echo "make build          - build binary into bin/api"
	@echo "make test           - run tests"

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

test:
	go test ./... -v -race

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
