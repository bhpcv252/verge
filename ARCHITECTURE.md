# Verge Architecture

**Architecture Diagram:** [View on Excalidraw](https://excalidraw.com/#json=y2wsuxsG9_UBdr7tsFJas,TUoBXTDoZ4To1fLhjaY-DA)

---

## Table of Contents

- [Functional Requirements](#functional-requirements)
- [Non-Functional Requirements](#non-functional-requirements)
- [Core Entities](#core-entities)
- [APIs](#apis)
  - [REST API](#rest-api)
    - [Repositories](#repositories)
      - [`POST /repos`](#post-repos)
      - [`GET /repos`](#get-repos)
      - [`GET /repos/:repo_id`](#get-reposrepo_id)
    - [Branches](#branches)
      - [`POST /repos/:repo_id/branches`](#post-reposrepo_idbranches)
      - [`GET /repos/:repo_id/branches`](#get-reposrepo_idbranches)
      - [`PATCH /repos/:repo_id/branches/:name`](#patch-reposrepo_idbranchesname)
      - [`DELETE /repos/:repo_id/branches/:name`](#delete-reposrepo_idbranchesname)
    - [Commits](#commits)
      - [`POST /repos/:repo_id/commits`](#post-reposrepo_idcommits)
      - [`GET /repos/:repo_id/commits`](#get-reposrepo_idcommits)
      - [`GET /repos/:repo_id/commits/:commit_id`](#get-reposrepo_idcommitscommit_id)
      - [`GET /repos/:repo_id/commits/:commit_id/parents`](#get-reposrepo_idcommitscommit_idparents)
    - [Merges](#merges)
      - [`POST /repos/:repo_id/merges`](#post-reposrepo_idmerges)
  - [gRPC API](#grpc-api)
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

- Support Redis as an optional BranchStore for
  sub-millisecond branch head reads when enabled by the operator

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

- Branch pointer advancement must use optimistic locking
  no blind overwrites

- The outbox event must be written in the same PostgreSQL transaction
  as the commit, a commit that exists without a corresponding outbox
  event is a consistency violation

- PostgreSQL must always be the source of truth; derived stores (Neo4j, Redis)
  are optional projections and must be rebuildable from PostgreSQL at any time

- The system must be horizontally scalable at the API layer
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

- The system must handle concurrent branch advancement gracefully
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
Written in the same transaction as every commit. Not part of the version control domain itself, it is the consistency
mechanism that keeps derived stores (Neo4j, Redis) in sync with PostgreSQL.

## APIs

### REST API

---

#### Repositories

---

##### `POST /repos`

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

##### `GET /repos`

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

##### `GET /repos/:repo_id`

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
  "message": "Repository 'repo_doc_abc123' does not exist."
}
```

---

#### Branches

---

##### `POST /repos/:repo_id/branches`

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

```json
{
  "error": "commit_not_found",
  "message": "Commit 'commit_bad' does not exist in repo 'repo_doc_abc123'."
}
```

---

##### `GET /repos/:repo_id/branches`

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

##### `PATCH /repos/:repo_id/branches/:name`

Advance a branch pointer to a new commit. Requires `expected_commit_id` for optimistic locking prevents blind overwrites when two callers attempt to advance the same branch concurrently.

**Request:**

```json
{
  "commit_id": "commit_v3",
  "expected_commit_id": "commit_v2"
}
```

| Field                | Type   | Required | Notes                                                       |
| -------------------- | ------ | -------- | ----------------------------------------------------------- |
| `commit_id`          | string | yes      | The new head commit - must exist in this repo               |
| `expected_commit_id` | string | yes      | The head the caller believes is current - rejected if stale |

**Response `200 OK`:**

```json
{
  "name": "main",
  "repo_id": "repo_doc_abc123",
  "commit_id": "commit_v3"
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
  "message": "Branch 'main' is at 'commit_v3', not 'commit_v2'. Fetch the latest head and retry.",
  "current_head": "commit_v3"
}
```

---

##### `DELETE /repos/:repo_id/branches/:name`

Delete a branch. Removes the branch pointer only - commits are not deleted.

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
  "message": "Branch 'main' is the default branch of 'repo_doc_abc123' and cannot be deleted."
}
```

---

#### Commits

---

##### `POST /repos/:repo_id/commits`

Create a new commit. Handles three cases:

- **Root commit** - `parent_ids` is empty. First commit in a repo.
- **Regular commit** - `parent_ids` has exactly one entry.
- **Merge commit** - use `POST /repos/:repo_id/merges` instead.

The commit is **not tied to a branch** - it is a standalone DAG node. Branch advancement is a separate operation (`PATCH /repos/:repo_id/branches/:name`), though `expected_head` here enforces that the branch has not moved before the product advances it after committing.

`idempotency_key` is optional. If provided and a commit with that key already exists in this repo, Verge returns the existing commit without creating a duplicate. Callers should always provide this on retry-prone operations.

**Request:**

```json
{
  "parent_ids": ["commit_v1"],
  "expected_head": "commit_v1",
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

| Field                   | Type     | Required | Notes                                                   |
| ----------------------- | -------- | -------- | ------------------------------------------------------- |
| `parent_ids`            | string[] | yes      | Empty for root, one for regular. Use `/merges` for two. |
| `expected_head`         | string   | no       | If set, Verge validates the target branch has not moved |
| `data_pointer.type`     | string   | yes      | One of: `s3`, `url`, `db`, `custom`                     |
| `data_pointer.location` | string   | yes      | Opaque to Verge                                         |
| `data_pointer.hash`     | string   | no       | SHA-256 for integrity - format: `sha256:...`            |
| `data_pointer.metadata` | object   | no       | Arbitrary JSON - opaque to Verge                        |
| `message`               | string   | yes      | Human-readable description                              |
| `author`                | string   | yes      | Identifier for the committing user or service           |
| `idempotency_key`       | string   | no       | Client-generated UUID - enables safe retries            |

**Response `201 Created`:**

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

If `idempotency_key` matches an existing commit, returns `200 OK` with the existing commit instead of `201`.

**Errors:**

| Status | Error code        | Cause                                                                      |
| ------ | ----------------- | -------------------------------------------------------------------------- |
| `400`  | `invalid_request` | Missing fields, invalid DataPointer type, two `parent_ids` (use `/merges`) |
| `404`  | `repo_not_found`  | Repository does not exist                                                  |
| `409`  | `branch_conflict` | `expected_head` was set and the branch has already advanced                |
| `422`  | `invalid_parent`  | One or more `parent_ids` do not exist within this repo                     |

```json
{
  "error": "invalid_parent",
  "message": "Parent 'commit_bad' does not exist in repo 'repo_doc_abc123'."
}
```

```json
{
  "error": "branch_conflict",
  "message": "Branch head has advanced past 'commit_v1'. Fetch the latest head and retry.",
  "current_head": "commit_v2"
}
```

---

##### `GET /repos/:repo_id/commits`

List or traverse commits in a repository. Controlled by the `traversal` param.

- `flat` - returns commits in reverse chronological order with no DAG traversal. Efficient. Use for simple lists.
- `dag` - traverses the DAG from the branch head, following parent links. Returns full reachable history. Use for version history panels.

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
      "data_pointer": { ... },
      "message": "Initial version",
      "author": "user_alice@company.com",
      "timestamp": "2024-04-05T09:00:00Z"
    }
  ],
  "next_cursor": null
}
```

**Errors:**

| Status | Error code         | Cause                                                  |
| ------ | ------------------ | ------------------------------------------------------ |
| `400`  | `invalid_request`  | `traversal=dag` used without `branch`, invalid params  |
| `404`  | `repo_not_found`   | Repository does not exist                              |
| `404`  | `branch_not_found` | `branch` param references a branch that does not exist |

---

##### `GET /repos/:repo_id/commits/:commit_id`

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

##### `GET /repos/:repo_id/commits/:commit_id/parents`

Get the immediate parent commits of a given commit. Returns full commit objects, not just IDs. Used for one-hop DAG traversal.

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
      "data_pointer": { ... },
      "message": "Suggested: expanded introduction",
      "author": "user_alice@company.com",
      "timestamp": "2024-04-05T10:30:00Z"
    },
    {
      "id": "commit_v2",
      "repo_id": "repo_doc_abc123",
      "parent_ids": ["commit_v1"],
      "data_pointer": { ... },
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

##### `POST /repos/:repo_id/merges`

Create a merge commit and advance the target branch atomically. The product supplies the already-computed merged DataPointer, Verge records the two-parent commit and moves the branch pointer in a single transaction.

`target_branch` can be any branch in the repo, not restricted to `main`.

`expected_target_head` is required for optimistic locking. Verge validates that the target branch is still at this commit before proceeding. If the branch has moved, the request is rejected.

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
  "message": "Branch 'main' is at 'commit_v3' but expected 'commit_v2'. Fetch latest heads and recompute merge.",
  "current_head": "commit_v3"
}
```

---

### gRPC API

Same operations, same validation rules, same error semantics as the REST API. The `.proto` file is the authoritative contract - the REST interface must remain consistent with it.

```protobuf
syntax = "proto3";
package verge.v1;

// ─── Shared types ─────────────────────────────────────────────────────────────

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

// ─── Repository service ───────────────────────────────────────────────────────

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
  repeated Repository repos = 1;
  string              next_cursor = 2;
}

// ─── Branch service ───────────────────────────────────────────────────────────

service BranchService {
  rpc CreateBranch  (CreateBranchRequest)  returns (Branch);
  rpc ListBranches  (ListBranchesRequest)  returns (ListBranchesResponse);
  rpc AdvanceBranch (AdvanceBranchRequest) returns (Branch);
  rpc DeleteBranch  (DeleteBranchRequest)  returns (DeleteBranchResponse);
}

message CreateBranchRequest {
  string repo_id          = 1;
  string name             = 2;
  string source_commit_id = 3;
}

message ListBranchesRequest {
  string repo_id = 1;
  int32  limit   = 2;
  string cursor  = 3;
}

message ListBranchesResponse {
  repeated Branch branches   = 1;
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

// ─── Commit service ───────────────────────────────────────────────────────────

service CommitService {
  rpc CreateCommit (CreateCommitRequest) returns (CreateCommitResponse);
  rpc GetCommit    (GetCommitRequest)    returns (Commit);
  rpc ListCommits  (ListCommitsRequest)  returns (ListCommitsResponse);
  rpc GetParents   (GetParentsRequest)   returns (GetParentsResponse);
}

message CreateCommitRequest {
  string          repo_id         = 1;
  repeated string parent_ids      = 2;
  string          expected_head   = 3;  // optional - optimistic check
  DataPointer     data_pointer    = 4;
  string          message         = 5;
  string          author          = 6;
  string          idempotency_key = 7;  // optional - client-generated UUID
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

// ─── Merge service ────────────────────────────────────────────────────────────

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

## Error Code Reference

All error responses follow this shape for both REST (JSON body) and gRPC (status detail):

```json
{
  "error": "machine_readable_code",
  "message": "Human readable explanation of what went wrong and how to fix it.",
  "current_head": "commit_id_if_relevant"
}
```

`current_head` is only present in branch conflict and stale merge target errors - it gives the caller the information needed to retry without a round-trip.

| Error code                     | Meaning                                                          |
| ------------------------------ | ---------------------------------------------------------------- |
| `invalid_request`              | Missing required field, wrong type, invalid enum value           |
| `repo_not_found`               | The `repo_id` does not exist                                     |
| `branch_not_found`             | The branch does not exist in this repo                           |
| `branch_already_exists`        | A branch with this name already exists in this repo              |
| `branch_conflict`              | Branch has advanced past `expected_commit_id` or `expected_head` |
| `commit_not_found`             | The commit does not exist in this repo                           |
| `invalid_parent`               | A `parent_id` does not exist within this repo                    |
| `stale_merge_target`           | Target branch has moved past `expected_target_head`              |
| `cannot_delete_default_branch` | Attempted to delete the repo's default branch                    |
| `internal_error`               | Unexpected server-side failure                                   |

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
