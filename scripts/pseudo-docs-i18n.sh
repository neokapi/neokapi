#!/usr/bin/env bash
#
# Regenerate a Docusaurus site's qps (pseudo-locale) translation files.
#
# qps is the runtime-correctness probe: every string that goes through the i18n
# machinery renders mangled (▒ Ĥöŵ îţ ŵöŕķš ▒), so one URL shows which chrome is
# translatable and which is hardcoded. It is not project content and not a
# target language in the recipe — the same posture `make l10n-pseudo` takes for
# the catalog-driven surfaces.
#
# Two steps, and the second is the reason this is a script rather than a
# one-liner:
#
#   1. `docusaurus write-translations --locale qps` writes the theme strings
#      with ENGLISH defaults. It is idempotent and picks up navbar/footer items
#      as they change, so it runs every time rather than being a one-off.
#
#   2. `kapi pseudo-translate` mangles them. But a Docusaurus translation file is
#      {"key": {"message": …, "description": …}}, and only `message` is shown to
#      a reader. `description` is authoring metadata that step 1 regenerates
#      verbatim; mangling it makes every future write-translations diff
#      unreadable. pseudo-translate has no key-path flag, so the descriptions are
#      restored from the pre-pseudo file afterwards.
#
# The output is COMMITTED, matching translations/qps.json on the landing side:
# the intermediate catalogs are generated, the artifact the build loads is in
# git. That is what lets the deploy build --locale qps without kapi on PATH.
#
# Usage:
#     scripts/pseudo-docs-i18n.sh bowrain/web/docs
set -euo pipefail

site="${1:?usage: $0 <docusaurus-site-dir>}"
repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$repo_root/$site"

kapi="$repo_root/bin/kapi"
[ -x "$kapi" ] || { echo "::error::$kapi not built — run 'make build' first"; exit 1; }

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

echo "==> write-translations --locale qps ($site)"
vpx docusaurus write-translations --locale qps >/dev/null

# Every translation file the step above wrote or refreshed. Listed with find
# into a plain loop rather than `mapfile`, which bash 3.2 (the macOS default)
# does not have — a developer running this locally would otherwise get a
# "command not found" from a script that works in CI.
count=$(find i18n/qps -name "*.json" | wc -l | tr -d ' ')
[ "$count" -gt 0 ] || { echo "::error::write-translations produced no files"; exit 1; }

find i18n/qps -name "*.json" | sort | while read -r f; do
  cp "$f" "$work/pre.json"
  KAPI_NO_PROJECT=1 KAPI_TELEMETRY=0 "$kapi" pseudo-translate "$f" \
    --target-lang qps -f json -o "$work/post.json" -q

  # Restore `description` from the pre-pseudo copy, keep the mangled `message`.
  python3 - "$work/pre.json" "$work/post.json" "$f" <<'PY'
import json, sys

pre, post, dest = sys.argv[1], sys.argv[2], sys.argv[3]
src = json.load(open(pre))
out = json.load(open(post))

restored = 0
for key, val in out.items():
    if isinstance(val, dict) and isinstance(src.get(key), dict) and "description" in src[key]:
        if val.get("description") != src[key]["description"]:
            val["description"] = src[key]["description"]
            restored += 1

with open(dest, "w") as fh:
    json.dump(out, fh, indent=2, ensure_ascii=False)
    fh.write("\n")

msgs = sum(1 for v in out.values() if isinstance(v, dict) and "▒" in str(v.get("message", "")))
print(f"    {dest}: {msgs}/{len(out)} messages pseudo-ized, {restored} descriptions restored")
PY
done

echo "==> done: i18n/qps is regenerated and ready to commit"
