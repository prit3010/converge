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
import re
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


def normalize_whitespace(text):
    text = text.replace("\r", "\n")
    text = re.sub(r"\s+", " ", text)
    return text.strip()


def looks_structured(text):
    t = text.strip()
    if not t:
        return True
    if (t.startswith("{") and t.endswith("}")) or (t.startswith("[") and t.endswith("]")):
        return True
    if re.search(r"parentUuid", t, re.IGNORECASE):
        return True
    if re.search(r"\bCONVERGE_[A-Z0-9_]+=", t):
        return True
    if re.search(r"\b[A-Z_]{3,}=[^ ]+", t):
        return True
    if t.startswith("{") and ":" in t:
        return True
    return False


def sanitize_summary(raw):
    if not isinstance(raw, str):
        return ""
    text = raw.replace("```", " ")
    lines = []
    for line in text.splitlines():
        cleaned = line.strip()
        if not cleaned:
            continue
        cleaned = re.sub(r"^\s{0,3}#{1,6}\s*", "", cleaned)
        cleaned = re.sub(r"^\s*[-*+]\s+", "", cleaned)
        cleaned = re.sub(r"^\s*\d+\.\s+", "", cleaned)
        cleaned = cleaned.strip("` ")
        if cleaned:
            lines.append(cleaned)

    if not lines:
        return ""

    chosen = ""
    for line in lines:
        if not looks_structured(line):
            chosen = line
            break

    if not chosen:
        flat = normalize_whitespace(" ".join(lines))
        if looks_structured(flat):
            return ""
        chosen = flat

    first_sentence = re.split(r"(?<=[.!?])\s+", chosen, maxsplit=1)[0]
    if first_sentence and len(first_sentence) >= 8:
        chosen = first_sentence

    chosen = normalize_whitespace(chosen)
    if looks_structured(chosen):
        return ""

    if len(chosen) > 160:
        chosen = chosen[:157].rstrip() + "..."
    return chosen


def text_from_content(content):
    if isinstance(content, str):
        return content.strip()
    if isinstance(content, list):
        parts = []
        for item in content:
            if isinstance(item, str):
                if item.strip():
                    parts.append(item.strip())
                continue
            if not isinstance(item, dict):
                continue
            item_type = str(item.get("type", "")).strip().lower()
            if item_type in ("text", "output_text", "message_text"):
                txt = item.get("text")
                if not isinstance(txt, str):
                    txt = item.get("content")
                if isinstance(txt, str) and txt.strip():
                    parts.append(txt.strip())
                continue
            if item_type == "":
                for key in ("text", "content", "value"):
                    txt = item.get(key)
                    if isinstance(txt, str) and txt.strip():
                        parts.append(txt.strip())
                        break
        return "\n".join(parts).strip()
    return ""


def extract_assistant_text(record):
    if not isinstance(record, dict):
        return ""

    msg = record.get("message")
    msg_role = ""
    if isinstance(msg, dict):
        msg_role = str(msg.get("role", "")).strip().lower()

    rec_type = str(record.get("type", "")).strip().lower()

    if rec_type.startswith("assistant"):
        if isinstance(msg, dict):
            txt = text_from_content(msg.get("content"))
            if txt:
                return txt
        txt = text_from_content(record.get("content"))
        if txt:
            return txt
        if isinstance(msg, str) and msg.strip():
            return msg.strip()

    if msg_role == "assistant":
        txt = text_from_content(msg.get("content"))
        if txt:
            return txt

    return ""


def extract_transcript_text(transcript_path):
    if not isinstance(transcript_path, str) or not transcript_path.strip():
        return ""
    try:
        with open(transcript_path, "r", encoding="utf-8", errors="ignore") as fh:
            lines = fh.read().splitlines()
    except Exception:
        return ""

    lines = lines[-400:]
    for raw in reversed(lines):
        line = raw.strip()
        if not line:
            continue
        if line.lower().startswith("assistant:"):
            return line.split(":", 1)[1].strip()
        try:
            record = json.loads(line)
        except Exception:
            continue
        text = extract_assistant_text(record)
        if text:
            return text
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

payload_candidates = [
    data.get("last_assistant_message"),
    data.get("lastAssistantMessage"),
    nested(data, "hook_input", "last_assistant_message"),
    nested(data, "stop", "last_assistant_message"),
    data.get("message"),
    data.get("response"),
    data.get("output_text"),
    nested(data, "output", "text"),
]

summary = ""
source = "fallback"
for candidate in payload_candidates:
    summary = sanitize_summary(candidate)
    if summary:
        source = "payload"
        break

if not summary:
    transcript_text = extract_transcript_text(transcript_path)
    summary = sanitize_summary(transcript_text)
    if summary:
        source = "transcript"

if not summary:
    event_name = event if event else "Hook"
    summary = sanitize_summary(f"{event_name} completed") or "Hook completed"
    source = "fallback"

message = f"Claude: {summary}"

print(event)
print(session_id)
print(transcript_path)
print(message)
print(source)
PY
}

decode_jsonish() {
  local text="$1"
  printf '%s' "$text" \
    | sed -E 's/\\\\n/ /g; s/\\\\r/ /g; s/\\\\t/ /g; s/\\\\"/"/g; s/\\\\\//\//g'
}

normalize_whitespace_sh() {
  local text="$1"
  printf '%s' "$text" \
    | tr '\r\n' '  ' \
    | sed -E 's/[[:space:]]+/ /g; s/^[[:space:]]+//; s/[[:space:]]+$//'
}

looks_structured_sh() {
  local text
  text="$(normalize_whitespace_sh "$1")"
  if [[ -z "$text" ]]; then
    return 0
  fi
  if printf '%s' "$text" | grep -Eq '^[[:space:]]*\{.*\}[[:space:]]*$|^[[:space:]]*\[.*\][[:space:]]*$'; then
    return 0
  fi
  if printf '%s' "$text" | grep -Eq 'parentUuid|CONVERGE_[A-Z0-9_]+=|\b[A-Z_]{3,}=[^ ]+'; then
    return 0
  fi
  return 1
}

sanitize_summary_sh() {
  local raw="$1"
  local text
  text="$(printf '%s' "$raw" | sed -E 's/```/ /g')"
  text="$(normalize_whitespace_sh "$text")"
  text="$(printf '%s' "$text" | sed -E 's/^[[:space:]]*#{1,6}[[:space:]]*//; s/^[[:space:]]*[-*+][[:space:]]+//; s/^[[:space:]]*[0-9]+\.[[:space:]]+//')"
  text="$(normalize_whitespace_sh "$text")"

  if looks_structured_sh "$text"; then
    printf ''
    return
  fi

  local sentence
  sentence="$(printf '%s' "$text" | sed -E 's/^([^.!?]*[.!?]).*/\1/')"
  if [[ -n "${sentence// }" && ${#sentence} -ge 8 ]]; then
    text="$sentence"
  fi

  text="$(normalize_whitespace_sh "$text")"
  if looks_structured_sh "$text"; then
    printf ''
    return
  fi

  if [[ ${#text} -gt 160 ]]; then
    text="${text:0:157}..."
  fi
  printf '%s' "$text"
}

extract_transcript_summary_without_python() {
  local transcript_path="$1"
  if [[ -z "$transcript_path" || ! -r "$transcript_path" ]]; then
    printf ''
    return
  fi

  local tail_data
  tail_data="$(tail -n 400 "$transcript_path" 2>/dev/null || true)"

  local candidate
  candidate="$(printf '%s\n' "$tail_data" | grep -E 'assistant:' | tail -n 1 | sed -E 's/^.*assistant:[[:space:]]*//I')"
  if [[ -z "${candidate// }" ]]; then
    candidate="$(printf '%s\n' "$tail_data" | grep -Eo '"text"[[:space:]]*:[[:space:]]*"([^"\\]|\\.)*"' | tail -n 1 | sed -E 's/^"text"[[:space:]]*:[[:space:]]*"(.*)"$/\1/')"
  fi
  candidate="$(decode_jsonish "$candidate")"
  sanitize_summary_sh "$candidate"
}

extract_fields_without_python() {
  local payload_file="$1"
  local event=""
  local session_id=""
  local transcript_path=""
  local payload_message=""
  local summary=""
  local source="fallback"

  event="$(grep -Eo '"(event|hook_event_name|type)"[[:space:]]*:[[:space:]]*"[^"]+"' "$payload_file" | head -n 1 | sed -E 's/.*:[[:space:]]*"([^"]+)"/\1/')"
  session_id="$(grep -Eo '"(session_id|conversation_id)"[[:space:]]*:[[:space:]]*"[^"]+"' "$payload_file" | head -n 1 | sed -E 's/.*:[[:space:]]*"([^"]+)"/\1/')"
  transcript_path="$(grep -Eo '"transcript_path"[[:space:]]*:[[:space:]]*"[^"]+"' "$payload_file" | head -n 1 | sed -E 's/.*:[[:space:]]*"([^"]+)"/\1/')"

  payload_message="$(grep -Eo '"last_assistant_message"[[:space:]]*:[[:space:]]*"([^"\\]|\\.)*"' "$payload_file" | head -n 1 | sed -E 's/^"last_assistant_message"[[:space:]]*:[[:space:]]*"(.*)"$/\1/')"
  if [[ -z "${payload_message// }" ]]; then
    payload_message="$(grep -Eo '"lastAssistantMessage"[[:space:]]*:[[:space:]]*"([^"\\]|\\.)*"' "$payload_file" | head -n 1 | sed -E 's/^"lastAssistantMessage"[[:space:]]*:[[:space:]]*"(.*)"$/\1/')"
  fi
  if [[ -z "${payload_message// }" ]]; then
    payload_message="$(grep -Eo '"(message|response|output_text)"[[:space:]]*:[[:space:]]*"([^"\\]|\\.)*"' "$payload_file" | head -n 1 | sed -E 's/^"(message|response|output_text)"[[:space:]]*:[[:space:]]*"(.*)"$/\2/')"
  fi

  payload_message="$(decode_jsonish "$payload_message")"
  summary="$(sanitize_summary_sh "$payload_message")"
  if [[ -n "${summary// }" ]]; then
    source="payload"
  else
    summary="$(extract_transcript_summary_without_python "$transcript_path")"
    if [[ -n "${summary// }" ]]; then
      source="transcript"
    fi
  fi

  if [[ -z "${summary// }" ]]; then
    local event_name="$event"
    if [[ -z "${event_name// }" ]]; then
      event_name="Hook"
    fi
    summary="$(sanitize_summary_sh "${event_name} completed")"
    if [[ -z "${summary// }" ]]; then
      summary="Hook completed"
    fi
    source="fallback"
  fi

  printf '%s\n%s\n%s\n%s\n%s\n' "$event" "$session_id" "$transcript_path" "Claude: $summary" "$source"
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
MESSAGE="$(printf '%s\n' "$fields_output" | sed -n '4p')"
MESSAGE_SOURCE="$(printf '%s\n' "$fields_output" | sed -n '5p')"

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

if [[ -z "${MESSAGE// }" ]]; then
  MESSAGE="Claude: ${EVENT} completed"
  MESSAGE_SOURCE="fallback"
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
  log_line "hook failed event=${EVENT} run_id=${RUN_ID} exit=${HOOK_EXIT} session=${SESSION_ID} transcript=${TRANSCRIPT_PATH} message_source=${MESSAGE_SOURCE}"
fi

exit 0
