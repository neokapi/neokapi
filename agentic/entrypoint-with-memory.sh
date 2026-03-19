#!/bin/bash
set -euo pipefail

AGENT_NAME="${AGENT_NAME:?}"
MEMORY_REPO="${MEMORY_REPO:?}"
MEMORY_DIR="/tmp/agent-memory"
ZEROCLAW_MEMORY="$HOME/.zeroclaw/memory"
MAX_PUSH_RETRIES=5

# --- Pull memory ---
if [ -d "$MEMORY_DIR/.git" ]; then
  git -C "$MEMORY_DIR" fetch origin
  git -C "$MEMORY_DIR" reset --hard origin/main
else
  git clone --depth 1 --filter=blob:none --sparse "$MEMORY_REPO" "$MEMORY_DIR"
  git -C "$MEMORY_DIR" sparse-checkout set "$AGENT_NAME"
fi

mkdir -p "$ZEROCLAW_MEMORY"
cp -r "$MEMORY_DIR/$AGENT_NAME/memory/"* "$ZEROCLAW_MEMORY/" 2>/dev/null || true

# --- Run agent ---
zeroclaw agent -m "$AGENT_TASK_MESSAGE"
EXIT_CODE=$?

# --- Push memory (with pull/rebase/retry for concurrent execution races) ---
if [ $EXIT_CODE -eq 0 ]; then
  mkdir -p "$MEMORY_DIR/$AGENT_NAME/memory"
  cp -r "$ZEROCLAW_MEMORY/"* "$MEMORY_DIR/$AGENT_NAME/memory/" 2>/dev/null || true
  cd "$MEMORY_DIR"
  git add "$AGENT_NAME/"

  if ! git diff --cached --quiet; then
    git commit -m "$AGENT_NAME: session $(date -u +%Y-%m-%dT%H:%M:%SZ)"

    for i in $(seq 1 $MAX_PUSH_RETRIES); do
      if git push origin main; then
        break
      fi
      echo "Push failed (attempt $i/$MAX_PUSH_RETRIES), rebasing..."
      git pull --rebase origin main
      # If rebase has conflicts (shouldn't for separate agent dirs), abort and skip
      if [ $? -ne 0 ]; then
        echo "Rebase conflict — skipping memory push this session"
        git rebase --abort
        break
      fi
    done
  fi
fi

exit $EXIT_CODE
