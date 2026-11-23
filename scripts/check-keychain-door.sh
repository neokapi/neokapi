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
# scripts/ is outside the scope: it holds developer tooling a person runs on
# purpose, never a test or a shipped binary.
set -euo pipefail

# Go: forbidden in every Go file outside core/keychain.
go_doors='99designs/keyring|keybase/go-keychain|danieljoos/wincred|"org\.freedesktop\.[Ss]ecret|Cred(Read|Write|Delete|Enumerate)W|advapi32|exec\.Command(Context)?\(.*"(/usr/bin/)?(security|secret-tool|cmdkey)"|(find|add|delete)-(generic|internet)-password'
# Go: the one library core/keychain wraps, allowed in tests for MockInit.
go_lib='zalando/go-keyring'
# TypeScript and JavaScript in the apps and packages.
ts_doors='keytar|@napi-rs/keyring|node-keyring|safeStorage|secret-tool|(find|add|delete)-(generic|internet)-password|cmdkey'

scan() {
	local found=""
	found+=$(git grep -nE "$go_doors" -- '*.go' ':!core/keychain/**' ':!scripts/**' || true)
	found+=$'\n'
	found+=$(git grep -nE "$go_lib" -- '*.go' ':!*_test.go' ':!core/keychain/**' ':!scripts/**' || true)
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
		printf 'package tool\nvar _ = exec.Command("security", "find-generic-password")\n' >scripts/tool/t.go
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
