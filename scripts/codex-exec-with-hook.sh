#!/usr/bin/env bash
set -uo pipefail

PROJECT_DIR="${CONVERGE_PROJECT_DIR:-$(pwd)}"
CONVERGE_BIN="${CONVERGE_BIN:-converge}"
STRICT_MODE="${CONVERGE_HOOK_STRICT:-0}"
HOOK_AGENT="${CONVERGE_HOOK_AGENT:-codex}"
HOOK_TAGS="${CONVERGE_HOOK_TAGS:-auto,codex}"
HOOK_EVAL="${CONVERGE_HOOK_EVAL:-0}"

generate_run_id() {
  if command -v uuidgen >/dev/null 2>&1; then
    uuidgen | tr '[:upper:]' '[:lower:]'
    return 0
  fi
  if [[ -r /proc/sys/kernel/random/uuid ]]; then
    cat /proc/sys/kernel/random/uuid
    return 0
  fi
  printf 'run-%s-%s\n' "$(date -u +%s)" "$RANDOM"
}

RUN_ID="${CONVERGE_HOOK_RUN_ID:-$(generate_run_id)}"

set +e
MESSAGE="$(codex exec --output-last-message "$@")"
CODEX_EXIT=$?
set -e

if [[ -z "${MESSAGE// }" ]]; then
  MESSAGE="codex exec exit=${CODEX_EXIT}"
fi

HOOK_CMD=("$CONVERGE_BIN" "hook" "complete" "--run-id" "$RUN_ID" "--agent" "$HOOK_AGENT" "-m" "$MESSAGE" "--tags" "$HOOK_TAGS")
if [[ "$HOOK_EVAL" == "1" ]]; then
  HOOK_CMD+=("--eval")
fi

set +e
(
  cd "$PROJECT_DIR" || exit 1
  "${HOOK_CMD[@]}"
)
HOOK_EXIT=$?
set -e

if [[ "$HOOK_EXIT" -ne 0 ]]; then
  echo "converge hook complete failed (run_id=$RUN_ID): exit $HOOK_EXIT" >&2
  if [[ "$STRICT_MODE" == "1" ]]; then
    exit "$HOOK_EXIT"
  fi
fi

exit "$CODEX_EXIT"
