# Verge Architecture

**Architecture Diagram:** [View on Excalidraw](https://excalidraw.com/#json=A-PBD8du8EgdC-xL5zh_T,Nzvc1jf4r-18ciDCsox4mQ)

---

## Table of Contents

- [Functional Requirements](#functional-requirements)
- [Non-Functional Requirements](#non-functional-requirements)
- [Core Entities](#core-entities)
- [APIs](#apis)
  - [REST API](#rest-api)
    - [Repositories](#repositories)
      - [`POST /v1/repos`](#post-v1repos)
      - [`GET /v1/repos`](#get-v1repos)
      - [`GET /v1/repos/:repo_id`](#get-v1reposrepo_id)
    - [Branches](#branches)
      - [`POST /v1/repos/:repo_id/branches`](#post-v1reposrepo_idbranches)
      - [`GET /v1/repos/:repo_id/branches`](#get-v1reposrepo_idbranches)
      - [`GET /v1/repos/:repo_id/branches/:name`](#get-v1reposrepo_idbranchesname)
      - [`PATCH /v1/repos/:repo_id/branches/:name`](#patch-v1reposrepo_idbranchesname)
      - [`DELETE /v1/repos/:repo_id/branches/:name`](#delete-v1reposrepo_idbranchesname)
    - [Commits](#commits)
      - [`POST /v1/repos/:repo_id/commits`](#post-v1reposrepo_idcommits)
      - [`GET /v1/repos/:repo_id/commits`](#get-v1reposrepo_idcommits)
      - [`GET /v1/repos/:repo_id/commits/:commit_id`](#get-v1reposrepo_idcommitscommit_id)
      - [`GET /v1/repos/:repo_id/commits/:commit_id/parents`](#get-v1reposrepo_idcommitscommit_idparents)
    - [Merges](#merges)
      - [`POST /v1/repos/:repo_id/merges`](#post-v1reposrepo_idmerges)
    - [Health](#health)
      - [`GET /health`](#get-health)
  - [gRPC API](#grpc-api)
- [Outbox Worker](#outbox-worker)
  - [Event Sources](#event-sources)
  - [Worker Modes](#worker-modes)
  - [Built-in Handlers](#built-in-handlers)
  - [Outbox Events](#outbox-events)
- [Storage Layer & Composite Routing](#storage-layer--composite-routing)
  - [BranchRouter](#branchrouter)
  - [CommitRouter](#commitrouter)
  - [GraphRouter](#graphrouter)
- [Error Code Reference](#error-code-reference)
  - [Error Mapping - REST to gRPC](#error-mapping---rest-to-grpc)

---

## Functional Requirements

- Allow products to create a named repository
  as a logical container for a commit graph

- Allow products to create immutable commits against
  a repository, each carrying a parent reference list, a
  DataPointer, a message, and an author

- Support root commits with zero parents (first commit in a repo)

- Support merge commits with exactly two parents

- Store the DataPointer as an opaque JSON blob,
  never reading, interpreting, or validating its contents beyond structure

- Allow products to create branches from any existing commit in a repository

- Allow products to advance a branch pointer to a new commit

- Allow products to delete a branch

- Allow products to merge any branch into any other branch by accepting a
  two-parent commit and a target branch name

- Advance the target branch pointer atomically when a merge commit is created

- Allow products to fetch the current head commit of any branch

- Allow products to fetch a full commit record by commit ID,
  including its DataPointer

- Allow products to list all branches in a repository

- Allow products to traverse commit history from any branch head,
  returning commits in reverse chronological order

- Support cursor-based pagination on all history and list queries

- Support filtering history queries by author, timestamp range, and branch

- Allow products to fetch the parent commits of any given
  commit for DAG traversal

- Validate that all parent IDs in a commit request exist
  within the same repository before inserting

- Reject a branch advancement if the branch head has moved
  since the product last read it (optimistic concurrency)

- Write a commit row, commit_parents rows, branch pointer
  update, and outbox event all within a single atomic transaction

- Propagate commit and branch changes to derived stores
  (Neo4j, Redis) asynchronously via the outbox

- Expose all operations over HTTP/REST and gRPC; and publish .proto
  files for all gRPC operations

- Support pluggable storage backends: PostgreSQL (required), Neo4j, Redis;
  selectable per storage interface via configuration

- PostgreSQL is always required as the source of truth

- Support Neo4j as an optional GraphStore for complex ancestry
  and merge-base queries when enabled by the operator

- Support Redis as an optional cache layer for sub-millisecond branch head
  reads and commit object lookups when enabled by the operator

- Support outbox replay; workers must be idempotent and
  replayable from any point without corrupting derived store state

- Must never expose Verge APIs directly to end users or
  frontend clients, all calls must originate from the product's backend

## Non-Functional Requirements

- System must never store, read, inspect,
  or interpret the actual data a product is versioning,
  only the pointer to it

- The commit log must be append-only, no commit
  may ever be updated or deleted once written

- Branch pointer advancement must use optimistic locking,
  no blind overwrites

- The outbox event must be written in the same PostgreSQL transaction
  as the commit, a commit that exists without a corresponding outbox
  event is a consistency violation

- PostgreSQL must always be the source of truth; derived stores (Neo4j, Redis)
  are optional projections and must be rebuildable from PostgreSQL at any time

- The system must be horizontally scalable at the API layer,
  the API service must be stateless

- History traversal queries must support pagination,
  unbounded result sets are not permitted

- The system must be deployable as a self-hosted microservice

- System must support service-to-service authentication only,
  API key or mTLS between the product backend and Verge

- Branch head reads must be the fastest operation in the system;
  when Redis is enabled, cache hit latency target is sub-millisecond

- All write operations must return a structured error response with
  a machine-readable error code and a human-readable message

- The system must handle concurrent branch advancement gracefully,
  returning a 409 with enough information for the caller to retry without data loss

- The system must be operationally runnable at small scale
  with only PostgreSQL, without requiring Redis or Neo4j

- All database writes must use explicit transactions,
  no implicit auto-commit on multi-step operations

- The outbox workers must be idempotent,
  replaying the same event twice must not corrupt derived store state

- The system must produce structured logs and expose metrics hooks
  for observability at all tiers

- The API surface must be identical regardless of which storage
  backends are active, backend selection must be invisible to the integrating product

- The .proto definitions must be the authoritative contract for the gRPC interface,
  the HTTP interface must be consistent with them

- SDK releases must not be required for basic integration,
  the raw HTTP and gRPC interfaces must be sufficient to fully integrate without any SDK

## Core Entities

**1. Repository:**
The top-level container. Everything else belongs to a repository. One product resource (one document,
one design file, one workflow) maps to one repository. It holds the default branch name and nothing else.

**2. Commit:**
The fundamental unit of the system. Immutable once written. Carries the parent list (the DAG edges),
the DataPointer (reference to the product's data), a message, an author, and a timestamp. This is the
node in the graph.

**3. CommitParent:**
The DAG edge. A separate entity (its own table) rather than a field on Commit, because one commit can have multiple parents.
Each row is a single directed edge: this commit came from that commit. Zero rows means root commit.
One row means regular commit. Two rows means merge commit.

**4. Branch:**
A mutable named pointer to a commit. The only mutable entity in the system (so far), everything else is append-only.
It moves forward as new commits are created. Carries just a name, a repo reference, and a commit reference.

**5. DataPointer:**
(Not a table) An embedded value type that lives inside every Commit. It is the only thing we store about
the product's actual data. Type, location, an optional hash, and optional metadata.

**6. OutboxEvent:**
Written in the same transaction as every commit or branch advance. Not part of the version control domain
itself, it is the consistency mechanism that keeps derived stores (Neo4j, Redis) in sync with PostgreSQL.
Two event types exist: `CommitCreated` and `BranchHeadMoved`.

## APIs

All REST endpoints are mounted under the `/v1` prefix.

### REST API

---

#### Repositories

---

##### `POST /v1/repos`

Create a new repository.

**Request:**

```json
{
  "name": "doc_abc123",
  "default_branch": "main"
}
```

| Field            | Type   | Required | Notes                      |
| ---------------- | ------ | -------- | -------------------------- |
| `name`           | string | yes      |                            |
| `default_branch` | string | yes      | Cannot be deleted once set |

**Response `201 Created`:**

```json
{
  "id": "repo_doc_abc123",
  "name": "doc_abc123",
  "default_branch": "main",
  "created_at": "2024-04-05T10:00:00Z"
}
```

**Errors:**

| Status | Error code        | Cause                     |
| ------ | ----------------- | ------------------------- |
| `400`  | `invalid_request` | Missing or invalid fields |

---

##### `GET /v1/repos`

List all repositories. Paginated.

**Query params:**

| Param    | Type    | Default | Notes                                 |
| -------- | ------- | ------- | ------------------------------------- |
| `limit`  | integer | 20      | Max 100                               |
| `cursor` | string  | -       | Opaque - taken from previous response |

**Response `200 OK`:**

```json
{
  "repos": [
    {
      "id": "repo_doc_abc123",
      "name": "doc_abc123",
      "default_branch": "main",
      "created_at": "2024-04-05T10:00:00Z"
    }
  ],
  "next_cursor": "opaque_cursor_string"
}
```

`next_cursor` is `null` when there are no more pages.

**Errors:**

| Status | Error code        | Cause                |
| ------ | ----------------- | -------------------- |
| `400`  | `invalid_request` | Invalid query params |

---

##### `GET /v1/repos/:repo_id`

Get a single repository by ID.

**Response `200 OK`:**

```json
{
  "id": "repo_doc_abc123",
  "name": "doc_abc123",
  "default_branch": "main",
  "created_at": "2024-04-05T10:00:00Z"
}
```

**Errors:**

| Status | Error code       | Cause                     |
| ------ | ---------------- | ------------------------- |
| `404`  | `repo_not_found` | Repository does not exist |

```json
{
  "error": "repo_not_found",
  "message": "The requested repository does not exist."
}
```

---

#### Branches

---

##### `POST /v1/repos/:repo_id/branches`

Create a new branch from any existing commit in this repository.

**Request:**

```json
{
  "name": "suggest-alice-20240405",
  "source_commit_id": "commit_v2"
}
```

| Field              | Type   | Required | Notes                           |
| ------------------ | ------ | -------- | ------------------------------- |
| `name`             | string | yes      | Must be unique within this repo |
| `source_commit_id` | string | yes      | Must exist within this repo     |

**Response `201 Created`:**

```json
{
  "name": "suggest-alice-20240405",
  "repo_id": "repo_doc_abc123",
  "commit_id": "commit_v2",
  "created_at": "2024-04-05T10:10:00Z"
}
```

**Errors:**

| Status | Error code              | Cause                                             |
| ------ | ----------------------- | ------------------------------------------------- |
| `400`  | `invalid_request`       | Missing or invalid fields                         |
| `404`  | `repo_not_found`        | Repository does not exist                         |
| `404`  | `commit_not_found`      | `source_commit_id` does not exist in this repo    |
| `409`  | `branch_already_exists` | Branch with this name already exists in this repo |

---

##### `GET /v1/repos/:repo_id/branches`

List all branches in a repository. Paginated.

**Query params:**

| Param    | Type    | Default | Notes   |
| -------- | ------- | ------- | ------- |
| `limit`  | integer | 20      | Max 100 |
| `cursor` | string  | -       | Opaque  |

**Response `200 OK`:**

```json
{
  "branches": [
    {
      "name": "main",
      "repo_id": "repo_doc_abc123",
      "commit_id": "commit_v2",
      "created_at": "2024-04-05T09:00:00Z"
    },
    {
      "name": "suggest-alice-20240405",
      "repo_id": "repo_doc_abc123",
      "commit_id": "commit_v3",
      "created_at": "2024-04-05T10:10:00Z"
    }
  ],
  "next_cursor": null
}
```

**Errors:**

| Status | Error code       | Cause                     |
| ------ | ---------------- | ------------------------- |
| `404`  | `repo_not_found` | Repository does not exist |

---

##### `GET /v1/repos/:repo_id/branches/:name`

Get a single branch by name, including its current head commit ID.

**Response `200 OK`:**

```json
{
  "name": "main",
  "repo_id": "repo_doc_abc123",
  "commit_id": "commit_v3",
  "created_at": "2024-04-05T09:00:00Z"
}
```

**Errors:**

| Status | Error code         | Cause                     |
| ------ | ------------------ | ------------------------- |
| `404`  | `repo_not_found`   | Repository does not exist |
| `404`  | `branch_not_found` | Branch does not exist     |

---

##### `PATCH /v1/repos/:repo_id/branches/:name`

Advance a branch pointer to a new commit. Requires `expected_commit_id` for optimistic locking,
prevents blind overwrites when two callers attempt to advance the same branch concurrently.

**Request:**

```json
{
  "commit_id": "commit_v3",
  "expected_commit_id": "commit_v2"
}
```

| Field                | Type   | Required | Notes                                                      |
| -------------------- | ------ | -------- | ---------------------------------------------------------- |
| `commit_id`          | string | yes      | The new head commit, must exist in this repo               |
| `expected_commit_id` | string | yes      | The head the caller believes is current, rejected if stale |

**Response `200 OK`:**

```json
{
  "name": "main",
  "repo_id": "repo_doc_abc123",
  "commit_id": "commit_v3",
  "created_at": "2024-04-05T09:00:00Z"
}
```

**Errors:**

| Status | Error code         | Cause                                                 |
| ------ | ------------------ | ----------------------------------------------------- |
| `400`  | `invalid_request`  | `expected_commit_id` missing                          |
| `404`  | `repo_not_found`   | Repository does not exist                             |
| `404`  | `branch_not_found` | Branch does not exist                                 |
| `404`  | `commit_not_found` | `commit_id` does not exist in this repo               |
| `409`  | `branch_conflict`  | Branch has already advanced past `expected_commit_id` |

```json
{
  "error": "branch_conflict",
  "message": "Branch 'main' has advanced. Current head is 'commit_v3' but expected 'commit_v2'. Fetch the latest head and retry.",
  "current_head": "commit_v3"
}
```

---

##### `DELETE /v1/repos/:repo_id/branches/:name`

Delete a branch. Removes the branch pointer only, commits are not deleted.

**Response `204 No Content`**

**Errors:**

| Status | Error code                     | Cause                               |
| ------ | ------------------------------ | ----------------------------------- |
| `404`  | `repo_not_found`               | Repository does not exist           |
| `404`  | `branch_not_found`             | Branch does not exist               |
| `409`  | `cannot_delete_default_branch` | Branch is the repo's default branch |

```json
{
  "error": "cannot_delete_default_branch",
  "message": "Cannot delete the default branch. Set a different default branch first."
}
```

---

#### Commits

---

##### `POST /v1/repos/:repo_id/commits`

Create a new commit. Handles two cases:

- **Root commit** - `parent_ids` is empty. First commit in a repo.
- **Regular commit** - `parent_ids` has exactly one entry.
- **Merge commit** - use `POST /v1/repos/:repo_id/merges` instead (two parents required).

The commit is a standalone DAG node and is **not tied to a branch**. Branch advancement is a separate
operation (`PATCH /v1/repos/:repo_id/branches/:name`). Optimistic locking lives at the branch level, not
the commit level.

`idempotency_key` is optional. If provided and a commit with that key already exists in this repo, Verge
returns the existing commit without creating a duplicate. The response will contain `"existing": true`.
Callers should always provide this on retry-prone operations.

**Request:**

```json
{
  "parent_ids": ["commit_v1"],
  "data_pointer": {
    "type": "db",
    "location": "documents/snapshots/doc_abc123/v_1712345678",
    "hash": "sha256:a3f1c9d2e..."
  },
  "message": "Added executive summary",
  "author": "user_alice@company.com",
  "idempotency_key": "uuid-client-generated-123"
}
```

| Field                   | Type     | Required | Notes                                                      |
| ----------------------- | -------- | -------- | ---------------------------------------------------------- |
| `parent_ids`            | string[] | yes      | Empty for root, one for regular. Use `/v1/merges` for two. |
| `data_pointer.type`     | string   | yes      | One of: `s3`, `url`, `db`, `custom`                        |
| `data_pointer.location` | string   | yes      | Opaque to Verge                                            |
| `data_pointer.hash`     | string   | no       | SHA-256 for integrity - format: `sha256:...`               |
| `data_pointer.metadata` | object   | no       | Arbitrary JSON - opaque to Verge                           |
| `message`               | string   | yes      | Human-readable description                                 |
| `author`                | string   | yes      | Identifier for the committing user or service              |
| `idempotency_key`       | string   | no       | Client-generated UUID - enables safe retries               |

**Response `201 Created`:**

```json
{
  "commit": {
    "id": "commit_v2",
    "repo_id": "repo_doc_abc123",
    "parent_ids": ["commit_v1"],
    "data_pointer": {
      "type": "db",
      "location": "documents/snapshots/doc_abc123/v_1712345678",
      "hash": "sha256:a3f1c9d2e..."
    },
    "message": "Added executive summary",
    "author": "user_alice@company.com",
    "timestamp": "2024-04-05T10:05:00Z"
  },
  "existing": false
}
```

If `idempotency_key` matches an existing commit, returns `200 OK` with the existing commit and
`"existing": true` instead of `201 Created`.

**Errors:**

| Status | Error code        | Cause                                                                         |
| ------ | ----------------- | ----------------------------------------------------------------------------- |
| `400`  | `invalid_request` | Missing fields, invalid DataPointer type, two `parent_ids` (use `/v1/merges`) |
| `404`  | `repo_not_found`  | Repository does not exist                                                     |
| `422`  | `invalid_parent`  | One or more `parent_ids` do not exist within this repo                        |

```json
{
  "error": "invalid_parent",
  "message": "One or more parent_ids do not exist in this repository."
}
```

---

##### `GET /v1/repos/:repo_id/commits`

List or traverse commits in a repository. Controlled by the `traversal` param.

- `flat` (default) - returns commits in reverse chronological order with no DAG traversal. Efficient for simple lists.
- `dag` - walks the commit graph from the branch head following parent links. Returns the full reachable history. Requires `branch`. Use for version history panels.

**Query params:**

| Param       | Type    | Default | Notes                                                |
| ----------- | ------- | ------- | ---------------------------------------------------- |
| `branch`    | string  | -       | Filter by branch head (required for `traversal=dag`) |
| `author`    | string  | -       | Filter by author                                     |
| `since`     | string  | -       | ISO 8601 - commits at or after this timestamp        |
| `until`     | string  | -       | ISO 8601 - commits at or before this timestamp       |
| `traversal` | string  | `flat`  | `flat` or `dag`                                      |
| `limit`     | integer | 20      | Max 100                                              |
| `cursor`    | string  | -       | Opaque                                               |

**Response `200 OK`:**

```json
{
  "commits": [
    {
      "id": "commit_v2",
      "repo_id": "repo_doc_abc123",
      "parent_ids": ["commit_v1"],
      "data_pointer": {
        "type": "db",
        "location": "documents/snapshots/doc_abc123/v_1712345678",
        "hash": "sha256:a3f1c9d2e..."
      },
      "message": "Added executive summary",
      "author": "user_alice@company.com",
      "timestamp": "2024-04-05T10:05:00Z"
    },
    {
      "id": "commit_v1",
      "repo_id": "repo_doc_abc123",
      "parent_ids": [],
      "data_pointer": { "...": "..." },
      "message": "Initial version",
      "author": "user_alice@company.com",
      "timestamp": "2024-04-05T09:00:00Z"
    }
  ],
  "next_cursor": null
}
```

**Errors:**

| Status | Error code        | Cause                                                 |
| ------ | ----------------- | ----------------------------------------------------- |
| `400`  | `invalid_request` | `traversal=dag` used without `branch`, invalid params |
| `404`  | `repo_not_found`  | Repository does not exist                             |

---

##### `GET /v1/repos/:repo_id/commits/:commit_id`

Get a single commit by ID including its full DataPointer.

**Response `200 OK`:**

```json
{
  "id": "commit_v2",
  "repo_id": "repo_doc_abc123",
  "parent_ids": ["commit_v1"],
  "data_pointer": {
    "type": "db",
    "location": "documents/snapshots/doc_abc123/v_1712345678",
    "hash": "sha256:a3f1c9d2e..."
  },
  "message": "Added executive summary",
  "author": "user_alice@company.com",
  "timestamp": "2024-04-05T10:05:00Z"
}
```

**Errors:**

| Status | Error code         | Cause                     |
| ------ | ------------------ | ------------------------- |
| `404`  | `repo_not_found`   | Repository does not exist |
| `404`  | `commit_not_found` | Commit does not exist     |

---

##### `GET /v1/repos/:repo_id/commits/:commit_id/parents`

Get the immediate parent commits of a given commit. Returns full commit objects, not just IDs.
Used for one-hop DAG traversal.

Root commits return an empty `parents` array. Merge commits return two entries.

**Response `200 OK`:**

```json
{
  "commit_id": "commit_v4",
  "parents": [
    {
      "id": "commit_v3",
      "repo_id": "repo_doc_abc123",
      "parent_ids": ["commit_v2"],
      "data_pointer": { "...": "..." },
      "message": "Suggested: expanded introduction",
      "author": "user_alice@company.com",
      "timestamp": "2024-04-05T10:30:00Z"
    },
    {
      "id": "commit_v2",
      "repo_id": "repo_doc_abc123",
      "parent_ids": ["commit_v1"],
      "data_pointer": { "...": "..." },
      "message": "Added executive summary",
      "author": "user_alice@company.com",
      "timestamp": "2024-04-05T10:05:00Z"
    }
  ]
}
```

**Errors:**

| Status | Error code         | Cause                     |
| ------ | ------------------ | ------------------------- |
| `404`  | `repo_not_found`   | Repository does not exist |
| `404`  | `commit_not_found` | Commit does not exist     |

---

#### Merges

---

##### `POST /v1/repos/:repo_id/merges`

Create a merge commit and advance the target branch atomically. The product supplies the
already-computed merged DataPointer; Verge records the two-parent commit and moves the branch pointer
in a single transaction.

`target_branch` can be any branch in the repo, not restricted to `main`.

`expected_target_head` is required for optimistic locking. Verge validates that the target branch is
still at this commit before proceeding. If the branch has moved, the request is rejected with `409`.

**Request:**

```json
{
  "parent_ids": ["commit_v3", "commit_v2"],
  "expected_target_head": "commit_v2",
  "target_branch": "main",
  "data_pointer": {
    "type": "db",
    "location": "documents/snapshots/doc_abc123/merged_v_1712347000",
    "hash": "sha256:c9e3f7b..."
  },
  "message": "Accepted Alice's suggested edits",
  "author": "user_bob@company.com"
}
```

| Field                  | Type     | Required | Notes                                                           |
| ---------------------- | -------- | -------- | --------------------------------------------------------------- |
| `parent_ids`           | string[] | yes      | Exactly two entries - source branch head and target branch head |
| `expected_target_head` | string   | yes      | Optimistic lock - must match current target branch head         |
| `target_branch`        | string   | yes      | Any existing branch in this repo                                |
| `data_pointer`         | object   | yes      | Pointer to the merged result in product storage                 |
| `message`              | string   | yes      |                                                                 |
| `author`               | string   | yes      |                                                                 |

**Response `201 Created`:**

```json
{
  "id": "commit_v4",
  "repo_id": "repo_doc_abc123",
  "parent_ids": ["commit_v3", "commit_v2"],
  "data_pointer": {
    "type": "db",
    "location": "documents/snapshots/doc_abc123/merged_v_1712347000",
    "hash": "sha256:c9e3f7b..."
  },
  "message": "Accepted Alice's suggested edits",
  "author": "user_bob@company.com",
  "timestamp": "2024-04-05T11:00:00Z"
}
```

**Errors:**

| Status | Error code           | Cause                                                |
| ------ | -------------------- | ---------------------------------------------------- |
| `400`  | `invalid_request`    | `parent_ids` does not have exactly two entries       |
| `404`  | `repo_not_found`     | Repository does not exist                            |
| `404`  | `branch_not_found`   | `target_branch` does not exist                       |
| `409`  | `stale_merge_target` | Target branch has moved past `expected_target_head`  |
| `422`  | `invalid_parent`     | One or both parent commits do not exist in this repo |

```json
{
  "error": "stale_merge_target",
  "message": "Branch 'main' has moved during the merge. Current head is 'commit_v3' but expected 'commit_v2'. Fetch the latest heads and retry.",
  "current_head": "commit_v3"
}
```

---

#### Health

---

##### `GET /health`

Liveness check. Returns `200 OK` with an empty body. Not versioned, available at the root, not under `/v1`.

---

### gRPC API

Same operations, same validation rules, same error semantics as the REST API. The `.proto` files are the
authoritative contract - the REST interface must remain consistent with them.

```protobuf
syntax = "proto3";
package verge.v1;

// Shared types

message DataPointer {
  string type     = 1;  // "s3" | "url" | "db" | "custom"
  string location = 2;
  string hash     = 3;  // optional - "sha256:..."
  bytes  metadata = 4;  // optional - raw JSON bytes, opaque to Verge
}

message Repository {
  string id             = 1;
  string name           = 2;
  string default_branch = 3;
  string created_at     = 4;  // ISO 8601
}

message Commit {
  string          id           = 1;
  string          repo_id      = 2;
  repeated string parent_ids   = 3;
  DataPointer     data_pointer = 4;
  string          message      = 5;
  string          author       = 6;
  string          timestamp    = 7;  // ISO 8601
}

message Branch {
  string name       = 1;
  string repo_id    = 2;
  string commit_id  = 3;
  string created_at = 4;  // ISO 8601
}

// Repository service

service RepositoryService {
  rpc CreateRepo (CreateRepoRequest) returns (Repository);
  rpc GetRepo    (GetRepoRequest)    returns (Repository);
  rpc ListRepos  (ListReposRequest)  returns (ListReposResponse);
}

message CreateRepoRequest {
  string name           = 1;
  string default_branch = 2;
}

message GetRepoRequest {
  string repo_id = 1;
}

message ListReposRequest {
  int32  limit  = 1;
  string cursor = 2;
}

message ListReposResponse {
  repeated Repository repos       = 1;
  string              next_cursor = 2;
}

// Branch service

service BranchService {
  rpc CreateBranch  (CreateBranchRequest)  returns (Branch);
  rpc GetBranch     (GetBranchRequest)     returns (Branch);
  rpc ListBranches  (ListBranchesRequest)  returns (ListBranchesResponse);
  rpc AdvanceBranch (AdvanceBranchRequest) returns (Branch);
  rpc DeleteBranch  (DeleteBranchRequest)  returns (DeleteBranchResponse);
}

message CreateBranchRequest {
  string repo_id          = 1;
  string name             = 2;
  string source_commit_id = 3;
}

message GetBranchRequest {
  string repo_id = 1;
  string name    = 2;
}

message ListBranchesRequest {
  string repo_id = 1;
  int32  limit   = 2;
  string cursor  = 3;
}

message ListBranchesResponse {
  repeated Branch branches    = 1;
  string          next_cursor = 2;
}

message AdvanceBranchRequest {
  string repo_id            = 1;
  string name               = 2;
  string commit_id          = 3;
  string expected_commit_id = 4;  // required - optimistic lock
}

message DeleteBranchRequest {
  string repo_id = 1;
  string name    = 2;
}

message DeleteBranchResponse {}

// Commit service

service CommitService {
  rpc CreateCommit (CreateCommitRequest) returns (CreateCommitResponse);
  rpc GetCommit    (GetCommitRequest)    returns (Commit);
  rpc ListCommits  (ListCommitsRequest)  returns (ListCommitsResponse);
  rpc GetParents   (GetParentsRequest)   returns (GetParentsResponse);
}

message CreateCommitRequest {
  string          repo_id         = 1;
  repeated string parent_ids      = 2;
  DataPointer     data_pointer    = 3;
  string          message         = 4;
  string          author          = 5;
  string          idempotency_key = 6;  // optional - client-generated UUID
}

message CreateCommitResponse {
  Commit commit   = 1;
  bool   existing = 2;  // true if idempotency_key matched an existing commit
}

message GetCommitRequest {
  string repo_id   = 1;
  string commit_id = 2;
}

message ListCommitsRequest {
  string repo_id   = 1;
  string branch    = 2;
  string author    = 3;
  string since     = 4;  // ISO 8601
  string until     = 5;  // ISO 8601
  string traversal = 6;  // "flat" | "dag"
  int32  limit     = 7;
  string cursor    = 8;
}

message ListCommitsResponse {
  repeated Commit commits     = 1;
  string          next_cursor = 2;
}

message GetParentsRequest {
  string repo_id   = 1;
  string commit_id = 2;
}

message GetParentsResponse {
  string          commit_id = 1;
  repeated Commit parents   = 2;
}

// Merge service

service MergeService {
  rpc CreateMerge (CreateMergeRequest) returns (Commit);
}

message CreateMergeRequest {
  string          repo_id              = 1;
  repeated string parent_ids           = 2;  // exactly two
  string          expected_target_head = 3;  // required - optimistic lock
  string          target_branch        = 4;
  DataPointer     data_pointer         = 5;
  string          message              = 6;
  string          author               = 7;
}
```

---

## Outbox Worker

The outbox worker is a **separate process** (`cmd/worker`) that reads `OutboxEvent` rows from PostgreSQL
and propagates changes to derived stores (Neo4j, Redis). It runs independently from the API server.

Every write that changes the commit graph or a branch pointer writes one or more `OutboxEvent` rows in
the **same database transaction** as the write itself. The worker then picks up these events and dispatches
them. If the worker crashes, events remain unprocessed (`processed = false`) and are picked up on restart.
Replaying an event must always be safe.

### Event Sources

The worker reads events through an `EventSource` interface with two built-in implementations:

**PollingSource** (default, `VERGE_OUTBOX_SOURCE_TYPE=polling`)

Polls the `outbox_events` table on a configurable interval (`VERGE_OUTBOX_POLL_INTERVAL`, default `500ms`),
fetching unprocessed rows in batches (`VERGE_OUTBOX_BATCH_SIZE`, default `100`). After successful
processing, it marks rows `processed = true`. No external dependencies required.

**DebeziumSource** (`VERGE_OUTBOX_SOURCE_TYPE=debezium`)

Reads CDC events from a Kafka topic written by the Debezium PostgreSQL connector. Debezium captures every
`INSERT` on the `outbox_events` table and publishes it as a Kafka message. The worker consumes from this
topic using a configurable consumer group (`VERGE_OUTBOX_DEBEZIUM_GROUP_ID`). Kafka offset commits serve
as the acknowledgement - no `processed = true` update needed. Suitable for high-throughput deployments
that want to decouple the outbox from polling.

The Debezium source understands the Debezium envelope format: it handles `c` (create), `r` (snapshot read),
and `u` (update) operations, and skips events where `after.processed = true` or operation is `d` (delete).

### Worker Modes

The worker has two dispatch modes, selected by whether an `EventBus` is configured:

**In-process mode** (default, `VERGE_OUTBOX_EVENTBUS_ENABLED=false`)

The worker dispatches each event directly to registered `OutboxHandler` implementations in the same
process. Handlers for Neo4j and Redis are wired at startup. Each event is dispatched to every handler
that declares interest in that event type. Handlers that fail do not block other events - the event is
left unprocessed and retried on the next poll.

**EventBus mode** (`VERGE_OUTBOX_EVENTBUS_ENABLED=true`)

The worker publishes the raw event batch to an external broker (e.g. Kafka) instead of dispatching in-process.
External consumers then run the handlers. Implement the `EventBus` interface to use any broker - the
built-in implementation targets Kafka (`VERGE_OUTBOX_EVENTBUS_TYPE=kafka`).

In this mode the worker process acts as a producer only. A separate consumer process runs the handlers.

### Built-in Handlers

**Neo4jHandler**:- handles `CommitCreated` events.

Writes Commit nodes and `PARENT_OF` edges to Neo4j. Uses Cypher `MERGE` statements so replaying the same
event is idempotent, a node or edge that already exists is not duplicated. If a parent commit node does
not yet exist in Neo4j (possible under parallel processing), it is created as a stub node that will be
filled in when its own `CommitCreated` event arrives.

```
CommitCreated event received
  │
  ├── MERGE (c:Commit {id: commit_id}) SET c.repo_id, c.author, c.message, c.timestamp
  └── For each parent_id:
        MERGE (p:Commit {id: parent_id})
        MERGE (c)-[:PARENT_OF]->(p)
```

**RedisHealHandler**:- handles `BranchHeadMoved` events.

Updates the branch head cache in Redis. The event payload carries a `version` field (Unix milliseconds
of the outbox event's `created_at` timestamp) which is used to prevent stale writes: a newer version
already in the cache will not be overwritten by a replayed older event. This makes the handler idempotent
under out-of-order delivery.

```
BranchHeadMoved event received
  │
  └── SetHead(repo_id, branch, commit_id, version)
        → only written if version > current cached version
```

### Outbox Events

Two event types are currently emitted:

**`CommitCreated`**:- emitted in the same transaction as every `INSERT INTO commits`. Also emitted for
merge commits.

```json
{
  "commit_id": "commit_v2",
  "repo_id": "repo_doc_abc123",
  "parent_ids": ["commit_v1"],
  "author": "user_alice@company.com",
  "message": "Added executive summary",
  "timestamp": "2024-04-05T10:05:00Z"
}
```

**`BranchHeadMoved`**:- emitted whenever a branch pointer is advanced: on `PATCH /v1/repos/:repo_id/branches/:name`
and atomically within `POST /v1/repos/:repo_id/merges`.

```json
{
  "repo_id": "repo_doc_abc123",
  "branch": "main",
  "commit_id": "commit_v2",
  "version": 1712345678000
}
```

`version` is a Unix millisecond timestamp used by `RedisHealHandler` for idempotent ordering.

---

## Storage Layer & Composite Routing

When multiple backends are enabled, the service layer does not talk directly to individual stores.
Instead, three router types sit between the services and the storage backends, implementing the same
store interface and routing reads and writes to the appropriate backend.

### BranchRouter

Routes branch operations across PostgreSQL and the Redis `BranchHeadStore`.

**Read path (`GetHead`):** tries Redis first. On a cache hit, returns immediately. On a cache miss or
Redis error, falls back to PostgreSQL and writes the result back to Redis to warm the cache.

**Write path (`Advance`):** writes to PostgreSQL (the authoritative store), then eagerly updates Redis
with the new head so subsequent reads hit cache without waiting for the outbox worker.

**Other operations** (`Create`, `GetByName`, `List`, `Delete`) always go directly to PostgreSQL.

Redis entries are given a TTL configured via `VERGE_STORAGE_REDIS_BRANCH_TTL` (default `30s`). The
outbox-driven `RedisHealHandler` re-populates expired entries from `BranchHeadMoved` events.

### CommitRouter

Routes commit reads across PostgreSQL and the Redis `CommitCache`.

**Read path (`GetByID`):** tries the Redis commit cache first. On a miss or error, falls back to PostgreSQL
and populates the cache for the next read.

**Write path (`Create`):** always writes to PostgreSQL. The cache is populated lazily on first read.

`GetByIdempotencyKey`, `List`, `GetParents`, and `ValidateParentsExist` always go to PostgreSQL since
these require consistent, queryable data that the commit cache does not index.

### GraphRouter

Routes ancestry and graph queries across Neo4j and the PostgreSQL graph store.

**Neo4j primary:** `TraverseDAG`, `GetAncestors`, and `FindMergeBase` are attempted against Neo4j first.
If Neo4j is unavailable or returns an error, the router falls back to the PostgreSQL implementation which
uses recursive CTEs for graph traversal.

This means all three graph operations work correctly even when Neo4j is disabled or unhealthy, at the
cost of slower query performance. Neo4j exists to accelerate these queries at scale, not to gate them.

---

## Error Code Reference

All error responses follow this shape for both REST (JSON body) and gRPC (status detail):

```json
{
  "error": "machine_readable_code",
  "message": "Human readable explanation of what went wrong and how to fix it.",
  "current_head": "commit_id_if_relevant"
}
```

`current_head` is only present in `branch_conflict` and `stale_merge_target` errors - it gives the caller
the current branch head so they can retry without an extra round-trip to fetch it.

| Error code                     | HTTP | Meaning                                                |
| ------------------------------ | ---- | ------------------------------------------------------ |
| `invalid_request`              | 400  | Missing required field, wrong type, invalid enum value |
| `repo_not_found`               | 404  | The `repo_id` does not exist                           |
| `branch_not_found`             | 404  | The branch does not exist in this repo                 |
| `branch_already_exists`        | 409  | A branch with this name already exists in this repo    |
| `branch_conflict`              | 409  | Branch has advanced past `expected_commit_id`          |
| `commit_not_found`             | 404  | The commit does not exist in this repo                 |
| `invalid_parent`               | 422  | A `parent_id` does not exist within this repo          |
| `stale_merge_target`           | 409  | Target branch has moved past `expected_target_head`    |
| `cannot_delete_default_branch` | 409  | Attempted to delete the repo's default branch          |
| `internal_error`               | 500  | Unexpected server-side failure                         |

---

### Error Mapping - REST to gRPC

| Error code                     | REST status | gRPC status           |
| ------------------------------ | ----------- | --------------------- |
| `invalid_request`              | `400`       | `INVALID_ARGUMENT`    |
| `repo_not_found`               | `404`       | `NOT_FOUND`           |
| `branch_not_found`             | `404`       | `NOT_FOUND`           |
| `commit_not_found`             | `404`       | `NOT_FOUND`           |
| `branch_already_exists`        | `409`       | `ALREADY_EXISTS`      |
| `branch_conflict`              | `409`       | `ABORTED`             |
| `stale_merge_target`           | `409`       | `ABORTED`             |
| `cannot_delete_default_branch` | `409`       | `FAILED_PRECONDITION` |
| `invalid_parent`               | `422`       | `FAILED_PRECONDITION` |
| `internal_error`               | `500`       | `INTERNAL`            |
