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
- `converge snap -m "..." [--tags] [--agent] [--eval=true|false]`
- `converge eval <cell>`
- `converge log [--limit 20] [--branch <name>] [--all]`
- `converge status`
- `converge diff <cellA> <cellB>`
- `converge restore <cell>`
- `converge watch [--debounce 3s]`
- `converge fork <name> [--switch]`
- `converge switch <branch>`
- `converge branches`
- `converge compare <cellA> <cellB> [--model <name>] [--max-diff-lines <n>]`
- `converge hook complete --run-id <id> --agent <name> -m <message> [--tags <csv>] [--eval] [--json]`
- `converge ui [--addr 127.0.0.1:7777]`

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
- Calls `converge hook complete --agent claude ...`.
- Exits `0` (non-blocking) and logs failures to `.converge/hooks/claude-post-response-hook.log`.

Troubleshooting:

- If hooks do not create cells, run `converge hook complete --json ...` manually and check returned `status`/`error`.
- For Claude hook failures, inspect `.converge/hooks/claude-post-response-hook.log`.
- For wrapper mode, enable `CONVERGE_HOOK_STRICT=1` to fail fast on integration issues.

## Storage

Converge stores local state in `.converge/`:

- `converge.db` (SQLite metadata + manifest entries)
- `objects/` (SHA256-addressed raw file blobs)

No cloud services are used.
