# Verge - Internal System Flows

This document traces every request that Google Docs-like product (example) sends to Verge, and follows it step by step through each internal component, what gets validated, what gets written, what gets rejected, and what happens asynchronously after the response is returned.

---

## Table of Contents

- [Flow 1 - Create Repository](#flow-1--create-repository)
- [Flow 2 - Create Commit](#flow-2--create-commit)
- [Flow 3 - Advance Branch](#flow-3--advance-branch)
- [Flow 4 - Create Branch (suggest mode)](#flow-4--create-branch-suggest-mode)
- [Flow 5 - Commit to Suggest Branch](#flow-5--commit-to-suggest-branch)
- [Flow 6 - Checkout (document opened)](#flow-6--checkout-document-opened)
- [Flow 7 - History Query](#flow-7--history-query)
- [Flow 8 - Merge](#flow-8--merge)
- [Flow 9 - Restore](#flow-9--restore)
- [Error Cases](#error-cases)

---

## Flow 1 - Create Repository

**Trigger:** Google Docs creates a new document. A Verge repo must exist before any commit can be made.

**Request from Google Docs:**

```http
POST /repos
{
  "name": "doc_abc123",
  "default_branch": "main"
}
```

### Step-by-step through Verge

```
1. API Layer
   │
   ├── Receives POST /repos
   ├── Validates request body:
   │     ✓ name present and non-empty
   │     ✓ default_branch present and non-empty
   │     ✓ no unknown fields
   └── Passes to Repository Manager

2. Repository Manager
   │
   ├── Generates uuid for repo_id
   │
   ├── Opens PostgreSQL transaction:
   │     INSERT INTO repos (id, name, default_branch, created_at)
   │     VALUES ('repo_doc_abc123', 'doc_abc123', 'main', NOW())
   │
   └── Commits transaction

3. API Layer
   │
   └── Returns 201 Created
         {
           "id": "repo_doc_abc123",
           "name": "doc_abc123",
           "default_branch": "main",
           "created_at": "2024-04-05T10:00:00Z"
         }
```

**State after this flow:**

| Table      | Change                                          |
| ---------- | ----------------------------------------------- |
| `repos`    | One new row inserted                            |
| `branches` | Nothing - branch row is created on first commit |
| `commits`  | Nothing                                         |

---

## Flow 2 - Create Commit

**Trigger:** Alice clicks "Save version". The product has serialized the document, stored the snapshot in its own DB, and constructed a DataPointer. It now calls Verge to create the commit. Branch advancement happens in Flow 3 - these are always two separate steps.

**Request from Google Docs:**

```http
POST /repos/repo_doc_abc123/commits
{
  "parent_ids": ["commit_v1"],
  "expected_head": "commit_v1",
  "data_pointer": {
    "type": "db",
    "location": "documents/snapshots/doc_abc123/v_1712345678",
    "hash": "sha256:a3f1c9d2e..."
  },
  "message": "Added executive summary section",
  "author": "user_alice@company.com",
  "idempotency_key": "uuid-alice-save-001"
}
```

### Step-by-step through Verge

```
1. API Layer
   │
   ├── Receives POST /repos/repo_doc_abc123/commits
   ├── Validates request schema:
   │     ✓ repo_id present in URL
   │     ✓ parent_ids is an array (zero, one entry valid - two entries rejected: use /merges)
   │     ✓ data_pointer.type is one of: s3, url, db, custom
   │     ✓ data_pointer.location is non-empty string
   │     ✓ data_pointer.hash format valid if present (must start with "sha256:")
   │     ✓ message and author present
   │     ✓ idempotency_key is string if present
   └── Passes to Consistency & Validation Layer

2. Consistency & Validation Layer
   │
   ├── Checks idempotency (if key provided):
   │     SELECT id FROM commits
   │     WHERE repo_id = 'repo_doc_abc123' AND idempotency_key = 'uuid-alice-save-001'
   │     → found: return 200 OK with existing commit (skip all further steps)
   │     → not found: proceed
   │
   ├── Validates repo exists:
   │     SELECT id FROM repos WHERE id = 'repo_doc_abc123'
   │     → not found: return 404 { "error": "repo_not_found" }
   │
   ├── Validates each parent_id exists within this repo:
   │     SELECT id FROM commits
   │     WHERE id = 'commit_v1' AND repo_id = 'repo_doc_abc123'
   │     → not found: return 422 { "error": "invalid_parent" }
   │     → found: proceed
   │
   ├── Validates expected_head if provided:
   │     (Does not touch branches table here - this is a hint for the caller.
   │      The actual optimistic lock happens during branch advancement in Flow 3.)
   │
   └── Passes to Commit Engine

3. Commit Engine
   │
   ├── Generates new uuid: commit_v2
   │
   ├── Opens PostgreSQL transaction:
   │
   │   a. INSERT INTO commits:
   │        INSERT INTO commits
   │          (id, repo_id, message, author, timestamp, data_pointer, idempotency_key)
   │        VALUES (
   │          'commit_v2',
   │          'repo_doc_abc123',
   │          'Added executive summary section',
   │          'user_alice@company.com',
   │          NOW(),
   │          '{"type":"db","location":"documents/snapshots/doc_abc123/v_1712345678",...}',
   │          'uuid-alice-save-001'
   │        )
   │
   │   b. INSERT INTO commit_parents:
   │        INSERT INTO commit_parents (commit_id, parent_id)
   │        VALUES ('commit_v2', 'commit_v1')
   │
   │   c. INSERT INTO outbox_events:
   │        INSERT INTO outbox_events (id, event_type, payload, created_at, processed)
   │        VALUES (
   │          'evt_001',
   │          'commit.created',
   │          '{ "commit_id": "commit_v2", "repo_id": "repo_doc_abc123",
   │             "parent_ids": ["commit_v1"], "timestamp": "2024-04-05T10:05:00Z" }',
   │          NOW(),
   │          false
   │        )
   │
   └── COMMIT transaction

4. API Layer
   │
   └── Returns 201 Created
         {
           "id": "commit_v2",
           "repo_id": "repo_doc_abc123",
           "parent_ids": ["commit_v1"],
           "data_pointer": { ... },
           "message": "Added executive summary section",
           "author": "user_alice@company.com",
           "timestamp": "2024-04-05T10:05:00Z"
         }

5. Async - Outbox Workers (after response returned to caller)
   │
   ├── Neo4j Sync Worker (if active):
   │     Reads evt_001 payload
   │     MERGE (c:Commit {id: 'commit_v2', repo_id: 'repo_doc_abc123'})
   │     MERGE (p:Commit {id: 'commit_v1'})
   │     MERGE (c)-[:PARENT_OF]->(p)
   │     Marks evt_001 processed = true
   │
   └── Redis Worker (if active):
         No action on commit creation - Redis is only invalidated on branch advancement
```

**State after this flow:**

| Table            | Change                                         |
| ---------------- | ---------------------------------------------- |
| `commits`        | New row: `commit_v2`                           |
| `commit_parents` | New row: `(commit_v2, commit_v1)`              |
| `branches`       | Unchanged - `main` still points to `commit_v1` |
| `outbox_events`  | New row: `evt_001`, `processed = false`        |

---

## Flow 3 - Advance Branch

**Trigger:** Immediately after Flow 2. Google Docs advances the `main` branch pointer from `commit_v1` to the newly created `commit_v2`. This is always a separate call from commit creation.

**Request from Google Docs:**

```http
PATCH /repos/repo_doc_abc123/branches/main
{
  "commit_id": "commit_v2",
  "expected_commit_id": "commit_v1"
}
```

### Step-by-step through Verge

```
1. API Layer
   │
   ├── Receives PATCH /repos/repo_doc_abc123/branches/main
   ├── Validates:
   │     ✓ repo_id in URL
   │     ✓ branch name in URL
   │     ✓ commit_id present in body
   │     ✓ expected_commit_id present - rejected with 400 if missing
   └── Passes to Branch Manager

2. Consistency & Validation Layer
   │
   ├── Validates repo exists:
   │     SELECT id FROM repos WHERE id = 'repo_doc_abc123'
   │     → not found: return 404
   │
   └── Validates commit_id exists in this repo:
         SELECT id FROM commits
         WHERE id = 'commit_v2' AND repo_id = 'repo_doc_abc123'
         → not found: return 404 { "error": "commit_not_found" }

3. Branch Manager
   │
   ├── Opens PostgreSQL transaction
   │
   ├── Attempts optimistic lock UPDATE:
   │     UPDATE branches
   │     SET commit_id = 'commit_v2'
   │     WHERE name = 'main'
   │       AND repo_id = 'repo_doc_abc123'
   │       AND commit_id = 'commit_v1'    ← must still be at v1
   │     RETURNING commit_id
   │
   │     → 0 rows updated: branch has moved - ROLLBACK
   │       SELECT commit_id FROM branches WHERE name='main' AND repo_id='repo_doc_abc123'
   │       return 409 {
   │         "error": "branch_conflict",
   │         "message": "Branch 'main' has advanced. Fetch the latest head and retry.",
   │         "current_head": "commit_v3"   ← actual current head returned to caller
   │       }
   │
   │     → 1 row updated: proceed
   │
   ├── INSERT INTO outbox_events:
   │     {
   │       event_type: 'branch.advanced',
   │       payload: { branch: 'main', repo_id: 'repo_doc_abc123',
   │                  old_commit_id: 'commit_v1', new_commit_id: 'commit_v2' }
   │     }
   │
   └── COMMIT transaction

4. API Layer
   │
   └── Returns 200 OK
         {
           "name": "main",
           "repo_id": "repo_doc_abc123",
           "commit_id": "commit_v2"
         }

5. Async - Outbox Workers
   │
   └── Redis Invalidation Worker (if active):
         Reads branch.advanced event for branch 'main'
         DEL branch:repo_doc_abc123:main    ← evicts stale cache entry
         Marks outbox event processed = true
```

**State after this flow:**

| Table           | Change                                |
| --------------- | ------------------------------------- |
| `branches`      | `main` now points to `commit_v2`      |
| `outbox_events` | New branch.advanced event             |
| Redis (async)   | `branch:repo_doc_abc123:main` evicted |

---

## Flow 4 - Create Branch (suggest mode)

**Trigger:** Alice enables "Suggest mode". Google Docs creates a Verge branch so Alice's edits are isolated from `main`.

**Request from Google Docs:**

```http
POST /repos/repo_doc_abc123/branches
{
  "name": "suggest-alice-20240405",
  "source_commit_id": "commit_v2"
}
```

### Step-by-step through Verge

```
1. API Layer
   │
   ├── Validates: repo_id in URL, name and source_commit_id present
   └── Passes to Repository Manager

2. Consistency & Validation Layer
   │
   ├── Validates repo exists                                     ✓
   │
   ├── Validates source_commit_id exists in this repo:
   │     SELECT id FROM commits
   │     WHERE id = 'commit_v2' AND repo_id = 'repo_doc_abc123'
   │     → not found: return 404 { "error": "commit_not_found" }
   │
   └── Validates branch name not already taken:
         SELECT name FROM branches
         WHERE name = 'suggest-alice-20240405' AND repo_id = 'repo_doc_abc123'
         → exists: return 409 { "error": "branch_already_exists" }

3. Branch Manager
   │
   ├── Opens PostgreSQL transaction
   │     INSERT INTO branches (name, repo_id, commit_id, created_at)
   │     VALUES ('suggest-alice-20240405', 'repo_doc_abc123', 'commit_v2', NOW())
   └── Commits transaction

4. API Layer
   │
   └── Returns 201 Created
         {
           "name": "suggest-alice-20240405",
           "repo_id": "repo_doc_abc123",
           "commit_id": "commit_v2",
           "created_at": "2024-04-05T10:10:00Z"
         }
```

**State after this flow:**

| Table      | Change                                                    |
| ---------- | --------------------------------------------------------- |
| `branches` | New row: `suggest-alice-20240405` pointing at `commit_v2` |

Both `main` and `suggest-alice-20240405` point to `commit_v2` - they share history up to this point. No data is copied. Creating a branch is a single row insert.

---

## Flow 5 - Commit to Suggest Branch

**Trigger:** Alice makes edits in suggest mode and saves. The product commits to the suggest branch, then advances it. Identical to Flows 2+3 but targeting the suggest branch.

**Step 5a - Create commit (same as Flow 2)**

```http
POST /repos/repo_doc_abc123/commits
{
  "parent_ids": ["commit_v2"],
  "expected_head": "commit_v2",
  "data_pointer": {
    "type": "db",
    "location": "documents/snapshots/doc_abc123/suggest_v_1712346000",
    "hash": "sha256:b8f2d1a..."
  },
  "message": "Suggested: expanded introduction paragraph",
  "author": "user_alice@company.com",
  "idempotency_key": "uuid-alice-suggest-001"
}
```

Internally identical to Flow 2. Creates `commit_v3`, parent = `commit_v2`. Outbox event written. Returns `commit_v3`.

**Step 5b - Advance suggest branch (same as Flow 3)**

```http
PATCH /repos/repo_doc_abc123/branches/suggest-alice-20240405
{
  "commit_id": "commit_v3",
  "expected_commit_id": "commit_v2"
}
```

The optimistic lock targets `suggest-alice-20240405`. The `main` branch is completely untouched - it still points to `commit_v2`.

**DAG state after this flow:**

```
commit_v1 ←── commit_v2 ←── commit_v3   ← suggest-alice-20240405 head
                  ▲
                  └── main head (still at commit_v2)
```

---

## Flow 6 - Checkout (document opened)

**Trigger:** Bob opens the document. Google Docs needs the current document state. Two sequential calls: get the branch head, then get the commit to extract the DataPointer.

### Step 6a - Get branch head

**Request:**

```http
GET /repos/repo_doc_abc123/branches?name=main
```

```
1. API Layer → Branch Manager
   │
   ├── Check Redis cache first (if active):
   │     GET branch:repo_doc_abc123:main
   │     → cache hit:  return commit_id immediately - no DB query
   │     → cache miss: proceed to PostgreSQL
   │
   ├── Query PostgreSQL:
   │     SELECT commit_id FROM branches
   │     WHERE name = 'main' AND repo_id = 'repo_doc_abc123'
   │     → not found: return 404
   │     → found: commit_id = 'commit_v2'
   │
   ├── Populate Redis on cache miss:
   │     SET branch:repo_doc_abc123:main 'commit_v2' EX 60
   │
   └── Returns 200 OK { "name": "main", "commit_id": "commit_v2", ... }
```

### Step 6b - Get commit

**Request:**

```http
GET /repos/repo_doc_abc123/commits/commit_v2
```

```
1. API Layer → Commit Engine
   │
   ├── SELECT * FROM commits
   │   WHERE id = 'commit_v2' AND repo_id = 'repo_doc_abc123'
   │   → not found: return 404
   │
   └── Returns 200 OK with full commit including data_pointer
```

After this flow, Google Docs has the DataPointer. It extracts `location` and loads the document snapshot directly from its own DB. Verge is no longer in the path.

---

## Flow 7 - History Query

**Trigger:** Bob opens the version history panel. Google Docs needs a paginated list of all saved versions, traversing the DAG from the branch head.

**Request from Google Docs:**

```http
GET /repos/repo_doc_abc123/commits?branch=main&traversal=dag&limit=20
```

### Step-by-step through Verge

```
1. API Layer
   │
   ├── Validates:
   │     ✓ traversal=dag requires branch param - returns 400 if missing
   │     ✓ limit is integer 1–100
   │     ✓ since/until are valid ISO 8601 if present
   └── Passes to Query & History Engine

2. Query & History Engine
   │
   ├── Resolve branch head:
   │     SELECT commit_id FROM branches
   │     WHERE name = 'main' AND repo_id = 'repo_doc_abc123'
   │     → commit_v2
   │
   ├── Routes to active GraphStore backend:
   │
   │   ── PostgreSQL path (Tier 1 / Tier 2): ──────────────────────────────
   │   │
   │   │   WITH RECURSIVE history AS (
   │   │     SELECT c.id, c.message, c.author, c.timestamp, c.data_pointer
   │   │     FROM commits c
   │   │     WHERE c.id = 'commit_v2' AND c.repo_id = 'repo_doc_abc123'
   │   │
   │   │     UNION ALL
   │   │
   │   │     SELECT c.id, c.message, c.author, c.timestamp, c.data_pointer
   │   │     FROM commits c
   │   │     INNER JOIN commit_parents cp ON c.id = cp.parent_id
   │   │     INNER JOIN history h ON cp.commit_id = h.id
   │   │   )
   │   │   SELECT * FROM history
   │   │   ORDER BY timestamp DESC
   │   │   LIMIT 21      ← fetch one extra to determine if next page exists
   │   │
   │   └── ── Neo4j path (Tier 3): ──────────────────────────────────────
   │         MATCH (start:Commit {id: 'commit_v2'})-[:PARENT_OF*0..]->(ancestor)
   │         WHERE ancestor.repo_id = 'repo_doc_abc123'
   │         RETURN ancestor ORDER BY ancestor.timestamp DESC LIMIT 21
   │
   ├── Determine pagination:
   │     result count = 21 → next page exists
   │       next_cursor = result[20].id
   │       return first 20 items only
   │     result count ≤ 20 → last page
   │       next_cursor = null
   │
   └── Passes to API Layer

3. API Layer
   │
   └── Returns 200 OK
         {
           "commits": [
             { "id": "commit_v2", "message": "Added executive summary", ... },
             { "id": "commit_v1", "message": "Initial version", ... }
           ],
           "next_cursor": null
         }
```

Google Docs renders the history panel using `message`, `author`, and `timestamp` directly from the response. If the user hovers to preview a diff, Google Docs loads the relevant snapshots using `data_pointer` and runs its own diff - Verge is not called again.

---

## Flow 8 - Merge

**Trigger:** A reviewer accepts Alice's suggested edits. Google Docs has already run its merge algorithm, stored the merged snapshot, and constructed a DataPointer. It calls Verge to record the merge commit and advance `main` atomically.

**State going in:**

- `main` head = `commit_v2`
- `suggest-alice-20240405` head = `commit_v3`
- Merged snapshot stored at `documents/snapshots/doc_abc123/merged_v_1712347000`

**Request from Google Docs:**

```http
POST /repos/repo_doc_abc123/merges
{
  "parent_ids": ["commit_v3", "commit_v2"],
  "expected_target_head": "commit_v2",
  "target_branch": "main",
  "data_pointer": {
    "type": "db",
    "location": "documents/snapshots/doc_abc123/merged_v_1712347000",
    "hash": "sha256:c9e3f7b..."
  },
  "message": "Accepted Alice's suggested edits - expanded introduction",
  "author": "user_bob@company.com"
}
```

### Step-by-step through Verge

```
1. API Layer
   │
   ├── Validates:
   │     ✓ parent_ids has exactly two entries - rejects with 400 if not
   │     ✓ expected_target_head present
   │     ✓ target_branch present
   │     ✓ data_pointer structure valid
   └── Passes to Consistency & Validation Layer

2. Consistency & Validation Layer
   │
   ├── Validates repo exists                                     ✓
   │
   ├── Validates both parent commits exist in this repo:
   │     SELECT id FROM commits
   │     WHERE id IN ('commit_v3', 'commit_v2')
   │       AND repo_id = 'repo_doc_abc123'
   │     → must return exactly 2 rows
   │     → fewer: return 422 { "error": "invalid_parent" }
   │
   ├── Validates target_branch exists and reads current head:
   │     SELECT commit_id FROM branches
   │     WHERE name = 'main' AND repo_id = 'repo_doc_abc123'
   │     → not found: return 404 { "error": "branch_not_found" }
   │     → found: current_head = 'commit_v2'
   │
   ├── Validates expected_target_head matches current head:
   │     expected = 'commit_v2', actual = 'commit_v2'  ✓
   │     → mismatch: return 409 {
   │         "error": "stale_merge_target",
   │         "message": "Branch 'main' is at 'commit_v3' but expected 'commit_v2'...",
   │         "current_head": "commit_v3"
   │       }
   │
   └── Passes to Commit Engine with verified current_head = 'commit_v2'

3. Commit Engine
   │
   ├── Generates: commit_v4 (the merge commit)
   │
   ├── Opens PostgreSQL transaction:
   │
   │   a. INSERT INTO commits → commit_v4
   │
   │   b. INSERT INTO commit_parents (two rows):
   │        (commit_v4, commit_v3)   ← source branch head
   │        (commit_v4, commit_v2)   ← target branch head at merge time
   │
   │   c. Optimistic lock UPDATE on main:
   │        UPDATE branches
   │        SET commit_id = 'commit_v4'
   │        WHERE name = 'main'
   │          AND repo_id = 'repo_doc_abc123'
   │          AND commit_id = 'commit_v2'
   │        RETURNING id
   │
   │        → 0 rows: main moved concurrently → ROLLBACK → return 409
   │        → 1 row: proceed
   │
   │   d. INSERT INTO outbox_events:
   │        { event_type: 'commit.created', is_merge: true,
   │          commit_id: 'commit_v4', branch: 'main',
   │          parent_ids: ['commit_v3', 'commit_v2'] }
   │
   └── COMMIT transaction

4. API Layer
   │
   └── Returns 201 Created
         {
           "id": "commit_v4",
           "repo_id": "repo_doc_abc123",
           "parent_ids": ["commit_v3", "commit_v2"],
           "data_pointer": { ... },
           "message": "Accepted Alice's suggested edits - expanded introduction",
           "author": "user_bob@company.com",
           "timestamp": "2024-04-05T11:00:00Z"
         }

5. Async Workers:
   ├── Neo4j:
   │     MERGE (c:Commit {id: 'commit_v4'})
   │     MERGE (p1:Commit {id: 'commit_v3'})
   │     MERGE (p2:Commit {id: 'commit_v2'})
   │     MERGE (c)-[:PARENT_OF]->(p1)
   │     MERGE (c)-[:PARENT_OF]->(p2)
   │
   └── Redis:
         DEL branch:repo_doc_abc123:main
         (suggest branch cache untouched - that branch head has not changed)
```

**DAG state after this flow:**

```
commit_v1 ←── commit_v2 ←────────────── commit_v4   ← main (merge commit)
                  └──── commit_v3 ───────────┘
                        ↑                            (two PARENT_OF edges from commit_v4)
              suggest-alice head (unchanged)
```

The suggest branch `suggest-alice-20240405` still points to `commit_v3`. Google Docs can now delete it via `DELETE /repos/repo_doc_abc123/branches/suggest-alice-20240405`.

---

## Flow 9 - Restore

**Trigger:** Bob decides `commit_v4` (the merge) was a mistake and wants to restore `commit_v1`. Verge history is append-only - restore means creating a new commit that reuses the old DataPointer, not deleting anything.

### Step 9a - Fetch the old commit

```http
GET /repos/repo_doc_abc123/commits/commit_v1
```

```
Commit Engine:
  SELECT * FROM commits
  WHERE id = 'commit_v1' AND repo_id = 'repo_doc_abc123'
  → returns commit_v1 with its data_pointer
```

Google Docs extracts `data_pointer.location`, loads the snapshot from its own DB, and sets it as the current document state. The snapshot already exists - no data is copied.

### Step 9b - Commit the restored state

```http
POST /repos/repo_doc_abc123/commits
{
  "parent_ids": ["commit_v4"],
  "expected_head": "commit_v4",
  "data_pointer": {
    "type": "db",
    "location": "documents/snapshots/doc_abc123/v_initial",
    "hash": "sha256:original_hash..."
  },
  "message": "Restored to version from 2024-04-05T09:00:00Z (before merge)",
  "author": "user_bob@company.com",
  "idempotency_key": "uuid-bob-restore-001"
}
```

Internally identical to Flow 2. Creates `commit_v5`, parent = `commit_v4`. The `data_pointer` is the same as `commit_v1` - Verge stores it again without knowing or caring it's a repeat.

### Step 9c - Advance branch to restore commit

```http
PATCH /repos/repo_doc_abc123/branches/main
{
  "commit_id": "commit_v5",
  "expected_commit_id": "commit_v4"
}
```

**DAG state after restore:**

```
commit_v1 ←── commit_v2 ←── commit_v4 ←── commit_v5   ← main (restore commit)
                  └── commit_v3 ──────┘
                                          data_pointer of v5 = data_pointer of v1
```

The audit trail shows: initial → executive summary → merge(suggest+main) → restored to initial. Nothing was deleted or rewritten.

---

## Error Cases

### Concurrent commit conflict (409)

Two users save at the exact same moment. Both try to advance `main` from `commit_v2`.

```
User A PATCH: UPDATE branches SET commit_id='commit_vA' WHERE commit_id='commit_v2' → 1 row ✓
User B PATCH: UPDATE branches SET commit_id='commit_vB' WHERE commit_id='commit_v2' → 0 rows ✗
              → 409 {
                  "error": "branch_conflict",
                  "message": "Branch 'main' has advanced. Fetch latest head and retry.",
                  "current_head": "commit_vA"
                }
```

Google Docs retries: read `current_head` from the error response, use it as the new `expected_commit_id`, resubmit.

### Parent not found (422)

Google Docs sends a `parent_id` that doesn't exist in this repo.

```
SELECT id FROM commits WHERE id = 'commit_bad' AND repo_id = 'repo_doc_abc123'
→ 0 rows
→ 422 { "error": "invalid_parent", "message": "Parent 'commit_bad' does not exist in repo 'repo_doc_abc123'." }
```

### Stale merge target (409)

`main` has advanced between the time Google Docs read the branch heads and when it calls `/merges`.

```
expected_target_head = 'commit_v2'
actual main head     = 'commit_v3'   ← moved
→ 409 {
    "error": "stale_merge_target",
    "message": "Branch 'main' is at 'commit_v3' but expected 'commit_v2'. Fetch latest heads and recompute merge.",
    "current_head": "commit_v3"
  }
```

### Idempotency key match (200 instead of 201)

Google Docs retries a commit that already succeeded. The idempotency key matches.

```
SELECT id FROM commits
WHERE repo_id = 'repo_doc_abc123' AND idempotency_key = 'uuid-alice-save-001'
→ found: commit_v2
→ 200 OK with commit_v2 (no new commit created, no duplicate)
```

### Invalid DataPointer type (400)

```
data_pointer.type = "ftp"   ← not a valid enum value
→ 400 { "error": "invalid_request", "message": "data_pointer.type must be one of: s3, url, db, custom." }
```

### Two parent_ids sent to /commits instead of /merges (400)

```
POST /repos/repo_doc_abc123/commits with parent_ids: ["a", "b"]
→ 400 {
    "error": "invalid_request",
    "message": "Commits accept zero or one parent_ids. For merge commits with two parents, use POST /repos/:repo_id/merges."
  }
```
