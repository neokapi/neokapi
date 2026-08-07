#!/usr/bin/env bash
# kpz-smoke: headless gate for the .kpz project snapshot hand-off + cached
# resume (AD-025 §5 / #787). Drives a project run (which caches overlays in
# the persistent block store), packs the working state into a .kpz, unpacks
# it into a fresh project, and checks that a cached re-run is byte-identical
# and that a no-`--log` pack is byte-deterministic.
#
# Usage: scripts/kpz-smoke.sh [path/to/kapi]
set -euo pipefail

KAPI="${1:-bin/kapi}"
if [[ ! -x "$KAPI" ]]; then
  echo "kpz-smoke: kapi binary not found at $KAPI (run 'make build' first)" >&2
  exit 1
fi
KAPI="$(cd "$(dirname "$KAPI")" && pwd)/$(basename "$KAPI")"

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

# Isolate plugins/caches from the developer environment (the project recipe
# is explicit via -p, so project discovery is fine).
export KAPI_PLUGINS_DIR_ONLY=1 NO_COLOR=1
export KAPI_CONFIG_DIR="$WORK/config" XDG_DATA_HOME="$WORK/data" XDG_CACHE_HOME="$WORK/cache"

mkproject() {
  local dir="$1"
  mkdir -p "$dir/.kapi"
  printf '{"greeting":"Hello world","farewell":"Goodbye now","cta":"Sign up today"}' > "$dir/app.json"
  cat > "$dir/kapi.yaml" <<'EOF'
version: "v1"
name: demo
defaults:
  source_language: en
  target_languages: [fr-FR]
flows:
  pseudo:
    steps:
      - tool: pseudo-translate
EOF
}

# ── .kpz ad-hoc workspace: extract → transform → merge (no project) ────────
echo "kpz-smoke: .kpz workspace (extract → transform → merge == one-shot)"
printf '{"greeting":"Hello world","farewell":"Goodbye now"}' > "$WORK/app.json"
(
  cd "$WORK" && export KAPI_NO_PROJECT=1
  "$KAPI" pseudo-translate app.json -o oneshot.json --target-lang qps >/dev/null
  "$KAPI" extract app.json -o work.kpz --target-lang qps >/dev/null
  "$KAPI" pseudo-translate work.kpz >/dev/null            # transform in place
  "$KAPI" merge work.kpz -o l10n/ >/dev/null              # emit
)
diff -q "$WORK/oneshot.json" "$WORK/l10n/app.json" >/dev/null || { echo "FAIL: workspace merge differs from one-shot"; exit 1; }

mkproject "$WORK/p1"
REC="$WORK/p1/kapi.yaml"

echo "kpz-smoke: project run (caches overlays)"
"$KAPI" run pseudo -p "$REC" -i "$WORK/p1/app.json" -o "$WORK/p1/out1.json" --target-lang fr-FR >/dev/null
[[ -f "$WORK/p1/.kapi/work/store.db" ]] || { echo "FAIL: project store not created"; exit 1; }

echo "kpz-smoke: cached re-run is byte-identical"
"$KAPI" run pseudo -p "$REC" -i "$WORK/p1/app.json" -o "$WORK/p1/out2.json" --target-lang fr-FR >/dev/null
diff -q "$WORK/p1/out1.json" "$WORK/p1/out2.json" >/dev/null || { echo "FAIL: cached re-run differs"; exit 1; }

echo "kpz-smoke: pack is deterministic without --log"
"$KAPI" pack -p "$REC" -o "$WORK/a.kpz" >/dev/null
"$KAPI" pack -p "$REC" -o "$WORK/b.kpz" >/dev/null
diff -q "$WORK/a.kpz" "$WORK/b.kpz" >/dev/null || { echo "FAIL: packs of the same state differ"; exit 1; }

echo "kpz-smoke: pack → unpack into a fresh project"
mkproject "$WORK/p2"
"$KAPI" unpack "$WORK/a.kpz" -p "$WORK/p2/kapi.yaml" >/dev/null
[[ -f "$WORK/p2/.kapi/work/store.db" ]] || { echo "FAIL: unpack did not rehydrate the project store"; exit 1; }

echo "kpz-smoke: OK (cached resume byte-identical; pack deterministic; pack/unpack round-trips)"
