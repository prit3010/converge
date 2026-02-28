#!/usr/bin/env bash
set -euo pipefail

# Usage:
#   bash test_converge_mvp.sh [/Users/prittamravi/converge]
REPO="${1:-/Users/prittamravi/converge}"

GO_BIN="${GO_BIN:-go}"
if ! command -v "$GO_BIN" >/dev/null 2>&1; then
  if [[ -x /usr/local/go/bin/go ]]; then
    GO_BIN=/usr/local/go/bin/go
  elif [[ -x /opt/homebrew/bin/go ]]; then
    GO_BIN=/opt/homebrew/bin/go
  else
    echo "ERROR: Go toolchain not found (set GO_BIN or install go)." >&2
    exit 1
  fi
fi

export PATH="$(dirname "$GO_BIN"):$PATH"

fail() {
  echo "❌ FAIL: $*" >&2
  exit 1
}

assert_contains() {
  local haystack="$1"
  local needle="$2"
  local msg="$3"
  grep -q "$needle" <<<"$haystack" || fail "$msg"
}

TMP_DIR="$(mktemp -d)"
WATCH_LOG="$(mktemp)"
WATCH_PID=""
FAKE_BIN_DIR=""

cleanup() {
  cd / >/dev/null 2>&1 || true
  if [[ -n "${WATCH_PID}" ]]; then
    kill -INT "$WATCH_PID" >/dev/null 2>&1 || true
    wait "$WATCH_PID" >/dev/null 2>&1 || true
  fi
  rm -rf "$TMP_DIR" "$WATCH_LOG" "$FAKE_BIN_DIR"
}
trap cleanup EXIT

echo "==> Using repo: $REPO"
echo "==> Using go:   $GO_BIN"

cd "$REPO"

echo "==> Running automated checks"
"$GO_BIN" test ./...
"$GO_BIN" test -race ./...
"$GO_BIN" build -o ./converge ./cmd/converge
BIN="$REPO/converge"

echo "==> Running end-to-end CLI flow in temp workspace"
cd "$TMP_DIR"

cat > go.mod <<'EOGO'
module example.com/tmpdemo
go 1.22
EOGO

"$BIN" init

cat > main.go <<'EOGO'
package main

func main() {}
EOGO

OUT1="$("$BIN" snap -m "baseline" --eval=false)"
echo "$OUT1"
assert_contains "$OUT1" "Created c_000001" "expected first cell c_000001"

cat > main.go <<'EOGO'
package main

import "fmt"

func main() { fmt.Println("v2") }
EOGO

cat > helper.go <<'EOGO'
package main

func helper() {}
EOGO

OUT2="$("$BIN" snap -m "attempt 2" --eval=false)"
echo "$OUT2"
assert_contains "$OUT2" "Created c_000002" "expected second cell c_000002"

# Create untracked file after snapshot so restore should preserve it.
echo "keep me" > notes.tmp

LOG1="$("$BIN" log --limit 5)"
echo "$LOG1"
assert_contains "$LOG1" "complexity(LOC)" "log should show LOC complexity"

DIFF_OUT="$("$BIN" diff c_000001 c_000002)"
echo "$DIFF_OUT"
assert_contains "$DIFF_OUT" "Added (" "diff should show added section"
assert_contains "$DIFF_OUT" "helper.go" "diff should include helper.go"

RESTORE_OUT="$("$BIN" restore c_000001)"
echo "$RESTORE_OUT"
assert_contains "$RESTORE_OUT" "Created safety cell: c_000003" "restore should create safety snapshot"

[[ -f notes.tmp ]] || fail "untracked file should be preserved after restore"
[[ ! -f helper.go ]] || fail "tracked file helper.go should be removed on restore"
grep -q 'func main() {}' main.go || fail "main.go should be restored to baseline"

echo "==> Running branch workflow"
BRANCH_CREATE="$("$BIN" fork feature-a --switch)"
echo "$BRANCH_CREATE"
assert_contains "$BRANCH_CREATE" "Created branch \"feature-a\"" "fork should create feature-a branch"
assert_contains "$BRANCH_CREATE" "Switched to \"feature-a\"" "fork --switch should switch active branch"

cat > branch.go <<'EOGO'
package main

func branchOnly() {}
EOGO
BRANCH_SNAP="$("$BIN" snap -m "feature branch change" --eval=false)"
echo "$BRANCH_SNAP"
assert_contains "$BRANCH_SNAP" "Branch: feature-a" "snap should report feature branch"

BRANCH_LIST="$("$BIN" branches)"
echo "$BRANCH_LIST"
assert_contains "$BRANCH_LIST" "feature-a" "branches should include feature-a"
assert_contains "$BRANCH_LIST" "main" "branches should include main"

SWITCH_MAIN="$("$BIN" switch main)"
echo "$SWITCH_MAIN"
assert_contains "$SWITCH_MAIN" "Switched to branch \"main\"" "switch should activate main"

LOG_MAIN="$("$BIN" log --branch main --limit 3)"
echo "$LOG_MAIN"
assert_contains "$LOG_MAIN" "[main]" "branch log should include branch labels"

COMPARE_ERR="$(mktemp)"
if OPENAI_API_KEY= "$BIN" compare c_000001 c_000002 >"$COMPARE_ERR" 2>&1; then
  fail "compare should fail without OPENAI_API_KEY"
fi
COMPARE_OUT="$(cat "$COMPARE_ERR")"
rm -f "$COMPARE_ERR"
echo "$COMPARE_OUT"
assert_contains "$COMPARE_OUT" "OPENAI_API_KEY" "compare should explain missing API key"

echo "==> Running on-demand eval"
EVAL_OUT="$("$BIN" eval c_000001)"
echo "$EVAL_OUT"

echo "==> Running hook integration smoke tests"

FAKE_BIN_DIR="$(mktemp -d)"
cat >"$FAKE_BIN_DIR/codex" <<'EOSH'
#!/usr/bin/env bash
set -euo pipefail
if [[ "${1:-}" == "exec" && "${2:-}" == "--output-last-message" ]]; then
  echo "codex wrapper smoke message"
  exit 0
fi
echo "unexpected codex args: $*" >&2
exit 2
EOSH
chmod +x "$FAKE_BIN_DIR/codex"

echo "// codex-wrapper-$(date +%s)" >> main.go
CODEX_WRAP_OUT="$(
  PATH="$FAKE_BIN_DIR:$PATH" \
  CONVERGE_BIN="$BIN" \
  CONVERGE_PROJECT_DIR="$TMP_DIR" \
  "$REPO/scripts/codex-exec-with-hook.sh" --prompt "smoke"
)"
echo "$CODEX_WRAP_OUT"
assert_contains "$CODEX_WRAP_OUT" "created run=" "codex wrapper should invoke converge hook complete"

LOG_HOOK="$("$BIN" log --limit 1 --all --no-color)"
echo "$LOG_HOOK"
assert_contains "$LOG_HOOK" "agent=codex" "latest cell should record codex agent metadata"

CLAUDE_TRANSCRIPT="$TMP_DIR/claude-transcript.log"
cat >"$CLAUDE_TRANSCRIPT" <<'EOT'
assistant: completed claude change
EOT

echo "// claude-hook-$(date +%s)" >> main.go
CLAUDE_STOP_PAYLOAD="$TMP_DIR/claude-stop.json"
cat >"$CLAUDE_STOP_PAYLOAD" <<EJSON
{"event":"Stop","session_id":"session-smoke-1","transcript_path":"$CLAUDE_TRANSCRIPT"}
EJSON
CLAUDE_STOP_OUT="$(
  CONVERGE_BIN="$BIN" \
  CONVERGE_PROJECT_DIR="$TMP_DIR" \
  "$REPO/scripts/claude-post-response-hook.sh" < "$CLAUDE_STOP_PAYLOAD"
)"
echo "$CLAUDE_STOP_OUT"
assert_contains "$CLAUDE_STOP_OUT" "created run=" "claude Stop hook should create or record run"

CLAUDE_END_PAYLOAD="$TMP_DIR/claude-end.json"
cat >"$CLAUDE_END_PAYLOAD" <<EJSON
{"event":"SessionEnd","session_id":"session-smoke-1","transcript_path":"$CLAUDE_TRANSCRIPT"}
EJSON
CLAUDE_END_OUT="$(
  CONVERGE_BIN="$BIN" \
  CONVERGE_PROJECT_DIR="$TMP_DIR" \
  "$REPO/scripts/claude-post-response-hook.sh" < "$CLAUDE_END_PAYLOAD"
)"
echo "$CLAUDE_END_OUT"
assert_contains "$CLAUDE_END_OUT" "duplicate run=" "claude SessionEnd should dedupe against Stop"

echo "==> Running watch mode smoke test"
"$BIN" watch --debounce 400ms >"$WATCH_LOG" 2>&1 &
WATCH_PID="$!"
sleep 0.8
echo "// watch-change-$(date +%s)" >> main.go

FOUND_WATCH=0
for _ in $(seq 1 12); do
  if grep -q "\[watch\]" "$WATCH_LOG"; then
    FOUND_WATCH=1
    break
  fi
  sleep 0.4
done

kill -INT "$WATCH_PID" >/dev/null 2>&1 || true
wait "$WATCH_PID" >/dev/null 2>&1 || true
WATCH_PID=""

cat "$WATCH_LOG"
[[ "$FOUND_WATCH" -eq 1 ]] || fail "watch mode did not auto-capture a cell"

echo "✅ PASS: Converge MVP checks completed successfully"
echo "   Temp workspace used: $TMP_DIR"
