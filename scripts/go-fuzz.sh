#!/usr/bin/env bash
#
# Run every native Go fuzz target in the repository, one at a time.
#
#   scripts/go-fuzz.sh [--fuzztime 30s] [--package <dir>] [--shard <i>/<n>]
#                      [--parallel <n>] [--tags <list>]
#   scripts/go-fuzz.sh --list [--package <dir>] [--shard <i>/<n>]
#   scripts/go-fuzz.sh --self-test
#
# The targets are discovered, never listed by hand: every tracked go.mod is a
# module, and every `func FuzzX(f *testing.F)` in a tracked _test.go file is a
# target in the package of that file. `go test -fuzz` accepts one target per
# invocation, so each target gets its own run of --fuzztime.
#
# A failing target does not stop the run. After the last target the script
# prints each failure with the input file Go saved under
# testdata/fuzz/<FuzzName>/ in the package, and exits 1. Committing that file
# makes it a seed that the ordinary test run replays.
#
# --package narrows the run to targets in that repo-relative directory or below
# it. --shard i/n deals the sorted list round-robin and keeps share i, so the
# shards of a CI matrix cover every target exactly once. --tags is the
# comma-separated build tag list; the default, fts5, is the tag the Makefile
# tests with ($(GOTAGS)). Discovering no target is an error.
set -euo pipefail

FUZZTIME="30s"
PACKAGE=""
SHARD=""
PARALLEL=""
TAGS="fts5"
MODE="run"

usage() {
  sed -n '3,8p' "$0" | sed 's/^# \{0,1\}//' >&2
  exit 2
}

# Module directories that go.work names. A module outside the workspace
# (plugins/*) is built with GOWORK=off, as the Makefile builds it.
workspace_modules() {
  local root="$1"
  [ -f "$root/go.work" ] || return 0
  sed -nE 's#^[[:space:]]*(use[[:space:]]+)?\./?([^[:space:]()]*)[[:space:]]*$#\2#p' "$root/go.work" |
    sed 's#^$#.#'
}

# The nearest module directory at or above a repo-relative directory.
module_of() {
  local dir="$1" modules="$2"
  while :; do
    if grep -qxF "$dir" <<< "$modules"; then
      echo "$dir"
      return 0
    fi
    [ "$dir" = "." ] && return 1
    dir="$(dirname "$dir")"
  done
}

# Every tracked module directory, repo-relative; the root module is ".".
tracked_modules() {
  git -C "$1" ls-files -- 'go.mod' '*/go.mod' | sed -E 's#/?go\.mod$##; s#^$#.#'
}

# One line per target: <module> <package dir> <FuzzName>, sorted.
discover() {
  local root="$1" modules file name dir mod
  modules="$(tracked_modules "$root")"
  { git -C "$root" grep -nE --full-name '^func Fuzz[A-Za-z0-9_]*\([A-Za-z_][A-Za-z0-9_]* \*testing\.F\)' -- '*_test.go' || true; } |
    sed -E 's/^([^:]*):[0-9]+:func (Fuzz[A-Za-z0-9_]*)\(.*/\1 \2/' |
    while read -r file name; do
      dir="$(dirname "$file")"
      case "/$dir/" in */testdata/*) continue ;; esac
      mod="$(module_of "$dir" "$modules")" || continue
      printf '%s %s %s\n' "$mod" "$dir" "$name"
    done | LC_ALL=C sort -u
}

select_targets() {
  local all="$1" pkg="$2" shard="$3" index total
  if [ -n "$pkg" ]; then
    pkg="${pkg#./}"
    pkg="${pkg%/}"
    [ -z "$pkg" ] && pkg="."
    if [ "$pkg" != "." ]; then
      all="$(awk -v p="$pkg" '$2 == p || index($2, p "/") == 1' <<< "$all")"
    fi
  fi
  if [ -n "$shard" ]; then
    if ! [[ "$shard" =~ ^([0-9]+)/([0-9]+)$ ]]; then
      echo "go-fuzz: shard must be <i>/<n>, got '$shard'" >&2
      return 2
    fi
    index="${BASH_REMATCH[1]}"
    total="${BASH_REMATCH[2]}"
    if [ "$total" -lt 1 ] || [ "$index" -lt 1 ] || [ "$index" -gt "$total" ]; then
      echo "go-fuzz: shard $shard is out of range" >&2
      return 2
    fi
    all="$(awk -v i="$index" -v n="$total" 'NF && (NR - 1) % n == i - 1' <<< "$all")"
  fi
  sed '/^$/d' <<< "$all"
}

# Print the per-module count and the targets.
list_targets() {
  local root="$1" all="$2" selected="$3"
  echo "go-fuzz: $(grep -c . <<< "$all") targets discovered, per module:"
  { tracked_modules "$root"; echo; cat <<< "$all"; } |
    awk 'NF == 1 { n[$1] += 0 } NF == 3 { n[$1]++ } END { for (m in n) printf "  %-26s %d\n", m, n[m] }' |
    LC_ALL=C sort
  if [ "$selected" != "$all" ]; then
    echo "go-fuzz: $(grep -c . <<< "$selected" || true) selected"
  fi
  awk '{ printf "  %s  %s\n", $2, $3 }' <<< "$selected"
}

run_targets() {
  local root="$1" selected="$2" mod dir name rel marker log status crashers
  local -a failures=()
  local workspace count=0 total
  workspace="$(workspace_modules "$root")"
  total="$(grep -c . <<< "$selected")"
  marker="$(mktemp)"
  log="$(mktemp)"
  # shellcheck disable=SC2064  # expand now: the paths are local
  trap "rm -f '$marker' '$log'" RETURN

  while read -r mod dir name; do
    count=$((count + 1))
    if [ "$mod" = "." ]; then rel="./$dir"; else rel="./${dir#"$mod"/}"; fi
    [ "$dir" = "$mod" ] && rel="."
    local -a env=()
    grep -qxF "$mod" <<< "$workspace" || env=(GOWORK=off)
    local -a args=(test -tags "$TAGS" -run '^$' -fuzz "^${name}\$" -fuzztime "$FUZZTIME")
    [ -n "$PARALLEL" ] && args+=(-parallel "$PARALLEL")
    echo "=== [$count/$total] $name ($dir, ${FUZZTIME})"
    touch "$marker"
    set +e
    (cd "$root/$mod" && env ${env[@]+"${env[@]}"} go "${args[@]}" "$rel") > "$log" 2>&1
    status=$?
    set -e
    tail -n 3 "$log" | sed 's/^/    /'
    if [ "$status" -ne 0 ]; then
      sed 's/^/    | /' "$log" | tail -n 60
      crashers="$(find "$root/$dir/testdata/fuzz/$name" -type f -newer "$marker" 2>/dev/null | sed "s#^$root/##" | paste -sd ' ' - || true)"
      failures+=("$name ($dir): ${crashers:-no input file written; see the log above}")
    fi
  done <<< "$selected"

  echo
  if [ "${#failures[@]}" -eq 0 ]; then
    echo "go-fuzz: all $total targets passed (${FUZZTIME} each)"
    return 0
  fi
  echo "go-fuzz: ${#failures[@]} of $total targets failed:"
  printf '  FAIL %s\n' "${failures[@]}"
  echo "Commit each input file beside the fix as a regression seed."
  return 1
}

self_test() {
  local tmp out status
  tmp="$(mktemp -d)"
  # shellcheck disable=SC2064  # expand now: tmp is local to this function
  trap "rm -rf '$tmp'" RETURN
  local script
  script="$(cd "$(dirname "$0")" && pwd)/$(basename "$0")"

  mkdir -p "$tmp/repo/a" "$tmp/repo/b/c" "$tmp/repo/sub/d" "$tmp/repo/a/testdata/x"
  cd "$tmp/repo"
  git init -q
  printf 'module example.com/root\n\ngo 1.21\n' > go.mod
  printf 'module example.com/sub\n\ngo 1.21\n' > sub/go.mod
  printf 'go 1.21\n\nuse (\n\t.\n)\n' > go.work
  cat > a/a_test.go <<'GO'
package a

import "testing"

// Fails as soon as the mutator grows the input past three bytes.
func FuzzAlphaBreaks(f *testing.F) {
	f.Add([]byte("ab"))
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 3 {
			t.Fatalf("too long: %q", b)
		}
	})
}

func FuzzBravo(f *testing.F) {
	f.Add("x")
	f.Fuzz(func(t *testing.T, s string) {})
}
GO
  cat > a/a.go <<'GO'
package a

// A function whose name starts with Fuzz is not a target.
func FuzzyMatch() {}
GO
  cat > a/testdata/x/x_test.go <<'GO'
package x

import "testing"

func FuzzIgnoredInTestdata(f *testing.F) {}
GO
  cat > b/c/c_test.go <<'GO'
package c

import "testing"

func FuzzCharlie(f *testing.F) {
	f.Add(1)
	f.Fuzz(func(t *testing.T, n int) {})
}

func TestNotFuzz(t *testing.T) {}
GO
  cat > sub/d/d_test.go <<'GO'
package d

import "testing"

func FuzzDelta(f *testing.F) {
	f.Add("d")
	f.Fuzz(func(t *testing.T, s string) {})
}
GO
  git add -A

  local all want
  all="$(discover "$tmp/repo")"
  want=$'. a FuzzAlphaBreaks\n. a FuzzBravo\n. b/c FuzzCharlie\nsub sub/d FuzzDelta'
  [ "$all" = "$want" ] || { printf 'self-test: discover: want\n%s\ngot\n%s\n' "$want" "$all" >&2; return 1; }

  # Shards partition the list: every target once, in no two shards.
  local merged
  merged="$( { select_targets "$all" "" 1/3; select_targets "$all" "" 2/3; select_targets "$all" "" 3/3; } | LC_ALL=C sort)"
  [ "$merged" = "$all" ] || { echo "self-test: shards 1-3/3 do not cover the list once" >&2; return 1; }
  [ "$(select_targets "$all" "" 2/3)" = ". a FuzzBravo" ] || { echo "self-test: shard 2/3" >&2; return 1; }
  [ "$(select_targets "$all" b "")" = ". b/c FuzzCharlie" ] || { echo "self-test: --package b" >&2; return 1; }
  [ -z "$(select_targets "$all" bb "")" ] || { echo "self-test: --package bb matched b" >&2; return 1; }
  if select_targets "$all" "" 4/3 >/dev/null 2>&1; then echo "self-test: accepted shard 4/3" >&2; return 1; fi

  # The runner keeps going past a failure, names the input Go saved, exits 1.
  set +e
  out="$("$script" --fuzztime 3s --parallel 2 2>&1)"
  status=$?
  set -e
  [ "$status" -eq 1 ] || { printf 'self-test: run exited %s, want 1\n%s\n' "$status" "$out" >&2; return 1; }
  for name in FuzzBravo FuzzCharlie FuzzDelta; do
    grep -q "=== \[[0-9]/4\] $name" <<< "$out" || { printf 'self-test: %s did not run after the failure\n%s\n' "$name" "$out" >&2; return 1; }
  done
  grep -qE '^  FAIL FuzzAlphaBreaks \(a\): a/testdata/fuzz/FuzzAlphaBreaks/[0-9a-f]+$' <<< "$out" ||
    { printf 'self-test: failure line without the saved input\n%s\n' "$out" >&2; return 1; }
  grep -q '1 of 4 targets failed' <<< "$out" || { printf 'self-test: summary\n%s\n' "$out" >&2; return 1; }

  # A clean run exits 0.
  "$script" --fuzztime 1s --package sub > /dev/null 2>&1 || { echo "self-test: a passing target failed" >&2; return 1; }

  # Finding nothing is an error, not a pass.
  if "$script" --list --package nowhere > /dev/null 2>&1; then
    echo "self-test: an empty selection passed" >&2
    return 1
  fi
  echo "go-fuzz: self-test passed"
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --fuzztime) FUZZTIME="${2:?}"; shift 2 ;;
    --package) PACKAGE="${2:?}"; shift 2 ;;
    --shard) SHARD="${2:?}"; shift 2 ;;
    --parallel) PARALLEL="${2:?}"; shift 2 ;;
    --tags) TAGS="${2:?}"; shift 2 ;;
    --list) MODE="list"; shift ;;
    --self-test) MODE="self-test"; shift ;;
    -h | --help) usage ;;
    *) echo "go-fuzz: unknown argument '$1'" >&2; usage ;;
  esac
done

if [ "$MODE" = "self-test" ]; then
  self_test
  exit 0
fi

ROOT="$(git rev-parse --show-toplevel)"
ALL="$(discover "$ROOT")"
if [ -z "$ALL" ]; then
  echo "go-fuzz: no fuzz targets found under $ROOT" >&2
  exit 1
fi
SELECTED="$(select_targets "$ALL" "$PACKAGE" "$SHARD")"
if [ -z "$SELECTED" ]; then
  echo "go-fuzz: no fuzz target matches --package '${PACKAGE}' --shard '${SHARD}'" >&2
  exit 1
fi

if [ "$MODE" = "list" ]; then
  list_targets "$ROOT" "$ALL" "$SELECTED"
  exit 0
fi
run_targets "$ROOT" "$SELECTED"
