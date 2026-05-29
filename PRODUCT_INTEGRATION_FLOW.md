# Verge - Integration Flow

This document walks through three integration scenarios from the **product's point of view**: what they need to build, what they hand off to Verge, and what they get back. Each product has different data formats, different merge complexity, and different scale characteristics. Verge's role is identical in all three cases: track the graph, store the pointer, stay out of the data.

> **Authentication:** If your Verge deployment has `VERGE_AUTH_ENABLED=true`, every API call from your backend must include the header `Authorization: Bearer <key>` (REST) or the gRPC metadata key `authorization: Bearer <key>`. The examples below omit this header for brevity; add it to every request in authenticated deployments.

---

## Table of Contents

- [Integration 1 - Google Docs (Collaborative Document Editor)](#integration-1-google-docs-collaborative-document-editor)
- [Integration 2 - Figma (Collaborative Design Tool)](#integration-2-figma-collaborative-design-tool)
- [Integration 3 - AI Workflow Builder](#integration-3-ai-workflow-builder)

---

## Integration 1 - Google Docs (Collaborative Document Editor)

### Context

A Google Docs-like product lets multiple users edit a shared document simultaneously. The document is rich text - paragraphs, headings, inline formatting, embedded images, comments.

### What the product owns

- **Document storage** - each saved version of the document is stored as a JSON snapshot in their database (e.g. PostgreSQL JSONB or a document store like MongoDB). The document format is their own schema, Verge never sees it.
- **Diff computation** - when showing a user what changed between version A and version B, the product computes the diff by loading both snapshots and running its own document diffing logic.
- **Merge logic** - when two branches of a document need to be merged (e.g. a suggested edit branch merging back into main), the product runs its own merge algorithm. Verge records the outcome.
- **Real-time sync** - the live collaborative editing layer (OT/CRDT) is entirely separate from Verge. Verge only gets involved when a version is explicitly saved or committed.

### What the product hands to Verge

Each document maps to one Verge repository. Every named save or auto-save checkpoint becomes a commit.

```
Verge repository  =  one document
Verge commit      =  one saved version of the document
Verge branch      =  one editing context (main, suggested-edits, etc.)
DataPointer       =  reference to the document snapshot in product storage
```

**DataPointer shape for this product:**

```json
{
  "type": "db",
  "location": "documents/snapshots/doc_abc123/v_1712345678",
  "hash": "sha256:a3f1c...",
  "metadata": {
    "word_count": 3420,
    "snapshot_format": "prosemirror-json-v2",
    "collaborators": ["user_1", "user_2"]
  }
}
```

The `location` is a row ID or path in the product's own snapshot storage. The `hash` is a SHA-256 of the
snapshot content, computed by the product before calling Verge, used for integrity verification on restore.
Verge stores this struct as an opaque blob - it never reads `snapshot_format` or `word_count`.

### What the product needs to build

**1. Snapshot writer**

Before calling Verge, the product must serialize the current document state into a stable, versioned
snapshot format and persist it to its own storage.

```
User clicks "Save version"
  │
  ├── Serialize current document to prosemirror-json-v2
  ├── Compute SHA-256 hash of the serialized bytes
  ├── Write snapshot to: documents/snapshots/{doc_id}/{timestamp}
  └── Construct DataPointer with location + hash
```

**2. Verge commit call**

After the snapshot is persisted, the product calls Verge. The commit is created first, then the branch
is advanced separately. `idempotency_key` should always be set on retry-prone operations - if the network
drops after Verge creates the commit but before the response arrives, retrying with the same key returns
the existing commit (`"existing": true`) instead of creating a duplicate.

```http
POST /v1/repos/repo_doc_abc123/commits
{
  "parent_ids": ["commit_v1"],
  "data_pointer": {
    "type": "db",
    "location": "documents/snapshots/doc_abc123/v_1712345678",
    "hash": "sha256:a3f1c...",
    "metadata": { "word_count": 3420 }
  },
  "message": "Version saved by Alice - added executive summary",
  "author": "user_alice@company.com",
  "idempotency_key": "uuid-alice-save-001"
}
```

Response `201 Created` (or `200 OK` with `"existing": true` on a retry):

```json
{
  "commit": {
    "id": "commit_v2",
    "repo_id": "repo_doc_abc123",
    "parent_ids": ["commit_v1"],
    "data_pointer": { "...": "..." },
    "message": "Version saved by Alice - added executive summary",
    "author": "user_alice@company.com",
    "timestamp": "2024-04-05T10:05:00Z"
  },
  "existing": false
}
```

After the commit is created, advance the branch:

```http
PATCH /v1/repos/repo_doc_abc123/branches/main
{
  "commit_id": "commit_v2",
  "expected_commit_id": "commit_v1"
}
```

**3. Version restore**

To restore a past version, the product fetches the old commit from Verge, extracts the DataPointer, loads
the snapshot, and commits it as a new commit pointing to the current head. History is never rewritten.

```
User clicks "Restore version X"
  │
  ├── GET /v1/repos/repo_doc_abc123/commits/commit_v1   ← fetch old commit
  ├── Extract data_pointer.location
  ├── Load snapshot from product storage                ← product's own DB
  ├── Replace current document state in memory
  ├── POST /v1/repos/repo_doc_abc123/commits            ← commit the restored state
  │     { parent_ids: [current_head], data_pointer: same as commit_v1, message: "Restored to v1" }
  └── PATCH /v1/repos/repo_doc_abc123/branches/main    ← advance branch to restore commit
```

**4. Branch for suggested edits**

When a user wants to make a tracked suggestion without affecting the main document:

```
User enables "Suggest mode"
  │
  ├── GET /v1/repos/repo_doc_abc123/branches/main  ← fetch current head
  └── POST /v1/repos/repo_doc_abc123/branches
        { "name": "suggest-alice-20240405", "source_commit_id": "commit_v2" }

Alice saves her edit
  │
  ├── POST /v1/repos/repo_doc_abc123/commits
  │     { parent_ids: ["commit_v2"], data_pointer: {...} }
  └── PATCH /v1/repos/repo_doc_abc123/branches/suggest-alice-20240405
        { commit_id: "commit_v3", expected_commit_id: "commit_v2" }

Alice finishes, reviewer approves → product calls POST /v1/repos/repo_doc_abc123/merges
```

**5. Diff display**

The product loads both commit snapshots from its own storage and computes the diff itself. Verge provides
the commit list and DataPointers only.

```
User opens version history panel
  │
  ├── GET /v1/repos/repo_doc_abc123/commits?branch=main&traversal=dag&limit=20
  │     ← Verge returns commit list with DataPointers
  ├── For each pair: load snapshot A and snapshot B from product storage
  └── Run product's own diff algorithm → render highlighted changes in UI
```

---

## Integration 2 - Figma (Collaborative Design Tool)

### Context

A Figma-like product manages design files. A file contains a tree of layers, frames, components, and
assets. The file format is complex, deeply nested, binary-heavy (images, fonts), and large. Multiple
designers work on the same file. The product needs branching so a designer can work on a redesign in
isolation, and merging so the redesign can be integrated back into the main file.

### What the product owns

- **File storage** - the design file is stored as a proprietary binary format in S3. Assets (images, fonts) are stored separately with content-addressable keys.
- **Layer tree diff** - comparing two versions of a design file to produce a visual diff is a hard problem the product owns entirely.
- **Merge logic** - when merging a branch back into main, the product must handle structural conflicts (two designers modified the same layer).
- **Asset deduplication** - the product handles deduplication of referenced assets. Verge only sees pointers.

### What the product hands to Verge

```
Verge repository  =  one design file
Verge commit      =  one saved version of the file
Verge branch      =  one design branch (main, redesign-v2, dark-mode-experiment)
DataPointer       =  reference to the .fig snapshot + asset manifest in S3
```

**DataPointer shape for this product:**

```json
{
  "type": "s3",
  "location": "s3://design-files/file_xyz/snapshots/1712345678.fig",
  "hash": "sha256:b9d2e...",
  "metadata": {
    "file_format_version": "fig-3.1",
    "asset_manifest": "s3://design-files/file_xyz/manifests/1712345678.json",
    "frame_count": 47,
    "page_count": 6,
    "size_bytes": 18700000
  }
}
```

### What the product needs to build

**1. File snapshot writer**

```
Designer hits Cmd+S or explicit "Save version"
  │
  ├── Serialize current file state to .fig binary
  ├── Upload .fig file to S3
  ├── Generate asset manifest and upload to S3
  ├── Compute SHA-256 of the .fig file
  └── Construct DataPointer with s3 location, hash, and metadata
```

**2. Verge commit call**

```http
POST /v1/repos/repo_file_xyz/commits
{
  "parent_ids": ["commit_prev_head"],
  "data_pointer": {
    "type": "s3",
    "location": "s3://design-files/file_xyz/snapshots/1712345678.fig",
    "hash": "sha256:b9d2e...",
    "metadata": {
      "file_format_version": "fig-3.1",
      "asset_manifest": "s3://design-files/file_xyz/manifests/1712345678.json",
      "frame_count": 47,
      "size_bytes": 18700000
    }
  },
  "message": "Redesigned onboarding flow - mobile breakpoints",
  "author": "designer_priya@company.com",
  "idempotency_key": "uuid-priya-save-002"
}
```

Then advance the branch:

```http
PATCH /v1/repos/repo_file_xyz/branches/main
{
  "commit_id": "commit_new",
  "expected_commit_id": "commit_prev_head"
}
```

**3. Branch creation for design branches**

```
Designer creates new branch in UI
  │
  └── POST /v1/repos/repo_file_xyz/branches
        { "name": "dark-mode-experiment", "source_commit_id": "main_head" }

Designer saves on the branch
  │
  ├── POST /v1/repos/repo_file_xyz/commits { parent_ids: ["main_head"], ... }
  └── PATCH /v1/repos/repo_file_xyz/branches/dark-mode-experiment
        { commit_id: "new_commit", expected_commit_id: "main_head" }
```

**4. Visual diff between versions**

```
Designer opens version history panel
  │
  ├── GET /v1/repos/repo_file_xyz/commits?branch=main&traversal=dag&limit=20
  ├── Designer selects two versions to compare
  ├── GET /v1/repos/repo_file_xyz/commits/commit_a   ← DataPointer for version A
  ├── GET /v1/repos/repo_file_xyz/commits/commit_b   ← DataPointer for version B
  ├── Download both .fig files from S3               ← product fetches from its own storage
  ├── Run product's layer-tree diff
  └── Render visual diff overlay in canvas
```

**5. Branch merge**

```
Designer requests branch merge into main
  │
  ├── GET /v1/repos/repo_file_xyz/branches/main             ← main head
  ├── GET /v1/repos/repo_file_xyz/branches/dark-mode-experiment  ← branch head
  ├── GET /v1/repos/repo_file_xyz/commits/{head_a}           ← DataPointer for branch head
  ├── GET /v1/repos/repo_file_xyz/commits/{head_b}           ← DataPointer for main head
  ├── Download both .fig files, run merge algorithm
  ├── Designer resolves conflicts in canvas
  ├── Upload merged .fig to S3, generate DataPointer
  └── POST /v1/repos/repo_file_xyz/merges
        {
          "parent_ids": ["dark_mode_head", "main_head"],
          "expected_target_head": "main_head",
          "target_branch": "main",
          "data_pointer": { merged file pointer },
          "message": "Merged dark mode experiment into main",
          "author": "designer_priya@company.com"
        }
```

**6. Garbage collection of old snapshots**

```
GC job (runs periodically)
  │
  ├── GET /v1/repos/repo_file_xyz/commits?traversal=dag&branch=main (paginated, all pages)
  ├── Collect all data_pointer.location values (S3 keys)
  ├── List all objects in S3 under design-files/{file_id}/snapshots/
  └── Delete any S3 object not referenced by any commit's DataPointer
```

---

## Integration 3 - AI Workflow Builder

### Context

An AI workflow product lets users build pipelines of AI operations - LLM calls, data transforms,
conditional logic, API calls. Each workflow is a JSON definition of nodes and edges. Users iterate
constantly: tweaking prompts, swapping models, adjusting parameters. They need to track what changed
between runs, branch to test a different prompt strategy, and restore a previous workflow state if a
change degrades performance.

### What the product owns

- **Workflow storage** - each workflow version is a JSON document stored in the product's database.
- **Execution engine** - running a workflow produces an execution record (inputs, outputs, latency, cost). Stored by the product, not Verge.
- **Diff computation** - comparing two workflow versions means comparing two JSON trees. Standard JSON diff.
- **Prompt evaluation** - determining whether a new version performs better is the product's domain.

### What the product hands to Verge

```
Verge repository  =  one workflow definition
Verge commit      =  one saved version of the workflow JSON
Verge branch      =  one experiment or deployment environment (main, experiment-gpt4o, staging, production)
DataPointer       =  reference to the workflow JSON in product's DB + execution linkage
```

**DataPointer shape for this product:**

```json
{
  "type": "db",
  "location": "workflows/versions/wf_123/v_1712345678",
  "hash": "sha256:c7a4f...",
  "metadata": {
    "node_count": 12,
    "model_providers": ["anthropic", "openai"],
    "trigger_type": "webhook",
    "linked_execution_id": "exec_9f3d2a"
  }
}
```

### What the product needs to build

**1. Workflow snapshot writer**

```
User saves workflow
  │
  ├── Canonicalize workflow JSON (sort keys, strip whitespace)
  ├── Compute SHA-256 of canonical JSON
  ├── Store version in product DB: workflows/versions/{wf_id}/{timestamp}
  └── Construct DataPointer with db location, hash, and metadata
```

**2. Verge commit call**

```http
POST /v1/repos/repo_wf_123/commits
{
  "parent_ids": ["commit_prev_head"],
  "data_pointer": {
    "type": "db",
    "location": "workflows/versions/wf_123/v_1712345678",
    "hash": "sha256:c7a4f...",
    "metadata": {
      "node_count": 12,
      "model_providers": ["anthropic", "openai"],
      "trigger_type": "webhook"
    }
  },
  "message": "Switched summarization node to Claude 3.5 Sonnet, adjusted temperature to 0.3",
  "author": "user_devraj@company.com",
  "idempotency_key": "uuid-devraj-save-003"
}
```

Then advance the branch:

```http
PATCH /v1/repos/repo_wf_123/branches/main
{
  "commit_id": "commit_new",
  "expected_commit_id": "commit_prev_head"
}
```

**3. Branching for experiments**

```
User creates experiment branch
  │
  └── POST /v1/repos/repo_wf_123/branches
        { "name": "experiment-claude-sonnet", "source_commit_id": "main_head" }

User commits to experiment branch
  │
  ├── POST /v1/repos/repo_wf_123/commits { parent_ids: ["main_head"], ... }
  └── PATCH /v1/repos/repo_wf_123/branches/experiment-claude-sonnet
        { commit_id: "exp_commit", expected_commit_id: "main_head" }

User runs experiment in staging, compares execution results
  → product's own evaluation logic - Verge not involved

Experiment wins → merge into main
  │
  └── POST /v1/repos/repo_wf_123/merges
        {
          "parent_ids": ["exp_head", "main_head"],
          "expected_target_head": "main_head",
          "target_branch": "main",
          "data_pointer": { merged workflow pointer },
          "message": "Promoted Claude Sonnet experiment to main"
        }
```

**4. Deployment environment tracking**

```
Environments as Verge branches:
  main        →  active development
  staging     →  validated and ready for review
  production  →  currently deployed

Deployment flow:
  POST /v1/repos/repo_wf_123/merges  { target_branch: "staging", ... }
  (run integration tests)
  POST /v1/repos/repo_wf_123/merges  { target_branch: "production", ... }

Production environment reads from production branch head:
  GET /v1/repos/repo_wf_123/branches/production  → gets commit_id
  GET /v1/repos/repo_wf_123/commits/{commit_id}  → gets DataPointer
```

**5. Rollback**

```
Bad version detected in production
  │
  ├── GET /v1/repos/repo_wf_123/commits?traversal=dag&branch=production
  │     ← find the last known good commit
  ├── Extract DataPointer from that commit
  └── POST /v1/repos/repo_wf_123/commits
        {
          parent_ids: [current_bad_head],
          data_pointer: { same pointer as last good commit },
          message: "Rollback to v12 - v13 caused 40% output degradation",
          author: "system"
        }
      then PATCH /v1/repos/repo_wf_123/branches/production
        { commit_id: rollback_commit, expected_commit_id: current_bad_head }
```

History is intact - the audit trail shows exactly what happened and why, without rewriting anything.

**6. Execution linking**

```
Workflow executed
  │
  ├── GET /v1/repos/repo_wf_123/branches/production  ← get current commit_id
  └── Record in product DB: { commit_id, execution_id, inputs, outputs, latency }
        (commit_id links the execution to its exact workflow version - Verge not involved)
```

---
