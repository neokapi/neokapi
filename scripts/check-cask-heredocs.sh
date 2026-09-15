#!/usr/bin/env bash
#
# Guard: the Homebrew casks the release workflows write carry no install hook
# and no retired vocabulary in their description.
#
# release.yml and release-bowrain.yml write the Kapi and Bowrain casks, stable
# and beta, into neokapi/homebrew-tap from heredocs on every tag. Two things
# have shipped in them:
#
#   - a `postflight` block. Homebrew 7 deprecates the block forms, and brew
#     prints "Calling `postflight` is deprecated!" whenever it loads the cask.
#     The block stripped the quarantine attribute, which the declarative
#     `postflight_steps` cannot express and which the apps do not need: both
#     are notarized and stapled, and stripping quarantine sidesteps Gatekeeper.
#   - a `desc` in retired vocabulary. deploy/homebrew/*.rb is read by
#     `make check-governed-prose`, but the heredocs are what reach users, so
#     each `desc` here is matched against the pattern check-vocabulary.sh uses.
#   - a `zap` entry naming ~/Library/Application Support/Kapi or
#     ~/Library/Caches/Kapi. A default macOS volume is case-insensitive, so
#     those are the kapi CLI's own config and cache roots (flows, plugins, terms
#     stores, content memory), and `brew uninstall --zap` on the desktop app
#     would delete them. Bowrain is the same for the bowrain plugin's sign-in
#     and config. Each app keeps its own data under `kapi-desktop` or
#     `bowrain-desktop` and its bundle id.
#
# deploy/homebrew/*.rb is held to the install-hook and zap rules too.
set -euo pipefail

cd "$(dirname "$0")/.."

readonly WORKFLOWS=(.github/workflows/release.yml .github/workflows/release-bowrain.yml)
readonly HOOK_RE='^[0-9]+:[[:space:]]+(uninstall_)?(pre|post)flight([[:space:]]|$)'
# Matched case-insensitively, as the volume resolves it.
readonly SHARED_DIR_RE='"~/Library/(Application Support|Caches)/(kapi|bowrain)/?"'

vocab_re=$(sed -n "s/^readonly RETIRED_VOCAB_RE='\(.*\)'\$/\1/p" scripts/check-vocabulary.sh)
if [ -z "$vocab_re" ]; then
  echo "check-cask-heredocs: could not read RETIRED_VOCAB_RE from scripts/check-vocabulary.sh" >&2
  exit 1
fi

# cask_lines prints every line inside a workflow's `<< CASK` heredocs, prefixed
# with its line number in the workflow.
cask_lines() {
  awk '
    /<< CASK$/ { inside = 1; next }
    inside && /^[[:space:]]*CASK$/ { inside = 0; next }
    inside { print FNR ": " $0 }
  ' "$1"
}

fail=0
report() {
  printf 'check-cask-heredocs: %s\n' "$1" >&2
  printf '%s\n' "$2" | sed 's/^/    line /' >&2
  fail=1
}

for wf in "${WORKFLOWS[@]}"; do
  lines=$(cask_lines "$wf")
  if [ -z "$lines" ]; then
    echo "check-cask-heredocs: no << CASK heredoc in $wf; this guard needs updating" >&2
    fail=1
    continue
  fi

  hooks=$(printf '%s\n' "$lines" | grep -E "$HOOK_RE" || true)
  [ -n "$hooks" ] && report "$wf writes a cask with an install hook" "$hooks"

  descs=$(printf '%s\n' "$lines" | grep -E '^[0-9]+:[[:space:]]+desc[[:space:]]' || true)
  if [ -z "$descs" ]; then
    echo "check-cask-heredocs: no desc in the casks $wf writes" >&2
    fail=1
  else
    retired=$(printf '%s\n' "$descs" | RE="$vocab_re" perl -ne 'print if /$ENV{RE}/' || true)
    [ -n "$retired" ] && report "$wf writes a cask desc in retired vocabulary" "$retired"
  fi

  shared=$(printf '%s\n' "$lines" | grep -i -E "$SHARED_DIR_RE" || true)
  [ -n "$shared" ] && report "$wf writes a cask that zaps a directory the CLI shares" "$shared"
done

for f in deploy/homebrew/*.rb; do
  [ -f "$f" ] || continue
  numbered=$(grep -n '' "$f" | sed 's/^\([0-9]*\):/\1: /')
  hooks=$(printf '%s\n' "$numbered" | grep -E "$HOOK_RE" || true)
  [ -n "$hooks" ] && report "$f has an install hook" "$hooks"
  shared=$(printf '%s\n' "$numbered" | grep -i -E "$SHARED_DIR_RE" || true)
  [ -n "$shared" ] && report "$f zaps a directory the CLI shares" "$shared"
done

if [ "$fail" -ne 0 ]; then
  cat >&2 <<'MSG'

A cask takes no preflight or postflight block: Homebrew deprecates them, and
the notarized apps need none. A cask desc uses the fixed vocabulary of
docs/internals/brand-communication.md. A cask zaps only its app's own data
(kapi-desktop, bowrain-desktop, the bundle id), never the kapi or bowrain
directories the CLI and its plugins keep under Application Support and Caches.
MSG
  exit 1
fi

echo "check-cask-heredocs: the release casks carry no install hook and no retired vocabulary"
