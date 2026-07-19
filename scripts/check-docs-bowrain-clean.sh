#!/usr/bin/env bash
#
# Guard: the neokapi (kapi / framework) docs must contain ZERO bowrain
# references, with ONE narrow, capped exception (R9): a sanctioned
# ":::note[Works with Bowrain]" admonition at defined seams — currently the
# kapi-bowrain plugin reference entry, governed team review, cross-project
# brand memory, and the CI loop page. R9 amends the no-mention rule to allow
# AT MOST `MAX_SANCTIONED_CALLOUTS` of these neutral callouts; it does not
# open the door to bowrain mentions anywhere else. bowrain is otherwise a
# strictly DOWNSTREAM product (AD-001); its docs live in bowrain/web/docs/.
#
# A second, separate exception is allowed only under web/docs/contribute/
# (architecture + notes-internal), where genuine cross-module facts may name
# bowrain — e.g. kapi-desktop's blank-import of bowrain/plugin/schema, the
# kapi-*/bowrain-* skill split, and the module tree. That directory is not in
# the user_facing sweep below, so it's untouched by either check here.
#
# Everything else user-facing (framework/kapi/react/reference/toolbox and the
# docs home page, which is also the product landing page) must not mention
# bowrain outside a sanctioned callout.
#
# See docs-intent-impl-audit.md (WS1) for the original rationale and R9 (the
# 2026-07-18 positioning sweep) for the callout exception.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

fail=0

# 1. No bowrain in user-facing neokapi docs, except inside a sanctioned
#    ":::note[Works with Bowrain]" callout — and at most MAX_SANCTIONED
#    of those, repo-wide, so R9's "≤4" ceiling is enforced by CI rather than
#    by convention.
user_facing=(
  web/docs/framework
  web/docs/kapi
  web/docs/react
  web/docs/reference
  web/docs/toolbox
  web/src
)
readonly MAX_SANCTIONED_CALLOUTS=4
readonly CALLOUT_MARKER=':::note[Works with Bowrain]'

# mask_callouts blanks the marker line, body, and closing ':::' of every
# sanctioned callout in a file, preserving line numbers so grep -n below still
# reports true line numbers for any *other* bowrain mention in the file.
mask_callouts() {
  awk '
    /^:::note\[Works with Bowrain\]$/ { skip = 1; print ""; next }
    skip && /^:::[[:space:]]*$/       { skip = 0; print ""; next }
    skip                              { print ""; next }
    { print }
  ' "$1"
}

sanctioned_count=0
hits=""
while IFS= read -r -d '' f; do
  n=$(grep -c -F "$CALLOUT_MARKER" "$f" 2>/dev/null || true)
  sanctioned_count=$((sanctioned_count + n))
  if file_hits=$(mask_callouts "$f" | grep -inE 'bowrain'); then
    hits="${hits}${f}:"$'\n'"${file_hits}"$'\n'
  fi
done < <(find "${user_facing[@]}" -type f -print0 2>/dev/null)

if [ -n "$hits" ]; then
  echo "✖ bowrain reference(s) found in user-facing neokapi docs outside a sanctioned 'Works with Bowrain' callout (must be zero):"
  echo "$hits"
  fail=1
fi

if [ "$sanctioned_count" -gt "$MAX_SANCTIONED_CALLOUTS" ]; then
  echo "✖ found ${sanctioned_count} sanctioned 'Works with Bowrain' callouts, exceeding R9's cap of ${MAX_SANCTIONED_CALLOUTS}:"
  grep -rn -F "$CALLOUT_MARKER" "${user_facing[@]}" 2>/dev/null || true
  fail=1
fi

# 2. No bowrain video assets staged in the neokapi static tree. They belong to
#    the bowrain site; publish-docs-assets 'merges, never drops', so any left
#    here get shipped into the neokapi docs-assets release.
if compgen -G "web/static/video/bowrain*" >/dev/null; then
  echo "✖ bowrain video assets present under web/static/video/ (remove them):"
  ls -d web/static/video/bowrain* 2>/dev/null || true
  fail=1
fi

if [ "$fail" -ne 0 ]; then
  echo ""
  echo "The neokapi docs site is downstream-clean by contract. Move bowrain content to bowrain/web/docs/."
  echo "Genuine cross-module facts may reference bowrain ONLY under web/docs/contribute/,"
  echo "or — capped at ${MAX_SANCTIONED_CALLOUTS} repo-wide (R9) — inside a '${CALLOUT_MARKER}' callout."
  exit 1
fi

echo "✓ neokapi docs are bowrain-clean"
