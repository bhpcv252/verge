.PHONY: up down build proto proto-check migrate-up migrate-down lint test test-integration test-e2e test-e2e-outbox test-all

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

test-integration:
	go test -tags integration ./...

test-e2e:
	go test -tags e2e ./...

test-e2e-outbox:
	go test -tags e2e,outbox ./... -timeout 30m

test-all:
	go test -tags integration,e2e,outbox ./... -timeout 30m
