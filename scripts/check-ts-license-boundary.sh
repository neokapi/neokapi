#!/usr/bin/env bash
#
# Guard: the licence boundary holds on the TypeScript side too.
#
# The licence line in this repo is directory containment under a LICENSE file
# and nothing else. `bowrain/LICENSE` is AGPL-3.0, `bowrain/plugin/LICENSE` and
# the root LICENSE are Apache-2.0, so every file takes the licence of its
# nearest ancestor LICENSE. Moving a file between those subtrees relicenses it.
#
# `make check-module-boundaries` asserts that line over Go import closures: no
# Apache module reaches a package under bowrain/, with no exception. The
# frontend had no equivalent. A React component under packages/ importing
# `@neokapi/ui` (which is the AGPL bowrain/packages/ui) compiles, bundles, ships
# inside the Apache desktop app, and nothing in the repo says a word. The pnpm
# workspace makes it a one-line mistake: every internal dependency is declared
# "*" and linkWorkspacePackages resolves it from source, so an Apache package
# can consume an AGPL one without even a version number to notice.
#
# What fails, for any .ts/.tsx/.js/.jsx/.mjs/.cjs file on the Apache side:
#
#   1. A relative import that resolves into an AGPL tree
#      (`import x from "../../bowrain/packages/ui/src/x"`).
#   2. An import of a package whose package.json sits in an AGPL tree
#      (`import { Button } from "@neokapi/ui"`). The names are read from the
#      tree's own manifests, so a package added under bowrain/ is covered the
#      day it exists.
#   3. A tsconfig under an Apache tree whose `paths` alias, `extends`, or any
#      other path value resolves into an AGPL tree. An alias hides both of the
#      above from a reader and from a plain grep.
#   4. A package.json under an Apache tree that declares a dependency on an
#      AGPL package, whatever the version spec.
#
# There is no allowlist, and there must never be one, for the reason the Go side
# carries none: an exception is how a boundary stops being one. Code both sides
# need belongs below the line, in packages/, where the Apache licence already
# covers it.
#
# Usage:
#     ./scripts/check-ts-license-boundary.sh              # scan tracked files
#     ./scripts/check-ts-license-boundary.sh --self-test  # prove the matcher both ways
#
# Wired into `make check-module-boundaries` (the `module-boundaries` CI job) and
# `make audit-modules`.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"

# Paths whose contents are generated, vendored, or built. A wails bindings tree
# is matched by its `bindings/github.com/` shape rather than by `bindings`
# alone, so a hand-written directory of that name is still scanned.
readonly SKIP_RE='(^|/)(node_modules|dist|build|coverage|storybook-static|\.next|\.turbo|\.vite)/|(^|/)bindings/github\.com/'
readonly SRC_RE='\.(ts|tsx|js|jsx|mjs|cjs)$'

# A module specifier: static import, re-export, dynamic import, or require. The
# quote has to follow the keyword, so prose that merely names a package is not a
# hit.
readonly SPEC_RE='(^|[^A-Za-z0-9_$])(from|import|require)[[:space:]]*\(?[[:space:]]*['"'"'"][^'"'"'"]+['"'"'"]'

# ── the tree's own licence map ───────────────────────────────────────────────

# list_files prints every candidate path in a tree, tree-relative. For the repo
# it lists tracked files plus untracked ones git would add, so a crossing is
# caught on the pre-push run rather than one commit later; anything gitignored
# stays out. The fixture tree of the self-test is listed straight off disk.
list_files() {
  local tree="$1"
  if [ "$tree" = "$root" ]; then
    git -C "$tree" ls-files --cached --others --exclude-standard
  else
    (cd "$tree" && find . -type f | sed 's|^\./||')
  fi | grep -vE "$SKIP_RE" || true
}

# licensed_dirs prints "agpl<TAB>dir" or "other<TAB>dir" for every LICENSE in
# the tree, the repo root spelled as the empty string. AGPL is recognised by the
# licence's own title, so the map follows the files rather than a list kept in
# this script.
licensed_dirs() {
  local tree="$1" f dir kind
  list_files "$tree" | grep -E '(^|/)LICENSE$' | while IFS= read -r f; do
    dir="$(dirname "$f")"
    [ "$dir" = "." ] && dir=""
    kind=other
    grep -qi 'GNU AFFERO GENERAL PUBLIC LICENSE' "$tree/$f" && kind=agpl
    printf '%s\t%s\n' "$kind" "$dir"
  done
}

# ── the matcher ──────────────────────────────────────────────────────────────

# classify reads paths on stdin and prints "agpl<TAB>path" or "other<TAB>path",
# resolving each path against its nearest ancestor LICENSE.
classify() {
  BOUNDARY_AGPL="$1" BOUNDARY_OTHER="$2" awk '
    function licenseof(path,   d, i, slash) {
      d = path
      while (1) {
        slash = 0
        for (i = length(d); i > 0; i--) if (substr(d, i, 1) == "/") { slash = i; break }
        d = (slash > 0) ? substr(d, 1, slash - 1) : ""
        if (d in A) return "agpl"
        if (d in B) return "other"
        if (d == "") return "other"
      }
    }
    BEGIN {
      n = split(ENVIRON["BOUNDARY_AGPL"], a, "\n");  for (i = 1; i <= n; i++) A[a[i]] = 1
      m = split(ENVIRON["BOUNDARY_OTHER"], b, "\n"); for (i = 1; i <= m; i++) B[b[i]] = 1
    }
    NF { printf "%s\t%s\n", licenseof($0), $0 }
  '
}

# side prints the paths on stdin that fall on the named side of the line.
side() {
  local want="$1" agpl="$2" other="$3"
  classify "$agpl" "$other" | awk -F'\t' -v want="$want" '$1 == want { print $2 }'
}

# specifiers prints "path<TAB>line<TAB>specifier<TAB>import" for every module
# specifier in the listed source files.
specifiers() {
  local tree="$1" list="$2"
  [ -n "$list" ] || return 0
  printf '%s\n' "$list" | (cd "$tree" && tr '\n' '\0' | xargs -0 grep -HnoE "$SPEC_RE" --) |
    awk -F: '{
      spec = $0
      sub(/^[^:]*:[0-9]+:/, "", spec)
      if (match(spec, /['"'"'"][^'"'"'"]+['"'"'"]/))
        printf "%s\t%s\t%s\timport\n", $1, $2, substr(spec, RSTART + 1, RLENGTH - 2)
    }'
}

# tsconfig_paths prints the same shape for every string in a tsconfig. `paths`,
# `extends`, `references` and `include` all point somewhere, and any of them
# reaching across the line is the same crossing; a string that names no path at
# all ("ESNext", "bundler") resolves to no licensed directory and is ignored.
tsconfig_paths() {
  local tree="$1" list="$2"
  [ -n "$list" ] || return 0
  printf '%s\n' "$list" | (cd "$tree" && tr '\n' '\0' | xargs -0 grep -HnoE '"[^"]+"' --) |
    awk -F: '{
      spec = $0
      sub(/^[^:]*:[0-9]+:/, "", spec)
      gsub(/"/, "", spec)
      if (spec != "") printf "%s\t%s\t%s\ttsconfig\n", $1, $2, spec
    }'
}

# judge reads "path<TAB>line<TAB>specifier<TAB>kind" and prints a violation for
# every specifier that lands on the AGPL side, either by resolving into an AGPL
# tree or by naming an AGPL package.
judge() {
  BOUNDARY_AGPL="$1" BOUNDARY_OTHER="$2" BOUNDARY_NAMES="$3" awk -F'\t' '
    function licenseof(path,   d, i, slash) {
      d = path
      while (1) {
        slash = 0
        for (i = length(d); i > 0; i--) if (substr(d, i, 1) == "/") { slash = i; break }
        d = (slash > 0) ? substr(d, 1, slash - 1) : ""
        if (d in A) return "agpl"
        if (d in B) return "other"
        if (d == "") return "other"
      }
    }
    # resolve normalises base/spec lexically, so "a/b" plus "../../bowrain/x" is
    # "bowrain/x". No filesystem lookup: the directory a specifier names is what
    # decides its licence, extension and index file alike.
    function resolve(base, spec,   parts, n, i, out, m, s) {
      n = split(base "/" spec, parts, "/")
      m = 0
      for (i = 1; i <= n; i++) {
        if (parts[i] == "" || parts[i] == ".") continue
        if (parts[i] == "..") { if (m > 0) m--; continue }
        out[++m] = parts[i]
      }
      s = ""
      for (i = 1; i <= m; i++) s = (s == "") ? out[i] : s "/" out[i]
      return s
    }
    function dirof(path,   i) {
      for (i = length(path); i > 0; i--) if (substr(path, i, 1) == "/") return substr(path, 1, i - 1)
      return ""
    }
    # pkgof takes the package part of a bare specifier, scope included.
    function pkgof(spec,   parts, n) {
      n = split(spec, parts, "/")
      if (substr(spec, 1, 1) == "@") return (n >= 2) ? parts[1] "/" parts[2] : spec
      return parts[1]
    }
    BEGIN {
      n = split(ENVIRON["BOUNDARY_AGPL"], a, "\n");  for (i = 1; i <= n; i++) A[a[i]] = 1
      m = split(ENVIRON["BOUNDARY_OTHER"], b, "\n"); for (i = 1; i <= m; i++) B[b[i]] = 1
      k = split(ENVIRON["BOUNDARY_NAMES"], c, "\n"); for (i = 1; i <= k; i++) if (c[i] != "") N[c[i]] = 1
    }
    NF >= 4 {
      file = $1; line = $2; spec = $3; kind = $4
      target = ""
      if (substr(spec, 1, 1) == ".") target = resolve(dirof(file), spec)
      else if (kind == "tsconfig") target = resolve("", spec)
      if (target != "" && licenseof(target "/x") == "agpl") {
        printf "%s:%s: %s reaches the AGPL tree at %s\n", file, line,
          (kind == "tsconfig" ? "tsconfig path" : "relative import"), target
        next
      }
      if (kind == "import" && (pkgof(spec) in N))
        printf "%s:%s: imports %s, whose package.json is under the AGPL tree\n", file, line, pkgof(spec)
    }
  '
}

# manifest_deps prints a violation for every dependency of an Apache manifest
# that names an AGPL package. It reads the four dependency objects at the top
# level of a formatted manifest (two-space indent, one key per line), which is
# what prettier writes and what `vp fmt` keeps.
manifest_deps() {
  local tree="$1" list="$2" names="$3" f
  [ -n "$list" ] || return 0
  while IFS= read -r f; do
    [ -n "$f" ] || continue
    BOUNDARY_NAMES="$names" awk -v f="$f" '
      BEGIN { k = split(ENVIRON["BOUNDARY_NAMES"], c, "\n"); for (i = 1; i <= k; i++) if (c[i] != "") N[c[i]] = 1 }
      /^  "(dependencies|devDependencies|peerDependencies|optionalDependencies)"[[:space:]]*:[[:space:]]*\{/ { inblock = 1; next }
      /^  \}/ { inblock = 0; next }
      inblock && match($0, /"[^"]+"[[:space:]]*:/) {
        dep = substr($0, RSTART + 1, RLENGTH - 3)
        sub(/"[[:space:]]*$/, "", dep)
        sub(/"$/, "", dep)
        if (dep in N) printf "%s:%d: depends on %s, whose package.json is under the AGPL tree\n", f, NR, dep
      }
    ' "$tree/$f"
  done <<<"$list"
}

# scan runs the whole check over one tree and prints one violation per line as
# "path:line: message". Returns 1 if it printed anything.
scan() {
  local tree="$1" licmap agpl_dirs other_dirs files agpl_names hits
  local apache_src apache_tsconfig apache_manifest

  licmap="$(licensed_dirs "$tree")"
  agpl_dirs="$(printf '%s\n' "$licmap" | awk -F'\t' '$1 == "agpl" { print $2 }')"
  other_dirs="$(printf '%s\n' "$licmap" | awk -F'\t' '$1 == "other" { print $2 }')"
  files="$(list_files "$tree")"

  # Package names that live on the AGPL side. Every manifest under an AGPL tree
  # counts, whether or not a pnpm-workspace.yaml glob currently makes it a
  # workspace member: a name that resolves today and one that resolves after the
  # next edit to those globs are the same import.
  agpl_names="$(
    printf '%s\n' "$files" | grep -E '(^|/)package\.json$' |
      side agpl "$agpl_dirs" "$other_dirs" |
      while IFS= read -r manifest; do
        [ -n "$manifest" ] || continue
        sed -n 's/^  "name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$tree/$manifest" | head -1
      done | grep -v '^$' | sort -u
  )"

  apache_src="$(printf '%s\n' "$files" | grep -E "$SRC_RE" | side other "$agpl_dirs" "$other_dirs")"
  apache_tsconfig="$(printf '%s\n' "$files" | grep -E '(^|/)tsconfig[^/]*\.json$' | side other "$agpl_dirs" "$other_dirs")"
  apache_manifest="$(printf '%s\n' "$files" | grep -E '(^|/)package\.json$' | side other "$agpl_dirs" "$other_dirs")"

  hits="$(
    {
      specifiers "$tree" "$apache_src" || true
      tsconfig_paths "$tree" "$apache_tsconfig" || true
    } | judge "$agpl_dirs" "$other_dirs" "$agpl_names"
    manifest_deps "$tree" "$apache_manifest" "$agpl_names" || true
  )"

  [ -z "$hits" ] && return 0
  printf '%s\n' "$hits"
  return 1
}

# ── self-test ────────────────────────────────────────────────────────────────

# A matcher that silently stops matching is worse than no check at all, so the
# fixture tree carries one of each violation and one of each near miss.
self_test() {
  local tmp status=0 out n
  tmp="$(mktemp -d)"
  # shellcheck disable=SC2064  # expand $tmp now, not at trap time
  trap "rm -rf '$tmp'" EXIT

  mkdir -p "$tmp/apache/src" "$tmp/agpl/packages/ui/src" "$tmp/agpl/plugin"
  head -20 "$root/LICENSE" >"$tmp/LICENSE"
  head -20 "$root/bowrain/LICENSE" >"$tmp/agpl/LICENSE"
  head -20 "$root/LICENSE" >"$tmp/agpl/plugin/LICENSE"

  cat >"$tmp/agpl/packages/ui/package.json" <<'EOF'
{
  "name": "@fixture/agpl-ui",
  "version": "0.0.0"
}
EOF
  cat >"$tmp/agpl/plugin/package.json" <<'EOF'
{
  "name": "@fixture/plugin",
  "version": "0.0.0"
}
EOF
  echo 'export const Button = () => null;' >"$tmp/agpl/packages/ui/src/index.ts"

  # Four crossings, one of each kind.
  cat >"$tmp/apache/src/relative.ts" <<'EOF'
import { Button } from "../../agpl/packages/ui/src/index";
export { Button };
EOF
  cat >"$tmp/apache/src/byname.tsx" <<'EOF'
import { Button } from "@fixture/agpl-ui";
const lazy = () => import("@fixture/agpl-ui/dialog");
export { Button, lazy };
EOF
  cat >"$tmp/apache/tsconfig.json" <<'EOF'
{
  "compilerOptions": {
    "target": "ESNext",
    "paths": { "@ui/*": ["../agpl/packages/ui/src/*"] }
  }
}
EOF
  cat >"$tmp/apache/package.json" <<'EOF'
{
  "name": "@fixture/apache",
  "dependencies": {
    "@fixture/agpl-ui": "*",
    "react": "^19.0.0"
  }
}
EOF

  # The near misses: a sibling import, a package on the Apache side of a nested
  # LICENSE, a bare npm dependency, prose naming an AGPL package, and AGPL code
  # free to import whatever it likes.
  cat >"$tmp/apache/src/clean.ts" <<'EOF'
// The desktop mounts the same shared app (@fixture/agpl-ui) the browser runs.
import { helper } from "./helper";
import { Field } from "@fixture/plugin";
import React from "react";
export { helper, Field, React };
EOF
  echo 'export const helper = () => null;' >"$tmp/apache/src/helper.ts"
  cat >"$tmp/agpl/packages/ui/src/consumer.ts" <<'EOF'
import { Button } from "@fixture/agpl-ui";
import { thing } from "../../../../apache/src/helper";
export { Button, thing };
EOF

  if out=$(scan "$tmp"); then
    echo "FAIL self-test: the matcher did not flag the planted crossings"
    status=1
  else
    n=$(printf '%s\n' "$out" | grep -c . || true)
    if [ "$n" -ne 5 ]; then
      echo "FAIL self-test: expected 5 crossings in the fixture tree, got ${n}:"
      printf '%s\n' "$out" | sed 's/^/    /'
      status=1
    elif ! printf '%s\n' "$out" | grep -q 'relative\.ts.*relative import' ||
      ! printf '%s\n' "$out" | grep -q 'byname\.tsx.*@fixture/agpl-ui' ||
      ! printf '%s\n' "$out" | grep -q 'tsconfig\.json.*tsconfig path' ||
      ! printf '%s\n' "$out" | grep -q 'package\.json.*depends on @fixture/agpl-ui'; then
      echo "FAIL self-test: the five hits are not one of each kind:"
      printf '%s\n' "$out" | sed 's/^/    /'
      status=1
    else
      echo "ok self-test: flags a relative import, a package name, a tsconfig alias and a dependency"
    fi

    if printf '%s\n' "$out" | grep -qE 'clean\.ts|consumer\.ts|helper\.ts'; then
      echo "FAIL self-test: a legitimate import was flagged:"
      printf '%s\n' "$out" | grep -E 'clean\.ts|consumer\.ts|helper\.ts' | sed 's/^/    /'
      status=1
    else
      echo "ok self-test: sibling imports, Apache package names, prose and AGPL-side code all pass"
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

if ! self_test >/dev/null 2>&1; then
  echo "FAIL check-ts-license-boundary.sh: the self-test failed, so the matcher is broken."
  self_test || true
  exit 1
fi

if hits=$(scan "$root"); then
  echo "check-ts-license-boundary: no Apache TypeScript reaches the AGPL tree"
  exit 0
fi

echo "FAIL the licence boundary is crossed on the TypeScript side:"
printf '%s\n' "$hits" | sed 's/^/    /'
echo ""
echo "Files under bowrain/ are AGPL-3.0 (bowrain/LICENSE); everything outside it,"
echo "bowrain/plugin/ included, is Apache-2.0. An Apache file that imports an AGPL"
echo "one ships AGPL code inside an Apache artefact, and the declaration on npm, in"
echo "the plugin registry and in the Homebrew formula is then false."
echo ""
echo "There is no allowlist here, the same way the Go check has none. Move what"
echo "both sides need down to where both may reach it: packages/ for shared"
echo "frontend code, packages/kapi-format for the content model."
exit 1
