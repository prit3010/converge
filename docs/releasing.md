# Releasing Converge

This repository uses GitHub Actions + GoReleaser for tagged releases.

## CI behavior

Workflow: `.github/workflows/ci.yml`

On pushes to `main` and on PRs, CI runs:

- `gofmt -l` enforcement
- `go test ./...`
- `go vet ./...`
- `go build ./...`
- `go test -race ./...` (Linux)

Recommended branch protection required checks:

- `fmt`
- `test`
- `vet-build`
- `race`

## Release behavior

Workflow: `.github/workflows/release.yml`

Trigger:

- tags matching `v*` (for example `v0.1.0`)

Release workflow:

1. checks out code,
2. sets up Go,
3. re-runs tests/build,
4. runs GoReleaser,
5. publishes release archives + checksums,
6. updates Homebrew Formula in `prit3010/homebrew-converge`.

## Required GitHub secret

In `prit3010/converge` repository settings, add:

- `HOMEBREW_TAP_GITHUB_TOKEN`

Use a fine-grained PAT with write access to `prit3010/homebrew-converge`.

## One-time tap setup

Create public repo:

- `prit3010/homebrew-converge`

Recommended minimal README in tap repo:

```md
# homebrew-converge

brew tap prit3010/converge
brew install converge
```

GoReleaser will manage Formula updates in that repository.

## Cut a release

```bash
git checkout main
git pull --ff-only
go test ./...
go build ./...
git tag -a v0.1.0 -m "v0.1.0"
git push origin v0.1.0
```

Then verify:

- release artifacts present in GitHub Releases,
- `checksums.txt` present,
- tap repo receives updated Formula,
- fresh install works:

```bash
brew tap prit3010/converge
brew install converge
converge version
```

## Optional local dry-run

If `goreleaser` is installed locally:

```bash
goreleaser release --snapshot --clean
```
