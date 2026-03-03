# Converge

Converge is a local-first experiment tracker for AI-assisted coding workflows.
It snapshots your working tree into sequential cells, then lets you compare,
restore, and evaluate iterations without committing every prompt attempt.

## Quick Start

From the repository root:

    go build -o converge ./cmd/converge

In a project you want to track:

    /path/to/converge init
    /path/to/converge snap -m "baseline"
    /path/to/converge fork feature-a --switch
    # edit files
    /path/to/converge snap -m "attempt 2"
    /path/to/converge switch main
    /path/to/converge log
    /path/to/converge diff c_000001 c_000002
    /path/to/converge compare c_000001 c_000002
    /path/to/converge ui --addr 127.0.0.1:7777
    /path/to/converge restore c_000001

## Commands

- `converge init`
- `converge snap -m "..." [--tags] [--agent] [--eval=true|false] [--json]`
- `converge eval <cell> [--json]`
- `converge log [--limit 20] [--branch <name>] [--all] [--json]`
- `converge status [--json]`
- `converge diff <cellA> <cellB> [--json]`
- `converge restore <cell> [--json]`
- `converge watch [--debounce 3s]`
- `converge fork <name> [--switch]`
- `converge switch <branch>`
- `converge branches [--json]`
- `converge compare <cellA> <cellB> [--model <name>] [--max-diff-lines <n>] [--json]`
- `converge hook complete --run-id <id> --agent <name> -m <message> [--tags <csv>] [--eval] [--json]`
- `converge hook git-commit --sha <sha> --branch <branch> --subject <subject>`
- `converge hooks install`
- `converge archives`
- `converge ui [--addr 127.0.0.1:7777]`

## Automation JSON Contract

The following commands support a machine-readable envelope with `--json`:
`snap`, `eval`, `log`, `status`, `diff`, `restore`, `branches`, `compare`.

Success:

```json
{
  "ok": true,
  "command": "snap",
  "data": {},
  "meta": {
    "schema_version": "v1",
    "timestamp": "2026-03-02T15:04:05Z"
  }
}
```

Error:

```json
{
  "ok": false,
  "command": "snap",
  "error": {
    "code": "NOT_FOUND",
    "message": "cell c_000123 not found",
    "exit_code": 4
  },
  "meta": {
    "schema_version": "v1",
    "timestamp": "2026-03-02T15:04:05Z"
  }
}
```

In `--json` mode, the error envelope is printed to stdout and the process still exits non-zero.

Standardized error codes:

- `VALIDATION` (`2`)
- `NOT_INITIALIZED` (`3`)
- `NOT_FOUND` (`4`)
- `CONFLICT` (`5`)
- `EXTERNAL` (`6`)
- `INTERNAL` (`1`)

## Repository Policy Files

Converge reads optional repository policy from:

- `.converge/config.toml`
- `.convergeignore`

Example `.converge/config.toml`:

```toml
[snapshot]
ignore = ["pattern/**", "*.local"]
max_file_size = "10MB"
binary_policy = "skip" # skip | include | fail

[eval]
tests = ["go test ./..."]
lint = ["golangci-lint run ./..."]
types = ["npx tsc --noEmit"]
```

Behavior:

- If any `eval` command list is configured, Converge runs exactly configured commands and bypasses built-in language autodetection.
- `max_file_size` files are skipped (not failed) by default policy behavior in this release.
- Default binary policy is `skip`.
- `.converge/` internals are always ignored.
- `.convergeignore` uses gitignore-style patterns (comments, globs, negation, trailing `/`).

On `converge init`, Converge creates `.convergeignore` if missing with an aggressive default seed:

- secrets/env patterns (for example `.env`, `.env.*`, key/cert files)
- dependency/build/cache directories
- logs/local DB files
- common archive/media/generated artifact patterns

## OpenAI Setup for `compare`

`converge compare` uses OpenAI in P0.

    export OPENAI_API_KEY=sk-...
    /path/to/converge compare c_000001 c_000002

If `OPENAI_API_KEY` is missing, `compare` exits with an actionable error and does not modify repository state.

## Demo Flow

    /path/to/converge init
    /path/to/converge snap -m "baseline"
    /path/to/converge fork feature-a --switch
    # edit files
    /path/to/converge snap -m "feature-a attempt"
    /path/to/converge switch main
    /path/to/converge compare c_000001 c_000002
    /path/to/converge ui --addr 127.0.0.1:7777

## Agent Hook Integrations

Use `converge hook complete` as the single backend entrypoint for post-agent auto-snap integrations.

Behavior guarantees:

- `run_id` is idempotent. Replays return `duplicate`.
- Unchanged working trees return `no_change` and do not create a new cell.
- Changed trees create one new cell and return `created`.
- Runtime errors return `failed` with an `error` message.

### Codex (`codex exec`) wrapper

Use the repository wrapper script so the hook call always runs after Codex returns:

    /Users/prittamravi/converge/scripts/codex-exec-with-hook.sh <codex exec args...>

Default metadata:

- `agent=codex`
- `tags=auto,codex`
- `source=agent_complete` (backend default)

Optional strict mode:

    CONVERGE_HOOK_STRICT=1 /Users/prittamravi/converge/scripts/codex-exec-with-hook.sh <args...>

In strict mode, wrapper exits non-zero if the hook call fails.

### Claude Code hooks (`Stop` + `SessionEnd`)

Converge includes `/Users/prittamravi/converge/scripts/claude-post-response-hook.sh`, wired in `.claude/settings.local.json` for both `Stop` and `SessionEnd`.

The script:

- Reads hook payload JSON from stdin.
- Computes a stable `run_id` from `session_id + transcript_path + transcript tail hash` to dedupe events.
- Selects message text in this strict order:
  1. `last_assistant_message` (and compatible key variants) from payload.
  2. Payload text fields (`message`, `response`, `output_text`) when they are plain human text.
  3. Transcript fallback: scan the newest ~400 transcript lines and pick the latest assistant text block.
  4. Deterministic fallback: `Claude: <Event> completed`.
- Sanitizes message summaries before persisting:
  1. Normalizes whitespace and strips markdown noise from leading prefixes.
  2. Rejects structured payload-like strings (for example JSON blobs and env metadata such as `CONVERGE_*=`).
  3. Prefers the first meaningful sentence/line.
  4. Truncates to 160 characters.
  5. Always stores `Claude: <summary>`.
- Calls `converge hook complete --agent claude ...`.
- Exits `0` (non-blocking) and logs failures to `.converge/hooks/claude-post-response-hook.log`.

Observability notes:

- Dedupe behavior is unchanged (`Stop` + `SessionEnd` for the same run still resolve to one cell).
- On hook failure, log entries include `message_source=payload|transcript|fallback`.

Troubleshooting:

- If hooks do not create cells, run `converge hook complete --json ...` manually and check returned `status`/`error`.
- For Claude hook failures, inspect `.converge/hooks/claude-post-response-hook.log`.
- For wrapper mode, enable `CONVERGE_HOOK_STRICT=1` to fail fast on integration issues.

## Git Commit Archive Rotation

Install hook integrations once per repository:

    /path/to/converge hooks install

What this installs:

- Managed Git `post-commit` wrapper with existing-hook chaining.
- Claude Code `.claude/settings.local.json` hook entries for `Stop` and `SessionEnd`, pointing to `scripts/claude-post-response-hook.sh`.

Behavior on each successful Git commit:

- Current `.converge` lineage is archived to `.converge/archives/<archive_id>/` when active state is non-empty.
- Active `.converge` state is reset to a fresh slate.
- A new baseline cell is created from Git-tracked files at `HEAD` (eval disabled by default).
- Hook is strict: failures return non-zero and print a replay command.

Managed hook behavior:

- Preserves an existing `.git/hooks/post-commit` script as `.git/hooks/post-commit.user`.
- Executes preserved user hook and Converge hook in the managed wrapper.
- Reinstall is idempotent.

Useful commands:

    /path/to/converge archives
    /path/to/converge hook git-commit --sha <sha> --branch <branch> --subject <subject>

UI support:

- `/path/to/converge ui` shows an archive selector with `Current` plus archived graphs.
- Archived graphs are read-only and browsable in-place.
- Compare is archive-scoped; cross-archive compare is rejected in v1.

## Storage

Converge stores local state in `.converge/`:

- `converge.db` (SQLite metadata + manifest entries)
- `objects/` (SHA256-addressed raw file blobs)

No cloud services are used.
