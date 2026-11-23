#!/usr/bin/env bash
# Proves that Go tests keep off the developer's own per-user locations.
#
# A test binary resolves the configuration directory, the data root, the
# caches, the per-user plugin root and the home-based agent configuration to
# directories of its own (core/dataroot.TestUserDir), and reaches the keychain
# only through core/keychain, which opens an in-memory keyring in a test
# binary.
#
#   --static   fail if code outside core/keychain imports the keyring
#              directly, since only that door is safe in a test (seconds)
#   (default)  also run the packages that resolve those locations with HOME
#              pointed at an empty sentinel directory and every override
#              (KAPI_CONFIG_DIR, KAPI_DATA_DIR, XDG_*) unset, and fail if a
#              test created or changed anything under the sentinel (minutes)
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"

direct=$(git grep -l 'zalando/go-keyring' -- '*.go' ':!*_test.go' ':!core/keychain/*' || true)
if [ -n "$direct" ]; then
	echo "check-test-isolation: reach the keychain through core/keychain, not the keyring directly:" >&2
	echo "$direct" | sed 's/^/  /' >&2
	exit 1
fi
if [ "${1:-}" = "--static" ]; then
	echo "check-test-isolation: every keychain access goes through core/keychain"
	exit 0
fi

gocache=$(go env GOCACHE)
gomodcache=$(go env GOMODCACHE)
gopath=$(go env GOPATH)
sentinel=$(mktemp -d)
log=$(mktemp)
trap 'rm -rf "$sentinel"' EXIT
mkdir -p "$sentinel/home"

run() {
	local dir=$1
	shift
	(cd "$dir" && env -u KAPI_CONFIG_DIR -u KAPI_DATA_DIR -u KAPI_PLUGINS_DIR -u KAPI_DESKTOP_CONFIG_DIR \
		-u XDG_CONFIG_HOME -u XDG_DATA_HOME -u XDG_CACHE_HOME -u BOWRAIN_CONFIG_DIR -u BOWRAIN_DESKTOP_CONFIG_DIR \
		-u CLAUDE_CONFIG_DIR -u KAPI_HOME_DIR \
		HOME="$sentinel/home" GOCACHE="$gocache" GOMODCACHE="$gomodcache" GOPATH="$gopath" \
		go test -trimpath -tags fts5 -count=1 "$@" >"$log" 2>&1) || {
		echo "check-test-isolation: tests failed in $dir (full log: $log):" >&2
		grep -E '^(--- FAIL|    --- FAIL|FAIL|ok )' "$log" | head -30 >&2
		exit 1
	}
}

run . ./core/dataroot/ ./core/keychain/ ./core/credentials/ ./core/channel/ ./core/flow/
run host . ./credentials/ ./config/ ./pluginhost/... ./venue/config/
run cli . ./selfupdate/
run apps/kapi-desktop ./backend/
run bowrain ./credentials/ ./apps/bowrain/backend/

# The go command keeps its own telemetry and caches under HOME; those are the
# toolchain's, not a test's.
written=$(cd "$sentinel/home" && find . -mindepth 1 \
	-not -path './Library' -not -path './Library/Application Support' \
	-not -path './Library/Application Support/go' -not -path './Library/Application Support/go/*' \
	-not -path './.config' -not -path './.config/go' -not -path './.config/go/*' -print)
if [ -n "$written" ]; then
	echo "check-test-isolation: a test wrote under the user's home:" >&2
	echo "$written" | sed 's|^\.|  ~|' | head -40 >&2
	exit 1
fi
rm -f "$log"
echo "check-test-isolation: no test wrote under the user's home or reached the keychain directly"
