#!/usr/bin/env bash
# Regenerate SRX parity golden fixtures from the REAL Okapi (the bundled tikal
# distribution's okapi-lib jar + defaultSegmentation.srx). The Okapi apps are a
# downloaded artifact that may live in the primary checkout rather than a
# worktree, so we search a few known locations. Run from anywhere.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
# shellcheck source-path=SCRIPTDIR source=../lib/workspace.sh
. "$ROOT/scripts/lib/workspace.sh"
neokapi_init_workspace "$ROOT"
# Candidate okapi-apps locations: this tree, then the enclosing checkout (this
# tree may be a git worktree under .claude/worktrees/<name>/), then the primary
# neokapi checkout in the workspace.
CANDS=(
  "$ROOT/bench/pseudobench/okapi-apps"
  "$(cd "$ROOT/../../../.." 2>/dev/null && pwd)/bench/pseudobench/okapi-apps"
  "$NEOKAPI_WORKSPACE_DIR/neokapi/bench/pseudobench/okapi-apps"
)
OKAPI=""
for c in "${CANDS[@]}"; do [ -d "$c/lib" ] && OKAPI="$c" && break; done
[ -n "$OKAPI" ] || { echo "okapi-apps not found (searched: ${CANDS[*]})" >&2; exit 1; }
SRX="$OKAPI/config/defaultSegmentation.srx"
CORPUS="$ROOT/core/segment/srx/testdata/parity/corpus.tsv"
GOLDEN="$ROOT/core/segment/srx/testdata/parity/golden.jsonl"
OUT="$(mktemp -d)"
echo "using Okapi at $OKAPI" >&2
javac -cp "$OKAPI/lib/*" -d "$OUT" "$ROOT/scripts/srx-parity/OkapiSeg.java"
java -cp "$OUT:$OKAPI/lib/*" OkapiSeg "$SRX" "$CORPUS" > "$GOLDEN"
echo "wrote $GOLDEN ($(wc -l < "$GOLDEN") lines)" >&2
