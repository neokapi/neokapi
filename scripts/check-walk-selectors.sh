#!/usr/bin/env bash
#
# Guard: every selector a recorded walk drives still exists in the apps.
#
# The walkthrough videos are documentation, and the only thing that reads the
# apps' testids, slots and rail labels from outside the apps is
# harness/src/driver/record-desktop.ts. Nothing compiles it against them and
# nothing runs it in CI, so a rename is silent until someone tries to re-record:
# in one sweep nine of the twelve registered walks had stopped working, four of
# them on a one-word rename (`sidebar-*` to `subnav-*`, `Termbases` to `Terms`,
# `AI Credentials` to `AI Models`). Several did not even throw, because a beat
# wrapped in `if (await n.count())` records the wrong screen under the right
# narration.
#
# What is checked: every structural handle the recorder names.
#
#     [data-testid="x"]   [data-testid^="x"]   getByTestId("x")
#     [data-slot="x"]     [data-preview="x"]   [aria-label="x"]
#     sidebar("Label")    tab("Label")         contextSection(page, "Label")
#
# Each must occur in the frontend sources the recorder drives. A templated
# attribute counts: `data-testid={`subnav-${item.id}`}` renders `subnav-source`,
# so a value is also accepted when its stem is a template in the sources and
# the remainder appears there as a quoted string.
#
# What is NOT checked, and why: free text matchers (`:has-text("…")`,
# `getByText("…")`, `getByRole(…, { name })`). Their strings are as often
# project data as UI source. A flow name comes from the sample project's recipe,
# a collection name from its collections, a store name from cmd/seed-demo, a
# language name from CLDR. Failing on those would train everyone to ignore this
# check, so they are listed as advisory and never gate.
#
# Usage:
#     ./scripts/check-walk-selectors.sh              # check the recorder
#     ./scripts/check-walk-selectors.sh --self-test  # prove the matcher both ways
#
# Wired into `make check-walk-selectors` (part of `make lint`), `make pre-push`,
# and the repo-guards job in .github/workflows/ci.yml.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

readonly RECORDER=harness/src/driver/record-desktop.ts

# The frontends the recorder drives: the kapi desktop app, the shared UI kit it
# and bowrain both mount, and the bowrain web + desktop apps.
readonly APP_DIRS=(
  apps/kapi-desktop/frontend/src
  packages
  bowrain/packages
  bowrain/apps
)

# ── extraction ───────────────────────────────────────────────────────────────

# structural_selectors prints one selector per line, as "kind<TAB>value", for
# every structural handle named in the file on stdin.
structural_selectors() {
  # A value carrying `${` is the recorder's own helper (sidebar(), the layer
  # slot builder), not a selector: its argument is checked where it is called.
  perl -ne '
    sub emit { my ($k, $v) = @_; return if $v =~ /\$\{/; print "$k\t$v\n" }
    while (/\[data-testid\^?=\\?"([^"\\]+)\\?"\]/g)   { emit("testid", $1) }
    while (/getByTestId\("([^"]+)"\)/g)               { emit("testid", $1) }
    while (/\[data-slot\^?=\\?"([^"\\]+)\\?"\]/g)     { emit("slot", $1) }
    while (/\[data-preview=\\?"([^"\\]+)\\?"\]/g)     { emit("preview", $1) }
    while (/\[aria-label=\\?"([^"\\]+)\\?"\]/g)       { emit("label", $1) }
    while (/\bsidebar\("([^"]+)"\)/g)                 { emit("label", $1) }
    while (/\btab\("([^"]+)"\)/g)                     { emit("text", $1) }
    while (/contextSection\(page, "([^"]+)"\)/g)      { emit("text", $1) }
  ' | sort -u
}

# advisory_selectors prints the free-text matchers, which are reported but never
# gate (see the header).
advisory_selectors() {
  perl -ne '
    while (/:has-text\("([^"]+)"\)/g)     { print "$1\n" }
    while (/getByText\("([^"]+)"/g)       { print "$1\n" }
  ' | sort -u
}

# ── the haystack ─────────────────────────────────────────────────────────────

# haystack_file writes every tracked frontend source under APP_DIRS into one
# file, so each lookup is a single grep over it rather than a tree walk.
haystack_file() {
  local out="$1" shift_unused
  git ls-files -z -- "${APP_DIRS[@]}" |
    tr '\0' '\n' |
    grep -E '\.(ts|tsx)$' |
    grep -vE '(^|/)__tests__/|\.test\.tsx?$|\.stories\.tsx?$' |
    while IFS= read -r f; do cat "$f"; printf '\n'; done >"$out"
}

# present reports whether one selector value is rendered by the haystack.
#
# Literal first. Failing that, a templated attribute: `foo-${x}` in the sources
# renders `foo-<anything>`, so accept `foo-bar` when `foo-${` appears and `bar`
# appears as a quoted string. That is the shape of `subnav-${item.id}` over ids
# declared as `id: "source"`, and of `queue-row-${entry.id}`.
present() {
  local value="$1" hay="$2" stem rest
  grep -qF -- "$value" "$hay" && return 0
  case "$value" in
    *-*) ;;
    *) return 1 ;;
  esac
  # Try every split point, longest stem first, so `review-inbox-project-` finds
  # its own template rather than `review-`.
  rest="$value"
  stem=""
  while [ -n "$rest" ]; do
    case "$rest" in
      *-*) ;;
      *) break ;;
    esac
    stem="${stem}${rest%%-*}-"
    rest="${rest#*-}"
    grep -qF -- "${stem}\${" "$hay" || continue
    [ -z "$rest" ] && return 0
    grep -qE -- "[\"'\`]${rest}[\"'\`]" "$hay" && return 0
  done
  return 1
}

# check_recorder reports every structural selector in $1 that no app renders.
check_recorder() {
  local file="$1" hay="$2" kind value missing=0
  while IFS=$'\t' read -r kind value; do
    [ -n "$value" ] || continue
    if ! present "$value" "$hay"; then
      echo "  ${kind} \"${value}\" is named by ${file} and rendered by no app source"
      missing=$((missing + 1))
    fi
  done < <(structural_selectors <"$file")
  return "$missing"
}

# ── self-test ────────────────────────────────────────────────────────────────

self_test() {
  local tmp status=0 out
  tmp="$(mktemp -d)"
  # shellcheck disable=SC2064  # expand $tmp now, not at trap time
  trap "rm -rf '$tmp'" EXIT

  cat >"$tmp/app.tsx" <<'EOF'
export function Rail() {
  return (
    <>
      <button aria-label="Toolbox" data-testid="rail-toolbox" />
      <div data-slot="review-queue" />
      <span data-preview="keyed-table" />
      {items.map((item) => (
        <a key={item.id} data-testid={`subnav-${item.id}`} />
      ))}
    </>
  );
}
const items = [{ id: "source" }, { id: "runs" }];
EOF

  cat >"$tmp/walk.ts" <<'EOF'
const sidebar = (label: string) => page.locator(`button[aria-label="${label}"]`);
await humanClick(page, sidebar("Toolbox"));
await page.getByTestId("subnav-source");
await page.locator('[data-slot="review-queue"]');
await page.locator('[data-preview="keyed-table"]');
await page.locator('[data-testid="rail-toolbox"]');
EOF

  cat >"$tmp/broken.ts" <<'EOF'
const sidebar = (label: string) => page.locator(`button[aria-label="${label}"]`);
await humanClick(page, sidebar("Termbases"));
await page.getByTestId("sidebar-connectors");
EOF

  if out=$(check_recorder "$tmp/walk.ts" "$tmp/app.tsx"); then
    echo "✓ self-test: a walk whose selectors all exist passes"
  else
    echo "✖ self-test: the matcher flagged a walk whose selectors all exist:"
    printf '%s\n' "$out"
    status=1
  fi

  if out=$(check_recorder "$tmp/broken.ts" "$tmp/app.tsx"); then
    echo "✖ self-test: the matcher did NOT flag a renamed label or a dead testid"
    status=1
  else
    if printf '%s\n' "$out" | grep -q 'Termbases' &&
      printf '%s\n' "$out" | grep -q 'sidebar-connectors'; then
      echo "✓ self-test: flags a renamed rail label and a dead testid (2 findings)"
    else
      echo "✖ self-test: expected both findings, got:"
      printf '%s\n' "$out"
      status=1
    fi
  fi

  return "$status"
}

# ── main ─────────────────────────────────────────────────────────────────────

if [ "${1:-}" = "--self-test" ]; then
  self_test
  exit $?
fi

# A matcher that silently stops matching is worse than no check at all.
if ! self_test >/dev/null; then
  echo "✖ check-walk-selectors.sh: self-test failed, the matcher is broken."
  self_test || true
  exit 1
fi

if [ ! -f "$RECORDER" ]; then
  echo "✖ walk selectors: $RECORDER is missing"
  exit 1
fi

hay="$(mktemp)"
# shellcheck disable=SC2064  # expand the path now, not at trap time
trap "rm -f '$hay'" EXIT
haystack_file "$hay"

total=$(structural_selectors <"$RECORDER" | grep -c . || true)

if out=$(check_recorder "$RECORDER" "$hay"); then
  echo "✓ walk selectors: all ${total} structural selectors in $RECORDER are rendered by an app"
  advisory=$(advisory_selectors <"$RECORDER" | grep -c . || true)
  echo "  (${advisory} free-text matcher(s) not checked; see the header)"
  exit 0
fi

echo "✖ walk selectors: a recorded walk drives a selector no app renders."
echo "  Nothing fails when this drifts: the walk throws mid-recording, or a"
echo "  guarded beat records the wrong screen under the right narration."
printf '%s\n' "$out"
exit 1
