# Verge - Testing

## Table of Contents

- [Philosophy](#philosophy)
- [Test Layers](#test-layers)
- [Unit Tests](#unit-tests)
- [Integration Tests](#integration-tests)
- [E2E Tests](#e2e-tests)
- [Test Naming and File Conventions](#test-naming-and-file-conventions)
- [Test Data and Fixtures](#test-data-and-fixtures)
- [Coverage Targets](#coverage-targets)

---

## Philosophy

- Tests assert what the system does (creates a commit, returns a 409, advances a branch) instead of how it does it (which SQL query ran, which function was called).

- Unit tests do not spin up databases. Integration tests do not mock the database. E2E tests do not mock anything. Never reach down a layer to test something that belongs to the layer below.

- Verge's contract with integrators is built on structured error responses. Every error code in the architecture (`branch_conflict`, `invalid_parent`, `stale_merge_target`, etc.) must have a test that asserts the exact error code and HTTP status. Happy paths are not enough.

---

## Test Layers

```
┌─────────────────────────────────────┐
│         E2E Tests                   │  Full HTTP server + real PostgreSQL
│         test/e2e/                   │  Tests complete integration flows
├─────────────────────────────────────┤
│         Integration Tests           │  Real DBs via testcontainers
│         internal/.../..._test.go    │  Tests storage layer + service layer together
├─────────────────────────────────────┤
│         Unit Tests                  │  No I/O, no DB, pure logic
│         internal/.../..._test.go    │  Tests domain validation + service logic with mocks
└─────────────────────────────────────┘
```

---

## Unit Tests

**Location:** co-located with the source file as `*_test.go`

**Runtime:** No database, no network, no filesystem.

**Mocking:** Use `gomock` for interfaces. Every storage interface and service interface should have a generated mock. Mocks live next to the interface definition.

---

### Domain Layer (`internal/domain/`)

Test all validation logic that lives on domain structs.

| Area                      | What to test                                                                                                                                   |
| ------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------- |
| `DataPointer` validation  | Valid types (`s3`, `url`, `db`, `custom`) accepted; any other string rejected                                                                  |
| `DataPointer` hash format | Hash accepted only when it starts with `sha256:`; empty hash accepted (optional field)                                                         |
| `DataPointer` location    | Rejected when empty string                                                                                                                     |
| `Commit` parent count     | 0 parents = root commit (valid); 1 parent = regular commit (valid); 2 parents = rejected at this layer (use merge path); 3+ parents = rejected |
| `Branch` name             | Rejected when empty; accepted when non-empty                                                                                                   |
| `Repository` name         | Rejected when empty; `default_branch` rejected when empty                                                                                      |

---

### Service Layer (`internal/service/`)

Test business logic that sits above storage. All storage calls are mocked.

#### RepoService

| Scenario                   | Expected behavior                        |
| -------------------------- | ---------------------------------------- |
| `CreateRepo` - valid input | Calls store insert, returns created repo |
| `GetRepo` - repo exists    | Returns repo from store                  |
| `GetRepo` - repo not found | Returns `repo_not_found` domain error    |
| `ListRepos` - no cursor    | Returns first page                       |
| `ListRepos` - with cursor  | Passes cursor to store                   |

#### BranchService

| Scenario                                                 | Expected behavior                                                   |
| -------------------------------------------------------- | ------------------------------------------------------------------- |
| `CreateBranch` - repo not found                          | Returns `repo_not_found`                                            |
| `CreateBranch` - source commit not in repo               | Returns `commit_not_found`                                          |
| `CreateBranch` - branch name already exists              | Returns `branch_already_exists`                                     |
| `CreateBranch` - valid                                   | Calls store insert, returns branch                                  |
| `AdvanceBranch` - `expected_commit_id` missing           | Returns `invalid_request`                                           |
| `AdvanceBranch` - commit not in repo                     | Returns `commit_not_found`                                          |
| `AdvanceBranch` - optimistic lock fails (0 rows updated) | Fetches current head, returns `branch_conflict` with `current_head` |
| `AdvanceBranch` - success                                | Returns updated branch                                              |
| `DeleteBranch` - target is default branch                | Returns `cannot_delete_default_branch`                              |
| `DeleteBranch` - branch not found                        | Returns `branch_not_found`                                          |
| `DeleteBranch` - valid                                   | Calls store delete                                                  |

#### CommitService

| Scenario                                             | Expected behavior                                                    |
| ---------------------------------------------------- | -------------------------------------------------------------------- |
| `CreateCommit` - idempotency key match               | Returns existing commit with `existing=true`, no store insert called |
| `CreateCommit` - repo not found                      | Returns `repo_not_found`                                             |
| `CreateCommit` - parent not in repo                  | Returns `invalid_parent`                                             |
| `CreateCommit` - two parent_ids supplied             | Returns `invalid_request` (use merge endpoint)                       |
| `CreateCommit` - root commit (empty parent_ids)      | No parent validation, inserts commit and outbox event                |
| `CreateCommit` - valid regular commit                | Inserts commit, commit_parent, outbox event in one transaction       |
| `GetCommit` - not found                              | Returns `commit_not_found`                                           |
| `GetCommit` - wrong repo                             | Returns `commit_not_found` (never leak cross-repo data)              |
| `ListCommits` - `traversal=dag` without branch param | Returns `invalid_request`                                            |
| `GetParents` - root commit                           | Returns empty parents array                                          |
| `GetParents` - merge commit                          | Returns exactly two parent commit objects                            |

#### MergeService

| Scenario                                                          | Expected behavior                                                                              |
| ----------------------------------------------------------------- | ---------------------------------------------------------------------------------------------- |
| `CreateMerge` - not exactly two parent_ids                        | Returns `invalid_request`                                                                      |
| `CreateMerge` - target branch not found                           | Returns `branch_not_found`                                                                     |
| `CreateMerge` - a parent commit not in repo                       | Returns `invalid_parent`                                                                       |
| `CreateMerge` - `expected_target_head` mismatch                   | Returns `stale_merge_target` with `current_head`                                               |
| `CreateMerge` - valid                                             | Inserts commit, two parent rows, advances branch, writes outbox event - all in one transaction |
| `CreateMerge` - concurrent: optimistic lock fails mid-transaction | Returns `stale_merge_target`                                                                   |

---

### Outbox Worker Layer (`internal/outbox/`)

The outbox worker is the async component that reads unprocessed events from `outbox_events` and propagates changes to Neo4j and Redis. Its guarantees are: every event is eventually processed, processing is idempotent (running the same event twice produces the same state), and a worker failure at any point leaves the system in a recoverable state.

Unit tests here mock both the outbox store and the derived stores (Neo4j store, Redis store).

#### Event routing

| Scenario                           | Expected behavior                                                            |
| ---------------------------------- | ---------------------------------------------------------------------------- |
| `commit.created` event dispatched  | Neo4j handler is called; Redis handler is not called                         |
| `branch.advanced` event dispatched | Redis handler is called; Neo4j handler is not called                         |
| Unknown event type                 | Event is logged and marked processed; no handler called; no error propagated |

#### Neo4j handler (`commit.created`)

| Scenario                                                | Expected behavior                                                                             |
| ------------------------------------------------------- | --------------------------------------------------------------------------------------------- |
| Regular commit (one parent)                             | `UpsertCommit` called once; `UpsertParentEdge` called once                                    |
| Root commit (zero parents)                              | `UpsertCommit` called once; `UpsertParentEdge` never called                                   |
| Merge commit (two parents, `is_merge: true` in payload) | `UpsertCommit` called once; `UpsertParentEdge` called twice                                   |
| `UpsertCommit` fails                                    | Event not marked processed; will be retried on next worker run                                |
| `UpsertParentEdge` fails after `UpsertCommit` succeeded | Event not marked processed; on retry, `UpsertCommit` runs again (idempotent via MERGE)        |
| Same event processed twice                              | Second run calls `UpsertCommit` and `UpsertParentEdge` again; state is identical (idempotent) |

#### Redis handler (`branch.advanced`)

| Scenario                                         | Expected behavior                                                       |
| ------------------------------------------------ | ----------------------------------------------------------------------- |
| Cache key exists and is stale                    | `InvalidateBranchHead` called; key deleted                              |
| Cache key does not exist                         | `InvalidateBranchHead` called; no error (DEL on missing key is a no-op) |
| `InvalidateBranchHead` fails (Redis unavailable) | Event not marked processed; will be retried; does not crash the worker  |
| Same `branch.advanced` event processed twice     | Second `InvalidateBranchHead` call is a no-op; state is correct         |

#### Mark processed

| Scenario                     | Expected behavior                                                                         |
| ---------------------------- | ----------------------------------------------------------------------------------------- |
| Handler succeeds             | `MarkProcessed` called with the event ID                                                  |
| Handler returns error        | `MarkProcessed` not called; event remains unprocessed for retry                           |
| `MarkProcessed` itself fails | Error is logged; event will be re-processed on next run (idempotent handlers absorb this) |

---

### Config Layer (`internal/config/`)

| Scenario                                     | Expected behavior                                    |
| -------------------------------------------- | ---------------------------------------------------- |
| Minimal valid config (only postgres DSN set) | Parses without error; Redis disabled; Neo4j disabled |
| Redis enabled flag set                       | `RedisEnabled = true`                                |
| Neo4j enabled flag set                       | `Neo4jEnabled = true`                                |
| Invalid log level                            | Returns parse error                                  |
| Missing PostgreSQL DSN                       | Returns parse error (required field)                 |

---

## Integration Tests

**Location:** co-located with the storage and service packages as `*_test.go`, using the `//go:build integration` build tag.

**Runtime:** Requires Docker. testcontainers-go spins up real PostgreSQL, Redis, and Neo4j containers. Each storage package manages its own container. Run with `go test -tags integration ./...` or `make test-integration`.

---

### PostgreSQL Storage (`internal/storage/postgres/`)

Each test gets a clean schema. Tests share a container within a package but each test isolates its data by using unique repo/commit IDs.

#### RepoStore

| Scenario                   | What to assert                                                    |
| -------------------------- | ----------------------------------------------------------------- |
| `Insert` - new repo        | Row exists in DB with correct fields                              |
| `GetByID` - exists         | Returns correct struct                                            |
| `GetByID` - not found      | Returns typed not-found error                                     |
| `List` - empty table       | Returns empty slice, null cursor                                  |
| `List` - multiple repos    | Returns in consistent order, cursor works across pages            |
| `List` - cursor pagination | Second page starts where first page ended, no duplicates, no gaps |

#### BranchStore

| Scenario                                    | What to assert                               |
| ------------------------------------------- | -------------------------------------------- |
| `Insert` - new branch                       | Row exists with correct commit_id            |
| `Insert` - duplicate name in same repo      | Error maps to `branch_already_exists`        |
| `Insert` - duplicate name in different repo | Succeeds (names are scoped to repo)          |
| `AdvanceHead` - correct expected_commit_id  | Updates row, returns new head                |
| `AdvanceHead` - wrong expected_commit_id    | Returns 0 rows, store returns conflict error |
| `AdvanceHead` - branch not found            | Returns not-found error                      |
| `Delete` - branch exists                    | Row removed                                  |
| `Delete` - branch not found                 | Returns not-found error                      |
| `GetHead` - exists                          | Returns commit_id                            |
| `GetHead` - not found                       | Returns not-found error                      |
| `List` with pagination                      | Cursor works correctly                       |

#### CommitStore

| Scenario                                 | What to assert                                                                                      |
| ---------------------------------------- | --------------------------------------------------------------------------------------------------- |
| `Insert` - root commit                   | Commit row inserted, zero rows in `commit_parents`                                                  |
| `Insert` - regular commit                | Commit row + one `commit_parents` row                                                               |
| `Insert` - with outbox event             | Commit row + outbox row inserted in same transaction; if transaction rolls back, neither row exists |
| `Insert` - idempotency key collision     | Returns existing commit, no duplicate inserted                                                      |
| `Insert` - parent_id from different repo | Returns `invalid_parent` error                                                                      |
| `GetByID` - exists                       | Returns full commit with DataPointer                                                                |
| `GetByID` - cross-repo lookup            | Returns not-found (repo_id scoped)                                                                  |
| `ListFlat` - empty repo                  | Returns empty slice                                                                                 |
| `ListFlat` - author filter               | Returns only matching commits                                                                       |
| `ListFlat` - since/until filter          | Returns only commits in range                                                                       |
| `ListDAG` - from head                    | Returns commits in reverse-chronological order following parent links                               |
| `ListDAG` - merge commit in history      | Both parent branches appear in traversal                                                            |
| `GetParents` - root commit               | Returns empty slice                                                                                 |
| `GetParents` - regular commit            | Returns one parent                                                                                  |
| `GetParents` - merge commit              | Returns two parents                                                                                 |

#### MergeStore

| Scenario                   | What to assert                                                                                                                      |
| -------------------------- | ----------------------------------------------------------------------------------------------------------------------------------- |
| `CreateMerge` - valid      | Commit row, two `commit_parents` rows, branch `commit_id` updated, outbox event written - all in one transaction                    |
| `CreateMerge` - lock fails | Simulated by advancing the branch between validation and the UPDATE; whole transaction rolls back; commit row and outbox row absent |

#### OutboxStore

| Scenario            | What to assert                                        |
| ------------------- | ----------------------------------------------------- |
| `Fetch unprocessed` | Returns only rows where `processed = false`           |
| `MarkProcessed`     | Sets `processed = true`, idempotent when called twice |

---

### Redis Storage (`internal/storage/redis/`)

Tests run against a real Redis container.

| Scenario                                   | What to assert                                                        |
| ------------------------------------------ | --------------------------------------------------------------------- |
| `SetBranchHead`                            | Key exists in Redis with correct value and TTL set                    |
| `GetBranchHead` - cache hit                | Returns commit_id                                                     |
| `GetBranchHead` - cache miss               | Returns miss signal (nil), no error                                   |
| `InvalidateBranchHead` - key exists        | Key deleted; subsequent `GetBranchHead` returns miss                  |
| `InvalidateBranchHead` - key doesn't exist | No error returned (idempotent)                                        |
| `GetBranchHead` - Redis unreachable        | Returns miss signal; does not panic or propagate a fatal error        |
| Set then advance then get                  | After `SetBranchHead`, `InvalidateBranchHead`, `GetBranchHead` → miss |

---

### Neo4j Storage (`internal/storage/neo4j/`)

Tests run against a real Neo4j container.

| Scenario                                               | What to assert                                                                  |
| ------------------------------------------------------ | ------------------------------------------------------------------------------- |
| `UpsertCommit` - new commit                            | Node exists in graph with correct `id` and `repo_id` properties                 |
| `UpsertCommit` - same commit twice                     | Exactly one node exists (MERGE is idempotent)                                   |
| `UpsertParentEdge` - new edge                          | `PARENT_OF` relationship exists between child and parent nodes                  |
| `UpsertParentEdge` - same edge twice                   | Exactly one relationship exists (idempotent)                                    |
| `UpsertParentEdge` - merge commit (two calls)          | Two distinct `PARENT_OF` relationships from child to each parent                |
| `ListAncestors` - linear chain (A → B → C, start at A) | Returns B then C in order                                                       |
| `ListAncestors` - root commit (no parents)             | Returns empty slice                                                             |
| `ListAncestors` - merge commit in history              | Both parent branches are traversed; all ancestors from both sides appear        |
| `ListAncestors` - pagination (limit=2 on chain of 5)   | First page returns 2 nodes and a cursor; second page returns remaining nodes    |
| `ListAncestors` - commit not in graph                  | Returns not-found error                                                         |
| Node committed to wrong repo                           | `ListAncestors` scoped to `repo_id` does not return nodes from a different repo |

---

### Service + Storage Integration (`internal/service/`)

These test the service layer wired to a real PostgreSQL store (no mocks). They catch bugs at the boundary between business logic and SQL that mocks cannot surface.

| Scenario                                                                | What to assert                                                                     |
| ----------------------------------------------------------------------- | ---------------------------------------------------------------------------------- |
| Create commit → advance branch                                          | Branch head reflects new commit ID in DB                                           |
| Concurrent branch advancement (two goroutines, same expected_commit_id) | Exactly one succeeds; the other gets `branch_conflict` with correct `current_head` |
| Merge → check branch head and parent rows                               | `main` points to merge commit; `commit_parents` has two rows for merge commit      |
| Idempotent commit retry                                                 | Two calls with same `idempotency_key` produce one DB row, both return same commit  |
| Outbox event written with commit                                        | After `CreateCommit`, outbox row exists and `processed = false`                    |
| Outbox rollback on commit failure                                       | If commit insert fails mid-transaction, outbox row is also absent                  |

---

## E2E Tests

**Location:** `test/e2e/`

**Runtime:** Starts a full Verge HTTP server backed by a real PostgreSQL container. Tests exercise the public REST API over HTTP using a real HTTP client. Run with `make test-e2e`.

**Scope:** These tests verify the system from the outside, exactly as an integrating product would see it. One test file per resource domain. Test complete flows, not individual endpoints in isolation.

---

### Repo E2E (`test/e2e/repos_test.go`)

| Scenario                                 | Assert                                                                                                    |
| ---------------------------------------- | --------------------------------------------------------------------------------------------------------- |
| `POST /repos` - valid                    | 201, body has `id`, `name`, `default_branch`, `created_at`                                                |
| `POST /repos` - missing `name`           | 400, `error: "invalid_request"`                                                                           |
| `POST /repos` - missing `default_branch` | 400, `error: "invalid_request"`                                                                           |
| `GET /repos/:id` - exists                | 200, correct fields                                                                                       |
| `GET /repos/:id` - not found             | 404, `error: "repo_not_found"`                                                                            |
| `GET /repos` - pagination                | First page returns `next_cursor`; second page with cursor returns remaining repos and `next_cursor: null` |

---

### Branch E2E (`test/e2e/branches_test.go`)

| Scenario                                                         | Assert                                                          |
| ---------------------------------------------------------------- | --------------------------------------------------------------- |
| `POST /repos/:id/branches` - valid                               | 201, branch fields correct                                      |
| `POST /repos/:id/branches` - source commit not in repo           | 404, `error: "commit_not_found"`                                |
| `POST /repos/:id/branches` - duplicate name                      | 409, `error: "branch_already_exists"`                           |
| `PATCH /repos/:id/branches/:name` - valid advance                | 200, commit_id updated                                          |
| `PATCH /repos/:id/branches/:name` - missing `expected_commit_id` | 400, `error: "invalid_request"`                                 |
| `PATCH /repos/:id/branches/:name` - stale `expected_commit_id`   | 409, `error: "branch_conflict"`, `current_head` present in body |
| `DELETE /repos/:id/branches/:name` - valid                       | 204                                                             |
| `DELETE /repos/:id/branches/:name` - default branch              | 409, `error: "cannot_delete_default_branch"`                    |
| `DELETE /repos/:id/branches/:name` - not found                   | 404, `error: "branch_not_found"`                                |
| `GET /repos/:id/branches` - pagination                           | Works correctly                                                 |

---

### Commit E2E (`test/e2e/commits_test.go`)

| Scenario                                                   | Assert                                                              |
| ---------------------------------------------------------- | ------------------------------------------------------------------- |
| `POST /repos/:id/commits` - root commit (empty parent_ids) | 201, `parent_ids: []` in response                                   |
| `POST /repos/:id/commits` - regular commit                 | 201, `parent_ids` has one entry                                     |
| `POST /repos/:id/commits` - two parent_ids supplied        | 400, `error: "invalid_request"` with message directing to `/merges` |
| `POST /repos/:id/commits` - invalid parent                 | 422, `error: "invalid_parent"`                                      |
| `POST /repos/:id/commits` - idempotency key repeat         | 200 (not 201), same commit returned                                 |
| `GET /repos/:id/commits/:commit_id` - exists               | 200, full commit with `data_pointer`                                |
| `GET /repos/:id/commits/:commit_id` - not found            | 404, `error: "commit_not_found"`                                    |
| `GET /repos/:id/commits?traversal=flat`                    | Returns commits in reverse chronological order                      |
| `GET /repos/:id/commits?traversal=dag&branch=main`         | Follows parent links from branch head                               |
| `GET /repos/:id/commits?traversal=dag` (no branch)         | 400, `error: "invalid_request"`                                     |
| `GET /repos/:id/commits?author=X`                          | Returns only commits by that author                                 |
| `GET /repos/:id/commits?since=X&until=Y`                   | Returns only commits in timestamp range                             |
| `GET /repos/:id/commits/:id/parents` - root commit         | `parents: []`                                                       |
| `GET /repos/:id/commits/:id/parents` - regular commit      | One parent, full commit object                                      |
| `GET /repos/:id/commits/:id/parents` - merge commit        | Two parents, full commit objects                                    |

---

### Merge E2E (`test/e2e/merges_test.go`)

| Scenario                                                | Assert                                                                         |
| ------------------------------------------------------- | ------------------------------------------------------------------------------ |
| `POST /repos/:id/merges` - valid                        | 201, two parent_ids in response; target branch head is the new merge commit ID |
| `POST /repos/:id/merges` - not exactly two parent_ids   | 400, `error: "invalid_request"`                                                |
| `POST /repos/:id/merges` - target branch not found      | 404, `error: "branch_not_found"`                                               |
| `POST /repos/:id/merges` - a parent not in repo         | 422, `error: "invalid_parent"`                                                 |
| `POST /repos/:id/merges` - stale `expected_target_head` | 409, `error: "stale_merge_target"`, `current_head` present in body             |
| Branch head after merge                                 | `GET /repos/:id/branches` shows target branch pointing to merge commit         |

---

### Full Flow E2E (`test/e2e/flows_test.go`)

These tests replicate the scenarios in `INTERNAL_FLOW.md` end-to-end. Each test sets up its own repo and tears nothing down (each test is isolated by using unique names/IDs).

| Flow                              | What it covers                                                                                                    |
| --------------------------------- | ----------------------------------------------------------------------------------------------------------------- |
| **Save flow** (Flows 2+3)         | Create commit → advance branch → verify branch head updated                                                       |
| **Suggest mode flow** (Flows 4+5) | Create branch → commit to it → verify main is untouched → verify suggest branch advanced                          |
| **Merge flow** (Flow 8)           | Set up two-branch state → call merge → verify main advanced to merge commit → verify DAG has two parents          |
| **Restore flow** (Flow 9)         | Commit to main several times → restore to an old commit → verify history is append-only (old commits still exist) |
| **Concurrent branch conflict**    | Two goroutines advance the same branch simultaneously → exactly one 200, one 409 with `current_head`              |
| **Idempotent retry**              | POST commit with idempotency key → POST again → exactly one commit in history                                     |

---

## Test Naming and File Conventions

**File placement:** Unit and integration tests live next to the file they test, as `foo_test.go`. E2E tests live in `test/e2e/`.

**Build tags:**

- Integration tests: `//go:build integration` at top of file
- E2E tests: `//go:build e2e` at top of file
- Unit tests: no build tag (run by default)

**Test function names:** `Test<Subject>_<Scenario>` - for example:

```go
func TestCreateCommit_IdempotencyKeyMatch(t *testing.T)
func TestAdvanceBranch_StaleExpectedCommitID(t *testing.T)
func TestMerge_ExactlyTwoParentsRequired(t *testing.T)
```

**Subtests:** Use `t.Run` when a single setup is shared across multiple assertion variants. Do not nest more than one level deep.

**Table-driven tests:** Use when multiple cases share identical setup and differ only in their input values. A good example is validating all `data_pointer.type` enum values.

---

## Test Data and Fixtures

**No shared global fixtures:** Each test creates its own data. Tests must be independently runnable in any order.

**ID generation:** Use real UUIDs in tests (`google/uuid`). Do not use hardcoded strings like `"commit_v1"` in tests.

**Timestamps:** Do not assert exact timestamps. Assert that a timestamp field is non-empty and parses as valid ISO 8601.

**DataPointer in tests:** Use a consistent minimal fixture:

```go
var testDataPointer = domain.DataPointer{
    Type:     "db",
    Location: "test/snapshots/fixture",
}
```

**Repo/branch naming in E2E:** Use `uuid.New().String()` as the repo name to avoid collisions across parallel test runs.

---

## Coverage Targets

These are minimums, not ceilings.

| Layer                        | Target                                                                             |
| ---------------------------- | ---------------------------------------------------------------------------------- |
| `internal/domain/`           | All                                                                                |
| `internal/service/`          | All happy paths and every named error code                                         |
| `internal/storage/postgres/` | Every store method, all error returns                                              |
| `internal/storage/redis/`    | Every store method, cache hit/miss/unavailable paths                               |
| `internal/storage/neo4j/`    | Every store method, idempotency on replay, pagination                              |
| `internal/api/rest/v1/`      | All handlers, focus on error mapping                                               |
| `internal/api/grpc/v1/`      | Same as REST                                                                       |
| `internal/outbox/`           | All event types, all handler outcomes, idempotency on double-processing            |
| `test/e2e/`                  | Not measured by line coverage, measured by flow coverage (see Full Flow E2E above) |

Run coverage with:

```bash
make test-cover        # unit only
make test-cover-all    # unit + integration
```
