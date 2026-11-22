#!/usr/bin/env bash
# Run with --self-test to check the boundary fixtures alone.
set -euo pipefail

exec python3 "$(dirname "$0")/check-ts-license-boundary.py" "$@"
