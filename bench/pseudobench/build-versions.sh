#!/bin/bash
# Build kapi binaries for multiple git tag versions.
#
# Usage: ./build-versions.sh v0.1.0 v0.2.0 v0.3.0
#
# Binaries are placed in ./versions/ as kapi-<tag>.
# The current branch is restored after building.

set -euo pipefail

REPO_ROOT="${REPO_ROOT:-../../}"
OUTPUT_DIR="${OUTPUT_DIR:-versions}"

if [ $# -eq 0 ]; then
    echo "Usage: $0 <tag1> [tag2] [tag3] ..."
    echo "Example: $0 v0.1.0 v0.2.0 v0.3.0"
    exit 1
fi

# Use the Go tool's build-versions command.
exec go run . build-versions -versions "$(IFS=,; echo "$*")" -repo "$REPO_ROOT" -output "$OUTPUT_DIR"
