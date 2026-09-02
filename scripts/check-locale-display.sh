#!/usr/bin/env bash
#
# Guard: a language is shown by name, not by its code alone.
#
# `fr` and `ar` are identifiers. They name a language to the machine, and to a
# reviewer who already knows the tag; every other reader sees an abbreviation
# and guesses. The Review queue's language filter shipped as a bare `<select>`
# whose options read `de`, `fr`, `ja`, `nb`, `ar`, next to a Collections matrix
# whose tooltips said `fr: 62%`, on a page whose own heading said "All
# languages". One surface, three registers, and the reader is left to know CLDR.
#
# What fails: a locale-named value rendered as a JSX text child.
#
#     <option value={l}>{l}</option>          <p>{locale}</p>
#     <span>{it.locale}</span>                <div>{targetLang}</div>
#
# The convention: the display name in the language the reader is reading,
# followed by the tag in muted monospace. "French (France) fr-FR" answers both
# "which language" and "which tag" at once. The tag alone belongs only in a
# context with no room for a name, and there it carries the name in its tooltip.
# The tag is drawn exactly as it was given: case is meaning in BCP 47
# (`zh-Hant`, `sr-Latn-RS`), so no `uppercase` or `text-transform` class goes
# near a locale code.
#
# What to use instead:
#
#   * `<LocaleLabel locale={code} />` wherever a name fits: a dropdown item, a
#     list row, a heading, a form field. `variant="short"` drops the region,
#     `source` marks the project's source language.
#   * `<LocaleLabel locale={code} compact />` for a table cell or a coverage
#     grid: the tag alone, with the name in its title.
#   * `<LocaleSelect ... clearLabel={t("All languages")} />` for a picker or a
#     language filter. It renders pill + name and searches on both.
#   * `localeLabel(code)` -> "French (fr)" for a single-string context: a
#     tooltip, an aria-label, a title. `formatLocale(tag, opts)` returns the
#     resolved `{ name, code, text, title }` for anything that is not JSX.
#   * `<LocalePill locale={code} />` for a dense grid that colour-codes its
#     languages. It too carries the name in its title.
#
# Usage:
#     ./scripts/check-locale-display.sh              # scan tracked files
#     ./scripts/check-locale-display.sh --self-test  # prove the matcher both ways
#
# Wired into `make check-locale-display` (part of `make lint`), `make pre-push`,
# and the repo-guards job in .github/workflows/ci.yml.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"

# ── the matcher ──────────────────────────────────────────────────────────────

# Rule 1: an identifier whose final segment names a language, rendered as a JSX
# text child. Deliberately narrow: a guard that fires on `{code}` or `{name}`
# would be noise, and noise is how a check gets ignored.
export LOCALE_IDENT='(?:[A-Za-z_$][A-Za-z0-9_$]*\.)?(?:locale|lang|language|localeCode|langCode|localeFilter|sourceLocale|targetLocale|sourceLang|targetLang|sourceLanguage|targetLanguage)'

# Rule 2: a hand-rolled language dropdown. Rule 1 alone missed the one that
# shipped, because its loop variable was `l`:
#
#     {locales.map((l) => (
#       <option key={l} value={l}>{l}</option>
#     ))}
#
# No identifier there names a language, so the name is not where the evidence
# is. The collection is. A `.map` over a locale-named list that builds `<option>`
# elements is a language picker written by hand, whatever it calls its variable.
export LOCALE_LIST='(?:locales|langs|languages|localeCodes|langCodes|localeList|localeOptions|targetLanguages|targetLocales|allLocales)'

# scan_files reads NUL-separated paths on stdin and prints "file:line:match" for
# every language shown as a bare code. Returns 1 if it printed anything.
#
# The match spans lines, because both shapes put the expression on a line of its
# own. perl slurps each file and reports the line the match starts on.
scan_files() {
  local hits
  # The patterns travel in the environment (see the `export`s above) rather than
  # interpolated into the script: perl would read the `$` in a character class
  # as one of its own variables.
  hits=$(xargs -0 perl -0777 -ne '
    my $ident = qr/$ENV{LOCALE_IDENT}/;
    my $list  = qr/$ENV{LOCALE_LIST}/;
    sub lineno { my ($text, $at) = @_; return 1 + (substr($text, 0, $at) =~ tr/\n//); }

    while (/>\s*\{\s*($ident)\s*\}\s*</g) {
      printf "%s:%d:{%s} rendered as a bare code\n", $ARGV, lineno($_, pos($_)), $1;
    }
    while (/\b$list\b[^;]{0,80}?\.map\s*\(.{0,400}?<option\b/gs) {
      printf "%s:%d:hand-rolled <option> language dropdown\n", $ARGV, lineno($_, $-[0]);
    }
  ' 2>/dev/null || true)
  [ -z "$hits" ] && return 0
  printf '%s\n' "$hits"
  return 1
}

# Files allowed to render a bare code. Each is here because rendering the code
# IS the job, and none of them leaves the reader without a name.
readonly CODE_FILES=(
  # The pill itself: it draws the code, and carries the name in its title.
  packages/ui/src/components/resource-browser/LocalePill.tsx
)

# ── self-test ────────────────────────────────────────────────────────────────

self_test() {
  local tmp status=0
  tmp="$(mktemp -d)"
  # shellcheck disable=SC2064  # expand $tmp now, not at trap time
  trap "rm -rf '$tmp'" EXIT

  # The exact shape that shipped in the Review queue, plus a bare heading.
  cat >"$tmp/bare.tsx" <<'EOF'
export function Filter({ locales, value, onChange }: Props) {
  return (
    <select value={value} onChange={onChange}>
      <option value="">All languages</option>
      {locales.map((l) => (
        <option key={l} value={l}>
          {l}
        </option>
      ))}
    </select>
  );
}
export const Row = (it: Item) => <span>{it.locale}</span>;
EOF

  cat >"$tmp/named.tsx" <<'EOF'
export function Filter({ locales, value, onChange }: Props) {
  return (
    <LocaleSelect
      value={value}
      onChange={onChange}
      locales={locales}
      clearLabel={t("All languages")}
    />
  );
}
export const Row = (it: Item) => <LocaleLabel locale={it.locale} />;
export const Pill = (it: Item) => <LocalePill locale={it.locale} showName />;
export const Cell = (it: Item) => <span title={localeLabel(it.locale)}>{it.pct}</span>;
EOF

  local out n
  if out=$(printf '%s\0' "$tmp/bare.tsx" | scan_files); then
    echo "✖ self-test: the matcher did NOT flag a planted bare code"
    status=1
  else
    n=$(printf '%s\n' "$out" | grep -c . || true)
    if [ "$n" -ne 2 ]; then
      echo "✖ self-test: expected 2 hits in the planted file, got ${n}:"
      printf '%s\n' "$out"
      status=1
    else
      echo "✓ self-test: flags a hand-rolled language dropdown and a bare row code (2 hits)"
    fi
  fi

  if out=$(printf '%s\0' "$tmp/named.tsx" | scan_files); then
    echo "✓ self-test: a label, a pill, a picker and a titled cell pass"
  else
    echo "✖ self-test: the matcher flagged a named rendering:"
    printf '%s\n' "$out"
    status=1
  fi

  return "$status"
}

# ── main ─────────────────────────────────────────────────────────────────────

cd "$root"

if [ "${1:-}" = "--self-test" ]; then
  self_test
  exit $?
fi

# A matcher that silently stops matching is worse than no check at all.
if ! self_test >/dev/null; then
  echo "✖ check-locale-display.sh: self-test failed, the matcher is broken."
  self_test || true
  exit 1
fi

exclude_re="$(printf '%s\n' "${CODE_FILES[@]}" | paste -sd'|' -)"

if hits=$(git ls-files -z -- '*.tsx' |
  tr '\0' '\n' |
  grep -vxE "$exclude_re" |
  grep -vE '(\.test\.tsx?|\.stories\.tsx?|/__tests__/|/e2e/|/stories/)' |
  tr '\n' '\0' |
  scan_files); then

  echo "✖ language(s) shown as a bare code:"
  printf '%s\n' "$hits"
  echo ""
  echo "A locale code is an identifier. Show the language name in the reader's"
  echo "own language, and keep the tag in muted monospace beside it:"
  echo ""
  echo "    <LocaleLabel locale={code} />                a row, an item, a heading"
  echo "    <LocaleLabel locale={code} compact />        a table cell or a grid"
  echo "    <LocaleSelect ... clearLabel={t(\"All languages\")} />   a picker or filter"
  echo "    localeLabel(code)                            a tooltip, title or aria-label"
  echo "    formatLocale(tag, opts)                      anything that is not JSX"
  echo ""
  echo "A compact label draws the tag alone and carries the name in its title, so"
  echo "the code is never a dead end. Never uppercase a tag: case is meaning in"
  echo "BCP 47. See packages/ui/src/lib/locale-name.ts and"
  echo "packages/ui/docs/judgement-colours.md."
  exit 1
fi

echo "✓ every language is shown by name"
exit 0
