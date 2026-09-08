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
# The second half checks the CROP selectors in harness/demos/*/demo.yaml, which
# name the element a beat's camera pushes onto. Two things are asserted:
#
#   1. an app renders it, the same way the recorder's selectors are checked;
#   2. it is not a layout container.
#
# The second one is about geometry rather than drift. The composition fits a
# crop region into an area wider than it is tall, and shows the whole window
# for anything that does not fit at 1.15x, so a region taller than roughly two
# thirds of the window can never be cropped into. An element whose own classes
# size it to its parent (`h-full`, `flex-1`, `min-h-0`, `inset-0`) is exactly
# that region: the beat then pushes into the middle of the window instead of
# onto the thing its caption names, and at the docs embed size the app text in
# it is unreadable. Twenty-three beats shipped that way (#2601).
#
# What is NOT checked here: a page section that is tall because of its content
# rather than its classes. Nothing static can see that, so the recorder prints
# the fit of every resolved crop and warns when the composition will fall back
# to the whole window (record-desktop.ts `reportCrop`).
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
readonly DEMOS=harness/demos
# The composition's own threshold (harness/src/lib/crop.ts FULL_WINDOW_FIT).
readonly FULL_WINDOW_FIT_DOC=1.15

# Classes that size an element to its parent. A crop over one of these is
# inert: it never fits the composition's crop area, so the beat records the
# whole window. Height is what binds, so the width-only fills (`w-full`) are
# deliberately absent.
readonly FILL_CLASSES=(h-full h-screen h-dvh min-h-full min-h-screen min-h-0 flex-1 inset-0 size-full)

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
source_files() {
  git ls-files -z -- "${APP_DIRS[@]}" |
    tr '\0' '\n' |
    grep -E '\.(ts|tsx)$' |
    grep -vE '(^|/)__tests__/|\.test\.tsx?$|\.stories\.tsx?$'
}

haystack_file() {
  local out="$1"
  source_files | while IFS= read -r f; do cat "$f"; printf '\n'; done >"$out"
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

# ── crop targets ─────────────────────────────────────────────────────────────

# crop_targets prints "demo/beat<TAB>kind<TAB>value" for every structural
# selector under a `crop:` key in the demo manifests given as arguments.
crop_targets() {
  local f demo
  for f in "$@"; do
    demo="$(basename "$(dirname "$f")")"
    DEMO="$demo" perl -ne '
      BEGIN { our ($beat, $incrop) = ("", 0) }
      if (/^  - id: (\S+)/) { $beat = $1; $incrop = 0 }
      if (/^    crop:/)     { $incrop = 1 }
      elsif (/^    \w/)     { $incrop = 0 }
      next unless $incrop;
      while (/\[data-(testid|slot|preview)="([^"\]]+)"\]/g) {
        print "$ENV{DEMO}/$beat\t$1\t$2\n";
      }
    ' <"$f"
  done | sort -u
}

# tag_classes prints "kind<TAB>value<TAB>classes" for every JSX element in the
# sources on stdin (one path per line) that carries a literal data-testid,
# data-slot or data-preview. An attribute passed as a prop (`dataSlot="x"`) or
# built from a template has no tag to read, so it is left out and its crop is
# checked for existence only.
tag_classes() {
  while IFS= read -r f; do
    perl -e '
      my $f = shift;
      open my $fh, "<", $f or exit 0;
      my @l = <$fh>;
      for my $i (0 .. $#l) {
        next unless $l[$i] =~ /data-(testid|slot|preview)="([^"{]+)"/;
        my ($kind, $value) = ($1, $2);
        my $start = $i;
        $start-- while $start > 0 && $l[$start] !~ /<[A-Za-z]/;
        my $end = $start;
        $end++ while $end < $#l && $l[$end] !~ />\s*$/;
        my $tag = join(" ", @l[$start .. $end]);
        # Only the class names, so an aria-label never reads as a class.
        my $cls = "";
        $cls .= " $1" while $tag =~ /className=\{?(?:cn\()?\s*"([^"]*)"/g;
        $cls .= " $1" while $tag =~ /className=\{cn\([^)]*?"([^"]*)"/g;
        print "$kind\t$value\t$cls\n";
      }
    ' "$f"
  done | sort -u
}

# fill_class reports whether a class list sizes its element to its parent.
fill_class() {
  local classes=" $1 " token
  for token in "${FILL_CLASSES[@]}"; do
    case "$classes" in *" $token "*) echo "$token"; return 0 ;; esac
  done
  return 1
}

# check_crop_targets reports every crop selector no app renders, and every one
# that names a layout container.
check_crop_targets() {
  local hay="$1" tags="$2" demo kind value classes token findings=0
  shift 2
  while IFS=$'\t' read -r demo kind value; do
    [ -n "$value" ] || continue
    if ! present "$value" "$hay"; then
      echo "  ${demo}: crop names data-${kind} \"${value}\", which no app source renders"
      findings=$((findings + 1))
      continue
    fi
    classes=$(awk -F'\t' -v k="$kind" -v v="$value" '$1 == k && $2 == v { print $3; exit }' "$tags")
    [ -n "$classes" ] || continue
    if token=$(fill_class "$classes"); then
      echo "  ${demo}: crop names data-${kind} \"${value}\", a \"${token}\" container; the beat would record the whole window"
      findings=$((findings + 1))
    fi
  done < <(crop_targets "$@")
  return "$findings"
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
      <div className="rounded-xl border bg-card p-0" data-slot="review-queue" />
      <span data-preview="keyed-table" />
      <div
        className="relative flex h-full w-full flex-col overflow-y-auto"
        data-testid="workbench"
      />
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

  # ── crop targets: one demo whose crops are elements, one whose crops are not ──
  mkdir -p "$tmp/demos/good-demo" "$tmp/demos/bad-demo"
  cat >"$tmp/demos/good-demo/demo.yaml" <<'EOF'
narration:
  - id: card
    kind: desktop
    beat: card
    crop: { selector: '[data-slot="review-queue"]' }
    zoom: 1
  - id: pair
    kind: desktop
    beat: pair
    crop:
      selector:
        - '[data-testid="rail-toolbox"]'
        - '[data-preview="keyed-table"]'
EOF
  cat >"$tmp/demos/bad-demo/demo.yaml" <<'EOF'
narration:
  - id: pane
    kind: desktop
    beat: pane
    crop: { selector: '[data-testid="workbench"]' }
  - id: gone
    kind: desktop
    beat: gone
    crop: { selector: '[data-slot="retired-panel"]' }
EOF

  local found
  found=$(crop_targets "$tmp/demos/good-demo/demo.yaml" "$tmp/demos/bad-demo/demo.yaml" | grep -c . || true)
  if [ "$found" = "5" ]; then
    echo "✓ self-test: reads every crop selector out of the manifests (5)"
  else
    echo "✖ self-test: expected 5 crop selectors from the fixtures, read ${found}"
    status=1
  fi

  printf '%s\n' "$tmp/app.tsx" | tag_classes >"$tmp/tags.tsv"
  if out=$(check_crop_targets "$tmp/app.tsx" "$tmp/tags.tsv" "$tmp/demos/good-demo/demo.yaml"); then
    echo "✓ self-test: a demo whose crops name real, croppable elements passes"
  else
    echo "✖ self-test: the matcher flagged crops that are fine:"
    printf '%s\n' "$out"
    status=1
  fi

  if out=$(check_crop_targets "$tmp/app.tsx" "$tmp/tags.tsv" "$tmp/demos/bad-demo/demo.yaml"); then
    echo "✖ self-test: the matcher did NOT flag a layout-container crop or a dead one"
    status=1
  else
    if printf '%s\n' "$out" | grep -q 'h-full' &&
      printf '%s\n' "$out" | grep -q 'retired-panel'; then
      echo "✓ self-test: flags a crop over an h-full pane and one no app renders (2 findings)"
    else
      echo "✖ self-test: expected both crop findings, got:"
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
tags="$(mktemp)"
# shellcheck disable=SC2064  # expand the paths now, not at trap time
trap "rm -f '$hay' '$tags'" EXIT
haystack_file "$hay"
source_files | tag_classes >"$tags"

status=0

total=$(structural_selectors <"$RECORDER" | grep -c . || true)
if out=$(check_recorder "$RECORDER" "$hay"); then
  echo "✓ walk selectors: all ${total} structural selectors in $RECORDER are rendered by an app"
  advisory=$(advisory_selectors <"$RECORDER" | grep -c . || true)
  echo "  (${advisory} free-text matcher(s) not checked; see the header)"
else
  echo "✖ walk selectors: a recorded walk drives a selector no app renders."
  echo "  Nothing fails when this drifts: the walk throws mid-recording, or a"
  echo "  guarded beat records the wrong screen under the right narration."
  printf '%s\n' "$out"
  status=1
fi

manifests=("$DEMOS"/*/demo.yaml)
crops=$(crop_targets "${manifests[@]}" | grep -c . || true)
if out=$(check_crop_targets "$hay" "$tags" "${manifests[@]}"); then
  echo "✓ crop targets: all ${crops} crop selectors in $DEMOS name an element an app renders and can be cropped into"
else
  echo "✖ crop targets: a beat crops to something the composition cannot push into."
  echo "  A crop region has to fit the crop area at ${FULL_WINDOW_FIT_DOC}x or the beat"
  echo "  records the whole window, so it names the row, card, chip or dialog"
  echo "  the caption is about rather than the pane holding it."
  printf '%s\n' "$out"
  status=1
fi

exit "$status"
