#!/usr/bin/env bash
# Assert that the `vite` alias names the core build the installed vite-plus
# uses, and that CI installs the same vp CLI the workspace declares.
#
# vite-plus ships its own vendored vite/rolldown build as
# @voidzero-dev/vite-plus-core, and pnpm-workspace.yaml aliases the bare `vite`
# package to that build so externally-installed plugins (@vitejs/plugin-react,
# @tailwindcss/vite) resolve `vite` to the SAME build vite-plus's defineConfig
# expects. Each vite-plus release depends on one exact core version, and from
# vite-plus 1.7.0 that version is not the vite-plus version: it encodes the
# bundled Vite (vite-plus 1.7.0 depends on core 8.3.8000, for Vite 8.3.8). So
# the alias must name exactly the core that the catalog's vite-plus release
# declares in its own package.json, whatever its version string.
#
# That declaration is read from node_modules/vite-plus/package.json. The
# lockfile cannot answer it: the `vite@*` override rewrites vite-plus's own
# `vite` dependency to the alias, so the lockfile records the alias, right or
# wrong. The manifest pnpm installs is the published one, and needs no network.
#
# The CI workflows install the `vp` CLI at an explicit version too (setup-vp's
# `version:` input), and setup-vp fetches the install script from that
# version's release tag, so the CLI must be the catalog's vite-plus version.
#
# Dependabot bumps the catalog entry but cannot follow the npm-alias
# indirection to bump the core pin, and nothing bumps a workflow's `version:`
# input. Drift splits the Plugin/UserConfig type universes and trips TS2321
# "Excessive stack depth" / TS2769 in every non-trivial vite.config.ts, or
# leaves the vendored core unable to load its rolldown binding. `vp migrate`
# moves the catalog and the alias together; this guard fails the build on any
# remaining drift so the mismatch is caught at PR time.
#
# Usage:
#   audit-vite-alias.sh              check the CI pins; check the alias too when
#                                    node_modules holds the catalog's vite-plus
#   audit-vite-alias.sh --installed  require the install, and check the alias
#   audit-vite-alias.sh --self-test  run the checks against passing and failing
#                                    fixtures
#   --root DIR                       check DIR instead of the repository
#
# Every failure is one line on stderr that says what to do.
set -euo pipefail

repo_root="$(cd "$(dirname "$0")/.." && pwd)"
script="$repo_root/scripts/audit-vite-alias.sh"

root="$repo_root"
installed=0
self_test=0
while [ $# -gt 0 ]; do
  case "$1" in
    --installed) installed=1 ;;
    --self-test) self_test=1 ;;
    --root)
      root="$2"
      shift
      ;;
    *)
      echo "audit-vite-alias: unknown argument $1; use --installed, --self-test or --root DIR" >&2
      exit 2
      ;;
  esac
  shift
done

# ── self-test ────────────────────────────────────────────────────────────────

# fixture DIR CATALOG ALIAS_CORE SETUP_VP INSTALLED_VERSION INSTALLED_CORE
# writes a minimal repository; an empty INSTALLED_VERSION means no install.
fixture() {
  local dir="$1" catalog="$2" alias_core="$3" setup_vp="$4" inst_version="$5" inst_core="$6"
  mkdir -p "$dir/.github/workflows"
  cat >"$dir/pnpm-workspace.yaml" <<EOF
catalog:
  vite: npm:@voidzero-dev/vite-plus-core@$alias_core
  vite-plus: $catalog
EOF
  cat >"$dir/.github/workflows/ci.yml" <<EOF
jobs:
  build:
    steps:
      - uses: actions/setup-go@v6
        with:
          version: "1.26.0"
      - uses: voidzero-dev/setup-vp@v1.22.0
        with:
          version: "$setup_vp"
          node-version: "26"
      - run: vp install
EOF
  if [ -n "$inst_version" ]; then
    mkdir -p "$dir/node_modules/vite-plus"
    cat >"$dir/node_modules/vite-plus/package.json" <<EOF
{
  "name": "vite-plus",
  "version": "$inst_version",
  "dependencies": {
    "oxlint": "=1.94.0",
    "vite": "npm:@voidzero-dev/vite-plus-core@$inst_core",
    "vitest": "5.1.0"
  }
}
EOF
  fi
}

if [ "$self_test" -eq 1 ]; then
  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' EXIT
  failed=0
  n=0
  # A Windows runner: every checked-out file has CRLF line endings, and a
  # python3 that writes CRLF the way Python's text mode does on Windows.
  mkdir -p "$tmp/crlf-bin"
  real_python=$(command -v python3)
  cat >"$tmp/crlf-bin/python3" <<EOF
#!/usr/bin/env bash
set -o pipefail
"$real_python" "\$@" | sed 's/\$/\r/'
EOF
  chmod +x "$tmp/crlf-bin/python3"
  crlf=0

  # case WANT_EXIT DESCRIPTION FLAGS -- fixture args
  check_case() {
    local want="$1" desc="$2" flags="$3"
    shift 3
    n=$((n + 1))
    local dir="$tmp/case$n"
    fixture "$dir" "$@"
    local path="$PATH"
    if [ "$crlf" -eq 1 ]; then
      local f
      while IFS= read -r f; do
        sed -i.bak 's/$/\r/' "$f" && rm -f "$f.bak"
      done < <(find "$dir" -type f)
      path="$tmp/crlf-bin:$PATH"
    fi
    local rc=0
    # shellcheck disable=SC2086 # FLAGS is a word list on purpose
    PATH="$path" bash "$script" --root "$dir" $flags >"$dir.out" 2>"$dir.err" || rc=$?
    local lines
    lines=$(wc -l <"$dir.err" | tr -d ' ')
    if [ "$want" -eq 0 ] && [ "$rc" -eq 0 ]; then
      echo "ok   pass: $desc"
    elif [ "$want" -ne 0 ] && [ "$rc" -ne 0 ] && [ "$lines" -eq 1 ]; then
      echo "ok   fail: $desc ($(cat "$dir.err"))"
    else
      echo "FAIL $desc: exit $rc, want $( [ "$want" -eq 0 ] && echo 0 || echo non-zero), $lines stderr lines" >&2
      cat "$dir.err" >&2
      failed=1
    fi
  }

  # The arrangements a release can be in.
  check_case 0 "1.6.0: core shares the vite-plus version" "--installed" \
    1.6.0 1.6.0 1.6.0 1.6.0 1.6.0
  check_case 0 "1.7.0: core carries its own version" "--installed" \
    1.7.0 8.3.8000 1.7.0 1.7.0 8.3.8000
  check_case 0 "before install: the alias check waits for node_modules" "" \
    1.7.0 8.3.8000 1.7.0 "" ""
  check_case 0 "before install: a stale node_modules is not read" "" \
    1.7.0 8.3.8000 1.7.0 1.6.0 1.6.0

  # The drift it exists to catch.
  check_case 1 "1.7.0 with the alias bumped to the vite-plus version" "--installed" \
    1.7.0 1.7.0 1.7.0 1.7.0 8.3.8000
  check_case 1 "alias left on the previous release's core" "--installed" \
    1.7.0 1.6.0 1.7.0 1.7.0 8.3.8000
  check_case 1 "alias drift is reported without --installed once installed" "" \
    1.8.0 8.3.8000 1.8.0 1.8.0 8.4.1000
  check_case 1 "a setup-vp version: that lags the catalog" "" \
    1.7.0 8.3.8000 1.6.0 "" ""
  check_case 1 "--installed with nothing installed" "--installed" \
    1.7.0 8.3.8000 1.7.0 "" ""
  check_case 1 "--installed with node_modules from another release" "--installed" \
    1.7.0 8.3.8000 1.7.0 1.6.0 1.6.0

  # The same checks on a Windows runner's CRLF files and CRLF python3 output.
  crlf=1
  check_case 0 "CRLF: 1.7.0: core carries its own version" "--installed" \
    1.7.0 8.3.8000 1.7.0 1.7.0 8.3.8000
  check_case 1 "CRLF: alias left on the previous release's core" "--installed" \
    1.7.0 1.6.0 1.7.0 1.7.0 8.3.8000
  check_case 1 "CRLF: a setup-vp version: that lags the catalog" "" \
    1.7.0 8.3.8000 1.6.0 "" ""
  crlf=0

  if [ "$failed" -ne 0 ]; then
    echo "audit-vite-alias: self-test failed" >&2
    exit 1
  fi
  echo "audit-vite-alias: self-test passed ($n cases)"
  exit 0
fi

# ── check ────────────────────────────────────────────────────────────────────

cd "$root"

semver='[0-9]+[.][0-9]+[.][0-9]+'
version='[0-9]+\.[0-9]+\.[0-9]+([-+][0-9A-Za-z.-]+)?'

catalog=$(grep -oE "^[[:space:]]+vite-plus:[[:space:]]*$semver" pnpm-workspace.yaml | grep -oE "$semver" | head -1 || true)
alias_core=$(grep -oE "^[[:space:]]+vite:[[:space:]]*['\"]?npm:@voidzero-dev/vite-plus-core@$version" pnpm-workspace.yaml |
  sed -E 's/.*vite-plus-core@//' | head -1 || true)

if [ -z "$catalog" ]; then
  echo "audit-vite-alias: pnpm-workspace.yaml has no 'vite-plus: X.Y.Z' catalog entry; add one" >&2
  exit 1
fi
if [ -z "$alias_core" ]; then
  echo "audit-vite-alias: pnpm-workspace.yaml has no 'vite: npm:@voidzero-dev/vite-plus-core@<version>' catalog alias; add one (npm view vite-plus@$catalog dependencies.vite)" >&2
  exit 1
fi

# Every workflow that installs the vp CLI names the version explicitly: the
# `version:` input inside the `with:` block of each setup-vp step. Other
# actions take a `version:` too (setup-go, for one), so only the lines between
# a setup-vp `uses:` and the next step count.
ci_files=$(grep -rlE 'uses: voidzero-dev/setup-vp@' .github 2>/dev/null || true)
for f in $ci_files; do
  while IFS= read -r pinned; do
    if [ "$pinned" != "$catalog" ]; then
      echo "audit-vite-alias: $f installs vp $pinned but the catalog's vite-plus is $catalog; set that setup-vp version: to \"$catalog\"" >&2
      exit 1
    fi
  done < <(awk -v re="$semver" '
    /uses: voidzero-dev\/setup-vp@/ { in_step = 1; next }
    in_step && /^[[:space:]]*-[[:space:]]/ { in_step = 0 }
    in_step && /uses:/ { in_step = 0 }
    in_step && match($0, "^[[:space:]]*version:[[:space:]]*\"" re "\"") {
      v = $0; sub(/^[[:space:]]*version:[[:space:]]*"/, "", v); sub(/".*$/, "", v); print v
    }' "$f")
done

manifest=node_modules/vite-plus/package.json
inst_version=""
inst_vite=""
if [ -f "$manifest" ]; then
  # Python on Windows writes print()'s newline as CRLF, and `read` keeps the
  # carriage return in the last field, so the versions compared below would
  # differ by an invisible character. The carriage return is removed here.
  read -r inst_version inst_vite < <(python3 -c '
import json, sys
m = json.load(open(sys.argv[1]))
print(m.get("version", "-"), (m.get("dependencies") or {}).get("vite", "-"))
' "$manifest" | tr -d '\r')
fi

if [ "$inst_version" != "$catalog" ]; then
  if [ "$installed" -eq 1 ]; then
    echo "audit-vite-alias: node_modules holds vite-plus ${inst_version:-nothing}, the catalog names $catalog; run vp install first" >&2
    exit 1
  fi
  echo "audit-vite-alias: the CI pins agree with vite-plus $catalog; the alias (core $alias_core) is checked once vp install has run"
  exit 0
fi

want_core="${inst_vite#npm:@voidzero-dev/vite-plus-core@}"
if [ "$want_core" = "$inst_vite" ]; then
  echo "audit-vite-alias: vite-plus $catalog declares vite as '$inst_vite', not an @voidzero-dev/vite-plus-core alias; update this script for the new layout" >&2
  exit 1
fi
if [ "$want_core" != "$alias_core" ]; then
  echo "audit-vite-alias: vite-plus $catalog depends on @voidzero-dev/vite-plus-core@$want_core but pnpm-workspace.yaml aliases vite to @$alias_core; set the catalog's vite to npm:@voidzero-dev/vite-plus-core@$want_core and run vp install" >&2
  exit 1
fi

echo "audit-vite-alias: vite-plus $catalog, its vite alias (core $alias_core) and the CI pins agree"
