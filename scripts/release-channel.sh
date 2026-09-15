#!/usr/bin/env bash
#
# release-channel.sh — resolve the release channel a tag publishes to.
#
# Beta is a superset fast ring that also carries finals, so beta users never
# fall behind stable. See docs/internals/auto-update.md, "Release channels".
#   prerelease (1.2.0-rc25) → beta only.
#   final      (1.2.0)      → BOTH stable and beta.
#
# Outputs, in GITHUB_OUTPUT form on stdout:
#   channel       legacy single channel (beta | stable) — for messages.
#   channels      per-channel artifacts to publish (formulae, casks):
#                 "beta" for a prerelease, "stable beta" for a final.
#   reg_channel   the registry channel value: "beta" for a prerelease,
#                 "" (universal — matches every query) for a final.
#   latest        make_latest for the GitHub release: "true" only for a kapi
#                 CLI final tag (vX.Y.Z), "false" for every other shape. See
#                 scripts/check-plugin-release-latest.sh.
#
# One script rather than a `case` in each workflow, because the two release
# tracks feed it different shapes: release.yml tags `vX.Y.Z`, release-bowrain.yml
# tags `bowrain-vX.Y.Z`. A test for a hyphen anywhere in the tag answers the
# question correctly on the first and never on the second — the track prefix
# carries its own hyphen, so every bowrain tag reads as a prerelease and a final
# would publish beta only. The prefix is stripped here, once, and the test asks
# about the version.
#
# Usage: release-channel.sh <tag-or-version>   # e.g. bowrain-v1.2.0, 1.2.0-rc25
#        release-channel.sh --self-test        # every shape, both tracks
#
# Typical call, from a workflow step with an id:
#     bash scripts/release-channel.sh "$TAG" >> "$GITHUB_OUTPUT"
set -euo pipefail

# strip_prefix prints the bare version of a tag: the track prefix and the
# leading v go, a prerelease suffix stays. A tag already reduced to its version
# passes through unchanged, so a workflow may pass either.
strip_prefix() {
  local raw="$1"
  case "$raw" in
    # A track-prefixed tag: everything up to the "-v" that precedes the number.
    *-v[0-9]*) raw="${raw##*-v}" ;;
  esac
  printf '%s' "${raw#v}"
}

# resolve prints the outputs for one tag or version.
resolve() {
  local version latest=false
  version="$(strip_prefix "$1")"
  # Only the kapi CLI's final tag takes GitHub's latest. A prerelease, a
  # track-prefixed tag and a bare version, which names no track, never do.
  if [[ "$1" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    latest=true
  fi
  case "$version" in
    *-*)
      printf 'channel=beta\nchannels=beta\nreg_channel=beta\nlatest=%s\n' "$latest"
      ;;
    *)
      printf 'channel=stable\nchannels=stable beta\nreg_channel=\nlatest=%s\n' "$latest"
      ;;
  esac
}

# ── self-test ────────────────────────────────────────────────────────────────
#
# Every input shape both tracks can produce, because the defect this guards
# against is precisely that one track's shape differs from the other's and the
# resolution was written for only one of them.

SELFTEST_STATUS=0

# expect checks that a tag resolves to the named channels and latest flag.
expect() {
  local tag="$1" want_channel="$2" want_channels="$3" want_reg="$4" want_latest="$5"
  local out got_channel got_channels got_reg got_latest
  out="$(resolve "$tag")"
  got_channel="$(printf '%s\n' "$out" | sed -n 's/^channel=//p')"
  got_channels="$(printf '%s\n' "$out" | sed -n 's/^channels=//p')"
  got_reg="$(printf '%s\n' "$out" | sed -n 's/^reg_channel=//p')"
  got_latest="$(printf '%s\n' "$out" | sed -n 's/^latest=//p')"
  if [ "$got_channel" = "$want_channel" ] &&
    [ "$got_channels" = "$want_channels" ] &&
    [ "$got_reg" = "$want_reg" ] &&
    [ "$got_latest" = "$want_latest" ]; then
    echo "✓ self-test: ${tag} → ${want_channel} (${want_channels}), latest=${want_latest}"
    return
  fi
  echo "✖ self-test: ${tag} → channel=${got_channel} channels=${got_channels} reg_channel=${got_reg} latest=${got_latest}"
  echo "             expected channel=${want_channel} channels=${want_channels} reg_channel=${want_reg} latest=${want_latest}"
  SELFTEST_STATUS=1
}

# calls_this_script checks that a workflow resolves its channels here rather
# than inline. The inline copy is what drifted: release.yml's test was correct
# for its own tags and wrong for the ones release-bowrain.yml inherited it with,
# and nothing could see the difference from inside either file.
calls_this_script() {
  local workflow="$1" root
  root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
  local path="$root/.github/workflows/$workflow"
  if [ ! -f "$path" ]; then
    echo "✖ self-test: ${workflow} does not exist"
    SELFTEST_STATUS=1
    return
  fi
  if ! grep -q 'scripts/release-channel.sh' "$path"; then
    echo "✖ self-test: ${workflow} does not resolve its channel through this script"
    SELFTEST_STATUS=1
    return
  fi
  if grep -q 'echo "channels=' "$path"; then
    echo "✖ self-test: ${workflow} still writes a channel inline:"
    grep -n 'echo "channels=' "$path" | sed 's/^/    /'
    SELFTEST_STATUS=1
    return
  fi
  echo "✓ self-test: ${workflow} resolves its channel through this script"
}

self_test() {
  # The kapi track: a plain vX.Y.Z tag. Only its final takes GitHub's latest.
  expect "v1.2.0" stable "stable beta" "" true
  expect "v10.20.30" stable "stable beta" "" true
  expect "v1.2.0-rc25" beta "beta" "beta" false
  # The bowrain track: the same versions behind a prefix that carries a hyphen.
  expect "bowrain-v1.2.0" stable "stable beta" "" false
  expect "bowrain-v1.2.0-rc25" beta "beta" "beta" false
  # Either track may pass the version its setup job already stripped. A bare
  # version names no track, so it never claims latest.
  expect "1.2.0" stable "stable beta" "" false
  expect "1.2.0-rc25" beta "beta" "beta" false
  # A per-plugin track, whose prefix carries two hyphens.
  expect "kapi-asr-v0.3.0" stable "stable beta" "" false
  expect "kapi-asr-v0.3.0-rc1" beta "beta" "beta" false

  calls_this_script release.yml
  calls_this_script release-bowrain.yml

  return "$SELFTEST_STATUS"
}

main() {
  case "${1:-}" in
    --self-test)
      self_test
      ;;
    -h | --help | "")
      sed -n '/^# Usage:/,/^#        release-channel.sh --self-test/p' "$0" | sed 's/^# \{0,1\}//'
      [ -n "${1:-}" ]
      ;;
    *)
      resolve "$1"
      ;;
  esac
}

main "$@"
