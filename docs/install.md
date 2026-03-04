# Install Converge

## Option 1 (Primary): Homebrew Formula on macOS

```bash
brew tap prit3010/converge
brew install converge
```

Equivalent explicit form:

```bash
brew install prit3010/converge/converge
```

Verify:

```bash
converge version
```

If you previously installed the old cask build, remove it first:

```bash
brew uninstall --cask converge
brew install converge
```

## Option 2: Go Install (macOS/Linux)

Requires Go 1.22+ in your `PATH`.

```bash
go install github.com/prit3010/converge/cmd/converge@latest
converge version
```

If `converge` is not found after install, add your Go bin directory to `PATH`:

```bash
# zsh
export PATH="$(go env GOPATH)/bin:$PATH"
```

## Option 3: Build from Source

```bash
git clone git@github.com:prit3010/converge.git
cd converge
go build -o converge ./cmd/converge
./converge version
```

## First Run

In any repository you want to track:

```bash
converge init
converge snap -m "baseline" --eval=false
converge log
```

## Upgrade

Homebrew:

```bash
brew update
brew upgrade converge
```

Go install:

```bash
go install github.com/prit3010/converge/cmd/converge@latest
```

## Uninstall

Homebrew:

```bash
brew uninstall converge
brew untap prit3010/converge
```

Go install:

```bash
rm -f "$(go env GOPATH)/bin/converge"
```
