.PHONY: help \
	up down build \
	up-polling up-polling-eventbus up-debezium up-debezium-monitoring \
	down-polling down-debezium \
	logs logs-worker logs-server logs-kafka-connect \
	status restart clean \
	setup-debezium-connector \
	proto proto-check \
	migrate-up migrate-down \
	lint test test-integration test-e2e test-e2e-outbox test-all

help:
	@echo "Verge - Available Commands"
	@echo ""
	@echo "Docker Compose (Deployment):"
	@echo "  make up                      Start services (defaults to polling mode)"
	@echo "  make up-polling              Start in polling mode (minimal)"
	@echo "  make up-polling-eventbus     Start in polling mode with Kafka EventBus"
	@echo "  make up-debezium             Start in Debezium CDC mode"
	@echo "  make up-debezium-monitoring  Start in Debezium CDC mode with Kafka UI"
	@echo "  make down                    Stop all services"
	@echo "  make build                   Build all containers"
	@echo "  make logs                    Show all logs"
	@echo "  make logs-worker             Show worker logs"
	@echo "  make logs-server             Show server logs"
	@echo "  make status                  Show service status"
	@echo "  make restart                 Restart all services"
	@echo "  make clean                   Remove all containers and volumes"
	@echo ""
	@echo "Debezium Setup:"
	@echo "  make setup-debezium-connector  Create Debezium CDC connector"
	@echo ""
	@echo "Development:"
	@echo "  make proto                   Generate protobuf files"
	@echo "  make proto-check             Check protobuf files"
	@echo "  make migrate-up              Run database migrations"
	@echo "  make migrate-down            Rollback database migrations"
	@echo "  make lint                    Run linter"
	@echo ""
	@echo "Testing:"
	@echo "  make test                    Run unit tests"
	@echo "  make test-integration        Run integration tests"
	@echo "  make test-e2e                Run e2e tests"
	@echo "  make test-e2e-outbox         Run e2e outbox tests"
	@echo "  make test-all                Run all tests"
	@echo ""

up:
	@echo "Starting Verge (default: polling mode)..."
	@echo "Use 'make up-polling' or 'make up-debezium' for explicit mode selection"
	docker compose -f docker-compose.yml -f docker-compose.polling.yml up --build -d postgres redis neo4j server worker
	@echo "Services started in polling mode."
	@echo ""
	@echo "View logs: make logs"

down:
	@echo "Stopping all services..."
	docker compose -f docker-compose.yml -f docker-compose.polling.yml down 2>/dev/null || true
	docker compose -f docker-compose.yml -f docker-compose.debezium.yml down 2>/dev/null || true
	@echo "All services stopped."

build:
	docker compose -f docker-compose.yml -f docker-compose.polling.yml build

up-polling:
	@echo "Starting Verge in POLLING mode..."
	@echo "Services: postgres, redis, neo4j, server, worker (polling)"
	docker compose -f docker-compose.yml -f docker-compose.polling.yml up --build -d postgres redis neo4j server worker
	@echo "Polling mode started."
	@echo ""
	@echo "View logs: make logs-worker"

up-polling-eventbus:
	@echo "Starting Verge in POLLING mode with EventBus..."
	@echo "Services: postgres, redis, neo4j, kafka, server, worker (polling + eventbus)"
	docker compose -f docker-compose.yml -f docker-compose.polling.yml --profile eventbus up --build -d
	@echo "Polling mode with EventBus started."
	@echo ""

down-polling:
	docker compose -f docker-compose.yml -f docker-compose.polling.yml down

up-debezium:
	@echo "Starting Verge in DEBEZIUM CDC mode..."
	@echo "Services: postgres, redis, neo4j, kafka, kafka-connect, server, worker (debezium)"
	docker compose -f docker-compose.yml -f docker-compose.debezium.yml up --build -d
	@echo "Debezium CDC mode started."
	@echo ""
	@echo ""
	@echo "Create Debezium connector:"
	@echo ""
	@echo "1. Wait for Kafka Connect to be ready:"
	@echo "   make logs-kafka-connect"
	@echo "   (Wait for 'Kafka Connect started')"
	@echo ""
	@echo "2. Create connector:"
	@echo "   make setup-debezium-connector"
	@echo ""
	@echo "3. Verify:"
	@echo "   curl http://localhost:8083/connectors/verge-outbox-connector/status"
	@echo ""

up-debezium-monitoring:
	@echo "Starting Verge in DEBEZIUM CDC mode with monitoring..."
	docker compose -f docker-compose.yml -f docker-compose.debezium.yml --profile monitoring up --build -d
	@echo "Debezium CDC mode with monitoring started."
	@echo ""
	@echo "Create Debezium connector:"
	@echo ""
	@echo "1. Wait for Kafka Connect to be ready:"
	@echo "   make logs-kafka-connect"
	@echo "   (Wait for 'Kafka Connect started')"
	@echo ""
	@echo "2. Create connector:"
	@echo "   make setup-debezium-connector"
	@echo ""
	@echo "3. Verify:"
	@echo "   curl http://localhost:8083/connectors/verge-outbox-connector/status"
	@echo ""

down-debezium:
	docker compose -f docker-compose.yml -f docker-compose.debezium.yml down

setup-debezium-connector:
	@echo "Setting up Debezium connector..."
	@echo "Waiting for Kafka Connect to be ready..."
	@sleep 5
	@curl -X POST http://localhost:8083/connectors \
		-H "Content-Type: application/json" \
		-d @debezium-connector.json \
		&& echo "" && echo "Debezium connector created!" \
		|| echo "" && echo "Failed to create connector (check if Kafka Connect is ready)"
	@echo ""
	@echo "Verify connector status:"
	@curl -s http://localhost:8083/connectors/verge-outbox-connector/status 2>/dev/null | jq . || echo "Connector not found or jq not installed"

logs:
	docker compose -f docker-compose.yml -f docker-compose.polling.yml logs -f 2>/dev/null || \
	docker compose -f docker-compose.yml -f docker-compose.debezium.yml logs -f

logs-worker:
	docker compose -f docker-compose.yml -f docker-compose.polling.yml logs -f worker 2>/dev/null || \
	docker compose -f docker-compose.yml -f docker-compose.debezium.yml logs -f worker

logs-server:
	docker compose -f docker-compose.yml -f docker-compose.polling.yml logs -f server 2>/dev/null || \
	docker compose -f docker-compose.yml -f docker-compose.debezium.yml logs -f server

logs-kafka-connect:
	docker compose -f docker-compose.yml -f docker-compose.debezium.yml logs -f kafka-connect

status:
	@echo "Service Status:"
	@docker compose -f docker-compose.yml -f docker-compose.polling.yml ps 2>/dev/null || \
	docker compose -f docker-compose.yml -f docker-compose.debezium.yml ps

restart:
	@echo "Restarting services..."
	docker compose -f docker-compose.yml -f docker-compose.polling.yml restart 2>/dev/null || \
	docker compose -f docker-compose.yml -f docker-compose.debezium.yml restart
	@echo "Services restarted."

clean:
	@echo "Cleaning up all containers and volumes..."
	docker compose -f docker-compose.yml -f docker-compose.polling.yml down -v 2>/dev/null || true
	docker compose -f docker-compose.yml -f docker-compose.debezium.yml down -v 2>/dev/null || true
	@echo "Cleanup complete."

proto:
	docker compose run --rm tools buf generate

proto-check:
	docker compose run --rm tools buf lint
	docker compose run --rm tools buf breaking --against '.git#branch=main'

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
