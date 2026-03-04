# Hook Integrations

Converge can auto-snapshot agent or git lifecycle events.

## Install managed hooks

```bash
converge hooks install-git
converge hooks install-claude
```

Or run both:

```bash
converge hooks install
```

These install:

- managed git `post-commit` wrapper with preserved existing-hook chaining,
- missing project hook scripts under `converge_scripts/` (`converge-post-commit-hook.sh` and `claude-post-response-hook.sh`),
- Claude Code hook entries in `.claude/settings.local.json` for `Stop` and `SessionEnd`.

## Manual agent completion hook

```bash
converge hook complete --run-id <id> --agent <name> -m "summary" --tags auto --json
```

Behavior:

- idempotent on `run_id`,
- unchanged trees return `no_change`,
- changed trees create one cell and return `created`.

## Codex wrapper

Use wrapper script for post-response hook calls:

```bash
scripts/codex-exec-with-hook.sh <codex exec args...>
```

Defaults:

- `agent=codex`
- `tags=auto,codex`
- `source=agent_complete`

Enable strict mode if desired:

```bash
CONVERGE_HOOK_STRICT=1 scripts/codex-exec-with-hook.sh <args...>
```

## Claude hook script

Converge installs:

- `converge_scripts/claude-post-response-hook.sh`

It reads payload JSON from stdin, computes stable run ids, sanitizes summaries, and calls `converge hook complete`.

Hook failures are logged at:

- `.converge/hooks/claude-post-response-hook.log`

## Git commit archive rotation

After `converge hooks install-git` (or `converge hooks install`), successful commits rotate active `.converge` state into archive storage and create a fresh baseline cell from tracked files at `HEAD`.

Useful commands:

```bash
converge archives
converge hook git-commit --sha <sha> --branch <branch> --subject <subject>
```
