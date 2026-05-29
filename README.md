# Verge

> The version control layer for your product

[![codecov](https://codecov.io/github/bhpcv252/verge/graph/badge.svg?token=QCPB1JQS9K)](https://codecov.io/github/bhpcv252/verge)

Verge is a backend service that gives any product versioning, branching, and history without storing or touching data.

You keep your data in your own storage. Verge keeps track of the graph: what changed, when, who made it, what came before it, and how branches relate to each other. It's the commit graph engine you'd otherwise have to build yourself.

---

## Who is this for?

Verge is for product engineers building applications where users need version history, branching, or the ability to roll back changes. You integrate Verge from your backend, it's never exposed directly to end users.

---

## How it works

Verge is built around a small set of concepts:

**Repository** - a container for one resource in your product (one document, one design file, one workflow). You create one repo per versioned resource.

**Commit** - an immutable snapshot reference. Each commit carries a `DataPointer` (the location of your data in your own storage), a parent reference, a message, and an author. Verge never reads your data, only the pointer.

**Branch** - a named pointer to a commit. You advance it when you want the branch head to move forward.

**DataPointer** - the only thing Verge stores about your actual data. It's a `type + location + optional hash + optional metadata` struct that you construct. Verge treats it as an opaque blob.

A typical save looks like this:

```
1. User saves → you serialize and store the snapshot in your own DB/S3
2. You construct a DataPointer pointing to it
3. POST /v1/repos/:repo_id/commits        → Verge records the commit
4. PATCH /v1/repos/:repo_id/branches/main → Verge advances the branch
```

Restore, branching, merging, and history queries all follow the same pattern. Your product owns the data and the logic, Verge owns the graph.

---

## API

Verge exposes both a REST API (all routes under `/v1`) and a gRPC API (`.proto` files included). No SDK is required, the raw HTTP interface is enough to fully integrate.

Core endpoints:

| Operation         | Endpoint                                                   |
| ----------------- | ---------------------------------------------------------- |
| Create repository | `POST /v1/repos`                                           |
| Create commit     | `POST /v1/repos/:repo_id/commits`                          |
| Advance branch    | `PATCH /v1/repos/:repo_id/branches/:name`                  |
| Create branch     | `POST /v1/repos/:repo_id/branches`                         |
| Merge branch      | `POST /v1/repos/:repo_id/merges`                           |
| Get branch head   | `GET /v1/repos/:repo_id/branches/:name`                    |
| Get commit        | `GET /v1/repos/:repo_id/commits/:commit_id`                |
| Traverse history  | `GET /v1/repos/:repo_id/commits?traversal=dag&branch=main` |

An interactive Swagger UI is available at `GET /docs` when the HTTP server is running. The raw OpenAPI 3.0 spec is at `GET /docs/openapi.yaml` (embedded at build time, suitable for SDK generation).

All write operations use optimistic locking. If two callers try to advance the same branch concurrently, one gets a `409` with the current head so it can retry safely without losing anything.

For the full API reference, see [ARCHITECTURE.md](./ARCHITECTURE.md).

---

## Storage Backends

Verge is designed to scale with your product:

**PostgreSQL** (required) - Always the source of truth. All commits, branches, and metadata are stored here.

**Redis** (optional) - Enable for sub-millisecond branch head reads and cached commit lookups. Verge maintains two separate Redis data structures: a branch head cache (with configurable TTL) and a commit object cache. Both fall back to PostgreSQL transparently on a miss.

**Neo4j** (optional) - Enable for complex ancestry queries at scale. Neo4j provides optimized graph traversal for deep history queries and merge-base operations, with automatic fallback to PostgreSQL recursive CTEs when unavailable.

You choose which backends to enable based on your product's scale and infrastructure. The API surface is identical regardless, you can start with PostgreSQL only and add Redis or Neo4j later without changing your integration.

All derived stores (Redis, Neo4j) are projections that can be rebuilt from PostgreSQL at any time.

---

## Authentication

Authentication is optional. When disabled (the default), Verge relies on network-level controls: mTLS,
a reverse proxy, or VPC isolation. When `VERGE_AUTH_ENABLED=true`, every `/v1` request and every gRPC
call must include a valid API key.

**HTTP:** set `Authorization: Bearer <key>` on every request.

**gRPC:** set the `authorization` metadata key to `Bearer <key>` on every call.

Keys are configured as a comma-separated list in `VERGE_AUTH_KEYS`. Multiple keys can be active at the
same time for zero-downtime rotation. Failed attempts increment the `verge_auth_failures_total` metric
and are logged as warnings.

---

## Observability

Verge is instrumented with OpenTelemetry throughout. All telemetry is a no-op when `VERGE_OTEL_ENABLED=false`
(the default).

Three exporter backends are built in:

- **`stdout`**: pretty-print spans and metrics to the console. Good for development.
- **`otlp`**: push spans and metrics to any OTLP-compatible backend (Datadog, Grafana Cloud, Honeycomb, Jaeger) via `VERGE_OTEL_OTLP_ENDPOINT`.
- **`prometheus`**: expose a `GET /metrics` scrape endpoint in OpenMetrics format.

Every HTTP request and gRPC call gets a server span with W3C TraceContext propagation, so Verge slots into
an existing distributed trace automatically. Every storage call gets a child span. Structured logs are
propagated through `context.Context` so every log line carries the request's ID and route.

See [ARCHITECTURE.md](./ARCHITECTURE.md#observability) for the full metrics reference and configuration variables.

---

## Running Verge

Verge ships as two binaries: `cmd/server` (the API server) and `cmd/worker` (the outbox worker).
The worker propagates commit and branch changes to Redis and Neo4j asynchronously. The API server
functions correctly without it, but Redis and Neo4j will not receive updates until the worker runs.

```bash
# Full stack via Docker Compose (recommended)
make up

# Or run natively
go run cmd/server/main.go   # API server
go run cmd/worker/main.go   # Outbox worker (separate terminal)
```

See [CONTRIBUTING.md](./CONTRIBUTING.md) for full setup instructions and all available `make` commands.

---

## Integration guides

- [Product integration flows](./PRODUCT_INTEGRATION_FLOW.md) - end-to-end walkthroughs for three different product types (document editor, design tool, AI workflow builder)
- [Architecture and API reference](./ARCHITECTURE.md) - full REST and gRPC API specs, entity model, storage backend details, outbox worker architecture, and error code reference

---

## Roadmap

Verge is early. Here's what's coming:

- **SDKs** - official clients for TypeScript/Node.js, Python, and Go.

---

## Contributing

Contributions are welcome. Please read [CONTRIBUTING.md](./CONTRIBUTING.md) before opening a PR.

---

## License

MIT - see [LICENSE](./LICENSE).
