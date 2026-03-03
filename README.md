# Converge

Converge is a local-first experiment tracker for AI-assisted coding.
It snapshots your repository into reproducible cells so you can compare attempts,
restore known-good states, and keep iteration history without noisy git commits.

## Why Converge

- Keep every coding attempt inspectable as a structured cell (snapshot + metadata + eval).
- Compare two attempts quickly with file-level diff plus optional AI semantic summary.
- Restore prior states safely while keeping everything local and offline-friendly.

## Install

Primary (macOS, Homebrew Cask):

```bash
brew tap prit3010/converge
brew install --cask converge
converge version
```

Secondary (Go toolchain):

```bash
go install github.com/prit3010/converge/cmd/converge@latest
converge version
```

From source:

```bash
git clone git@github.com:prit3010/converge.git
cd converge
go build -o converge ./cmd/converge
./converge version
```

More install paths and troubleshooting: [docs/install.md](docs/install.md).

## 2-Minute Quickstart

```bash
# in your project
converge init
converge snap -m "baseline" --eval=false

# make edits, then capture a second attempt
converge snap -m "attempt 2" --eval=false

# inspect and compare
converge log
converge diff c_000001 c_000002

# optional local UI
converge ui --addr 127.0.0.1:7777
```

For a full walkthrough including branching and restore: [docs/quickstart.md](docs/quickstart.md).

## Core Concepts

- `cell`: one captured experiment state (snapshot, metadata, stats, eval result).
- `branch`: named line of experimentation from a specific cell head.
- `restore`: materialize files from a previous cell (with safety snapshot behavior).
- `compare`: AI summary of semantic differences between two cells.

## Common Workflows

- Manual snapshots during iteration: `snap`, `status`, `log`.
- Branch and compare alternatives: `fork`, `switch`, `diff`, `compare`.
- Hook-based auto capture after agent or git events: `hooks install`, `hook complete`.

Hook integration details live in [docs/hooks.md](docs/hooks.md).

## Command Cheat Sheet

| Command | Purpose |
| --- | --- |
| `converge init` | Initialize `.converge` state in current repo |
| `converge snap -m "..."` | Create a new cell from working tree |
| `converge status` | Show delta from branch head cell |
| `converge log [--branch <name>]` | List cell history |
| `converge diff <cellA> <cellB>` | Show file-level and line-level differences |
| `converge compare <cellA> <cellB>` | Generate AI summary of semantic differences |
| `converge restore <cell>` | Restore tracked files to a cell state |
| `converge fork <name> --switch` | Create/switch to branch for new attempt |
| `converge branches` | List branches and heads |
| `converge ui` | Start local dashboard |
| `converge version` | Show version/build metadata |

## Configuration and `compare` Setup

Converge reads optional repository policy from:

- `.converge/config.toml`
- `.convergeignore`

Example config:

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

`compare` requires an OpenAI API key:

```bash
export OPENAI_API_KEY=sk-...
converge compare c_000001 c_000002
```

## Troubleshooting

- If `compare` fails immediately, verify `OPENAI_API_KEY` is set.
- If Homebrew cannot find `converge`, run `brew update` then retry `brew install --cask converge`.
- If Converge commands fail in a repo, run `converge init` first.
- If hooks do not create cells, use `converge hook complete --json ...` manually to inspect status.

## Development and Release

- Install and workflow docs: [docs/install.md](docs/install.md), [docs/quickstart.md](docs/quickstart.md)
- Hook integrations: [docs/hooks.md](docs/hooks.md)
- Release process and CI/CD: [docs/releasing.md](docs/releasing.md)

## Storage

Converge stores local state in `.converge/`:

- `converge.db` (SQLite metadata + manifest entries)
- `objects/` (SHA256-addressed raw file blobs)

No cloud services are required.
