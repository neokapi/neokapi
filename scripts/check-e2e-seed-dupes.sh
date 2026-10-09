#!/usr/bin/env bash
#
# Guard: the e2e seed documents live once, in bowrain/e2e/shared/seed-files/.
#
# Every e2e suite (the web app's, the desktop app's, the server's) uploads the
# same seed documents, and a second copy under an app's own e2e/ tree drifts
# the moment one of them is edited. The shared tree is the superset and the
# canonical location, so an app-local file whose bytes equal a shared one is a
# copy and fails here.
#
# What fails:
#     a file under bowrain/apps/web/e2e/ with the same content hash as a file
#     under bowrain/e2e/shared/seed-files/, whatever either is named
#
# What passes:
#     identical files within one tree (not this guard's concern), and files
#     that merely share a name
#
# Usage:
#     ./scripts/check-e2e-seed-dupes.sh              # scan tracked files
#     ./scripts/check-e2e-seed-dupes.sh --self-test  # prove the matcher both ways
#
# Wired into `make check-e2e-seed-dupes` (part of `make lint`), `make pre-push`,
# and the repo-guards job in .github/workflows/ci.yml.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"

readonly SHARED_ROOT=bowrain/e2e/shared/seed-files
readonly APP_ROOT=bowrain/apps/web/e2e

# ── the matcher ──────────────────────────────────────────────────────────────

sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum -- "$1" | cut -d' ' -f1
  else
    shasum -a 256 -- "$1" | cut -d' ' -f1
  fi
}

# hash_tree prints "<sha256> <path>" for every file under the root given as
# $1, found with the lister named in $2 (git_files or all_files).
hash_tree() {
  local f
  "$2" "$1" | while IFS= read -r -d '' f; do
    printf '%s %s\n' "$(sha256_of "$f")" "$f"
  done | sort
}

# shellcheck disable=SC2329  # invoked by name through hash_tree
git_files() { git ls-files -z -- "$1"; }
# shellcheck disable=SC2329  # invoked by name through hash_tree
all_files() { find "$1" -type f -print0 | sort -z; }

# scan_roots prints "a == b (sha256 …)" for every pair of files, one under
# each root, whose content hashes match. Returns 1 if it printed anything,
# 0 if clean.
scan_roots() {
  local a="$1" b="$2" lister="$3" hits
  hits=$(join -j 1 <(hash_tree "$a" "$lister") <(hash_tree "$b" "$lister") |
    awk '{ printf "%s == %s (sha256 %s)\n", $2, $3, substr($1, 1, 12) }')
  [ -z "$hits" ] && return 0
  printf '%s\n' "$hits"
  return 1
}

# ── self-test ────────────────────────────────────────────────────────────────

self_test() {
  local tmp status=0
  tmp="$(mktemp -d)"
  # shellcheck disable=SC2064  # expand $tmp now, not at trap time
  trap "rm -rf '$tmp'" EXIT

  mkdir -p "$tmp/shared" "$tmp/app/nested" "$tmp/app2"
  printf '<h1>About</h1>\n' >"$tmp/shared/about.html"
  printf '{"k":"v"}\n' >"$tmp/shared/strings.json"
  printf '# Notes\n' >"$tmp/shared/notes.md"
  # A planted copy under a different name, two levels down.
  printf '<h1>About</h1>\n' >"$tmp/app/nested/copy.html"
  # Same name as a shared file, different bytes: not a copy.
  printf '{"k":"other"}\n' >"$tmp/app/strings.json"
  # Two identical files within the app tree: outside this guard's scope.
  printf 'same\n' >"$tmp/app2/one.txt"
  printf 'same\n' >"$tmp/app2/two.txt"

  local out
  if out=$(scan_roots "$tmp/shared" "$tmp/app" all_files); then
    echo "✖ self-test: the matcher did NOT flag a planted byte-identical copy"
    status=1
  else
    local n
    n=$(printf '%s\n' "$out" | grep -c . || true)
    if [ "$n" -ne 1 ] || ! printf '%s' "$out" | grep -q 'about.html == .*copy.html'; then
      echo "✖ self-test: expected exactly the planted copy, got:"
      printf '%s\n' "$out"
      status=1
    else
      echo "✓ self-test: flags a byte-identical copy under another name (1 hit)"
    fi
  fi

  if out=$(scan_roots "$tmp/shared" "$tmp/app2" all_files); then
    echo "✓ self-test: duplicates within one tree and shared names with different bytes pass"
  else
    echo "✖ self-test: the matcher flagged content that is not a cross-tree copy:"
    printf '%s\n' "$out"
    status=1
  fi

  return "$status"
}

# ── main ─────────────────────────────────────────────────────────────────────

cd "$root"

if [ "${1:-}" = "--self-test" ]; then
  self_test
  exit $?
fi

# Run the self-test first: a matcher that silently stops matching is worse than
# no check at all, and it costs milliseconds.
if ! self_test >/dev/null; then
  echo "✖ check-e2e-seed-dupes.sh: self-test failed — the matcher is broken."
  self_test || true
  exit 1
fi

if hits=$(scan_roots "$SHARED_ROOT" "$APP_ROOT" git_files); then
  echo "✓ no e2e seed file is duplicated between $SHARED_ROOT and $APP_ROOT"
  exit 0
fi

echo "✖ e2e seed file(s) duplicated across the two trees:"
printf '%s\n' "$hits"
echo ""
echo "The shared tree is canonical. Point the suite at $SHARED_ROOT"
echo "(see bowrain/apps/web/e2e/helpers/api-client.ts) and delete the copy."
exit 1
