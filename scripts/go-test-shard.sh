#!/usr/bin/env bash
#
# Print the `go test` flags that select one shard of a package's tests.
#
#   scripts/go-test-shard.sh <package-dir> <i>/<n>
#
# The tests of one Go package run one after another, so a package with minutes
# of tests sets the length of the CI job that runs it. Sharding runs the
# package in n jobs side by side, each with these flags.
#
# The top-level test names are read from the package's _test.go files and
# dealt round-robin in name order. Shards 1 to n-1 each select their share by
# name with -run. Shard n uses -skip with the names of every other shard, so a
# test this script did not list (declared in a way the pattern below misses, or
# built only under a tag) runs there. No test falls outside every shard.
#
#   scripts/go-test-shard.sh --self-test
set -euo pipefail

names() {
  local dir="$1"
  # TestMain is the package's entry point, not a test.
  cat "$dir"/*_test.go 2>/dev/null |
    sed -nE 's/^func ((Test|Fuzz|Example)[A-Za-z0-9_]*)\(.*/\1/p' |
    grep -vx 'TestMain' | LC_ALL=C sort -u || true
}

shard_flags() {
  local dir="$1" spec="$2" index total
  if ! [[ "$spec" =~ ^([0-9]+)/([0-9]+)$ ]]; then
    echo "go-test-shard: shard must be <i>/<n>, got '$spec'" >&2
    return 2
  fi
  index="${BASH_REMATCH[1]}"
  total="${BASH_REMATCH[2]}"
  if [ "$total" -lt 1 ] || [ "$index" -lt 1 ] || [ "$index" -gt "$total" ]; then
    echo "go-test-shard: shard $spec is out of range" >&2
    return 2
  fi

  local all mine others
  all="$(names "$dir")"
  if [ -z "$all" ]; then
    echo "go-test-shard: no tests found in $dir" >&2
    return 1
  fi
  mine="$(awk -v i="$index" -v n="$total" '(NR - 1) % n == i - 1' <<< "$all" | paste -sd '|' -)"
  others="$(awk -v i="$index" -v n="$total" '(NR - 1) % n != i - 1' <<< "$all" | paste -sd '|' -)"

  if [ "$index" -lt "$total" ]; then
    # A shard with no test of its own selects nothing.
    echo "-run ^(${mine:-NoTestInThisShard})\$"
  elif [ -n "$others" ]; then
    echo "-skip ^(${others})\$"
  else
    echo "-run ."
  fi
}

self_test() {
  local tmp
  tmp="$(mktemp -d)"
  # shellcheck disable=SC2064  # expand now: tmp is local to this function
  trap "rm -rf '$tmp'" RETURN
  cat > "$tmp/a_test.go" <<'GO'
package p

func TestMain(m *testing.M) {}
func TestDelta(t *testing.T) {}
func TestAlpha(t *testing.T) {}
func ExampleEcho() {}
GO
  cat > "$tmp/b_test.go" <<'GO'
package p

func FuzzCharlie(f *testing.F) {}
func TestBravo(t *testing.T) {}
func helper() {}
func (s *suite) TestMethod() {}
GO
  local got want
  want="ExampleEcho FuzzCharlie TestAlpha TestBravo TestDelta"
  got="$(names "$tmp" | tr '\n' ' ' | sed 's/ $//')"
  [ "$got" = "$want" ] || { echo "self-test: names: want '$want', got '$got'" >&2; return 1; }

  got="$(shard_flags "$tmp" 1/3)"
  [ "$got" = '-run ^(ExampleEcho|TestBravo)$' ] || { echo "self-test: 1/3: got '$got'" >&2; return 1; }
  got="$(shard_flags "$tmp" 2/3)"
  [ "$got" = '-run ^(FuzzCharlie|TestDelta)$' ] || { echo "self-test: 2/3: got '$got'" >&2; return 1; }
  # The last shard names what the others run, so anything unlisted lands in it.
  got="$(shard_flags "$tmp" 3/3)"
  [ "$got" = '-skip ^(ExampleEcho|FuzzCharlie|TestBravo|TestDelta)$' ] || { echo "self-test: 3/3: got '$got'" >&2; return 1; }
  got="$(shard_flags "$tmp" 1/1)"
  [ "$got" = '-run .' ] || { echo "self-test: 1/1: got '$got'" >&2; return 1; }
  # More shards than tests: the surplus shards run nothing, the last runs the rest.
  got="$(shard_flags "$tmp" 6/9)"
  [ "$got" = '-run ^(NoTestInThisShard)$' ] || { echo "self-test: 6/9: got '$got'" >&2; return 1; }

  if shard_flags "$tmp" 4/3 2>/dev/null; then echo "self-test: accepted shard 4/3" >&2; return 1; fi
  if shard_flags "$tmp" two 2>/dev/null; then echo "self-test: accepted shard 'two'" >&2; return 1; fi
  mkdir "$tmp/empty"
  if shard_flags "$tmp/empty" 1/2 2>/dev/null; then echo "self-test: accepted a package with no tests" >&2; return 1; fi
  echo "go-test-shard: self-test passed"
}

if [ "${1:-}" = "--self-test" ]; then
  self_test
  exit 0
fi
if [ "$#" -ne 2 ]; then
  echo "usage: $0 <package-dir> <i>/<n>   |   $0 --self-test" >&2
  exit 2
fi
shard_flags "$1" "$2"
