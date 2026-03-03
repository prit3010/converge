# Converge Architecture

This document explains how Converge stores experiment history, how commands flow through the system, and where to extend behavior safely.

## Design Goals

- Local-first operation (no required cloud service).
- Reproducible experiment cells.
- Safe restore and branch switching.
- Simple, explicit package boundaries.

## High-Level Topology

```text
CLI (cobra commands)
  -> core.Service (domain orchestration)
     -> snapshot.Snapshot (walk workspace + build manifest)
     -> store.Store (content-addressed blobs on disk)
     -> db.DB (SQLite metadata + manifest index)
     -> eval.Runner (best-effort quality checks)

Optional surfaces:
- watch.Watch (debounced auto-snap)
- hook handlers (agent completion + git commit archive rotation)
- ui.Server (local dashboard + JSON APIs)
- llm.Comparer (semantic compare via OpenAI)
```

## Core Packages

| Package | Responsibility |
| --- | --- |
| `cmd/converge` | Process entrypoint; calls CLI root execute. |
| `internal/cli` | Command wiring, flags, output formatting, hook installers. |
| `internal/core` | Domain workflows: create cell, restore, branch/fork/switch, archive rotation. |
| `internal/snapshot` | Captures manifest `path -> {hash, mode, size}` from working tree using policy rules. |
| `internal/store` | Content-addressed object store (`sha256` hash to immutable blob). |
| `internal/db` | SQLite schema, migrations, query/update methods, sequence allocator. |
| `internal/eval` | Best-effort tests/lint/type checks (detected or policy-driven). |
| `internal/watch` | FS watcher + debounce for auto capture. |
| `internal/ui` | Embedded static UI and HTTP API endpoints for browsing cells/diffs. |
| `internal/llm` | Semantic cell comparison using LLM prompt built from manifests and diffs. |

## Data Model (SQLite)

Converge persists structured metadata in `.converge/converge.db`.

- `cells`: one row per experiment snapshot.
  - Includes lineage (`parent_id`), branch, message/source/agent/tags, diff stats, LOC stats, eval fields.
- `manifest_entries`: `(cell_id, path, hash, mode, size)`.
  - Maps each tracked file in a cell to a blob hash.
- `branches`: named branch heads (`name -> head_cell_id`).
- `meta`: singleton metadata (`active_branch`, `head_cell`).
- `cell_sequences`: monotonic allocator backing `c_000001` ids.
- `agent_runs`: idempotency + outcome tracking for `hook complete` events.

## On-Disk Layout

```text
.converge/
  converge.db
  objects/
    ab/
      <sha256>
  archives/
    a_YYYYMMDDTHHMMSSZ_<sha8>/
      converge.db
      objects/
      meta.json
  restore.lock
  archive.lock
```

Notes:

- Objects are deduplicated by content hash.
- Archive directories are immutable snapshots of previous active state, usually created on git commits.
- Lock files are used to avoid watcher-trigger loops during restore/archive flows.

## Key Runtime Flows

### 1) Manual snapshot (`converge snap`)

1. CLI opens `core.Service` with repo policy.
2. `snapshot.Capture` walks project files (respecting ignore/binary/size policy).
3. Each file body is written to `store.Store` and hashed.
4. `core.Service` computes delta/LOC stats vs branch head manifest.
5. DB transaction inserts `cells` + `manifest_entries` and advances branch head.

### 2) Restore (`converge restore <cell>`)

1. Validate target cell exists.
2. Create safety snapshot (`source=restore_safety`).
3. Write `restore.lock`.
4. Materialize target manifest files from object store.
5. Remove tracked files that existed in current head but not in target manifest.
6. Update active branch head to target cell and remove lock.

### 3) Agent completion hook (`converge hook complete`)

1. Validate `run-id`, `agent`, `message`.
2. Reserve `agent_runs` row for idempotency.
3. Attempt `CreateCellIfChanged`.
4. Finalize run status as `created`, `no_change`, `duplicate`, or `failed`.

### 4) Git commit rotation (`converge hook git-commit`)

1. Acquire archive lock.
2. Move active DB/objects into timestamped archive directory.
3. Start fresh active DB/objects.
4. Capture new baseline from `git ls-files` tracked files at current `HEAD`.

### 5) Semantic compare (`converge compare A B`)

1. Load manifests for A and B.
2. Build file-level diff and bounded patch context.
3. Send prompt to OpenAI model (requires `OPENAI_API_KEY`).
4. Parse summary/winner/highlights from model output.

## Extension Points

- Repo policy: `.converge/config.toml` controls snapshot/eval behavior.
- Ignore rules: `.convergeignore` controls tracked file inclusion.
- Evaluation commands: override default detection with explicit `tests/lint/types` commands.
- Harness integrations: call `converge hook complete` from Claude/Codex/other automation surfaces.

## Safety Invariants

- Restores and branch switches create safety snapshots before modifying files.
- Restore/archive lock files suppress watch-loop feedback during destructive operations.
- `hook complete` is idempotent on `run-id`.
- Object store is content-addressed and deduplicated.

## Non-Goals

- Remote telemetry or cloud persistence.
- Hidden background mutation outside explicit commands/hooks.

