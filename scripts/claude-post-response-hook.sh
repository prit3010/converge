#!/usr/bin/env bash
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="${CONVERGE_PROJECT_DIR:-$(cd "$SCRIPT_DIR/.." && pwd)}"
CONVERGE_BIN="${CONVERGE_BIN:-converge}"
LOG_DIR="$PROJECT_DIR/.converge/hooks"
LOG_FILE="$LOG_DIR/claude-post-response-hook.log"

mkdir -p "$LOG_DIR" 2>/dev/null || true

log_line() {
  local message="$1"
  printf '%s %s\n' "$(date -u '+%Y-%m-%dT%H:%M:%SZ')" "$message" >>"$LOG_FILE" 2>/dev/null || true
}

sha256_text() {
  local input="$1"
  if command -v shasum >/dev/null 2>&1; then
    LC_ALL=C printf '%s' "$input" | LC_ALL=C shasum -a 256 | awk '{print $1}'
    return
  fi
  if command -v sha256sum >/dev/null 2>&1; then
    printf '%s' "$input" | sha256sum | awk '{print $1}'
    return
  fi
  if command -v openssl >/dev/null 2>&1; then
    printf '%s' "$input" | openssl dgst -sha256 | awk '{print $2}'
    return
  fi
  printf 'hash-unavailable'
}

extract_fields_with_python() {
  local payload_file="$1"
  python3 - "$payload_file" <<'PY'
import json
import sys

path = sys.argv[1]
try:
    with open(path, "r", encoding="utf-8") as fh:
        data = json.load(fh)
except Exception:
    data = {}

def nested(src, *parts):
    cur = src
    for part in parts:
        if not isinstance(cur, dict) or part not in cur:
            return None
        cur = cur.get(part)
    return cur

def pick(*values):
    for value in values:
        if isinstance(value, str) and value.strip():
            return value.strip()
    return ""

event = pick(
    data.get("event"),
    data.get("hook_event_name"),
    data.get("type"),
    nested(data, "hook", "event"),
)
session_id = pick(
    data.get("session_id"),
    data.get("conversation_id"),
    nested(data, "session", "id"),
)
transcript_path = pick(
    data.get("transcript_path"),
    nested(data, "session", "transcript_path"),
    nested(data, "transcript", "path"),
)
fallback_message = pick(
    data.get("message"),
    data.get("response"),
    data.get("output_text"),
)

print(event)
print(session_id)
print(transcript_path)
print(fallback_message)
PY
}

extract_fields_without_python() {
  local payload_file="$1"
  local event=""
  local session_id=""
  local transcript_path=""
  local fallback_message=""

  event="$(grep -Eo '"(event|hook_event_name|type)"[[:space:]]*:[[:space:]]*"[^"]+"' "$payload_file" | head -n 1 | sed -E 's/.*:[[:space:]]*"([^"]+)"/\1/')"
  session_id="$(grep -Eo '"(session_id|conversation_id)"[[:space:]]*:[[:space:]]*"[^"]+"' "$payload_file" | head -n 1 | sed -E 's/.*:[[:space:]]*"([^"]+)"/\1/')"
  transcript_path="$(grep -Eo '"transcript_path"[[:space:]]*:[[:space:]]*"[^"]+"' "$payload_file" | head -n 1 | sed -E 's/.*:[[:space:]]*"([^"]+)"/\1/')"
  fallback_message="$(grep -Eo '"(message|response|output_text)"[[:space:]]*:[[:space:]]*"[^"]+"' "$payload_file" | head -n 1 | sed -E 's/.*:[[:space:]]*"([^"]+)"/\1/')"

  printf '%s\n%s\n%s\n%s\n' "$event" "$session_id" "$transcript_path" "$fallback_message"
}

PAYLOAD_FILE="$(mktemp)"
trap 'rm -f "$PAYLOAD_FILE"' EXIT
cat >"$PAYLOAD_FILE" || true

if command -v python3 >/dev/null 2>&1; then
  fields_output="$(extract_fields_with_python "$PAYLOAD_FILE")"
else
  fields_output="$(extract_fields_without_python "$PAYLOAD_FILE")"
fi

EVENT="$(printf '%s\n' "$fields_output" | sed -n '1p')"
SESSION_ID="$(printf '%s\n' "$fields_output" | sed -n '2p')"
TRANSCRIPT_PATH="$(printf '%s\n' "$fields_output" | sed -n '3p')"
FALLBACK_MESSAGE="$(printf '%s\n' "$fields_output" | sed -n '4p')"

if [[ "$EVENT" != "Stop" && "$EVENT" != "SessionEnd" ]]; then
  exit 0
fi

TRANSCRIPT_TAIL=""
if [[ -n "$TRANSCRIPT_PATH" && -r "$TRANSCRIPT_PATH" ]]; then
  TRANSCRIPT_TAIL="$(tail -n 160 "$TRANSCRIPT_PATH" 2>/dev/null || true)"
fi

TAIL_HASH="$(sha256_text "$TRANSCRIPT_TAIL")"
RUN_ID_HASH="$(sha256_text "${SESSION_ID}|${TRANSCRIPT_PATH}|${TAIL_HASH}")"
RUN_ID="claude-${RUN_ID_HASH:0:32}"

MESSAGE="$(printf '%s\n' "$TRANSCRIPT_TAIL" | sed '/^[[:space:]]*$/d' | tail -n 1 | cut -c1-500)"
if [[ -z "${MESSAGE// }" ]]; then
  MESSAGE="$FALLBACK_MESSAGE"
fi
if [[ -z "${MESSAGE// }" ]]; then
  MESSAGE="Claude ${EVENT} hook"
fi

HOOK_CMD=("$CONVERGE_BIN" "hook" "complete" "--run-id" "$RUN_ID" "--agent" "claude" "-m" "$MESSAGE" "--tags" "auto,claude")

set +e
(
  cd "$PROJECT_DIR" || exit 1
  "${HOOK_CMD[@]}"
)
HOOK_EXIT=$?
set -e

if [[ "$HOOK_EXIT" -ne 0 ]]; then
  log_line "hook failed event=${EVENT} run_id=${RUN_ID} exit=${HOOK_EXIT} session=${SESSION_ID} transcript=${TRANSCRIPT_PATH}"
fi

exit 0
