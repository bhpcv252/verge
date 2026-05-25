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

Test business logic that sits above storage. All storage calls are mocked with hand-rolled mocks.

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

### Outbox Worker (`internal/outbox/`)

The outbox worker reads unprocessed events from `outbox_events` and either dispatches them
in-process to registered handlers, or publishes them to an external broker via an `EventBus`.
Unit tests here use hand-rolled mocks for the database pool, handlers, and EventBus.

#### Worker - in-process dispatch mode (`bus == nil`)

| Scenario                                        | Expected behavior                                                                           |
| ----------------------------------------------- | ------------------------------------------------------------------------------------------- |
| Event type matches a registered handler         | Handler `Handle` called; event ID added to processed list; `processed = true` in DB         |
| Event type has no registered handler            | `dispatch` logs and returns nil; event IS still marked processed (no-match is not an error) |
| Handler returns an error                        | Event ID NOT added to processed list; worker logs and continues to next event               |
| Multiple handlers registered, only one matches  | Only the matching handler's `Handle` called; other handlers not called                      |
| Empty outbox (no unprocessed rows)              | No `UPDATE` issued; transaction committed cleanly                                           |
| Batch of mixed events (some succeed, some fail) | Successfully dispatched events marked processed; failed ones left unprocessed for retry     |

#### Worker - EventBus mode (`bus != nil`)

| Scenario                   | Expected behavior                                                                    |
| -------------------------- | ------------------------------------------------------------------------------------ |
| `Publish` succeeds         | All events in batch marked processed; no in-process handler is called                |
| `Publish` returns an error | No events marked processed; transaction rolls back; in-process handlers never called |

#### `RedisHealHandler` (`internal/outbox/redis_handler.go`)

Handles `BranchHeadMoved` events by calling `SetHead` on the `BranchHeadStore` with the
version from the payload. This is a heal pattern: it writes the current head into Redis
rather than invalidating it.

| Scenario                                | Expected behavior                                                       |
| --------------------------------------- | ----------------------------------------------------------------------- |
| `EventTypes()`                          | Returns exactly `["BranchHeadMoved"]`                                   |
| Valid `BranchHeadMoved` payload         | `SetHead` called with correct `repoID`, `branch`, `commitID`, `version` |
| Malformed JSON payload                  | Returns error; `SetHead` never called                                   |
| `SetHead` returns error                 | Error propagated to worker; event not marked processed                  |
| Same event processed twice (idempotent) | Second `SetHead` call is a no-op if version guard is in place in Redis  |

#### `Neo4jHandler` (`internal/outbox/neo4j_handler.go`)

Handles `CommitCreated` events by upserting commit nodes and `PARENT_OF` edges in Neo4j.
Full driver behavior is tested in integration. Unit tests focus on parsing and routing.

| Scenario                      | Expected behavior                                            |
| ----------------------------- | ------------------------------------------------------------ |
| `EventTypes()`                | Returns exactly `["CommitCreated"]`                          |
| Malformed JSON payload        | Returns error without calling the Neo4j driver               |
| Valid payload with no parents | Driver called once for node upsert; no edge upsert attempted |

---

### Composite Routers (`internal/storage/composite/`)

Routers implement read-from-fast-store / write-through / fallback logic. All dependencies
are mocked with hand-rolled mocks implementing the relevant interfaces.

#### `BranchRouter`

| Scenario                                                        | Expected behavior                                                                         |
| --------------------------------------------------------------- | ----------------------------------------------------------------------------------------- |
| `GetHead` - Redis hit                                           | Returns value from Redis; `pg.GetByName` never called                                     |
| `GetHead` - Redis miss (`ErrCacheMiss`)                         | Falls back to `pg.GetByName`; calls `redis.SetHead` to warm cache; returns postgres value |
| `GetHead` - Redis error (not `ErrCacheMiss`)                    | Logs error; falls back to `pg.GetByName`; calls `redis.SetHead`; returns postgres value   |
| `GetHead` - Redis error, then postgres error                    | Returns postgres error to caller                                                          |
| `GetHead` - Redis miss, `SetHead` fails after postgres fallback | `SetHead` failure is non-fatal; postgres value still returned to caller                   |
| `Advance` - postgres succeeds                                   | Returns updated branch; calls `redis.SetHead` to sync cache (non-fatal if it fails)       |
| `Advance` - postgres fails (conflict or not found)              | Returns postgres error; `redis.SetHead` never called                                      |
| `Advance` - postgres succeeds, `redis.SetHead` fails            | Branch returned successfully; Redis failure is logged and swallowed                       |
| `Create`, `GetByName`, `List`, `Delete`                         | Delegate directly to postgres; Redis never called                                         |

#### `CommitRouter`

| Scenario                                                                      | Expected behavior                                                                       |
| ----------------------------------------------------------------------------- | --------------------------------------------------------------------------------------- |
| `GetByID` - cache hit                                                         | Returns cached commit; `pg.GetByID` never called                                        |
| `GetByID` - cache miss (`ErrCacheMiss`)                                       | Falls back to `pg.GetByID`; calls `cache.SetCommit` to populate cache; returns pg value |
| `GetByID` - cache error (not `ErrCacheMiss`)                                  | Logs error; falls back to `pg.GetByID`; calls `cache.SetCommit`; returns pg value       |
| `GetByID` - cache miss, `SetCommit` fails after postgres fallback             | `SetCommit` failure is non-fatal; postgres value still returned                         |
| `GetByID` - cache miss, postgres returns not-found                            | Returns postgres not-found error; `SetCommit` never called                              |
| `Create`, `GetByIdempotencyKey`, `List`, `GetParents`, `ValidateParentsExist` | Delegate directly to postgres; cache never called                                       |

#### `GraphRouter`

| Scenario                         | Expected behavior                                                 |
| -------------------------------- | ----------------------------------------------------------------- |
| `TraverseDAG` - Neo4j succeeds   | Returns Neo4j result; `pg.TraverseDAG` never called               |
| `TraverseDAG` - Neo4j errors     | Logs error; falls back to `pg.TraverseDAG` and returns its result |
| `GetAncestors` - Neo4j succeeds  | Returns Neo4j result; `pg.GetAncestors` never called              |
| `GetAncestors` - Neo4j errors    | Falls back to postgres                                            |
| `FindMergeBase` - Neo4j succeeds | Returns Neo4j result; `pg.FindMergeBase` never called             |
| `FindMergeBase` - Neo4j errors   | Falls back to postgres                                            |

---

### Kafka EventBus (`internal/outbox/eventbus/kafka/`)

Unit tests focus on message serialisation and consumer dispatch logic. Tests that require
a real Kafka broker are integration tests (build tag: `integration`).

#### `message.go` - serialisation round-trip

| Scenario                              | Expected behavior                                                |
| ------------------------------------- | ---------------------------------------------------------------- |
| `toKafkaMessage` → `fromKafkaMessage` | All fields (`ID`, `EventType`, `Payload`, `CreatedAt`) preserved |

#### `Consumer.dispatch`

| Scenario                                | Expected behavior                    |
| --------------------------------------- | ------------------------------------ |
| Event type matches a registered handler | Handler `Handle` called              |
| Event type has no registered handler    | No handler called; no error returned |

#### `Consumer.Run` (unit, with mock reader)

| Scenario                        | Expected behavior                                                         |
| ------------------------------- | ------------------------------------------------------------------------- |
| Malformed JSON message received | Message skipped and offset committed; consumer continues without crashing |

---

### Config Layer (`internal/config/`)

| Group    | Scenario                                                         | Expected behavior                                                                                                                                                             |
| -------- | ---------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Defaults | Only `VERGE_STORAGE_POSTGRES_URL` set                            | All other fields take documented default values                                                                                                                               |
| Server   | Both HTTP and GRPC disabled                                      | Returns validation error                                                                                                                                                      |
| Server   | Only HTTP enabled / only GRPC enabled                            | Loads without error                                                                                                                                                           |
| Server   | HTTP or GRPC port = 0 or 65535                                   | Returns validation error (`gt=0,lt=65535`)                                                                                                                                    |
| Server   | HTTP or GRPC port = 1 or 65534                                   | Loads without error (boundary check)                                                                                                                                          |
| Postgres | URL missing                                                      | Returns validation error (`required`)                                                                                                                                         |
| Postgres | URL not a valid URL                                              | Returns validation error (`url`)                                                                                                                                              |
| Redis    | Enabled = true, URL set                                          | Loads without error                                                                                                                                                           |
| Redis    | Enabled = true, URL empty                                        | Returns validation error (`required-if-enabled`)                                                                                                                              |
| Redis    | Enabled = true, URL invalid                                      | Returns validation error (`url`)                                                                                                                                              |
| Redis    | Enabled = false, URL empty or present                            | Loads without error                                                                                                                                                           |
| Redis    | `BRANCH_TTL` set to `"2m"`                                       | `BranchTTL` parsed as `2 * time.Minute`                                                                                                                                       |
| Redis    | `BRANCH_TTL` set to invalid string                               | Returns parse error                                                                                                                                                           |
| Neo4j    | Enabled = true, URL set                                          | Loads without error                                                                                                                                                           |
| Neo4j    | Enabled = true, URL empty                                        | Returns validation error (`required-if-enabled`)                                                                                                                              |
| Neo4j    | Enabled = true, URL invalid                                      | Returns validation error (`url`)                                                                                                                                              |
| Neo4j    | Enabled = false, URL empty or present                            | Loads without error                                                                                                                                                           |
| Outbox   | `POLL_INTERVAL` = `"1s"`                                         | `PollInterval` parsed as `time.Second`                                                                                                                                        |
| Outbox   | `POLL_INTERVAL` = invalid string                                 | Returns parse error                                                                                                                                                           |
| Outbox   | `BATCH_SIZE` = `"50"`                                            | `BatchSize` parsed as `50`                                                                                                                                                    |
| Outbox   | `BATCH_SIZE` = non-integer string                                | Returns parse error                                                                                                                                                           |
| EventBus | Enabled = true, Type = `"kafka"` / `"rabbitmq"` / `"sqs"` / etc. | Loads without error (any non-empty type is valid)                                                                                                                             |
| EventBus | Enabled = true, Type = `""`                                      | Returns validation error (`required-if-enabled`). Tested via `validate()` directly because `envDefault:"kafka"` prevents an empty string reaching validation through `Load()` |
| EventBus | Enabled = false, Type = anything including `""`                  | Loads without error (type is irrelevant when disabled)                                                                                                                        |
| Kafka    | Custom brokers and topic set                                     | Fields parsed correctly                                                                                                                                                       |
| Registry | `VERGE_` env var present that is not in `knownVergeKeys`         | Test fails, forcing the dev to register the new var                                                                                                                           |

---

### REST Handlers (`internal/api/rest/v1/`)

The service layer is mocked with a hand-rolled function-field mock. Tests use `httptest.NewRecorder` and call the handler directly via the router. They focus on two things: does the handler correctly parse the request and call the service with the right input, and does it correctly map service errors to the right HTTP status and error code body.

The handler test only verifies that the HTTP response has the right status code and error code field when the service returns that error.

#### RepoHandler

| Scenario                                            | What to assert                                                       |
| --------------------------------------------------- | -------------------------------------------------------------------- |
| `POST /repos` - valid body                          | Service called with correct input; 201 returned with serialized repo |
| `POST /repos` - invalid JSON body                   | 400, `error: "invalid_request"` before service is called             |
| `POST /repos` - missing `name`                      | 400, `error: "invalid_request"` before service is called             |
| `POST /repos` - missing `default_branch`            | 400, `error: "invalid_request"` before service is called             |
| `GET /repos/:id` - service returns repo             | 200 with correct JSON shape                                          |
| `GET /repos/:id` - service returns `repo_not_found` | 404, `error: "repo_not_found"`                                       |
| `GET /repos/:id` - service returns unexpected error | 500, `error: "internal_error"`                                       |
| `GET /repos` - no params                            | 200, passes default limit to service                                 |
| `GET /repos` - invalid limit param                  | 400, `error: "invalid_request"`                                      |
| `GET /repos` - limit out of range                   | 400, `error: "invalid_request"`                                      |

#### BranchHandler

| Scenario                                                                            | What to assert              |
| ----------------------------------------------------------------------------------- | --------------------------- |
| `POST /repos/:id/branches` - valid                                                  | 201, branch fields correct  |
| `POST /repos/:id/branches` - service returns `repo_not_found`                       | 404                         |
| `POST /repos/:id/branches` - service returns `commit_not_found`                     | 404                         |
| `POST /repos/:id/branches` - service returns `branch_already_exists`                | 409                         |
| `PATCH /repos/:id/branches/:name` - valid advance                                   | 200, `commit_id` updated    |
| `PATCH /repos/:id/branches/:name` - missing `expected_commit_id`                    | 400                         |
| `PATCH /repos/:id/branches/:name` - service returns `branch_conflict`               | 409, `current_head` in body |
| `DELETE /repos/:id/branches/:name` - valid                                          | 204                         |
| `DELETE /repos/:id/branches/:name` - service returns `branch_not_found`             | 404                         |
| `DELETE /repos/:id/branches/:name` - service returns `cannot_delete_default_branch` | 409                         |
| `GET /repos/:id/branches` - no params                                               | 200, first page             |

#### CommitHandler

| Scenario                                                          | What to assert                  |
| ----------------------------------------------------------------- | ------------------------------- |
| `POST /repos/:id/commits` - valid (root commit)                   | 201, `parent_ids: []`           |
| `POST /repos/:id/commits` - valid (regular commit)                | 201, `parent_ids` has one entry |
| `POST /repos/:id/commits` - two parent_ids in body                | 400, `error: "invalid_request"` |
| `POST /repos/:id/commits` - service returns `repo_not_found`      | 404                             |
| `POST /repos/:id/commits` - service returns `invalid_parent`      | 422                             |
| `POST /repos/:id/commits` - idempotency key repeat                | 200 (not 201)                   |
| `GET /repos/:id/commits/:id` - service returns commit             | 200, full commit shape          |
| `GET /repos/:id/commits/:id` - service returns `commit_not_found` | 404                             |
| `GET /repos/:id/commits` - `traversal=dag` without `branch` param | 400, `error: "invalid_request"` |
| `GET /repos/:id/commits/:id/parents` - root commit                | 200, `parents: []`              |

#### MergeHandler

| Scenario                                                        | What to assert                    |
| --------------------------------------------------------------- | --------------------------------- |
| `POST /repos/:id/merges` - valid                                | 201, two `parent_ids` in response |
| `POST /repos/:id/merges` - not exactly two parent_ids           | 400, `error: "invalid_request"`   |
| `POST /repos/:id/merges` - service returns `branch_not_found`   | 404                               |
| `POST /repos/:id/merges` - service returns `invalid_parent`     | 422                               |
| `POST /repos/:id/merges` - service returns `stale_merge_target` | 409, `current_head` in body       |

---

## Integration Tests

**Build tag:** `//go:build integration`

**Runtime:** each test spins up a real container via `testcontainers-go`. Use the helpers in `testhelper/` for Postgres. Add equivalent helpers for Redis and Neo4j containers as new stores are tested.

---

### Postgres Storage (`internal/storage/postgres/`)

#### `BranchStore`

Existing tests cover `Create`, `GetByName`, `List`, and `Delete`. The following tests must be added or updated for `Advance`:

| Scenario                                             | What to assert                                                                                                                                                                      |
| ---------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `Advance` - success                                  | Branch `commit_id` updated in DB                                                                                                                                                    |
| `Advance` - success: outbox event written atomically | An `outbox_events` row exists with `event_type = "BranchHeadMoved"`, `processed = false`, and payload containing correct `repo_id`, `branch`, `commit_id`, and a non-zero `version` |
| `Advance` - conflict: outbox event NOT written       | When `Advance` returns `branch_conflict`, no new `outbox_events` row is written                                                                                                     |
| `Advance` - outbox INSERT fails mid-transaction      | Branch `commit_id` is NOT updated (entire transaction rolled back)                                                                                                                  |
| `Advance` - stale `expected_commit_id`               | Returns `branch_conflict` with correct `CurrentHead`                                                                                                                                |
| `Advance` - branch not found                         | Returns `branch_not_found`                                                                                                                                                          |

---

### Redis Storage (`internal/storage/redis/`)

Tests run against a real Redis container (testcontainers).

#### `BranchHeadStore` (`GetHead` / `SetHead`)

| Scenario                                          | What to assert                                                                   |
| ------------------------------------------------- | -------------------------------------------------------------------------------- |
| `SetHead` → `GetHead`                             | Returns the correct `commitID`                                                   |
| `GetHead` on a key that was never set             | Returns `interfaces.ErrCacheMiss`                                                |
| `SetHead` with higher version after lower version | `GetHead` returns the newer `commitID` (higher version wins)                     |
| `SetHead` with lower version after higher version | `GetHead` still returns the previously written `commitID` (stale write rejected) |
| `SetHead` twice with same version                 | Second write accepted (idempotent); value unchanged                              |
| TTL applied: key expires after `BranchTTL`        | After TTL elapses, `GetHead` returns `ErrCacheMiss`                              |

#### `CommitCache` (`GetCommit` / `SetCommit`)

| Scenario                                       | What to assert                                                       |
| ---------------------------------------------- | -------------------------------------------------------------------- |
| `SetCommit` → `GetCommit`                      | Returns correct commit with all fields preserved                     |
| `GetCommit` on a key that was never set        | Returns `interfaces.ErrCacheMiss`                                    |
| Key written with corrupted JSON (manual `SET`) | `GetCommit` returns `ErrCacheMiss` (corrupted entry treated as miss) |
| No TTL on commit entries                       | Key persists beyond any reasonable test duration                     |

---

### Neo4j Storage (`internal/storage/neo4j/`)

Tests run against a real Neo4j container (testcontainers). Each test creates its own isolated
repo ID so that graphs do not bleed across test cases.

#### `Neo4jHandler` - commit node and edge upserts

| Scenario                                              | What to assert                                                                |
| ----------------------------------------------------- | ----------------------------------------------------------------------------- |
| `Handle` - root commit (zero parents)                 | Commit node exists in graph; no `PARENT_OF` edge created                      |
| `Handle` - regular commit (one parent)                | Commit node exists; one `PARENT_OF` edge from child to parent                 |
| `Handle` - merge commit (two parents)                 | Commit node exists; two `PARENT_OF` edges, one to each parent                 |
| `Handle` - same `CommitCreated` event processed twice | Exactly one commit node and the correct number of edges (MERGE is idempotent) |
| `Handle` - parent node did not exist yet              | Parent stub node created; properties filled when parent's own event arrives   |

#### `GraphStore` - DAG traversal queries

| Scenario                                                | What to assert                                                                       |
| ------------------------------------------------------- | ------------------------------------------------------------------------------------ |
| `TraverseDAG` - linear chain from head                  | Returns commits in `timestamp DESC` order; all commits in chain present              |
| `TraverseDAG` - with `Author` filter                    | Only commits by that author returned                                                 |
| `TraverseDAG` - with `Since` / `Until` filters          | Only commits within the time range returned                                          |
| `TraverseDAG` - pagination: `limit=2` on chain of 5     | First call returns 2 commits and a non-empty cursor; second call returns remaining 3 |
| `TraverseDAG` - empty `Head` param                      | Returns error                                                                        |
| `GetAncestors` - linear chain (start at A, parents B→C) | Returns B and C; A itself is excluded                                                |
| `GetAncestors` - root commit (no parents)               | Returns empty slice, no error                                                        |
| `GetAncestors` - merge commit in history                | Ancestors from both parent branches appear in result                                 |
| `FindMergeBase` - two branches with common ancestor     | Returns the correct LCA commit                                                       |
| `FindMergeBase` - no common ancestor                    | Returns error                                                                        |
| Repo isolation                                          | Queries scoped to `repo_id` never return nodes belonging to a different repo         |

---

### Service + Storage Integration (`internal/service/`)

These test the service layer wired to a real PostgreSQL store (no mocks). They catch bugs at the boundary between business logic and SQL that mocks cannot surface.

| Scenario                                                                | What to assert                                                                                            |
| ----------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------- |
| Create commit → advance branch                                          | Branch head reflects new commit ID in DB                                                                  |
| Concurrent branch advancement (two goroutines, same expected_commit_id) | Exactly one succeeds; the other gets `branch_conflict` with correct `current_head`                        |
| Merge → check branch head and parent rows                               | `main` points to merge commit; `commit_parents` has two rows for merge commit                             |
| Idempotent commit retry                                                 | Two calls with same `idempotency_key` produce one DB row, both return same commit                         |
| Outbox event written with commit                                        | After `CreateCommit`, an `outbox_events` row exists with `processed = false`                              |
| Outbox rollback on commit failure                                       | If commit insert fails mid-transaction, outbox row is also absent                                         |
| Outbox event written with `AdvanceBranch`                               | After `AdvanceBranch`, a `BranchHeadMoved` outbox row exists with `processed = false` and correct payload |

---

### Outbox Worker Integration (`internal/outbox/`)

Tests run against a real PostgreSQL container (testcontainers). The worker is run directly by calling `poll()` once rather than starting the ticker loop.

| Scenario                                                      | What to assert                                                       |
| ------------------------------------------------------------- | -------------------------------------------------------------------- |
| Unprocessed events present, handler succeeds                  | After `poll`, rows have `processed = true` and `processed_at` is set |
| Handler returns error                                         | Row remains `processed = false`; available for retry on next poll    |
| Two workers polling simultaneously (`FOR UPDATE SKIP LOCKED`) | Each event processed exactly once across both workers                |
| EventBus mode: mock bus `Publish` succeeds                    | All events in batch marked processed; bus received all events        |
| EventBus mode: mock bus `Publish` fails                       | No events marked processed; transaction rolled back                  |
| Empty outbox                                                  | `poll` returns nil; no UPDATE statement executed                     |

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
func TestBranchRouter_GetHead_CacheMiss_FallsBackToPostgres(t *testing.T)
func TestRedisHealHandler_ValidPayload_CallsSetHead(t *testing.T)
func TestWorker_HandlerError_EventNotMarkedProcessed(t *testing.T)
```

**Subtests:** Use `t.Run` when a single setup is shared across multiple assertion variants. Do not nest more than one level deep.

**Table-driven tests:** Use when multiple cases share identical setup and differ only in their input values. A good example is validating all `data_pointer.type` enum values or all `EventBus.Type` strings that must be accepted.

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

| Layer                             | Target                                                                                       |
| --------------------------------- | -------------------------------------------------------------------------------------------- |
| `internal/domain/`                | All                                                                                          |
| `internal/config/`                | All fields: defaults, validation rules, every env var in `knownVergeKeys`                    |
| `internal/service/`               | All happy paths and every named error code                                                   |
| `internal/storage/postgres/`      | Every store method; all error returns; outbox event written atomically with `Advance`        |
| `internal/storage/redis/`         | `GetHead`/`SetHead` hit/miss/version-guard; `GetCommit`/`SetCommit` hit/miss/corrupted       |
| `internal/storage/neo4j/`         | Handler upsert idempotency; all three `GraphStore` query methods; pagination; repo isolation |
| `internal/storage/composite/`     | All fallback paths for each router; non-fatal cache/Redis failures swallowed correctly       |
| `internal/outbox/`                | All event types; both dispatch modes (in-process and EventBus); handler error handling       |
| `internal/outbox/eventbus/kafka/` | Message round-trip; consumer dispatch; malformed message handling                            |
| `internal/api/rest/v1/`           | All handlers; focus on error mapping                                                         |
| `test/e2e/`                       | Not measured by line coverage; measured by flow coverage (see Full Flow E2E above)           |
