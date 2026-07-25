#!/usr/bin/env bash
#
# Guard: every registry resolution in pnpm-lock.yaml must keep its `tarball:` URL.
#
# pnpm records each package as
#
#     resolution: {integrity: sha512-…, tarball: https://registry.npmjs.org/…}
#
# The `tarball:` field pins the exact URL the bytes came from. Without it pnpm
# derives the URL from whatever registry configuration happens to be in effect
# at install time, so the lockfile no longer states where a package was
# resolved from — it only states what its hash was. That is a weaker guarantee
# than this repository maintains elsewhere (Dependabot cooldown windows,
# pnpm `minimumReleaseAge`, cosign verification of plugin releases).
#
# Why this is a hard check and not an auto-fix:
#
#   Dependabot resolves the lockfile with its own bundled pnpm, which omits
#   `tarball:`. The pinned pnpm (package.json#packageManager) is
#   format-preserving in BOTH directions — given a lockfile with the field it
#   keeps it, given one without it does not add it back. So the information is
#   destroyed by the strip and cannot be recovered by re-resolving. An
#   auto-relock workflow reports "already matches" and passes, which is worse
#   than no guard at all: it looks like protection while the provenance is
#   already gone.
#
#   Two such PRs (#1414, #1418) merged green before this check existed. Nothing
#   in CI covered lockfile provenance, and the stripped lockfile installs fine,
#   so every signal said yes.
#
# The invariant is exact: as of this writing all resolutions in pnpm-lock.yaml
# carry both `integrity:` and `tarball:` — there are no legitimate
# integrity-only registry entries to carve out. Non-registry resolutions (git,
# file, directory, link) do not use this shape at all and so never match.
#
# If a future pnpm legitimately changes the lockfile format, update this guard
# deliberately in the same commit that changes the format — do not silence it.
#
# Remediation when this fails:
#     vp install --lockfile-only     # vp resolves the pinned pnpm
# on a lockfile that still HAS the fields. If they are already gone, recover
# them from the last good commit first:
#     git checkout <good-ref> -- pnpm-lock.yaml && vp install --lockfile-only
#
# Usage:
#     ./scripts/check-lockfile-provenance.sh              # check the lockfile
#     ./scripts/check-lockfile-provenance.sh --self-test  # prove it both ways

set -euo pipefail

LOCKFILE="${LOCKFILE:-pnpm-lock.yaml}"

# A resolution line for a registry package. Indentation is pnpm's own.
RESOLUTION_RE='^[[:space:]]*resolution: \{integrity: '

count_resolutions() { grep -cE "$RESOLUTION_RE" "$1" || true; }
count_without_tarball() { grep -E "$RESOLUTION_RE" "$1" | grep -vc 'tarball: ' || true; }

check_file() {
  local f="$1" total missing
  total="$(count_resolutions "$f")"
  missing="$(count_without_tarball "$f")"

  if [[ "$total" -eq 0 ]]; then
    echo "error: no registry resolutions found in $f — is this a pnpm lockfile?" >&2
    return 1
  fi

  if [[ "$missing" -gt 0 ]]; then
    echo "error: $missing of $total registry resolutions in $f have no 'tarball:' URL." >&2
    echo >&2
    echo "The lockfile no longer records where these packages were resolved from." >&2
    echo "This is usually a Dependabot npm PR: its bundled pnpm omits the field," >&2
    echo "and the pinned pnpm cannot add it back, so re-resolving will not fix it." >&2
    echo >&2
    # Collect before printing: piping grep into `head` inside a `pipefail`
    # shell makes the function exit 141 (SIGPIPE) instead of 1.
    local offenders
    offenders="$(grep -nE "$RESOLUTION_RE" "$f" | grep -v 'tarball: ' || true)"
    echo "First offending entries:" >&2
    printf '%s\n' "$offenders" | sed -n '1,5p' >&2
    echo >&2
    echo "Recover the field from the last good commit, then re-resolve:" >&2
    echo "  git checkout <good-ref> -- $f && vp install --lockfile-only" >&2
    return 1
  fi

  echo "pnpm-lock.yaml: all $total registry resolutions carry a tarball URL"
}

self_test() {
  local tmp good bad rc=0
  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' RETURN
  good="$tmp/good.yaml"
  bad="$tmp/bad.yaml"

  cat >"$good" <<'EOF'
packages:
  '@scope/pkg@1.0.0':
    resolution: {integrity: sha512-AAA==, tarball: https://registry.npmjs.org/@scope/pkg/-/pkg-1.0.0.tgz}
  other@2.0.0:
    resolution: {integrity: sha512-BBB==, tarball: https://registry.npmjs.org/other/-/other-2.0.0.tgz}
EOF

  # The exact mutation Dependabot's resolver performs: drop `tarball:`.
  sed 's/, tarball: [^}]*}/}/' "$good" >"$bad"

  echo "self-test: a lockfile WITH tarball URLs must pass"
  if ! LOCKFILE="$good" check_file "$good" >/dev/null; then
    echo "  FAIL: rejected a good lockfile" >&2
    rc=1
  else
    echo "  ok"
  fi

  echo "self-test: a lockfile WITHOUT tarball URLs must fail"
  if LOCKFILE="$bad" check_file "$bad" >/dev/null 2>&1; then
    echo "  FAIL: accepted a stripped lockfile" >&2
    rc=1
  else
    echo "  ok"
  fi

  [[ "$rc" -eq 0 ]] && echo "self-test passed"
  return "$rc"
}

case "${1:-}" in
  --self-test) self_test ;;
  "") check_file "$LOCKFILE" ;;
  *)
    echo "usage: $0 [--self-test]" >&2
    exit 2
    ;;
esac
