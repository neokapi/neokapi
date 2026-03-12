#!/bin/bash
# Run all e2e tests (kapi CLI + Bowrain server).
#
# For kapi-only:    bash kapi/e2e/run.sh
# For bowrain-only: bash bowrain/e2e/server/run.sh
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

echo "============================================"
echo "gokapi E2E Tests (all)"
echo "============================================"
echo ""

# ── Kapi CLI E2E ───────────────────────────────────────────────
echo "── Kapi CLI E2E ──────────────────────────────"
bash "$ROOT_DIR/kapi/e2e/run.sh"

echo ""

# ── Bowrain Server E2E ─────────────────────────────────────────
echo "── Bowrain Server E2E ────────────────────────"
bash "$ROOT_DIR/bowrain/e2e/server/run.sh"

echo ""
echo "All e2e tests passed!"
