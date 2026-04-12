# Verge - Technical Setup

> **Design Philosophy**: Verge is a self-hosted microservice. Products deploy it into their own infrastructure. We provide deployment flexibility, observability hooks, and infrastructure-agnostic design.

---

## Table of Contents

- [Technology Stack](#technology-stack)
- [Project Structure](#project-structure)
- [Observability Integration](#observability-integration)
- [Authentication & Security](#authentication--security)
- [Configuration](#configuration)
- [Development Setup](#development-setup)
- [Testing Strategy](#testing-strategy)
- [Build & Release](#build--release)

---

## Technology Stack

### Core

- **Language**: Go 1.22+
- **HTTP Framework**: chi (lightweight, idiomatic, stdlib-compatible)
- **gRPC**: official Go gRPC implementation
- **Database Driver**: pgx (PostgreSQL)
- **Redis Client**: go-redis
- **Neo4j Driver**: neo4j-go-driver

### Libraries

```go
// HTTP & gRPC
chi v5
grpc v1.60+

// API Documentation
openapi (spec-first REST contracts)
swagger-ui (API exploration & testing)

// Database
pgx v5
golang-migrate
neo4j-go-driver v5
go-redis v9

// Observability (standards-based)
zerolog              // structured logging (JSON)
prometheus/client_go // metrics (Prometheus format)
otel                 // OpenTelemetry tracing

// Testing
testify
testcontainers-go
gomock

// Security
crypto/tls
crypto/x509

// Configuration
viper
validator

// Utilities
google/uuid
shopspring/decimal
```

---

## Project Structure

```
verge/
├── cmd/
│   ├── server/
│   │   └── main.go
│   ├── worker/
│   │   └── main.go
│
├── internal/
│   ├── api/
│   │   ├── rest/
│   │   │   ├── v1/
│   │   │   │   ├── handlers.go
│   │   │   │   ├── repos.go
│   │   │   │   ├── branches.go
│   │   │   │   ├── commits.go
│   │   │   │   └── merges.go
│   │   │   └── router.go
│   │   │
│   │   └── grpc/
│   │       ├── v1/
│   │       │   ├── server.go
│   │       │   ├── repos.go
│   │       │   ├── branches.go
│   │       │   ├── commits.go
│   │       │   └── merges.go
│   │       └── server.go
│   │
│   ├── domain/
│   ├── service/
│   ├── storage/
│   ├── outbox/
│   ├── config/
│   ├── observability/
│   └── auth/
│
├── pkg/
├── api/
│   ├── proto/
│   │   └── verge/v1/
│   └── openapi/
│       └── verge-v1.yaml
│
├── migrations/
├── scripts/
├── test/
├── docs/
├── .github/
├── go.mod
├── Makefile
└── README.md
```

---

## Observability Integration

### Observability Philosophy

Verge does not integrate with specific observability vendors.

Instead, it emits **industry-standard signals**:

- Logs → structured JSON (stdout)
- Metrics → Prometheus format
- Traces → OpenTelemetry (OTLP)

This allows products to plug Verge into any observability stack without vendor lock-in.

---

### Logging

- Output: **stdout**
- Format: **structured JSON**

```json
{
  "level": "info",
  "message": "commit created",
  "repo_id": "repo_123",
  "trace_id": "abc123"
}
```

```bash
VERGE_LOG_LEVEL=info
VERGE_LOG_FORMAT=json
```

---

### Metrics

- Endpoint: `/metrics`
- Format: **Prometheus exposition format**

Examples:

```
verge_http_requests_total
verge_commits_created_total
verge_outbox_events_processed_total
```

---

### Tracing

- Standard: **OpenTelemetry**
- Export: **OTLP**

```bash
VERGE_TRACING_ENABLED=true
VERGE_OTEL_EXPORTER=otlp
VERGE_OTEL_ENDPOINT=http://collector:4318
```

---

### Health Checks

```
GET /health
GET /ready
```

---

## Authentication & Security

### mTLS

```bash
VERGE_TLS_ENABLED=true
VERGE_TLS_CERT_FILE=/certs/server.crt
VERGE_TLS_KEY_FILE=/certs/server.key
VERGE_TLS_CA_FILE=/certs/ca.crt
```

- Client cert validated against CA
- Identity extracted from CN/SAN

---

## Configuration

### Priority

1. Environment variables
2. Config file
3. Defaults

---

### Example

```bash
VERGE_HTTP_PORT=8080
VERGE_POSTGRES_HOST=localhost
VERGE_REDIS_ENABLED=false
VERGE_NEO4J_ENABLED=false
```

---

## Development Setup

```bash
git clone <repo>
cd verge

go mod download
make proto

docker compose up -d postgres

make migrate-up
make test
make run
```

---

## Testing Strategy

### Unit Tests

- Domain + service logic

### Integration Tests

- Real DBs via testcontainers

### E2E Tests

- Full flows

---

## Build & Release

### Docker

```dockerfile
FROM golang:1.22-alpine AS builder
RUN go build -o verge-server

FROM alpine
COPY --from=builder /verge-server .
ENTRYPOINT ["./verge-server"]
```

---

### Versioning

- Semantic Versioning: `v1.0.0`
- API versioning: `/v1` (REST + gRPC)
- Parallel versions supported

---

### Release Artifacts

- Docker images
- Binaries
- Helm charts
- Proto files

---

### CI/CD

- Run tests
- Build binaries
- Build & push Docker images
- Tag releases
