#!/usr/bin/env bash
#
# Regenerate the Compass sample's decision record (samples/compass/context/state).
#
# The record says which units a person approved, for which language, and when.
# Those three facts are the input. Everything else on a row (the source basis,
# the governing fingerprint, the document key the shard is named by) is what
# kapi records when a person decides, so this script has kapi make the
# decisions again rather than writing the rows itself:
#
#   committed record ──read──> approvals ──kapi apply──> ledger ──persist──> shards
#
# The approvals are recorded against the sample's current source, so each row
# carries the basis `kapi status` compares against, and a sample copied into a
# sandbox reports its decisions as current rather than as recorded before their
# basis.
#
# Deterministic: the clock is the only thing that moves between runs, and every
# decision's time is set back to the one the input record carries for it.
#
# Usage:
#     make regen-compass-sample
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SAMPLE="$ROOT/samples/compass"
KAPI="$ROOT/bin/kapi"

if [ ! -x "$KAPI" ]; then
  echo "regen-compass-state: $KAPI is missing, run 'make build' first." >&2
  exit 1
fi

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
P="$WORK/proj"

# The dogfood recipe sits above this tree and is found by an upward walk, so
# every invocation below opts out of it and out of the developer's own config,
# plugins, caches and workspace.
export KAPI_NO_PROJECT=1 KAPI_TELEMETRY=0 KAPI_PLUGINS_DIR_ONLY=1
export KAPI_CONFIG_DIR="$WORK/iso/config" XDG_DATA_HOME="$WORK/iso/data"
export XDG_CACHE_HOME="$WORK/iso/cache" KAPI_PLUGINS_DIR="$WORK/iso/plugins"
export KAPI_DATA_DIR="$WORK/iso/kapi-data"
export KAPI_ACTOR=person

echo "==> working copy"
mkdir -p "$P"
cp -R "$SAMPLE/." "$P/"
rm -rf "$P/.kapi/work"
cp -R "$SAMPLE/context/state" "$WORK/record"

echo "==> read the context in"
"$KAPI" store import "$P/context" -p "$P/kapi.yaml" >/dev/null

echo "==> decide each recorded unit again"
python3 - "$WORK/record" > "$WORK/approve.jsonl" <<'PY'
import json, os, sys
record = sys.argv[1]
for name in sorted(os.listdir(record)):
    for line in open(os.path.join(record, name), encoding="utf-8"):
        line = line.strip()
        if not line:
            continue
        row = json.loads(line)
        if not row.get("decision"):
            continue
        # A person's decision on the translation as it stands: the record
        # names no revision, so the decision takes the edition as it is.
        print(json.dumps({
            "op": "decide",
            "at": {"doc": "site/locales/%s.json" % row["variant"],
                   "block": row["unit"], "edition": row["variant"]},
            "if_match": "*",
            "outcome": "reject" if row["status"] == "rejected" else "establish",
        }))
PY
"$KAPI" apply -p "$P/kapi.yaml" "$WORK/approve.jsonl" >"$WORK/apply.log" 2>&1 ||
  { cat "$WORK/apply.log" >&2; exit 1; }

# Reading a record in writes the ledger out to the checkout's shards, which is
# the file form the sample ships.
echo "==> write the ledger out"
"$KAPI" store import "$P/context" -p "$P/kapi.yaml" >/dev/null

echo "==> settle the clock and write back"
python3 - "$WORK/record" "$P/.kapi/state" "$SAMPLE/context/state" <<'PY'
import json, os, shutil, sys
record, shards, dest = sys.argv[1:4]

def rows(directory):
    for name in sorted(os.listdir(directory)):
        for line in open(os.path.join(directory, name), encoding="utf-8"):
            line = line.strip()
            if line:
                yield name, json.loads(line)

decided_at = {}
for _, row in rows(record):
    if row.get("decision"):
        decided_at[(row["unit"], row["variant"])] = row["decision"]["at"]

out = {}
for name, row in rows(shards):
    key = (row.get("unit"), row.get("variant"))
    # Only the decisions this script made: they carry the source basis. The
    # input record read back in is left behind.
    if key not in decided_at or not row.get("decision") or not row.get("contentHash"):
        continue
    at = decided_at[key]
    row["decision"]["at"] = at
    row["updated"] = at
    out.setdefault(name, {})[key] = row

missing = set(decided_at) - {k for shard in out.values() for k in shard}
if missing:
    sys.exit("regen-compass-state: no decision recorded for %s" % sorted(missing)[:5])

shutil.rmtree(dest)
os.makedirs(dest)
for name, shard in out.items():
    with open(os.path.join(dest, name), "w", encoding="utf-8") as fh:
        for key in sorted(shard):
            # The serialization kapi writes its own shards in.
            fh.write(json.dumps(shard[key], ensure_ascii=False, separators=(",", ":")) + "\n")
print("    %d decision(s) in %d shard(s)" % (sum(len(s) for s in out.values()), len(out)))
PY

echo "regen-compass-state: done"
