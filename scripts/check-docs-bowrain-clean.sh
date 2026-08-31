#!/usr/bin/env bash
#
# Guard: the neokapi (kapi / framework) docs must contain ZERO bowrain
# references, with ONE narrow, capped exception (R9): a sanctioned
# ":::note[Works with Bowrain]" admonition at defined seams — currently the
# kapi-bowrain plugin reference entry, governed team review, cross-project
# context, and the CI loop page. R9 amends the no-mention rule to allow
# AT MOST `MAX_SANCTIONED_CALLOUTS` of these neutral callouts; it does not
# open the door to bowrain mentions anywhere else. bowrain is otherwise a
# strictly DOWNSTREAM product (F-01); its docs live in bowrain/web/docs/.
#
# The architecture corpus and the implementation notes are held to the same
# zero: they describe the framework and nothing downstream of it, and no Apache
# module reaches a package under bowrain/ (asserted by make check-module-
# boundaries), so no cross-module fact needs the name. The one carve-out is
# web/docs/contribute/implementation/repo/, whose notes describe this
# monorepo's own build and publishing and may name its bowrain/ directory.
#
# Everything else user-facing (framework/kapi/react/reference/toolbox and the
# docs home page, which is also the product landing page) must not mention
# bowrain outside a sanctioned callout.
#
# The measurement datasets under web/src/pages/*/_*.json are exempt, and they
# are the only exemption by file rather than by tree. They are generated, not
# authored: a scenario prompt, an agent transcript, a converter's name. The rule
# above governs what kapi SAYS about itself, and a dataset says what was
# measured. Suppressing a name there does not make the claim narrower, it makes
# the evidence partial, and skilleval used to blank a whole result's transcripts
# to satisfy this sweep. Whether a scenario should name the platform at all is a
# question for the scenario, in scripts/skilleval/scenarios.go, where a person
# writes it.
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
  web/docs/contribute/architecture
  web/docs/contribute/implementation
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
done < <(find "${user_facing[@]}" -type f ! -name '_*.json' \
  ! -path 'web/docs/contribute/implementation/repo/*' -print0 2>/dev/null)

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
  echo "Only web/docs/contribute/implementation/repo/ (this monorepo's own build notes) may name bowrain,"
  echo "or, capped at ${MAX_SANCTIONED_CALLOUTS} repo-wide (R9), a '${CALLOUT_MARKER}' callout."
  exit 1
fi

echo "✓ neokapi docs are bowrain-clean"
