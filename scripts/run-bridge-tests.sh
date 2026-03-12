#!/usr/bin/env bash
# Run bridge filter integration tests using a shared pool of pre-started JVMs.
#
# Usage:
#   scripts/run-bridge-tests.sh [pool-size] [extra-go-test-args...]
#
# Environment:
#   NEOKAPI_BRIDGE_JAR  — path to the okapi-bridge JAR (required)
#   JAVA_HOME          — optional, used to find java binary
#
# Example:
#   NEOKAPI_BRIDGE_JAR=~/.cache/neokapi/bridge/okapi-bridge.jar scripts/run-bridge-tests.sh 4
set -euo pipefail

POOL_SIZE="${1:-4}"
shift 2>/dev/null || true

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "Starting $POOL_SIZE bridge JVMs..."
eval "$("$SCRIPT_DIR/start-bridge-pool.sh" "$POOL_SIZE")"
echo "Bridge pool started: $NEOKAPI_BRIDGE_ADDRS"
echo "Bridge PIDs: $BRIDGE_PIDS"

cleanup() {
    echo "Shutting down bridge JVMs..."
    for pid in $BRIDGE_PIDS; do
        kill "$pid" 2>/dev/null || true
    done
    # Wait briefly for graceful shutdown.
    sleep 1
    for pid in $BRIDGE_PIDS; do
        kill -9 "$pid" 2>/dev/null || true
    done
}
trap cleanup EXIT

echo "Running bridge filter tests..."
NEOKAPI_BRIDGE_ADDRS="$NEOKAPI_BRIDGE_ADDRS" \
    go test -tags=integration -count=1 -v "$@" ./core/plugin/bridge/filters/...
