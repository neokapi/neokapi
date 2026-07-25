#!/usr/bin/env bash
#
# Guard: re-resolving pnpm-lock.yaml with the pinned pnpm must be a no-op.
#
# The committed lockfile must be exactly what `package.json#packageManager`'s
# pnpm produces. If re-resolving changes it, the lockfile was written by a
# different resolver and the repository is carrying someone else's output.
#
# This is the npm counterpart of the `go mod tidy` no-op expectation, and it is
# deliberately a *stability* invariant rather than a *content* one.
#
# Why not check lockfile content instead:
#
#   An earlier version of this guard asserted that every registry resolution
#   keeps its `tarball:` provenance URL, because Dependabot's bundled pnpm
#   strips the field. That invariant turned out not to be satisfiable: the
#   pinned pnpm preserves whatever form it is handed, but in a clean CI
#   environment it *emits* the stripped form — so CI could not reproduce the
#   committed file, and the guard demanded output the toolchain would not
#   produce. It blocked every npm update while protecting nothing.
#
#   Idempotence has none of that failure mode. It is satisfiable by
#   construction — commit whatever the pinned toolchain emits — and it still
#   catches the original defect, because a Dependabot lockfile written by a
#   different resolver will not survive a re-resolve unchanged.
#
# What this does NOT check: that the lockfile agrees with package.json. That is
# already covered by `vp install --frozen-lockfile`, which every CI job that
# installs dependencies runs via .github/actions/setup-node-ui.
#
# Note this command REWRITES pnpm-lock.yaml. On failure the regenerated file is
# left in the working tree on purpose, so the fix is to review and commit it.
#
# Usage:
#     ./scripts/check-lockfile-idempotent.sh

set -euo pipefail

LOCKFILE="pnpm-lock.yaml"

if [[ ! -f "$LOCKFILE" ]]; then
  echo "error: $LOCKFILE not found — run from the repository root" >&2
  exit 1
fi

before="$(git hash-object "$LOCKFILE")"

# `vp` resolves the pnpm pinned in package.json#packageManager. A bare `pnpm` now
# resolves the same pinned version — #1461 fixed the `packageManagerRegistries`
# setting that made pnpm's self-install fail on any machine whose default registry
# is a mirror — so either would work today. `vp` is kept because it is what every
# CI job and Makefile target already uses, and it needs no `packageManager`
# self-install step at all.
if ! command -v vp >/dev/null 2>&1; then
  echo "error: vp (vite-plus) not found — it provides the pinned pnpm" >&2
  exit 1
fi

vp install --lockfile-only

after="$(git hash-object "$LOCKFILE")"

if [[ "$before" != "$after" ]]; then
  echo "error: $LOCKFILE is not what the pinned pnpm produces." >&2
  echo >&2
  echo "Re-resolving changed it, which means the committed lockfile was written" >&2
  echo "by a different resolver — most often a Dependabot npm PR, whose bundled" >&2
  echo "pnpm differs from package.json#packageManager." >&2
  echo >&2
  echo "Summary of the difference:" >&2
  git --no-pager diff --stat -- "$LOCKFILE" >&2 || true
  echo >&2
  echo "The regenerated lockfile is in your working tree. Review and commit it." >&2
  exit 1
fi

echo "pnpm-lock.yaml is what the pinned pnpm produces (re-resolve was a no-op)"
