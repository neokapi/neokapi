#!/usr/bin/env bash
# Guard: every keychain access goes through core/keychain.
#
#   scripts/check-keychain-door.sh              # check this repository
#   scripts/check-keychain-door.sh --self-test  # plant each door and see it caught
#
# core/keychain opens an in-memory keyring in a test binary, so a test that
# reaches a secret through it never touches the developer's real keychain.
# Any other door would: a keyring library, the macOS `security` tool, the
# Linux Secret Service (`secret-tool`, org.freedesktop.secrets), the Windows
# credential manager (CredRead/CredWrite, `cmdkey`), or a keyring package in
# the apps' TypeScript. This fails on any of them in the Go code (tests
# included, which may import go-keyring only for keyring.MockInit) and in the
# TypeScript of the apps and packages.
#
# One call is exempt, approved by the founder on 2026-11-23: the skill
# evaluation in scripts/skilleval/paired_isolation.go reads the developer's
# Claude Code credential with `security find-generic-password`, so it can sign
# in inside its isolated environment. The exemption is that exact line in that
# file. A second door there, another verb, or a changed call is refused, as is
# anything elsewhere under scripts/.
set -euo pipefail

# Go: forbidden in every Go file outside core/keychain.
go_doors='99designs/keyring|keybase/go-keychain|danieljoos/wincred|"org\.freedesktop\.[Ss]ecret|Cred(Read|Write|Delete|Enumerate)W|advapi32|exec\.Command(Context)?\(.*"(/usr/bin/)?(security|secret-tool|cmdkey)"|(find|add|delete)-(generic|internet)-password'
# Go: the one library core/keychain wraps, allowed in tests for MockInit.
go_lib='zalando/go-keyring'
# The one exempt call (see above): its file and its exact, trimmed line.
exempt_file='scripts/skilleval/paired_isolation.go'
exempt_line='command := exec.CommandContext(ctx, "/usr/bin/security", "find-generic-password", "-s", "Claude Code-credentials", "-w")'
# TypeScript and JavaScript in the apps and packages.
ts_doors='keytar|@napi-rs/keyring|node-keyring|safeStorage|secret-tool|(find|add|delete)-(generic|internet)-password|cmdkey'

# go_door_hits prints the Go door matches, leaving out the exempt call when it
# is the only match in its file and reads exactly as approved.
go_door_hits() {
	local hits in_file
	hits=$(git grep -nE "$go_doors" -- '*.go' ':!core/keychain/**' || true)
	in_file=$(printf '%s\n' "$hits" | grep -F "$exempt_file:" || true)
	if [ -n "$in_file" ] && [ "$(printf '%s\n' "$in_file" | wc -l | tr -d ' ')" = 1 ] &&
		[ "$(printf '%s' "$in_file" | cut -d: -f3- | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')" = "$exempt_line" ]; then
		hits=$(printf '%s\n' "$hits" | grep -vF "$exempt_file:" || true)
	fi
	printf '%s\n' "$hits"
}

scan() {
	local found=""
	found+=$(go_door_hits)
	found+=$'\n'
	found+=$(git grep -nE "$go_lib" -- '*.go' ':!*_test.go' ':!core/keychain/**' || true)
	found+=$'\n'
	found+=$(git grep -nE "$ts_doors" -- 'apps/**/*.ts' 'apps/**/*.tsx' 'apps/**/*.js' \
		'bowrain/apps/**/*.ts' 'bowrain/apps/**/*.tsx' 'bowrain/apps/**/*.js' \
		'packages/**/*.ts' 'packages/**/*.tsx' 'bowrain/packages/**/*.ts' 'bowrain/packages/**/*.tsx' \
		'package.json' '**/package.json' || true)
	printf '%s\n' "$found" | sed '/^$/d'
}

if [ "${1:-}" = "--self-test" ]; then
	tmp=$(mktemp -d)
	trap 'rm -rf "$tmp"' EXIT
	(
		cd "$tmp"
		git init -q
		mkdir -p core/keychain host apps/kapi-desktop/frontend/src scripts/tool
		printf 'package keychain\nimport _ "github.com/zalando/go-keyring"\n' >core/keychain/k.go
		printf 'package host\nimport _ "github.com/zalando/go-keyring"\n' >host/mock_test.go
		mkdir -p scripts/skilleval
		printf 'package main\n\t%s\n' "$exempt_line" >"$exempt_file"
		git add -A
		if [ -n "$(scan)" ]; then
			echo "check-keychain-door self-test: a permitted use was reported:" >&2
			scan >&2
			exit 1
		fi
		plant() {
			printf '%s\n' "$2" >"$1"
			git add -A
			if [ -z "$(scan)" ]; then
				echo "check-keychain-door self-test: missed $1: $2" >&2
				exit 1
			fi
			git rm -q --cached "$1"
			rm "$1"
		}
		plant host/a.go 'import _ "github.com/zalando/go-keyring"'
		plant host/b.go 'out, _ := exec.Command("security", "find-generic-password", "-s", "x").Output()'
		plant host/c_test.go 'exec.CommandContext(ctx, "/usr/bin/security", "dump-keychain")'
		plant host/d.go 'cmd := exec.Command("secret-tool", "lookup", "service", "kapi")'
		plant host/e.go 'const bus = "org.freedesktop.secrets"'
		plant host/f.go 'r, _, _ := procCredReadW.Call()'
		plant host/g.go 'import _ "github.com/99designs/keyring"'
		plant host/h.go 'exec.Command("cmdkey", "/list")'
		plant apps/kapi-desktop/frontend/src/x.ts 'import keytar from "keytar";'
		plant scripts/tool/k.go 'import _ "github.com/zalando/go-keyring"'
		plant scripts/tool/t.go 'var _ = exec.Command("security", "find-generic-password")'
		# The exempt file: a second door, another verb, or a changed call is refused.
		exempt_variant() {
			printf 'package main\n%s\n' "$1" >"$exempt_file"
			git add -A
			if [ -z "$(scan)" ]; then
				echo "check-keychain-door self-test: the exemption let through: $1" >&2
				exit 1
			fi
			printf 'package main\n\t%s\n' "$exempt_line" >"$exempt_file"
			git add -A
		}
		exempt_variant "	$exempt_line
	_ = exec.Command(\"security\", \"dump-keychain\")"
		exempt_variant '	command := exec.CommandContext(ctx, "/usr/bin/security", "add-generic-password", "-s", "Claude Code-credentials", "-w")'
		exempt_variant '	command := exec.CommandContext(ctx, "/usr/bin/security", "find-generic-password", "-s", "other", "-w")'
		if [ -n "$(scan)" ]; then
			echo "check-keychain-door self-test: the exempt call was reported:" >&2
			scan >&2
			exit 1
		fi
	) || exit 1
	echo "✓ check-keychain-door self-test: every planted door is caught, permitted uses pass"
	exit 0
fi

cd "$(git rev-parse --show-toplevel)"
found=$(scan)
if [ -n "$found" ]; then
	echo "check-keychain-door: reach the keychain through core/keychain, which a test binary keeps in memory:" >&2
	printf '%s\n' "$found" | sed 's/^/  /' >&2
	exit 1
fi
echo "✓ check-keychain-door: every keychain access goes through core/keychain"
