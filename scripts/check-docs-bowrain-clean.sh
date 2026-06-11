#!/usr/bin/env bash
#
# Guard: the neokapi (kapi / framework) docs must contain ZERO bowrain
# references. bowrain is a strictly DOWNSTREAM product (AD-001); its docs live in
# bowrain/web/docs/. A NARROW exception is allowed only under
# web/docs/contribute/ (architecture + notes-internal), where genuine
# cross-module facts may name bowrain — e.g. kapi-desktop's blank-import of
# bowrain/plugin/schema, the kapi-*/bowrain-* skill split, and the module tree.
# Everything user-facing (framework/kapi/react/reference/toolbox and the docs
# home page, which is also the product landing page) must not mention bowrain.
#
# See docs-intent-impl-audit.md (WS1) for the rationale.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

fail=0

# 1. No bowrain in user-facing neokapi docs.
user_facing=(
  web/docs/framework
  web/docs/kapi
  web/docs/react
  web/docs/reference
  web/docs/toolbox
  web/src
)
if hits=$(grep -rinE 'bowrain' "${user_facing[@]}" 2>/dev/null); then
  echo "✖ bowrain reference(s) found in user-facing neokapi docs (must be zero):"
  echo "$hits"
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
  echo "Genuine cross-module facts may reference bowrain ONLY under web/docs/contribute/."
  exit 1
fi

echo "✓ neokapi docs are bowrain-clean"
