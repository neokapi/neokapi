#!/usr/bin/env bash
#
# run-agent-cycle.sh — Run one cycle of all agents for a workspace.
#
# Simulates a typical workday cycle:
#   1. Alex (engineer): checks upstream, pushes source content
#   2. Sophie (fr-FR translator): translates a batch of blocks
#   3. Thomas (de-DE translator): translates a batch of blocks
#   4. Mei (reviewer): reviews all translations
#
# Usage:
#   ./run-agent-cycle.sh [--continuous]
#
# With --continuous, runs a cycle every 5 minutes.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
CONTINUOUS="${1:-}"

run_cycle() {
  echo ""
  echo "============================================="
  echo "  Agent Cycle — $(date -Iseconds)"
  echo "============================================="
  echo ""

  # Alex: engineer check
  echo "--- Alex (L10N Engineer) ---"
  AGENT=alex ROLE=engineer bash "${SCRIPT_DIR}/agent-session.sh" 2>&1 || true
  echo ""

  # Sophie: translate fr-FR
  echo "--- Sophie (French Translator) ---"
  AGENT=sophie ROLE=translator LOCALE=fr-FR BATCH_SIZE=20 bash "${SCRIPT_DIR}/agent-session.sh" 2>&1 || true
  echo ""

  # Thomas: translate de-DE
  echo "--- Thomas (German Translator) ---"
  AGENT=thomas ROLE=translator LOCALE=de-DE BATCH_SIZE=20 bash "${SCRIPT_DIR}/agent-session.sh" 2>&1 || true
  echo ""

  # Mei: review
  echo "--- Mei (Reviewer) ---"
  AGENT=mei ROLE=reviewer bash "${SCRIPT_DIR}/agent-session.sh" 2>&1 || true
  echo ""

  echo "============================================="
  echo "  Cycle complete — $(date -Iseconds)"
  echo "============================================="
}

if [ "$CONTINUOUS" = "--continuous" ]; then
  echo "Running agent cycles continuously (every 5 minutes)..."
  echo "Press Ctrl+C to stop."
  while true; do
    run_cycle
    echo ""
    echo "Next cycle in 5 minutes..."
    sleep 300
  done
else
  run_cycle
fi
