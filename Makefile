.PHONY: up down build proto proto-check migrate-up migrate-down lint test test-all

up:
	docker compose up --build

down:
	docker compose down

build:
	docker compose build

proto:
	docker compose run --rm tools buf generate

migrate-up:
	docker compose run --rm tools migrate \
	-path=/workspace/migrations \
	-database "postgres://verge:changeme@postgres:5432/verge?sslmode=disable" up

migrate-down:
	docker compose run --rm tools migrate \
	-path=/workspace/migrations \
	-database "postgres://verge:changeme@postgres:5432/verge?sslmode=disable" down

lint:
	docker compose run --rm tools golangci-lint run

test:
	go test ./...

test-all:
	go test -tags integration,e2e ./...
