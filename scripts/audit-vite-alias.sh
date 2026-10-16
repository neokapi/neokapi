#!/usr/bin/env bash
# Assert that every statement of the Vite+ version agrees.
#
# vite-plus ships its own vendored vite/rolldown build as
# @voidzero-dev/vite-plus-core, and pnpm-workspace.yaml aliases the bare `vite`
# package to that build so externally-installed plugins (@vitejs/plugin-react,
# @tailwindcss/vite) resolve `vite` to the SAME build vite-plus's defineConfig
# expects. vite-plus and its vendored core are versioned 1:1, so the alias MUST
# pin the same version as the `vite-plus` catalog entry every package depends
# on through `catalog:`. The CI workflows install the `vp` CLI at an explicit
# version too (setup-vp's `version:` input), and setup-vp fetches the install
# script from that version's release tag, so the CLI must be the same version
# as well.
#
# Dependabot bumps the catalog entry but cannot follow the npm-alias
# indirection to bump the core pin, and nothing bumps a workflow's `version:`
# input. Drift splits the Plugin/UserConfig type universes and trips TS2321
# "Excessive stack depth" / TS2769 in every non-trivial vite.config.ts, or runs
# CI on a different `vp` than the workspace. `vp migrate` moves the catalog and
# the alias together; this guard fails the build on any remaining drift so the
# mismatch is caught at PR time.
set -euo pipefail

cd "$(dirname "$0")/.."

semver='[0-9]+\.[0-9]+\.[0-9]+'

catalog=$(grep -oE "^[[:space:]]+vite-plus:[[:space:]]*$semver" pnpm-workspace.yaml | grep -oE "$semver" | head -1)
alias_core=$(grep -oE "vite-plus-core@$semver" pnpm-workspace.yaml | grep -oE "$semver" | head -1)

if [ -z "$catalog" ]; then
  echo "audit-vite-alias: could not read the vite-plus catalog entry from pnpm-workspace.yaml" >&2
  exit 1
fi
if [ -z "$alias_core" ]; then
  echo "audit-vite-alias: could not read the vite catalog alias from pnpm-workspace.yaml" >&2
  exit 1
fi

if [ "$catalog" != "$alias_core" ]; then
  cat >&2 <<EOF
audit-vite-alias: version drift between vite-plus and its vite catalog alias.

  pnpm-workspace.yaml  catalog vite-plus           = $catalog
  pnpm-workspace.yaml  vite -> vite-plus-core@     = $alias_core

vite-plus and @voidzero-dev/vite-plus-core are versioned 1:1, so the alias must
match the catalog entry. Run \`vp migrate\` (it moves both), or update the
alias to @voidzero-dev/vite-plus-core@$catalog and re-run \`vp install\`, then
commit the refreshed pnpm-lock.yaml.
EOF
  exit 1
fi

# Every workflow that installs the vp CLI names the version explicitly: the
# `version:` input inside the `with:` block of each setup-vp step. Other
# actions take a `version:` too (setup-go, for one), so only the lines between
# a setup-vp `uses:` and the next step count.
ci_files=$(grep -rlE 'uses: voidzero-dev/setup-vp@' .github)
drift=0
for f in $ci_files; do
  while IFS= read -r pinned; do
    if [ "$pinned" != "$catalog" ]; then
      echo "audit-vite-alias: $f installs vp $pinned, the workspace is on $catalog" >&2
      drift=1
    fi
  done < <(awk -v re="$semver" '
    /uses: voidzero-dev\/setup-vp@/ { in_step = 1; next }
    in_step && /^[[:space:]]*-[[:space:]]/ { in_step = 0 }
    in_step && /uses:/ { in_step = 0 }
    in_step && match($0, "^[[:space:]]*version:[[:space:]]*\"" re "\"") {
      v = $0; sub(/^[[:space:]]*version:[[:space:]]*"/, "", v); sub(/".*$/, "", v); print v
    }' "$f")
done
if [ "$drift" -ne 0 ]; then
  cat >&2 <<EOF

setup-vp installs the vp CLI at the \`version:\` named in the workflow, and
fetches the install script from that version's release tag. Set every
\`version:\` to $catalog so CI runs the same vp the workspace declares.
EOF
  exit 1
fi

echo "audit-vite-alias: vite-plus, its vite catalog alias and the CI pins agree ($catalog)"
