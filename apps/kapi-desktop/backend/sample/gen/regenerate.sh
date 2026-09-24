#!/usr/bin/env bash
#
# Regenerate the KapiMart sample's history.
#
# The sample models a project that has been running for a while, so its content
# memory is a projection of approvals rather than an imported corpus: entries
# arrive through the record absorber and carry the unit they were approved for.
# That only happens if the loop actually runs, so this script runs it.
#
#   historic translations ──recycle──> target files ──absorb──> memory entries
#                                           │                        │
#                                           └──approve──> unit-state ledger
#
# What it writes back into the sample:
#
#   kapimart/src/<locale>/*.{json,properties}   the committed targets
#   kapimart/context/memory/memory.json           memory, every entry with a unit
#   kapimart/context/state/*.jsonl                their approval records
#
# Everything else stays untranslated on purpose. A file is committed only when
# the historic translations cover it well enough to ship without English
# standing in for the target language; the rest is the work the project still
# has in front of it, which is what the convergence hero and the plan are there
# to show.
#
# One source translates and stays unapproved: the message catalogue named in
# lib/targets.py's UNREVIEWED_SOURCES. Its targets are committed and no approval
# change-set is written for them, so the shipped project opens with those units
# in the review queue and Review has a unit to draw. Edit that list to change
# which content arrives that way; nothing here is keyed on a state-shard name.
#
# Deterministic: the only thing that moves between runs is the clock, and every
# timestamp is rewritten to a fixed instant at the end.
#
# Usage:
#     make regen-kapimart-sample
#     ./apps/kapi-desktop/backend/sample/gen/regenerate.sh --keep   # keep the scratch tree
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../../../../.." && pwd)"
SAMPLE="$ROOT/apps/kapi-desktop/backend/sample/kapimart"
GEN="$ROOT/apps/kapi-desktop/backend/sample/gen"
KAPI="$ROOT/bin/kapi"

KEEP=0
[ "${1:-}" = "--keep" ] && KEEP=1

if [ ! -x "$KAPI" ]; then
  echo "regenerate: $KAPI is missing — run 'make build' first." >&2
  exit 1
fi

WORK="$(mktemp -d)"
if [ "$KEEP" -eq 0 ]; then
  trap 'rm -rf "$WORK"' EXIT
else
  echo "regenerate: scratch tree kept at $WORK"
fi

P="$WORK/proj"

# The dogfood recipe sits above this tree and is found by an upward walk, so
# every invocation below has to opt out of it and out of the developer's own
# config, plugins and caches.
export KAPI_NO_PROJECT=1 KAPI_TELEMETRY=0 KAPI_PLUGINS_DIR_ONLY=1
export KAPI_CONFIG_DIR="$WORK/iso/config" XDG_DATA_HOME="$WORK/iso/data"
export XDG_CACHE_HOME="$WORK/iso/cache" KAPI_PLUGINS_DIR="$WORK/iso/plugins"

echo "==> working copy"
mkdir -p "$P/context/memory"
for area in web src legal marketing; do
  mkdir -p "$P/$area"
  cp -R "$SAMPLE/$area/en" "$P/$area/en"
done
cp "$SAMPLE/kapi.yaml" "$P/kapi.yaml"
cp "$SAMPLE/context/voice.yaml" "$P/context/voice.yaml"
cp "$SAMPLE/context/terms.json" "$P/context/terms.json"
mkdir -p "$P/.kapi"
cp "$SAMPLE/.kapi/.gitignore" "$P/.kapi/.gitignore"

# The historic translations, as the corpus the loop reuses. They are an input to
# this script and are never shipped: what ships is what the record teaches.
cp "$GEN/historic.memory.json" "$P/context/memory/memory.json"

# Converging with the recycle flow keeps the whole run offline — no provider, no
# credential, no network — so this is reproducible on any machine.
python3 "$GEN/lib/recipe.py" pin-recycle "$P/kapi.yaml"

# The context files are inert until they are read into this machine's store,
# which a person decides, so the import says whose decision this run is.
echo "==> read the context in"
KAPI_ACTOR=person "$KAPI" context import "$P/context" -p "$P/kapi.yaml" >/dev/null

echo "==> converge (offline: content memory only)"
"$KAPI" up -p "$P/kapi.yaml" >/dev/null 2>&1 || true

echo "==> keep the catalogs that ship, strip them to the target language"
python3 "$GEN/lib/targets.py" prune "$P" "$GEN/shipped.json"

echo "==> answers the corpus never held"
python3 "$GEN/lib/targets.py" answer "$P" "$GEN/authored.json"

echo "==> the contested answer"
python3 "$GEN/lib/targets.py" answer "$P" "$GEN/contested.json"

echo "==> approve what the loop produced, apart from the sources held for review"
"$KAPI" status -p "$P/kapi.yaml" --review --json > "$WORK/review.json" 2>/dev/null
python3 "$GEN/lib/targets.py" approvals "$WORK/review.json" "$WORK/approve.jsonl"
"$KAPI" apply -p "$P/kapi.yaml" "$WORK/approve.jsonl" >/dev/null

# The decisions are in the ledger from the moment they are applied. The sample
# ships them as files, so write the record back out beside the other context.
KAPI_ACTOR=person "$KAPI" context snapshot -p "$P/kapi.yaml" --out "$P/context" >/dev/null

# The corpus must not already hold the answers, or the absorber correctly
# declines to learn them and the memory comes back empty of provenance. The
# store is derived, and dropping it also clears the per-target absorb stamps
# that would otherwise make the next pass a no-op.
echo "==> clear the corpus and the derived store"
printf '{\n  "schemaVersion": "1.0",\n  "kind": "kapi-memory",\n  "entries": []\n}\n' \
  > "$P/context/memory/memory.json"
rm -f "$P/.kapi/work/store.db" "$P/.kapi/work/store.db-wal" "$P/.kapi/work/store.db-shm"

echo "==> absorb the committed record"
(cd "$ROOT/apps/kapi-desktop" && go run -tags "fts5" ./backend/sample/gen/absorb -project "$P/kapi.yaml")

echo "==> export"
"$KAPI" memory export -p "$P/kapi.yaml" --format bundle -o "$P/context/memory/memory.json" >/dev/null

echo "==> settle the clock"
python3 "$GEN/lib/targets.py" settle "$P"

echo "==> write back"
python3 "$GEN/lib/targets.py" install "$P" "$SAMPLE" "$GEN/shipped.json"

echo
echo "regenerate: done"
python3 "$GEN/lib/targets.py" summary "$SAMPLE"
