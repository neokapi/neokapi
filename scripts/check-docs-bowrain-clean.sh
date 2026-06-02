#!/usr/bin/env bash
#
# Guard: the neokapi (kapi / framework) docs + landing must contain ZERO bowrain
# references. bowrain is a strictly DOWNSTREAM product (AD-001); its docs live in
# bowrain/web/docs/. A NARROW exception is allowed only under
# web/docs/docs/contribute/ (architecture + notes-internal), where genuine
# cross-module facts may name bowrain — e.g. kapi-desktop's blank-import of
# bowrain/plugin/schema, the kapi-*/bowrain-* skill split, and the module tree.
# Everything user-facing (framework/kapi/react/reference/toolbox, the docs
# homepage components, and the marketing landing) must not mention bowrain.
#
# See docs-intent-impl-audit.md (WS1) for the rationale.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

fail=0

# 1. No bowrain in user-facing neokapi docs/landing.
user_facing=(
  web/docs/docs/framework
  web/docs/docs/kapi
  web/docs/docs/react
  web/docs/docs/reference
  web/docs/docs/toolbox
  web/docs/src
  web/landing/src
  web/landing/index.html
)
if hits=$(grep -rinE 'bowrain' "${user_facing[@]}" 2>/dev/null); then
  echo "✖ bowrain reference(s) found in user-facing neokapi docs/landing (must be zero):"
  echo "$hits"
  fail=1
fi

# 2. No bowrain video assets staged in the neokapi static tree. They belong to
#    the bowrain site; publish-docs-assets 'merges, never drops', so any left
#    here get shipped into the neokapi docs-assets release.
if compgen -G "web/docs/static/video/bowrain*" >/dev/null; then
  echo "✖ bowrain video assets present under web/docs/static/video/ (remove them):"
  ls -d web/docs/static/video/bowrain* 2>/dev/null || true
  fail=1
fi

if [ "$fail" -ne 0 ]; then
  echo ""
  echo "The neokapi docs site is downstream-clean by contract. Move bowrain content to bowrain/web/docs/."
  echo "Genuine cross-module facts may reference bowrain ONLY under web/docs/docs/contribute/."
  exit 1
fi

echo "✓ neokapi docs/landing are bowrain-clean"
