# Verge

> The version control layer for your product

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
1. User saves -> you serialize and store the snapshot in your own DB/S3
2. You construct a DataPointer pointing to it
3. POST /repos/:repo_id/commits        -> Verge records the commit
4. PATCH /repos/:repo_id/branches/main -> Verge advances the branch
```

Restore, branching, merging, and history queries all follow the same pattern. Your product owns the data and the logic, Verge owns the graph.

---

## API

Verge exposes both a REST API and a gRPC API (`.proto` files included). No SDK is required - the raw HTTP interface is enough to fully integrate.

Core endpoints:

| Operation         | Endpoint                                                |
| ----------------- | ------------------------------------------------------- |
| Create repository | `POST /repos`                                           |
| Create commit     | `POST /repos/:repo_id/commits`                          |
| Advance branch    | `PATCH /repos/:repo_id/branches/:name`                  |
| Create branch     | `POST /repos/:repo_id/branches`                         |
| Merge branch      | `POST /repos/:repo_id/merges`                           |
| Get branch head   | `GET /repos/:repo_id/branches`                          |
| Get commit        | `GET /repos/:repo_id/commits/:commit_id`                |
| Traverse history  | `GET /repos/:repo_id/commits?traversal=dag&branch=main` |

All write operations use optimistic locking. If two callers try to advance the same branch concurrently, one gets a `409` with the current head so it can retry safely without losing anything.

For the full API reference, see [ARCHITECTURE.md](./ARCHITECTURE.md).

---

## Storage Backends

Verge is designed to scale with your product:

**PostgreSQL** (required) - Always the source of truth. All commits, branches, and metadata are stored here.

**Redis** (optional) - Enable for sub-millisecond branch head reads. Verge uses Redis as a cache layer for frequently accessed branch pointers.

**Neo4j** (optional) - Enable for complex ancestry queries at large scale. Neo4j provides optimized graph traversal for deep history queries and merge-base operations.

You choose which backends to enable based on your product's scale and infrastructure management capabilities. The API surface is identical regardless of which backends are active - you can start with PostgreSQL only and add Redis or Neo4j later without changing your integration.

All derived stores (Redis, Neo4j) are projections that can be rebuilt from PostgreSQL at any time.

---

## Integration guides

- [Internal system flows](./INTERNAL_FLOW.md) - what happens inside Verge on every request: what gets validated, what gets written, what gets rejected, and what happens asynchronously
- [Product integration flows](./PRODUCT_INTEGRATION_FLOW.md) - end-to-end walkthroughs for three different product types (document editor, design tool, AI workflow builder)
- [Architecture and API reference](./ARCHITECTURE.md) - full REST and gRPC API specs, entity model, storage backend details, and error code reference

---

## Roadmap

Verge is early. Here's what's coming:

- **SDKs** - official clients for TypeScript/Node.js, Python, and Go so you don't have to hand-roll HTTP calls

---

## Contributing

Contributions are welcome. Please read [CONTRIBUTING.md](./CONTRIBUTING.md) before opening a PR.

---

## License

MIT - see [LICENSE](./LICENSE).
