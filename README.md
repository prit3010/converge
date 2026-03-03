# Converge

Converge is a local-first experiment tracker for AI coding.
It captures each attempt as a reproducible **cell** so you can compare outcomes, branch ideas, and restore known-good states without noisy WIP commits.

## What You Get

- Reproducible cells: snapshot manifest + metadata + eval results.
- Local diffs and semantic compare between two cells.
- Safe restore flow with an automatic safety snapshot.
- Branch-style experimentation on top of local history.
- Agent and git hook integrations for automatic capture.

## Install

### Option 1 (Primary): Homebrew (macOS)

```bash
brew tap prit3010/converge
brew install --cask converge
converge version
```

### Option 2: Go install (macOS/Linux)

```bash
go install github.com/prit3010/converge/cmd/converge@latest
converge version
```

### Option 3: Build from source

```bash
git clone git@github.com:prit3010/converge.git
cd converge
go build -o converge ./cmd/converge
./converge version
```

## 2-Minute Setup

```bash
# initialize tracking in your repo
converge init

# baseline snapshot
converge snap -m "baseline" --eval=false

# make edits, then capture another attempt
converge snap -m "attempt 2" --eval=false

# inspect + diff
converge log
converge diff c_000001 c_000002
```

Optional semantic compare:

```bash
export OPENAI_API_KEY=sk-...
converge compare c_000001 c_000002
```

## Setup Agent Harnesses (Claude + Codex)

Converge can auto-capture snapshots when your coding harness finishes.

### Claude Code

From your project root:

```bash
converge hooks install
```

This installs:

- a managed `.git/hooks/post-commit` wrapper (preserves an existing `post-commit` hook),
- Claude hooks in `.claude/settings.local.json` for `Stop` and `SessionEnd`,
- command permissions needed for the hook script to call `converge hook complete`.

### Codex

Use the included wrapper script instead of calling `codex exec` directly:

```bash
scripts/codex-exec-with-hook.sh "implement feature X"
```

Or with normal `codex exec` flags:

```bash
scripts/codex-exec-with-hook.sh --model gpt-5 "fix flaky tests"
```

Useful environment overrides:

- `CONVERGE_HOOK_AGENT` (default: `codex`)
- `CONVERGE_HOOK_TAGS` (default: `auto,codex`)
- `CONVERGE_HOOK_EVAL=1` to run eval on created cells
- `CONVERGE_HOOK_STRICT=1` to fail the wrapper if hook capture fails

### Other harnesses

Any harness can integrate by calling:

```bash
converge hook complete --run-id <unique-id> --agent <name> -m "<summary>" --tags auto --json
```

`run-id` is idempotent: repeating the same `run-id` returns `duplicate` and does not create a second cell.

## Deployment Tags (Release Trigger)

Releases are tag-driven.

- GitHub Actions release workflow runs on tags matching `v*`.
- Typical production tag format: `vMAJOR.MINOR.PATCH` (example: `v0.2.0`).
- Tag push runs tests/build, then GoReleaser publishes release artifacts + checksums and updates the Homebrew Cask tap.

Cut a release:

```bash
git checkout main
git pull --ff-only
go test ./...
go build ./...

git tag -a v0.2.0 -m "v0.2.0"
git push origin v0.2.0
```

Release automation expects `HOMEBREW_TAP_GITHUB_TOKEN` in GitHub repo secrets.

## Command Cheat Sheet

| Command | Purpose |
| --- | --- |
| `converge init` | Initialize `.converge` state in current repo |
| `converge snap -m "..."` | Create a new cell from working tree |
| `converge status` | Show delta from branch head cell |
| `converge log [--branch <name>]` | List cell history |
| `converge diff <cellA> <cellB>` | Show file/line differences |
| `converge compare <cellA> <cellB>` | Generate AI semantic summary |
| `converge restore <cell>` | Restore tracked files to a cell state |
| `converge fork <name> --switch` | Create/switch to branch for a new attempt |
| `converge switch <name>` | Switch branches and restore branch head |
| `converge branches` | List branches and heads |
| `converge hooks install` | Install managed git + Claude hooks |
| `converge ui` | Start local dashboard |

## Storage Layout

Converge stores local state in `.converge/`:

- `converge.db`: SQLite metadata (`cells`, manifests, branches, runs)
- `objects/`: content-addressed blobs (`sha256 -> file bytes`)
- `archives/`: archived state packs created by git-commit rotation

No cloud dependency is required.

## Documentation

- Install guide: [docs/install.md](docs/install.md)
- End-to-end quickstart: [docs/quickstart.md](docs/quickstart.md)
- Hook integrations: [docs/hooks.md](docs/hooks.md)
- Release process: [docs/releasing.md](docs/releasing.md)
- Architecture guide: [docs/architecture.md](docs/architecture.md)

## License

MIT (see [LICENSE](LICENSE)).
