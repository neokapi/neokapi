#!/usr/bin/env bash
# Proves that Go tests keep off the developer's own per-user locations.
#
#   scripts/check-test-isolation.sh              # run the packages and check
#   scripts/check-test-isolation.sh --self-test  # prove the check catches a write
#
# A test binary resolves the configuration directory, the data root, the
# caches, the per-user plugin root and the home-based agent configuration to
# directories of its own (core/dataroot.TestUserDir). This runs the packages
# that resolve those locations with HOME pointed at an empty sentinel
# directory and every override (KAPI_CONFIG_DIR, KAPI_DATA_DIR, XDG_*) unset,
# and fails if a test created or changed anything under the sentinel.
#
# The self-test runs a throwaway package whose test writes one file into
# HOME, and passes only when the check reports that file; then it runs one
# that writes nothing, and passes only when the check reports nothing.
#
# The keychain half is check-keychain-door.sh.
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"

gocache=$(go env GOCACHE)
gomodcache=$(go env GOMODCACHE)
gopath=$(go env GOPATH)
sentinel=$(mktemp -d)
log=$(mktemp)
trap 'rm -rf "$sentinel" "$log"' EXIT
mkdir -p "$sentinel/home"

# run DIR [go test args...] runs the tests with HOME at the sentinel and every
# override unset, and stops the script with the failures when a test fails.
run() {
	local dir=$1 start=$SECONDS
	shift
	(cd "$dir" && env -u KAPI_CONFIG_DIR -u KAPI_DATA_DIR -u KAPI_PLUGINS_DIR -u KAPI_DESKTOP_CONFIG_DIR \
		-u XDG_CONFIG_HOME -u XDG_DATA_HOME -u XDG_CACHE_HOME -u BOWRAIN_CONFIG_DIR -u BOWRAIN_DESKTOP_CONFIG_DIR \
		-u CLAUDE_CONFIG_DIR -u KAPI_HOME_DIR -u VP_HOME \
		HOME="$sentinel/home" GOCACHE="$gocache" GOMODCACHE="$gomodcache" GOPATH="$gopath" \
		go test -trimpath -tags fts5 -count=1 "$@" >"$log" 2>&1) || {
		echo "check-test-isolation: tests failed in $dir:" >&2
		grep -E -A12 -- '--- FAIL' "$log" | head -200 >&2
		grep -E '^(FAIL|ok )' "$log" >&2
		exit 1
	}
	echo "check-test-isolation: $dir passed in $((SECONDS - start))s"
}

# written prints what is under the sentinel home, apart from the go command's
# own telemetry and caches, which are the toolchain's and not a test's.
written() {
	(cd "$sentinel/home" && find . -mindepth 1 \
		-not -path './Library' -not -path './Library/Application Support' \
		-not -path './Library/Application Support/go' -not -path './Library/Application Support/go/*' \
		-not -path './.config' -not -path './.config/go' -not -path './.config/go/*' -print)
}

if [ "${1:-}" = "--self-test" ]; then
	pkg=$(mktemp -d)
	trap 'rm -rf "$sentinel" "$log" "$pkg"' EXIT
	printf 'module selftest\n\ngo %s\n' "$(go env GOVERSION | sed 's/^go//; s/^\([0-9]*\.[0-9]*\).*/\1/')" >"$pkg/go.mod"
	cat >"$pkg/plant_test.go" <<'GO'
package selftest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPlant(t *testing.T) {
	if os.Getenv("PLANT") == "" {
		return
	}
	if err := os.WriteFile(filepath.Join(os.Getenv("HOME"), ".planted"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
}
GO
	PLANT=1 GOWORK=off run "$pkg" .
	if [ "$(written)" != "./.planted" ]; then
		echo "check-test-isolation self-test: a planted write into HOME was not reported (saw: $(written))" >&2
		exit 1
	fi
	rm -f "$sentinel/home/.planted"
	PLANT='' GOWORK=off run "$pkg" .
	if [ -n "$(written)" ]; then
		echo "check-test-isolation self-test: a test that writes nothing was reported: $(written)" >&2
		exit 1
	fi
	echo "✓ check-test-isolation self-test: a write into HOME is reported, a clean run is not"
	exit 0
fi

# The packages embed the compiled message catalogs, which a fresh checkout
# does not have.
go run -trimpath ./core/i18n/gen/catalogs

run . ./core/dataroot/ ./core/keychain/ ./core/credentials/ ./core/channel/ ./core/flow/
run host . ./credentials/ ./config/ ./pluginhost/... ./venue/config/
run cli . ./selfupdate/
run apps/kapi-desktop ./backend/
run bowrain ./credentials/ ./apps/bowrain/backend/

found=$(written)
if [ -n "$found" ]; then
	echo "check-test-isolation: a test wrote under the user's home:" >&2
	echo "$found" | sed 's|^\.|  ~|' | head -40 >&2
	exit 1
fi
echo "check-test-isolation: no test wrote under the user's home"
