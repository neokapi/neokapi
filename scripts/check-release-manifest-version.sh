#!/usr/bin/env bash
#
# check-release-manifest-version.sh: a plugin release tag and the manifest it
# was cut from name the same version.
#
# release-sourcecode.yml takes its version from the tag (sourcecode-v0.3.0 is
# 0.3.0): it stamps the binary, names the tarballs and registers that version.
# The manifest in each tarball is copied unchanged from the tagged commit. It is
# the version `kapi plugin list` shows, and the registry job reads
# min_kapi_version from it. A tag cut before the manifest was bumped would
# register the new version with the previous release's manifest and minimum
# kapi. The release runs this check before it builds, and fails such a tag.
#
# Usage: check-release-manifest-version.sh <tag> <manifest.json>
#        check-release-manifest-version.sh --self-test
set -euo pipefail

# check compares a <plugin>-vX.Y.Z tag with the manifest's `version`.
check() {
  local tag="$1" manifest="$2" tag_version manifest_version
  case "$tag" in
    *-v[0-9]*) tag_version="${tag##*-v}" ;;
    *)
      echo "check-release-manifest-version: ${tag} is not a <plugin>-vX.Y.Z tag" >&2
      return 1
      ;;
  esac
  if [ ! -f "$manifest" ]; then
    echo "check-release-manifest-version: ${manifest} does not exist" >&2
    return 1
  fi
  manifest_version="$(jq -r '.version // ""' "$manifest")"
  if [ "$tag_version" != "$manifest_version" ]; then
    echo "::error file=${manifest}::the tag ${tag} releases ${tag_version}, but ${manifest} declares version \"${manifest_version}\". Bump the manifest and tag the commit that carries it." >&2
    return 1
  fi
  echo "check-release-manifest-version: ${tag} matches ${manifest} (${manifest_version})"
}

# ── self-test ────────────────────────────────────────────────────────────────

SELFTEST_STATUS=0

# expect runs check on a tag and a manifest body and compares the outcome.
expect() {
  local want="$1" tag="$2" body="$3" dir rc
  dir="$(mktemp -d)"
  printf '%s\n' "$body" >"$dir/manifest.json"
  if check "$tag" "$dir/manifest.json" >/dev/null 2>&1; then rc=pass; else rc=fail; fi
  rm -rf "$dir"
  if [ "$rc" = "$want" ]; then
    echo "✓ self-test: ${tag} with ${body} → ${want}"
  else
    echo "✖ self-test: ${tag} with ${body} → ${rc}, expected ${want}"
    SELFTEST_STATUS=1
  fi
}

# calls_this_script checks that a release workflow runs the check on the
# manifest it packages, with the tag it was pushed as.
calls_this_script() {
  local workflow="$1" manifest="$2" root path
  root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
  path="$root/.github/workflows/$workflow"
  if grep -F 'scripts/check-release-manifest-version.sh' "$path" |
    grep -F '"$GITHUB_REF_NAME"' | grep -qF "$manifest"; then
    echo "✓ self-test: ${workflow} checks its tag against ${manifest}"
  else
    echo "✖ self-test: ${workflow} does not run this check on \"\$GITHUB_REF_NAME\" and ${manifest}"
    SELFTEST_STATUS=1
  fi
}

# committed_manifest_is_releasable checks the manifest a tag would be cut from
# carries the two fields a release reads from it.
committed_manifest_is_releasable() {
  local manifest="$1" root version min
  root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
  version="$(jq -r '.version // ""' "$root/$manifest")"
  min="$(jq -r '.min_kapi_version // ""' "$root/$manifest")"
  if [ -n "$version" ] && [ -n "$min" ]; then
    echo "✓ self-test: ${manifest} declares version ${version} and min_kapi_version ${min}"
  else
    echo "✖ self-test: ${manifest} declares version \"${version}\" and min_kapi_version \"${min}\""
    SELFTEST_STATUS=1
  fi
}

self_test() {
  expect pass "sourcecode-v0.3.0" '{"plugin":"sourcecode","version":"0.3.0"}'
  # The case this guards: a tag on a commit whose manifest was never bumped.
  expect fail "sourcecode-v0.3.0" '{"plugin":"sourcecode","version":"0.2.0"}'
  expect fail "sourcecode-v0.3.0-rc1" '{"plugin":"sourcecode","version":"0.3.0"}'
  expect pass "sourcecode-v0.3.0-rc1" '{"plugin":"sourcecode","version":"0.3.0-rc1"}'
  expect fail "sourcecode-v0.3.0" '{"plugin":"sourcecode"}'
  expect fail "v0.3.0" '{"plugin":"sourcecode","version":"0.3.0"}'

  calls_this_script release-sourcecode.yml plugins/sourcecode/manifest.json
  committed_manifest_is_releasable plugins/sourcecode/manifest.json

  return "$SELFTEST_STATUS"
}

main() {
  case "${1:-}" in
    --self-test)
      self_test
      ;;
    -h | --help | "")
      sed -n '/^# Usage:/,/^#        check-release-manifest-version.sh --self-test/p' "$0" | sed 's/^# \{0,1\}//'
      [ -n "${1:-}" ]
      ;;
    *)
      check "$1" "${2:?usage: check-release-manifest-version.sh <tag> <manifest.json>}"
      ;;
  esac
}

main "$@"
