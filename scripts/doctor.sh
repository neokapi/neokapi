#!/usr/bin/env bash
# doctor.sh — report whether this machine can build and test neokapi.
#
# A bare `go test ./...` in this repo fails two ways that name neither cause:
# without -tags fts5 the content-memory and terms migrations fail at runtime
# with "no such function: fts5", and without ICU on PKG_CONFIG_PATH the cgo
# packages fail with a bare "[build failed]". `make` handles both; this reports
# what is missing when it cannot.
#
# Invoked as `make doctor`. Read-only: it installs nothing and changes nothing.
set -uo pipefail

missing=0
optional_missing=0

ok()   { printf '  \033[32mok\033[0m       %s\n' "$1"; }
bad()  { printf '  \033[31mmissing\033[0m  %s\n' "$1"; missing=$((missing + 1)); }
warn() { printf '  \033[33moptional\033[0m %s\n' "$1"; optional_missing=$((optional_missing + 1)); }

echo "Required to build and test:"

if command -v go >/dev/null 2>&1; then
	ok "go — $(go version | awk '{print $3}')"
else
	bad "go — https://go.dev/dl/"
fi

if command -v pkg-config >/dev/null 2>&1; then
	ok "pkg-config"
else
	bad "pkg-config — cgo cannot locate native libraries without it"
fi

# ICU is checked through pkg-config with the same PKG_CONFIG_PATH the Makefile
# exports, so this answers the question that actually matters: can the cgo
# build find ICU as invoked via make?
if command -v pkg-config >/dev/null 2>&1 && pkg-config --exists icu-uc icu-i18n 2>/dev/null; then
	ok "ICU (icu-uc, icu-i18n) — $(pkg-config --modversion icu-uc 2>/dev/null)"
else
	case "$(uname -s)" in
	Darwin) bad "ICU — brew bundle   (installs icu4c; the Makefile finds the keg)" ;;
	Linux) bad "ICU — sudo apt-get install libicu-dev pkg-config" ;;
	*) bad "ICU development libraries (icu-uc, icu-i18n)" ;;
	esac
fi

echo
echo "Required for the frontend workspaces:"

if command -v node >/dev/null 2>&1; then
	ok "node — $(node --version)"
else
	bad "node — the pnpm workspace at the repo root needs it"
fi

if command -v vp >/dev/null 2>&1; then
	ok "vp (Vite+) — run 'vp install' at the repo root, never per-directory"
else
	bad "vp (Vite+) — https://viteplus.dev ; this repo uses vp, not npx"
fi

echo
echo "Optional — only for specific work:"

if command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then
	ok "docker — Postgres-backed suites (make test-integration) can run"
else
	warn "docker — without it the Postgres-backed suites skip themselves"
fi

if command -v golangci-lint >/dev/null 2>&1; then
	ok "golangci-lint — make lint"
else
	warn "golangci-lint — needed by 'make lint' / 'make pre-push'"
fi

if command -v onnxruntime_test >/dev/null 2>&1 || [ -n "$(ls -d /opt/homebrew/opt/onnxruntime /usr/local/opt/onnxruntime 2>/dev/null)" ]; then
	ok "onnxruntime — the -tags onnx ML plugin builds"
else
	warn "onnxruntime — only for '-tags onnx' plugin builds (kapi-sat, kapi-check)"
fi

echo
if [ "$missing" -gt 0 ]; then
	echo "$missing required item(s) missing."
	case "$(uname -s)" in
	Darwin) echo "On macOS, 'brew bundle' at the repo root installs the toolchain (see Brewfile)." ;;
	Linux) echo "On Debian/Ubuntu: sudo apt-get install libicu-dev pkg-config" ;;
	esac
	exit 1
fi

if [ "$optional_missing" -gt 0 ]; then
	echo "Everything required is present. $optional_missing optional item(s) absent — fine unless you need them."
else
	echo "Everything is present."
fi
