#!/usr/bin/env bash
#
# Guard: every publishable npm package declares its license AND ships the text.
#
# Two separate things have to be true for a package on npm to be properly
# licensed, and they fail independently:
#
#   1. `license` in package.json — the SPDX identifier. This is the only thing
#      the registry UI, `npm view`, and automated license scanners read. A
#      package without it is reported as UNLICENSED / unknown.
#   2. A LICENSE file in the tarball — the license *text*. Apache-2.0 §4(a)
#      requires a copy of the License to be distributed with the work; the SPDX
#      string in package.json is a label, not the grant.
#
# Both defects have already shipped from this repository, one of each kind:
#
#   * @neokapi/kapi-format@1.0.1 published with NO `license` field at all. The
#     LICENSE file was sitting in the package directory the whole time — the
#     text shipped, the declaration did not, so scanners saw an undeclared
#     package.
#   * @neokapi/i18n-react-lint had no LICENSE file in its package directory
#     (0.1.0 and 0.1.1 both), yet the published tarballs contain one. pnpm
#     substitutes the *workspace-root* LICENSE when a workspace package has
#     none, so `pnpm pack` quietly papered over the gap. That fallback is not
#     something to rely on: it is pnpm-specific and workspace-specific, so a
#     bare `npm publish`, a `npm pack`, or packing the directory outside the
#     workspace all ship no license text at all.
#
# Note what does NOT help. i18n-react-lint listed "LICENSE" in its `files`
# array the entire time. npm silently ignores `files` entries that do not
# exist, so a declared-but-absent LICENSE is indistinguishable from a correct
# one by reading package.json alone. Only the file's presence on disk counts.
#
# What fails, for every tracked package.json that is not `"private": true`:
#
#   - no `license` field, or a `license` that is not a non-empty string. The
#     two deprecated spellings — the `{"type": …, "url": …}` object and the
#     `licenses` array — are rejected as well; modern npm reads neither, so a
#     package using them is undeclared in practice.
#   - no LICENSE file beside the package.json
#   - a declared `files` array that does not list LICENSE
#
# The third rule is belt-and-braces: npm's packer force-includes LICENSE
# regardless of `files`, so omitting it is not itself a defect. It is required
# here so that the tarball contents are legible from package.json alone,
# instead of resting on packer behaviour that differs between npm and pnpm.
#
# This guard deliberately does NOT validate the SPDX identifier against the
# license text. Picking the wrong identifier is worse than omitting it, and
# that judgement stays with a human — the packages here are Apache-2.0
# (framework and shared frontend) except under bowrain/, which is AGPL-3.0.
# Today every non-private package is under packages/; nothing under bowrain/ is
# publishable, so no AGPL package is currently in scope.
#
# Usage:
#     ./scripts/check-package-licenses.sh              # scan tracked packages
#     ./scripts/check-package-licenses.sh --self-test  # prove it both ways
#
# Wired into `make check-package-licenses` (part of `make lint`) and the
# repo-guards job in .github/workflows/ci.yml.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"

# ── the checker ──────────────────────────────────────────────────────────────

# read_manifests parses each package.json given on stdin (one relative path per
# line, resolved against $1) and prints one TAB-separated record per file:
#
#     <relpath>  <private|public>  <license-state>  <files-state>
#
# license-state: "ok" or a reason. files-state: "absent" (no files array),
# "listed", or "unlisted". Parsing is a real JSON parse, not grep: `license`
# and `private` are top-level keys, but a package.json is still free-form JSON
# and may be formatted any way at all.
#
# The program goes in via `-c`, not `python3 -`, so that stdin stays free for
# the path list.
READ_MANIFESTS_PY=$(
  cat <<'PY'
import json, os, sys

base = sys.argv[1]
for line in sys.stdin:
    rel = line.strip()
    if not rel:
        continue
    path = os.path.join(base, rel)
    try:
        with open(path, encoding="utf-8") as fh:
            pkg = json.load(fh)
    except (OSError, ValueError) as err:
        print("\t".join([rel, "public", "unparsable: %s" % err, "absent"]))
        continue

    if not isinstance(pkg, dict):
        print("\t".join([rel, "public", "unparsable: top level is not an object", "absent"]))
        continue

    visibility = "private" if pkg.get("private") is True else "public"

    lic = pkg.get("license")
    if lic is None:
        # `licenses` (plural, array) is npm's long-deprecated form. Name it
        # explicitly so the fix is obvious rather than looking like a typo.
        if "licenses" in pkg:
            lic_state = "uses the deprecated `licenses` array; use a single `license` SPDX string"
        else:
            lic_state = "no `license` field"
    elif isinstance(lic, dict):
        lic_state = "uses the deprecated `{type, url}` license object; use an SPDX string"
    elif not isinstance(lic, str) or not lic.strip():
        lic_state = "`license` is not a non-empty string"
    else:
        lic_state = "ok"

    files = pkg.get("files")
    if not isinstance(files, list):
        files_state = "absent"
    else:
        listed = False
        for entry in files:
            if not isinstance(entry, str):
                continue
            entry = entry.strip()
            # Negations ("!src/*.test.ts") exclude, they do not include.
            if entry.startswith("!"):
                continue
            if os.path.basename(entry).upper().startswith("LICEN"):
                listed = True
                break
        files_state = "listed" if listed else "unlisted"

    print("\t".join([rel, visibility, lic_state, files_state]))
PY
)

read_manifests() {
  python3 -c "$READ_MANIFESTS_PY" "$1"
}

# scan_packages checks every package.json path read from stdin, relative to the
# root given as $1. Prints one "path: problem" line per violation. Returns 1 if
# it printed anything, 0 if clean.
scan_packages() {
  local base="$1" problems=() rel visibility lic_state files_state dir

  while IFS=$'\t' read -r rel visibility lic_state files_state; do
    [ -n "$rel" ] || continue

    # A private package is never published, so it needs neither declaration nor
    # text. Most manifests in this repo are apps and config roots, not packages.
    if [ "$visibility" = "private" ]; then
      continue
    fi

    if [ "$lic_state" != "ok" ]; then
      problems+=("$rel: $lic_state")
    fi

    dir="$(dirname "$rel")"
    if [ ! -f "$base/$dir/LICENSE" ]; then
      problems+=("$rel: no LICENSE file beside it (in $dir/)")
    fi

    if [ "$files_state" = "unlisted" ]; then
      problems+=("$rel: declares a \`files\` array that does not list LICENSE")
    fi
  done < <(read_manifests "$base")

  [ ${#problems[@]} -eq 0 ] && return 0
  printf '%s\n' "${problems[@]}"
  return 1
}

# list_tracked_manifests prints every tracked package.json, one per line.
# node_modules is excluded: a committed dependency tree is not ours to license.
list_tracked_manifests() {
  git -C "$root" ls-files -- '*package.json' | grep -v '/node_modules/' || true
}

# ── self-test ────────────────────────────────────────────────────────────────

self_test() {
  local tmp status=0 out n
  tmp="$(mktemp -d)"
  # shellcheck disable=SC2064  # expand $tmp now, not at trap time
  trap "rm -rf '$tmp'" RETURN

  # One correct package, and one of every failure mode. The private package is
  # deliberately as broken as the others: it must still pass.
  mkdir -p "$tmp"/{good,no-files-array,no-field,no-file,files-unlisted,private-exempt,object-license,licenses-array}

  cat >"$tmp/good/package.json" <<'EOF'
{
  "name": "@scope/good",
  "private": false,
  "license": "Apache-2.0",
  "files": ["dist", "LICENSE"]
}
EOF
  echo "Apache License, Version 2.0 …" >"$tmp/good/LICENSE"

  # No `files` array at all: npm then ships everything not ignored, LICENSE
  # included. Nothing to assert, so this must pass — the files rule only bites
  # a package that opts into an explicit allowlist.
  cat >"$tmp/no-files-array/package.json" <<'EOF'
{
  "name": "@scope/no-files-array",
  "private": false,
  "license": "MIT"
}
EOF
  echo "MIT License …" >"$tmp/no-files-array/LICENSE"

  cat >"$tmp/no-field/package.json" <<'EOF'
{
  "name": "@scope/no-field",
  "private": false,
  "files": ["dist", "LICENSE"]
}
EOF
  echo "Apache License, Version 2.0 …" >"$tmp/no-field/LICENSE"

  # The exact shape i18n-react-lint had: declares LICENSE, does not have one.
  cat >"$tmp/no-file/package.json" <<'EOF'
{
  "name": "@scope/no-file",
  "private": false,
  "license": "Apache-2.0",
  "files": ["dist", "LICENSE"]
}
EOF

  cat >"$tmp/files-unlisted/package.json" <<'EOF'
{
  "name": "@scope/files-unlisted",
  "private": false,
  "license": "Apache-2.0",
  "files": ["dist", "!src/*.test.ts"]
}
EOF
  echo "Apache License, Version 2.0 …" >"$tmp/files-unlisted/LICENSE"

  cat >"$tmp/object-license/package.json" <<'EOF'
{
  "name": "@scope/object-license",
  "private": false,
  "license": {"type": "Apache-2.0", "url": "https://example.invalid/LICENSE"},
  "files": ["dist", "LICENSE"]
}
EOF
  echo "Apache License, Version 2.0 …" >"$tmp/object-license/LICENSE"

  cat >"$tmp/licenses-array/package.json" <<'EOF'
{
  "name": "@scope/licenses-array",
  "private": false,
  "licenses": [{"type": "Apache-2.0"}],
  "files": ["dist", "LICENSE"]
}
EOF
  echo "Apache License, Version 2.0 …" >"$tmp/licenses-array/LICENSE"

  cat >"$tmp/private-exempt/package.json" <<'EOF'
{
  "name": "@scope/private-exempt",
  "private": true,
  "files": ["dist"]
}
EOF

  # Each broken package must be flagged, on its own, for the right reason.
  local case_dir expect
  for case_dir in no-field no-file files-unlisted object-license licenses-array; do
    # The backticks are literal characters in the expected message, so the
    # single quotes are deliberate — nothing here should expand.
    # shellcheck disable=SC2016
    case "$case_dir" in
      no-field) expect='no `license` field' ;;
      no-file) expect='no LICENSE file beside it' ;;
      files-unlisted) expect='does not list LICENSE' ;;
      object-license) expect='deprecated `{type, url}` license object' ;;
      licenses-array) expect='deprecated `licenses` array' ;;
    esac
    if out=$(printf '%s/package.json\n' "$case_dir" | scan_packages "$tmp"); then
      echo "✖ self-test: did NOT flag $case_dir/"
      status=1
    elif ! printf '%s' "$out" | grep -qF "$expect"; then
      echo "✖ self-test: flagged $case_dir/ for the wrong reason:"
      printf '%s\n' "$out" | sed 's/^/  /'
      status=1
    else
      echo "✓ self-test: flags $case_dir/ ($expect)"
    fi
  done

  # Correct packages — with and without a `files` array — and a (broken)
  # private one must all pass.
  for case_dir in good no-files-array private-exempt; do
    if out=$(printf '%s/package.json\n' "$case_dir" | scan_packages "$tmp"); then
      echo "✓ self-test: passes $case_dir/"
    else
      echo "✖ self-test: flagged $case_dir/, which is correct as written:"
      printf '%s\n' "$out" | sed 's/^/  /'
      status=1
    fi
  done

  # And all of them together must produce exactly the five violations above —
  # proving the scan does not stop at the first bad package.
  if out=$(find "$tmp" -name package.json | sed "s|^$tmp/||" | scan_packages "$tmp"); then
    echo "✖ self-test: a tree containing five broken packages passed"
    status=1
  else
    n=$(printf '%s\n' "$out" | grep -c . || true)
    if [ "$n" -ne 5 ]; then
      echo "✖ self-test: expected 5 violations across the tree, got ${n}:"
      printf '%s\n' "$out" | sed 's/^/  /'
      status=1
    else
      echo "✓ self-test: reports all 5 violations in one pass"
    fi
  fi

  return "$status"
}

# ── main ─────────────────────────────────────────────────────────────────────

cd "$root"

if [ "${1:-}" = "--self-test" ]; then
  self_test
  exit $?
fi

# Run the self-test first: a check that silently stops checking is worse than
# no check at all, and it costs milliseconds.
if ! self_test >/dev/null; then
  echo "✖ check-package-licenses.sh: self-test failed — the checker is broken."
  self_test || true
  exit 1
fi

if hits=$(list_tracked_manifests | scan_packages "$root"); then
  n=$(list_tracked_manifests | wc -l | tr -d ' ')
  echo "✓ every publishable package declares a license and ships its text (${n} manifests scanned)"
  exit 0
fi

echo "✖ npm package license metadata problem(s):"
printf '%s\n' "$hits" | sed 's/^/  /'
echo ""
echo "Every non-private package.json needs BOTH halves of its licensing:"
echo ""
echo "  license field   the SPDX id, e.g. \"license\": \"Apache-2.0\" — this is"
echo "                  what npm and license scanners read."
echo "  LICENSE file    the license text, beside the package.json. Copy it from"
echo "                  a sibling package so the wording stays identical:"
echo "                    cp LICENSE packages/<pkg>/LICENSE"
echo "  files entry     if package.json declares \`files\`, list LICENSE in it."
echo ""
echo "Do not rely on pnpm substituting the workspace-root LICENSE at pack time —"
echo "it does, but npm does not, so the text vanishes outside \`pnpm pack\`."
echo "If a package should never be published, mark it \"private\": true instead."
exit 1
