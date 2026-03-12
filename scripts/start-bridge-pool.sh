#!/usr/bin/env bash
# Start N bridge JVM processes and export their gRPC addresses.
#
# Usage:
#   eval "$(scripts/start-bridge-pool.sh [N] [jar-path])"
#
# This exports:
#   NEOKAPI_BRIDGE_ADDRS  — comma-separated list of gRPC addresses
#   BRIDGE_PIDS          — space-separated list of JVM PIDs
#
# Each JVM prints its gRPC address as the first line of stdout.
set -euo pipefail

N="${1:-4}"
JAR="${2:-${NEOKAPI_BRIDGE_JAR:?Set NEOKAPI_BRIDGE_JAR or pass JAR path as second argument}}"

if [[ -n "${JAVA_HOME:-}" ]]; then
    JAVA="$JAVA_HOME/bin/java"
else
    JAVA="java"
fi

if [[ ! -f "$JAR" ]]; then
    echo "error: JAR not found: $JAR" >&2
    exit 1
fi

ADDRS=()
PIDS=()
TMPDIR_POOL=$(mktemp -d)
trap 'rm -rf "$TMPDIR_POOL"' EXIT

for i in $(seq 1 "$N"); do
    ADDR_FILE="$TMPDIR_POOL/addr-$i"

    # Start JVM in background, capture its stdout to a file for address reading.
    "$JAVA" -jar "$JAR" > "$ADDR_FILE" 2>/dev/null &
    PID=$!
    PIDS+=("$PID")

    # Wait for the JVM to print its address (first line of stdout).
    DEADLINE=$((SECONDS + 30))
    while [[ $SECONDS -lt $DEADLINE ]]; do
        if [[ -s "$ADDR_FILE" ]]; then
            ADDR=$(head -1 "$ADDR_FILE")
            if [[ -n "$ADDR" ]]; then
                break
            fi
        fi
        sleep 0.1
    done

    if [[ -z "${ADDR:-}" ]]; then
        echo "error: JVM $i (PID $PID) did not print address within 30s" >&2
        # Clean up already-started JVMs.
        for p in "${PIDS[@]}"; do
            kill "$p" 2>/dev/null || true
        done
        exit 1
    fi

    ADDRS+=("$ADDR")
done

# Join addresses with commas.
ADDR_STR=$(IFS=,; echo "${ADDRS[*]}")

echo "export NEOKAPI_BRIDGE_ADDRS='$ADDR_STR'"
echo "export BRIDGE_PIDS='${PIDS[*]}'"
