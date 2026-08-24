#!/usr/bin/env bash
#
# Regenerate a Docusaurus site's qps (pseudo-locale) translations.
#
# qps is the runtime-correctness probe: every string that goes through the i18n
# machinery renders mangled (▒ Ĥöŵ îţ ŵöŕķš ▒), so one URL shows which text is
# translatable and which is hardcoded. It is not project content and not a
# target language in the recipe — the posture `make l10n-pseudo` already takes
# for the catalog-driven surfaces.
#
# TWO TIERS, and they are separated because only one can run in CI.
#
#   CONTENT (--content-only, the default in `make l10n-pseudo`)
#     The docs pages themselves, pseudo-translated with kapi into
#     i18n/qps/docusaurus-plugin-content-docs/current/. Needs kapi and nothing
#     else, so it rides the l10n pipeline and the autofix bot keeps it current
#     as the docs change. Without that, a page added after the last manual run
#     falls back to English in the pseudo build — which reads as "this string is
#     not translatable" when it only means "not regenerated". A probe that lies
#     is worse than no probe.
#
#   THEME (--with-theme)
#     navbar, footer, code.json, current.json. These come from
#     `docusaurus write-translations`, which needs the site's own node_modules —
#     and bowrain/web/docs is deliberately outside the pnpm workspace (PR #425),
#     so the l10n CI job cannot install them. Run this by hand when navbar or
#     footer items change; they change rarely and are committed.
#
# The theme tier also needs a fix-up: a Docusaurus translation file is
# {"key": {"message": …, "description": …}} and only `message` is shown to a
# reader. `description` is authoring metadata that write-translations
# regenerates verbatim, so mangling it makes every future diff unreadable.
# pseudo-translate has no key-path flag, so the descriptions are restored after.
#
# Usage:
#     scripts/pseudo-docs-i18n.sh bowrain/web/docs                # content only
#     scripts/pseudo-docs-i18n.sh bowrain/web/docs --with-theme   # + theme JSON
set -euo pipefail

site="${1:?usage: $0 <docusaurus-site-dir> [--with-theme]}"
mode="${2:---content-only}"
repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$repo_root/$site"

kapi="$repo_root/bin/kapi"
[ -x "$kapi" ] || { echo "::error::$kapi not built — run 'make build' first"; exit 1; }

export KAPI_NO_PROJECT=1 KAPI_TELEMETRY=0

# ── content ─────────────────────────────────────────────────────────────────
out="i18n/qps/docusaurus-plugin-content-docs/current"
rm -rf "$out"

n=0
while IFS= read -r f; do
  rel="${f#docs/}"
  mkdir -p "$out/$(dirname "$rel")"
  "$kapi" pseudo-translate "$f" --target-lang qps -o "$out/$rel" -q >/dev/null
  n=$((n + 1))
done < <(find docs -name "*.md" -o -name "*.mdx" | sort)
echo "==> $site: $n pages pseudo-translated into $out"

[ "$mode" = "--with-theme" ] || exit 0

# ── theme ───────────────────────────────────────────────────────────────────
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

echo "==> write-translations --locale qps"
vpx docusaurus write-translations --locale qps >/dev/null

# Only the files write-translations owns; the content tree above is not its.
find i18n/qps -maxdepth 2 -name "*.json" | sort | while read -r f; do
  cp "$f" "$work/pre.json"
  "$kapi" pseudo-translate "$f" --target-lang qps -f json -o "$work/post.json" -q

  python3 - "$work/pre.json" "$work/post.json" "$f" <<'PY'
import json, sys

src = json.load(open(sys.argv[1]))
out = json.load(open(sys.argv[2]))

restored = 0
for key, val in out.items():
    if isinstance(val, dict) and isinstance(src.get(key), dict) and "description" in src[key]:
        if val.get("description") != src[key]["description"]:
            val["description"] = src[key]["description"]
            restored += 1

with open(sys.argv[3], "w") as fh:
    json.dump(out, fh, indent=2, ensure_ascii=False)
    fh.write("\n")

msgs = sum(1 for v in out.values() if isinstance(v, dict) and "▒" in str(v.get("message", "")))
print(f"    {sys.argv[3]}: {msgs}/{len(out)} messages pseudo-ized, {restored} descriptions restored")
PY
done
