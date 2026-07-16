#!/usr/bin/env bash
# Assert the `vite` catalog alias tracks the vite-plus devDependency version.
#
# vite-plus 0.2.x ships its own vendored vite/rolldown build as
# @voidzero-dev/vite-plus-core, and pnpm-workspace.yaml aliases the bare `vite`
# package to that build so externally-installed plugins (@vitejs/plugin-react,
# @tailwindcss/vite) resolve `vite` to the SAME build vite-plus's defineConfig
# expects. vite-plus and its vendored core are versioned 1:1 (vite-plus 0.2.4 →
# core 0.2.4), so the alias MUST pin the same version as the vite-plus devDep.
#
# Dependabot bumps the vite-plus devDependency but cannot follow the npm-alias
# indirection to bump the catalog entry, so the two silently drift — splitting
# the Plugin/UserConfig type universes and tripping TS2321 "Excessive stack
# depth" / TS2769 in every non-trivial vite.config.ts. This guard fails the
# build on that drift so the mismatch is caught at PR time, not by a puzzling
# type error days later.
set -euo pipefail

cd "$(dirname "$0")/.."

semver='[0-9]+\.[0-9]+\.[0-9]+'

vite_plus=$(grep -oE '"vite-plus":[[:space:]]*"[^"]*"' package.json | grep -oE "$semver" | head -1)
alias_core=$(grep -oE "vite-plus-core@$semver" pnpm-workspace.yaml | grep -oE "$semver" | head -1)

if [ -z "$vite_plus" ]; then
  echo "audit-vite-alias: could not read the vite-plus version from package.json" >&2
  exit 1
fi
if [ -z "$alias_core" ]; then
  echo "audit-vite-alias: could not read the vite catalog alias from pnpm-workspace.yaml" >&2
  exit 1
fi

if [ "$vite_plus" != "$alias_core" ]; then
  cat >&2 <<EOF
audit-vite-alias: version drift between vite-plus and its vite catalog alias.

  package.json         vite-plus                 = $vite_plus
  pnpm-workspace.yaml  vite -> vite-plus-core@   = $alias_core

vite-plus and @voidzero-dev/vite-plus-core are versioned 1:1, so the alias must
match the vite-plus devDependency. Update the catalog entry in
pnpm-workspace.yaml to @voidzero-dev/vite-plus-core@$vite_plus and re-run
\`vp install\`, then commit the refreshed pnpm-lock.yaml.
EOF
  exit 1
fi

echo "audit-vite-alias: vite-plus and vite catalog alias agree ($vite_plus)"
