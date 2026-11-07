SQLC_VERSION    := v1.31.1
WIRE_VERSION    := v0.7.0
SWAG_VERSION    := v1.16.6
MIGRATE_VERSION := v4.20.1

-include .env
export

DB_URL ?= postgres://$(DB_USER):$(DB_PASSWORD)@$(DB_HOST):$(DB_PORT)/$(DB_NAME)?sslmode=$(DB_SSLMODE)
MIGRATE := go run -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@$(MIGRATE_VERSION)

.PHONY: run build tidy wire sqlc swag generate migrate-up migrate-down migrate-create docker-up docker-down test test-run test-db-up test-db-down lint

run:
	go run ./cmd/api

build:
	CGO_ENABLED=0 go build -o bin/api ./cmd/api

tidy:
	go mod tidy

wire:
	go run github.com/google/wire/cmd/wire@$(WIRE_VERSION) ./internal/di

sqlc:
	go run github.com/sqlc-dev/sqlc/cmd/sqlc@$(SQLC_VERSION) generate

swag:
	go run github.com/swaggo/swag/cmd/swag@$(SWAG_VERSION) init -g cmd/api/main.go -o docs --parseInternal

generate: sqlc swag wire

migrate-up:
	$(MIGRATE) -path db/migrations -database "$(DB_URL)" up

migrate-down:
	$(MIGRATE) -path db/migrations -database "$(DB_URL)" down 1

migrate-create:
	@test -n "$(name)" || (echo "usage: make migrate-create name=<name>" && exit 1)
	$(MIGRATE) create -ext sql -dir db/migrations -seq $(name)

# Starts Postgres and Mailpit and applies migrations. Add --profile app to also run the API in a container.
docker-up:
	docker compose up -d postgres migrate mailpit

docker-down:
	docker compose down

# Integration tests run against a real Postgres and Mailpit (compose services postgres-test
# and mailpit-test), no mocks. `test` starts them, runs the suite, and always tears them down.
test:
	@$(MAKE) --no-print-directory test-db-up
	@go test -count=1 ./... ; status=$$?; $(MAKE) --no-print-directory test-db-down; exit $$status

# Run the suite against already running test services.
test-run:
	go test -count=1 ./...

test-db-up:
	docker compose --profile test up -d --wait postgres-test mailpit-test

test-db-down:
	docker compose --profile test rm -sfv postgres-test mailpit-test

lint:
	go vet ./...
