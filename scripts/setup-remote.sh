#!/bin/bash
# Setup script for Claude Code on the web (remote environments).
# Installs project dependencies so that builds and tests work in the cloud VM.
# Ref: https://code.claude.com/docs/en/claude-code-on-the-web

set -euo pipefail

# Only run in remote (cloud) environments
if [ "${CLAUDE_CODE_REMOTE:-}" != "true" ]; then
  exit 0
fi

echo "==> Setting up neokapi remote environment"

# ── Go 1.26 ──────────────────────────────────────────────────────────────────
# The cloud image ships a recent Go but may not have 1.26 yet.
REQUIRED_GO="1.26"
CURRENT_GO=$(go version 2>/dev/null | grep -oP 'go\K[0-9]+\.[0-9]+' || echo "0.0")

if [ "$(printf '%s\n' "$REQUIRED_GO" "$CURRENT_GO" | sort -V | head -n1)" != "$REQUIRED_GO" ]; then
  echo "==> Installing Go ${REQUIRED_GO} (current: ${CURRENT_GO})"
  GO_ARCHIVE="go${REQUIRED_GO}.0.linux-amd64.tar.gz"
  curl -fsSL "https://go.dev/dl/${GO_ARCHIVE}" -o "/tmp/${GO_ARCHIVE}"
  sudo rm -rf /usr/local/go
  sudo tar -C /usr/local -xzf "/tmp/${GO_ARCHIVE}"
  rm -f "/tmp/${GO_ARCHIVE}"
  # Persist PATH for subsequent Bash tool calls
  if [ -n "${CLAUDE_ENV_FILE:-}" ]; then
    echo "PATH=/usr/local/go/bin:${HOME}/go/bin:${PATH}" >> "$CLAUDE_ENV_FILE"
  fi
  export PATH="/usr/local/go/bin:${HOME}/go/bin:${PATH}"
  echo "    Go $(go version)"
fi

# ── Go module dependencies ───────────────────────────────────────────────────
echo "==> Downloading Go modules (all four)"
go mod download
(cd platform && go mod download)
(cd kapi     && go mod download)
(cd bowrain  && go mod download)

# ── Node.js / frontend dependencies ──────────────────────────────────────────
echo "==> Installing frontend dependencies"
(cd packages/ui               && vp install --frozen-lockfile --prefer-offline)
(cd packages/flow-editor      && vp install --frozen-lockfile --prefer-offline)
(cd bowrain/apps/web           && vp install --frozen-lockfile --prefer-offline)
(cd kapi/apps/kapi-web      && vp install --frozen-lockfile --prefer-offline)
(cd bowrain/apps/bowrain/frontend && vp install --frozen-lockfile --prefer-offline)

echo "==> Remote environment ready"
