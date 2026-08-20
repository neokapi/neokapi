#!/usr/bin/env bash
# check-deploy-paths — assert the deploy workflow triggers on every framework
#                      directory compiled into the deployed binaries.
#
# bowrain-server and bowrain-worker link the framework: core, host, memory,
# providers and terms all end up inside them. If the deploy workflow's push
# paths do not list one of those, a change there alters the deployed binary and
# ships nothing — until some later, unrelated bowrain/ change carries it into
# production, recorded as that change.
#
# That is not hypothetical. On 2026-08-20, #2120 (core, host) and #2122 (core)
# both altered code inside bowrain-server and triggered no deploy; production
# ran without them until a manual dispatch. The paths were widened in response,
# and this exists so the next framework directory cannot reopen the hole in
# silence.
#
# The linked set is derived from `go list -deps`, never hardcoded here — a list
# maintained by hand is the same failure with an extra step.
set -euo pipefail

cd "$(dirname "$0")/.."
WORKFLOW=".github/workflows/deploy-server.yml"

if ! command -v go >/dev/null 2>&1; then
  echo "check-deploy-paths: go not installed — skipped"
  exit 0
fi

linked=$(
  cd bowrain
  { go list -deps ./cmd/bowrain-server; go list -deps ./cmd/bowrain-worker; } 2>/dev/null \
    | grep '^github.com/neokapi/neokapi/' \
    | sed 's|github.com/neokapi/neokapi/||' \
    | grep -v '^bowrain/' \
    | awk -F/ '{print $1}' | sort -u
)

if [ -z "$linked" ]; then
  echo "check-deploy-paths: could not resolve the dependency graph" >&2
  exit 1
fi

# The push paths block, up to the next top-level key.
paths=$(awk '/^  push:/{p=1} p&&/^[a-z]/{p=0} p' "$WORKFLOW")

missing=""
for d in $linked; do
  grep -qE "^ *- \"$d/\*\*\"" <<<"$paths" || missing="$missing $d"
done

if [ -n "$missing" ]; then
  echo "ERROR: $WORKFLOW does not deploy on changes to:$missing" >&2
  echo "  Those directories are compiled into bowrain-server/bowrain-worker, so a" >&2
  echo "  change there changes what runs in production. Add \"<dir>/**\" to the" >&2
  echo "  workflow's push paths." >&2
  exit 1
fi

echo "check-deploy-paths: OK ($(tr '\n' ' ' <<<"$linked"))"
