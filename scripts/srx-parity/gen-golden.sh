#!/usr/bin/env bash
# Regenerate SRX parity golden fixtures from the REAL Okapi (the bundled tikal
# distribution's okapi-lib jar + defaultSegmentation.srx). The Okapi apps are a
# downloaded artifact that may live in the primary checkout rather than a
# worktree, so we search a few known locations. Run from anywhere.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
# Candidate okapi-apps locations: this tree, then the primary neokapi checkout.
CANDS=(
  "$ROOT/bench/pseudobench/okapi-apps"
  "$(cd "$ROOT/../../../.." 2>/dev/null && pwd)/bench/pseudobench/okapi-apps"
  "$HOME/src/neokapi/neokapi/bench/pseudobench/okapi-apps"
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
