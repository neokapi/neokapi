#!/usr/bin/env bash
# Run this checkout's own bin/kapi in a throwaway sandbox that follows the
# in-repo isolation contract (CLAUDE.md, "Dogfooding kapi").
#
#   kapi-sandbox.sh init              fresh sandbox with a copy of samples/compass
#   kapi-sandbox.sh run <kapi args>   run bin/kapi in the sandbox project directory
#   kapi-sandbox.sh path              print the sandbox directory
#   kapi-sandbox.sh clean             delete the sandbox
#
# The sandbox lives under the system temp directory, keyed by the checkout path,
# so repeated calls from one checkout reuse it. Every KAPI_* variable the caller
# exported is dropped, HOME points into the sandbox, and KAPI_NO_PROJECT=1 is
# set, so a project command needs `-p .` and can never bind to the dogfood
# recipe at the repo root.
#
# kapi records who drove a context operation from the agent host's marker
# variables (host/contextactor.go), and refuses some operations to an agent.
# The sandbox clears those markers, as kapi/e2e does, so kapi sees a person.
#
# Environment:
#   KAPI_SANDBOX_FIXTURE   directory to copy as the project (default samples/compass)
#   KAPI_SANDBOX_AS_AGENT  non-empty keeps the agent host markers, to verify
#                          what kapi does when an agent drives it
set -euo pipefail

die() {
	printf 'kapi-sandbox: %s\n' "$*" >&2
	exit 2
}

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)
# scripts/ -> verify/ -> skills/ -> .claude/ -> checkout root. This holds for
# the primary checkout and for a worktree under .claude/worktrees/<name>/.
root=$(cd "$script_dir/../../../.." && pwd -P)
[[ -f "$root/go.work" && -d "$root/kapi/cmd/kapi" ]] ||
	die "cannot find the checkout root above $script_dir"

kapi_bin="$root/bin/kapi"
fixture=${KAPI_SANDBOX_FIXTURE:-$root/samples/compass}
as_agent=${KAPI_SANDBOX_AS_AGENT:-}

tmp=${TMPDIR:-/tmp}
tmp=${tmp%/}
key=$(printf '%s' "$root" | cksum | cut -d' ' -f1)
sandbox="$tmp/kapi-verify-$(basename "$root")-$key"
project="$sandbox/project"

# check_binary refuses a missing bin/kapi, and one older than any Go source,
# go.mod, go.sum or go.work outside the trees kapi does not link.
check_binary() {
	[[ -x "$kapi_bin" ]] ||
		die "$kapi_bin is missing. Build it: make -C '$root' build"
	local newer
	newer=$(cd "$root" && find . \
		\( -path ./.git -o -path ./.claude -o -path ./bin -o -path ./bowrain \
		-o -path ./apps -o -path ./plugins -o -path ./harness -o -path ./web \
		-o -path ./scripts -o -path ./samples -o -path ./packages -o -path ./docs \
		-o -name node_modules -o -name testdata \) -prune -o \
		-type f \( -name '*.go' ! -name '*_test.go' -o -name go.mod \
		-o -name go.sum -o -name go.work \) -newer "$kapi_bin" -print -quit)
	[[ -z "$newer" ]] ||
		die "$kapi_bin is older than ${newer#./}. Rebuild it: make -C '$root' build"
}

# sandbox_env runs its arguments with the caller's KAPI_* variables removed and
# the isolation contract applied.
sandbox_env() {
	local unset_args=() name
	while IFS= read -r name; do
		unset_args+=(-u "$name")
	done < <(compgen -e | grep '^KAPI_' || true)
	if [[ -z "$as_agent" ]]; then
		for name in CLAUDECODE CLAUDE_CODE_SESSION_ID CODEX_SESSION_ID; do
			unset_args+=(-u "$name")
		done
	fi
	env ${unset_args[@]+"${unset_args[@]}"} \
		HOME="$sandbox/home" \
		KAPI_NO_PROJECT=1 \
		KAPI_TELEMETRY=0 \
		DO_NOT_TRACK=1 \
		KAPI_NO_UPDATE_CHECK=1 \
		KAPI_CONFIG_DIR="$sandbox/config" \
		KAPI_DATA_DIR="$sandbox/data/kapi" \
		XDG_CONFIG_HOME="$sandbox/home/.config" \
		XDG_DATA_HOME="$sandbox/data" \
		XDG_CACHE_HOME="$sandbox/cache" \
		KAPI_PLUGINS_DIR_ONLY=1 \
		KAPI_PLUGINS_DIR="$sandbox/plugins" \
		NO_COLOR=1 \
		"$@"
}

make_dirs() {
	mkdir -p "$sandbox/home" "$sandbox/config" "$sandbox/data/kapi" \
		"$sandbox/cache" "$sandbox/plugins" "$project"
}

cmd=${1:-}
[[ $# -gt 0 ]] && shift
case "$cmd" in
init)
	check_binary
	[[ -f "$fixture/kapi.yaml" ]] || die "fixture $fixture has no kapi.yaml"
	rm -rf "$sandbox"
	make_dirs
	cp -R "$fixture/." "$project/"
	rm -rf "$project/.kapi/work"
	printf 'sandbox: %s\n' "$sandbox" >&2
	printf 'project: %s (copied from %s)\n' "$project" "${fixture#"$root"/}" >&2
	;;
run)
	[[ $# -gt 0 ]] || die "usage: kapi-sandbox.sh run <kapi args...>"
	check_binary
	[[ -d "$project" ]] || die "no sandbox yet. Run: $0 init"
	make_dirs
	printf 'sandbox: %s\n' "$sandbox" >&2
	cd "$project"
	status=0
	sandbox_env "$kapi_bin" "$@" || status=$?
	exit "$status"
	;;
path)
	printf '%s\n' "$sandbox"
	;;
clean)
	rm -rf "$sandbox"
	printf 'removed: %s\n' "$sandbox" >&2
	;;
*)
	die "usage: kapi-sandbox.sh init | run <kapi args...> | path | clean"
	;;
esac
