#!/usr/bin/env bash
set -euo pipefail

PROJECT_DIR="${CONVERGE_PROJECT_DIR:-}"
if [[ -z "$PROJECT_DIR" ]]; then
  PROJECT_DIR="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
fi

CONVERGE_BIN="${CONVERGE_BIN:-converge}"

SHA="$(git -C "$PROJECT_DIR" rev-parse HEAD 2>/dev/null || true)"
if [[ -z "$SHA" ]]; then
  echo "converge post-commit hook: unable to resolve commit SHA" >&2
  exit 1
fi

BRANCH="$(git -C "$PROJECT_DIR" branch --show-current 2>/dev/null || true)"
if [[ -z "$BRANCH" ]]; then
  BRANCH="detached"
fi

SUBJECT="$(git -C "$PROJECT_DIR" log -1 --pretty=%s "$SHA" 2>/dev/null || true)"
if [[ -z "$SUBJECT" ]]; then
  SUBJECT="(no subject)"
fi

if ! "$CONVERGE_BIN" hook git-commit --sha "$SHA" --branch "$BRANCH" --subject "$SUBJECT"; then
  echo "converge post-commit hook failed" >&2
  echo "Replay command:" >&2
  echo "  $CONVERGE_BIN hook git-commit --sha \"$SHA\" --branch \"$BRANCH\" --subject \"$SUBJECT\"" >&2
  exit 1
fi
