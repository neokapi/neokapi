#!/bin/bash
# Pre-commit/push verification hook for Claude Code.
# Blocks git commit and git push if builds fail.
set -euo pipefail

INPUT=$(cat)
COMMAND=$(echo "$INPUT" | jq -r '.tool_input.command // empty')

# Only intercept git commit and git push
case "$COMMAND" in
  git\ commit*|git\ push*|*"&&"*git\ commit*|*"&&"*git\ push*)
    ;;
  *)
    exit 0
    ;;
esac

cd "$CLAUDE_PROJECT_DIR" || exit 1

echo "=== Pre-git verification ===" >&2

# 1. Go builds (all 4 modules)
echo "Checking Go builds..." >&2
if ! go build ./... 2>&1; then
  echo "BLOCKED: Framework module build failed" >&2
  exit 2
fi
if ! (cd platform && go build ./...) 2>&1; then
  echo "BLOCKED: Platform module build failed" >&2
  exit 2
fi
if ! (cd kapi && go build ./...) 2>&1; then
  echo "BLOCKED: Kapi module build failed" >&2
  exit 2
fi
if ! (cd bowrain && go build ./cmd/bowrain-server/) 2>&1; then
  echo "BLOCKED: Bowrain server build failed" >&2
  exit 2
fi

# 2. Go tests (all 4 modules)
echo "Running Go tests..." >&2
if ! go test ./... -count=1 2>&1; then
  echo "BLOCKED: Framework tests failed" >&2
  exit 2
fi
if ! (cd platform && go test ./... -count=1) 2>&1; then
  echo "BLOCKED: Platform tests failed" >&2
  exit 2
fi
if ! (cd kapi && go test ./... -count=1) 2>&1; then
  echo "BLOCKED: Kapi tests failed" >&2
  exit 2
fi
if ! (cd bowrain && go test $(go list ./... | grep -v '/apps/bowrain') -count=1) 2>&1; then
  echo "BLOCKED: Bowrain tests failed" >&2
  exit 2
fi

# 3. Check for accidental binaries in staged files
STAGED=$(git diff --cached --name-only 2>/dev/null || true)
if echo "$STAGED" | grep -qE '\.class$|\.jar$|\.exe$|\.o$|\.so$|\.dylib$'; then
  echo "BLOCKED: Binary files detected in staged changes" >&2
  echo "$STAGED" | grep -E '\.class$|\.jar$|\.exe$|\.o$|\.so$|\.dylib$' >&2
  exit 2
fi

# 4. If package.json or package-lock.json changed, verify npm workspace
if echo "$STAGED" | grep -qE 'package(-lock)?\.json$'; then
  echo "Checking npm workspace..." >&2
  if ! (cd packages/ui && npm ci && npx tsc) 2>&1; then
    echo "BLOCKED: npm workspace (packages/ui) build failed" >&2
    exit 2
  fi
fi

echo "=== All checks passed ===" >&2
exit 0
