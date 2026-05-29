# Contributing to Verge

Thanks for wanting to contribute to Verge, a version control system for non-code assets.

---

## What kind of help is useful right now?

As this project is in early development phase, the following help is much appreciated.

**Documentation** - If something is unclear, wrong, or missing, open an issue or a PR directly.

**Feature requests** - open an issue and describe what you're trying to do and why the current API doesn't support it. Be specific about your use case. "I'm building X and I need Y because Z" is a lot more useful than a vague feature name.

**Bug reports** are always welcome. If something is broken, open an issue and describe what you sent, what came back, and what you expected instead. A minimal reproduction case helps a lot.

**Code contributions** - read the section below before starting anything significant.

---

## Before you start building something

Please open an issue before starting any non-trivial code change. It avoids the frustrating situation where you put real time into something that doesn't align with where the project is heading, or that someone else is already working on.

Things most likely to get merged:

- Bug fixes with a clear reproduction case
- Documentation
- New storage backend implementations that follow the pluggable backend interface
- SDK contributions in TypeScript, Python, or Go.
- Test coverage for existing flows

---

## Getting started

### Prerequisites

- **Go 1.25.0 or later** - Check with `go version`
- **Docker & Docker Compose** - Required for running PostgreSQL, Redis, Neo4j, Kafka, and build tools
- **Make** - For running build commands
- **Git** - For version control

### Initial setup

```bash
# Clone the repository
git clone https://github.com/bhpcv252/verge.git
cd verge

# Install git hooks (optional but recommended)
# Runs lint, tests, and formatting checks before commits
# Install lefthook first: https://github.com/evilmartians/lefthook
lefthook install

# Download Go dependencies
go mod download

# Generate protobuf files (requires Docker)
make proto

# Start PostgreSQL (required for local development)
docker compose up -d postgres

# Run database migrations
make migrate-up

# Verify everything works
make test
```

### Running locally

**Option 1: Docker Compose (recommended for full stack)**

```bash
# Start all services (PostgreSQL, Redis, Neo4j, API server, outbox worker)
# Defaults to polling mode
make up

# Or explicitly choose a mode:
make up-polling              # PostgreSQL + Redis + Neo4j, worker polls the outbox table
make up-polling-eventbus     # Same, but worker publishes to Kafka; external consumers run handlers
make up-debezium             # Debezium CDC mode - worker reads from Kafka topic

# API server will be available at:
# - HTTP:  http://localhost:8080
# - gRPC:  http://localhost:9090
# - Health: http://localhost:8080/health

# Stop all services
make down
```

**Option 2: Native Go (for faster iteration)**

```bash
# Start infrastructure only
docker compose up -d postgres

# Run migrations
make migrate-up

# Run the API server
go run cmd/server/main.go

# In a separate terminal, run the outbox worker
go run cmd/worker/main.go
```

The server and worker are separate binaries. The worker is responsible for propagating outbox events to
derived stores (Redis, Neo4j). For PostgreSQL-only development you can skip the worker; the API still
functions correctly without it.

**Environment variables**

All configuration is via environment variables with the `VERGE_` prefix. Copy `.env.example` to `.env` and
adjust. Key groups:

- `VERGE_SERVER_*`: HTTP and gRPC listen ports and enable/disable toggles.
- `VERGE_STORAGE_*`: PostgreSQL URL (always required); Redis and Neo4j URLs and enable flags.
- `VERGE_AUTH_ENABLED` / `VERGE_AUTH_KEYS`: optional API key auth. Set `VERGE_AUTH_ENABLED=true` and provide
  at least one key in `VERGE_AUTH_KEYS` (comma-separated). All `/v1` requests then require
  `Authorization: Bearer <key>`.
- `VERGE_OTEL_ENABLED` / `VERGE_OTEL_EXPORTER`: observability toggle and exporter backend (`stdout`,
  `otlp`, `prometheus`). Disabled by default; when off, all telemetry is a no-op.
- `VERGE_OUTBOX_*`: outbox worker source type, polling interval, Debezium/Kafka settings.

---

## Testing

### Unit tests

```bash
make test
# equivalent: go test ./...
```

Runs all unit and integration tests that do not require Docker.

### E2E tests

```bash
make test-e2e
# equivalent: go test -tags e2e ./...
```

Spins up real PostgreSQL containers via testcontainers. Requires Docker to be running.
Covers the full REST and gRPC API surface for all four resources (repos, branches, commits, merges).

### Outbox and matrix tests

```bash
make test-e2e-outbox
# equivalent: go test -tags e2e,outbox ./... -timeout 30m
```

Requires Docker. Spins up PostgreSQL, Redis, and Neo4j containers and runs the full test matrix
across four storage tiers:

| Tier                   | Backends active    |
| ---------------------- | ------------------ |
| `postgres-only`        | PostgreSQL only    |
| `postgres+redis`       | PostgreSQL + Redis |
| `postgres+neo4j`       | PostgreSQL + Neo4j |
| `postgres+redis+neo4j` | All three backends |

Also includes Kafka/Redpanda tests that exercise the full outbox pipeline end-to-end
(producer worker → Kafka → consumer → Neo4j/Redis projections).

### All tests

```bash
make test-all
# equivalent: go test -tags integration,e2e,outbox ./... -timeout 30m
```

---

## Opening a pull request

1. Fork the repo and create a branch off `main`. Give it a name that says what it does: `fix/branch-conflict-response`, `feat/typescript-sdk`, `docs/grpc-examples`.
2. Keep your commits focused. One logical change per commit makes reviews easier.
3. If you changed API behavior, update the relevant docs.
4. If you changed how something works, add or update tests.
5. Open a PR against `main`. Describe what the change does, why it's needed, and how to test it.

---

## Code conventions

- Match the style of whatever file you're editing
- Keep functions focused on one thing
- Handle errors explicitly
- Any new API behavior needs a structured error response with a machine-readable `error` code and a human-readable `message`
- New storage backend operations go through the composite router layer, not directly to services
- Auth middleware lives in `internal/auth`. HTTP and gRPC variants are separate files (`http.go`, `grpc.go`). Both receive a `*auth.Validator` that is `nil` when auth is disabled, which makes them pass-through with zero overhead.
- Observability lives in `internal/observability`. The `Provider` carries a `Tracer`, `Meter`, `Logger`, and pre-registered `Metrics`. Pass `*observability.Provider` into components that need telemetry; do not use global OTel calls directly. For request-scoped logging, propagate the logger through context using `observability.WithLogger` / `observability.L(ctx)`.

---

## Commit messages

Write them in the imperative: `Add idempotency key support to commit creation`, not `Added` or `Adds`. Keep the subject line under 72 characters.
If the change needs more explanation, add a body after a blank line.

---

## Security issues

Please don't open a public GitHub issue for security vulnerabilities. [Email](mailto:sayhellotosonu@gmail.com) the maintainers directly instead.

---

## License

By contributing, you agree your changes will be licensed under the MIT License.
