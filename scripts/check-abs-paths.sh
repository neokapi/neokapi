#!/usr/bin/env bash
#
# Guard: no absolute home directory may be committed to this repository.
#
# A path like /Users/<someone>/src/... only resolves on one laptop. Baked into
# a Makefile, a script, a workflow, a test fixture or a doc, it silently breaks
# every other clone — and, in a generated artefact, leaks the author's
# directory layout. Locations outside the repository are named by environment
# variable with a repo-relative default instead
# (web/docs/contribute/workspace-paths.md).
#
# What fails:
#     /Users/<name>            macOS home directory
#     /home/<name>/            Linux home directory (trailing slash required —
#                              see the note on false positives below)
#     C:\Users\<name>          Windows profile directory (any escaping depth)
#
# What passes, deliberately and narrowly:
#
#   1. PLACEHOLDER_USERS — names that are documentation stand-ins, not real
#      accounts. They appear in CLI help transcripts, Storybook fixtures and
#      unit-test tables where a plausible absolute path is the point.
#
#   2. SYSTEM_HOMES — fixed, machine-independent paths that merely live under
#      /home/. These are not anybody's home directory.
#
#   3. VENDORED_FILES — vendored or generated files that carry an upstream
#      author's home path in content this repository does not author. Excluded
#      wholesale rather than by name, so a real leak in a file we DO author is
#      still caught.
#
# `/home/<name>` is matched only with a trailing slash. Without that, the check
# fires on i18next resource keys ("/home/title") and on URLs
# ("http://uli.unicode.org/home/uli-documents"), neither of which is a path.
# The trade-off is that a bare "/home/someone" with nothing after it is not
# caught; every real-world instance has a path after it.
#
# Usage:
#     ./scripts/check-abs-paths.sh              # scan tracked files
#     ./scripts/check-abs-paths.sh --self-test  # prove the matcher both ways
#
# Wired into `make check-abs-paths` (part of `make lint`), `make pre-push`, and
# the repo-guards job in .github/workflows/ci.yml.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"

# ── the matcher ──────────────────────────────────────────────────────────────

# A user name: starts alphanumeric, so "/Users/..." in an elided example path
# ("Path is like /Users/.../MyApp/kapi.yaml") is not read as a name.
readonly NAME='[A-Za-z0-9][A-Za-z0-9._-]*'
readonly ABS_HOME_RE="(/Users/${NAME}|/home/${NAME}/|[A-Za-z]:\\\\+Users\\\\+${NAME})"

# 1. Documentation / fixture placeholders.
readonly PLACEHOLDER_USERS='me|dev|demo|user|test|you'
# 2. Fixed system paths that live under /home/ but are not a user's home:
#    linuxbrew — the Linux Homebrew prefix (/home/linuxbrew/.linuxbrew),
#                asserted by cli/selfupdate source detection;
#    runner    — the GitHub Actions runner's working home;
#    node      — the node Docker images' unprivileged user.
readonly SYSTEM_HOMES='linuxbrew|runner|node'
readonly ALLOWED_NAMES="${PLACEHOLDER_USERS}|${SYSTEM_HOMES}"

# 3. Vendored / generated content authored elsewhere. Excluded by path so a
#    real leak in a file we DO author is still caught. (The reasons are spelled
#    out without quoting the offending paths — this script scans itself.)
readonly VENDORED_FILES=(
  # The SQLite amalgamation, verbatim from sqlite.org. Its documentation
  # comments use a Linux home directory as the database-URI example.
  core/storage/sqlite3.h
  core/storage/sqlite3-binding.h
  # Generated from upstream OpenXLIFF fixture files, whose `<file original="…">`
  # attributes record the fixture author's own checkout.
  cli/parity/formats/fixtures_xliff_generated.go
  # Generated parity output: quotes upstream Okapi fixture bytes verbatim,
  # including `original="…"` attributes holding Linux and Windows home paths.
  web/static/data/parity-fixtures.json
)

# scan_files reads NUL-separated paths on stdin and prints "file:line:match"
# for every absolute home path that is not allowlisted. Returns 1 if it printed
# anything, 0 if clean.
scan_files() {
  local hits
  # -I skips binary files; -o reports one line per match, so a line holding both
  # an allowed and a disallowed path is judged per match; -H forces the filename
  # prefix, which grep otherwise omits when an xargs batch holds a single file.
  hits=$(xargs -0 grep -HnoIE "$ABS_HOME_RE" -- 2>/dev/null |
    grep -vE "(/Users/|/home/|Users\\\\+)(${ALLOWED_NAMES})/?\$" || true)
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

  # The leak fixture is interpolated, not a literal heredoc: spelling a real
  # home path out in this file would make the guard flag itself once the script
  # is tracked. "$u" is not in the matcher's name class, so the source form is
  # inert while the file it generates is a genuine leak.
  local u=somebody
  cat >"$tmp/leak.txt" <<EOF
OKAPI_REPO ?= /Users/$u/src/okapi/Okapi
export CACHE=/home/$u/.cache/kapi
set WINPATH=C:\\Users\\$u\\AppData
EOF

  cat >"$tmp/clean.txt" <<'EOF'
# Project root: /Users/me/my-project
projectPath: "/Users/dev/projects/acme/kapi.yaml"
browsePath: "/Users/demo/exports/output.tmx"
resolvedPath="/home/user/project/resources/my-tm.db"
{"/home/linuxbrew/.linuxbrew/bin/kapi", SourceHomebrew},
i18nextKey := "/home/title"
seeAlso: http://uli.unicode.org/home/uli-documents
comment: // Path is like /Users/.../MyApp/kapi.yaml
windows: "Chemin : C:\\Users\\test"
EOF

  local out
  if out=$(printf '%s\0' "$tmp/leak.txt" | scan_files); then
    echo "✖ self-test: the matcher did NOT flag a planted absolute home path"
    status=1
  else
    local n
    n=$(printf '%s\n' "$out" | grep -c . || true)
    if [ "$n" -ne 3 ]; then
      echo "✖ self-test: expected 3 hits in the planted file, got ${n}:"
      printf '%s\n' "$out"
      status=1
    else
      echo "✓ self-test: flags /Users/, /home/ and C:\\Users\\ leaks (3 hits)"
    fi
  fi

  if out=$(printf '%s\0' "$tmp/clean.txt" | scan_files); then
    echo "✓ self-test: allowlisted placeholders, system homes and non-paths pass"
  else
    echo "✖ self-test: the matcher flagged allowlisted content:"
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
  echo "✖ check-abs-paths.sh: self-test failed — the matcher is broken."
  self_test || true
  exit 1
fi

# Scan tracked files only. Untracked build output and node_modules are not our
# problem; .gitignore'd machine-local files (e.g. .claude/settings.local.json)
# are expected to hold real paths.
exclude_re="$(printf '%s\n' "${VENDORED_FILES[@]}" | paste -sd'|' -)"

if hits=$(git ls-files -z |
  tr '\0' '\n' | grep -vxE "$exclude_re" | tr '\n' '\0' |
  scan_files); then
  echo "✓ no absolute home paths in tracked files"
  exit 0
fi

echo "✖ absolute home path(s) found in tracked files:"
printf '%s\n' "$hits"
echo ""
echo "Absolute home paths resolve on exactly one machine. Name the location"
echo "with an environment variable that has a repo-relative default instead:"
echo ""
echo "  Makefile      NEOKAPI_WORKSPACE_DIR ?= \$(abspath \$(CURDIR)/..)"
echo "  shell         : \"\${NEOKAPI_WORKSPACE_DIR:=\$(cd \"\$ROOT/..\" && pwd)}\""
echo "  prose         \$NEOKAPI_WORKSPACE_DIR/<repo>/…"
echo ""
echo "See web/docs/contribute/workspace-paths.md."
exit 1
