#!/usr/bin/env bash
#
# Guard: the committed reference dataset must be reproducible from THIS repo.
#
# packages/reference-data/data is generated and committed, and the docs site
# turns it into one MDX page per entry. Anything the generator picks up from
# outside this repository therefore lands in git and gets built — so the
# dataset must never contain entries that only exist because of what the
# author happened to have installed or checked out next door.
#
# This is the reference-data face of the kapi dogfood isolation contract
# (CLAUDE.md): in-repo tooling does not silently consume a developer's
# environment.
#
# How it went wrong once: `make generate-reference-docs` auto-detected a
# sibling okapi-bridge checkout, wrote ~93 formats and ~156 tools instead of
# the built-in ~38/34, and page generation turned that into ~180 extra
# committed MDX pages — enough to run the docs Docusaurus build out of heap on
# CI. Bridge inclusion is now opt-in (WITH_BRIDGE=1) and this guard makes a
# committed bridge entry fail loudly instead of via an OOM ten minutes later.
#
# Allowed sources in the committed dataset:
#   built-in  — the framework's own registry, always reproducible
#   plugin    — in-repo Mode-C plugins (plugins/<name>/manifest.json), also
#               reproducible: they are checked in here
#
# Rejected:
#   okapi     — okapi-bridge, which lives in a SEPARATE repository
#
# Usage:
#     ./scripts/check-reference-provenance.sh
#
# Wired into `make lint`, `make pre-push`, and the repo-guards CI job.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

readonly DATA_DIR="packages/reference-data/data"
readonly ALLOWED='built-in plugin'

fail=0
for f in "$DATA_DIR"/formats.json "$DATA_DIR"/tools.json; do
  [ -f "$f" ] || continue
  # One source value per line, deduplicated, so the report names the offender
  # rather than just a count.
  bad=$(python3 - "$f" <<'PY'
import json, sys
allowed = {"built-in", "plugin"}
data = json.load(open(sys.argv[1]))
entries = data.get("entries", data if isinstance(data, list) else [])
offenders = {}
for e in entries:
    src = e.get("source", "")
    if src not in allowed:
        offenders.setdefault(src, []).append(e.get("id", "?"))
for src, ids in sorted(offenders.items()):
    shown = ", ".join(sorted(ids)[:5])
    more = "" if len(ids) <= 5 else f" (+{len(ids) - 5} more)"
    print(f"{src}: {len(ids)} entries — {shown}{more}")
PY
)
  if [ -n "$bad" ]; then
    echo "✖ ${f} contains entries from outside this repository:"
    printf '  %s\n' "$bad"
    fail=1
  fi
done

if [ "$fail" -ne 0 ]; then
  cat <<'EOF'

The committed reference dataset must be reproducible from a fresh clone of
this repo alone. Allowed sources: built-in (the framework registry) and
plugin (in-repo plugins/<name>/manifest.json).

You almost certainly ran:

    make generate-reference-docs WITH_BRIDGE=1

and committed the result. Regenerate without it and commit that instead:

    make generate-reference-docs
    make generate-reference-pages

WITH_BRIDGE=1 is for inspecting the combined surface locally, not for
committing. See the BRIDGE_PLUGIN comment in the Makefile.
EOF
  exit 1
fi

echo "✓ reference dataset is reproducible from this repo (built-in + in-repo plugins only)"
