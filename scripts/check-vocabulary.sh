#!/usr/bin/env bash
#
# Guard: retired framing must not reappear in user-facing prose.
#
# The positioning canon (web/docs/contribute/implementation/positioning.md, R12)
# retires a small set of phrases. Prose drifts back to them the way an absolute
# home path creeps into a Makefile: nobody decides to, and nothing fails. A
# retired phrase re-enters through a copied paragraph, a stale doc, or a fresh
# session that read the code first and re-derived the old frame from package
# names. This makes that a build failure rather than something a reader notices
# months later.
#
# What fails, and why it is retired:
#
#   "brand memory"        R12(b). Bowrain is the context graph; brand is one
#                         coordinate on it, not the frame. ("content memory"
#                         survives untouched — a different object, still the
#                         customer-facing term for approved wording.)
#   "brand-first"         R12(a). Superseded by context-first.
#   "Kapi drafts"         R10. The "Kapi drafts, Bowrain governs" slogan
#                         diminishes kapi, which does the heavy lifting. Only
#                         the "Kapi drafts" half is matched: "Bowrain governs
#                         and stewards …" is ordinary, correct prose.
#   "for your whole team" R10. Team is one angle, not the venue.
#   "localization platform"
#                         R2/R12. The product describing ITSELF as one. Every
#                         transactional email carried this as its tagline
#                         through four sweeps. Note the deliberate narrowness:
#                         R2's vocabulary policy keeps l10n as SEO vocabulary on
#                         bowrain.cloud, so "localization" is NOT banned — only
#                         the product calling itself a localization platform,
#                         which R2 retires as a headline and R12 as a frame.
#
# What this guard CANNOT catch: a hero that leads with translation. That is a
# judgement about emphasis, not a phrase, and it stays a review question.
#
# Usage:
#     ./scripts/check-vocabulary.sh              # scan swept surfaces
#     ./scripts/check-vocabulary.sh --self-test  # prove the matcher both ways
#
# Wired into `make check-vocabulary` (part of `make lint`), `make pre-push`, and
# the repo-guards job in .github/workflows/ci.yml.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"

# ── the matcher ──────────────────────────────────────────────────────────────
#
# Case-insensitive, and tolerant of the line wrapping and markdown emphasis that
# prose actually contains: "brand **memory**" and a "brand\nmemory" split across
# two lines both count. Matching is per-line, so the wrap case is handled by
# normalising whitespace before the scan rather than by the pattern.
readonly RETIRED_RE='brand[[:space:]*_]+memor(y|ies)|brand-first|Kapi[[:space:]*_]+drafts|for your whole team|locali[sz]ation[[:space:]*_]+platform'

# ── scope ────────────────────────────────────────────────────────────────────
#
# The surfaces swept to R12. Adding a surface here is how a sweep gets locked in.
readonly SWEPT_SURFACES=(
  README.md
  bowrain/README.md
  web/docs
  web/src
  bowrain/web/docs/docs
  bowrain/web/docs/src
  # The agent skill is read by an assistant on every run, which makes it the
  # highest-leverage prose in the repo and the easiest to forget (P-4).
  cli/skills
  # Transactional email is the highest-REACH prose in the product: its header
  # goes out with every message the platform sends. It carried "Localization
  # platform" through four sweeps because the emails are a separate build with
  # their own catalogs, so nothing else looked here (P-7).
  bowrain/emails/src
  bowrain/mailer/subjects
  # The landing is the surface a stranger meets first, and the last place the
  # retired framing survived (P-1).
  bowrain/web/landing/src
  # The in-product nav and shell (P-3). The hub is Context now, and its change
  # surface is Changes rather than Experiments.
  bowrain/packages/app/src
  bowrain/packages/ui/src
)

# Surfaces deliberately NOT scanned yet, each with the worklist item that will
# sweep it and add it above. Printed on every run: an unscanned surface must be
# visible, not silently absent, or a green check reads as "all prose is clean"
# when it means "the prose we swept is clean".
readonly PENDING_SURFACES=(
)

# Files inside a swept surface that legitimately spell a retired phrase.
readonly ALLOWED_FILES=(
  # The canon itself: it must name the retired phrases in order to retire them.
  web/docs/contribute/implementation/positioning.md
)

# scan_paths prints "file:line:match" for every retired phrase under the given
# paths, and "file: (phrase wrapped across a line break)" for a file that is
# dirty only once its newlines are flattened. Returns 1 if it printed anything.
#
# Two passes rather than one: a per-line grep gives exact line numbers for the
# ordinary case, and a flattened whole-file match catches a phrase that prose
# wrapping split in two. Reporting the wrapped case at file level — rather than
# guessing a line — keeps each occurrence to exactly one hit.
scan_paths() {
  local hits="" exclude_re f line_hits
  exclude_re="$(printf '%s\n' "${ALLOWED_FILES[@]}" | paste -sd'|' -)"

  while IFS= read -r -d '' f; do
    if [ -n "$exclude_re" ] && printf '%s\n' "$f" | grep -qxE "$exclude_re"; then
      continue
    fi
    # Flattened match decides whether the file is dirty at all.
    if ! tr '\n' ' ' <"$f" 2>/dev/null | grep -qiE "$RETIRED_RE"; then
      continue
    fi
    if line_hits=$(grep -HnoiE "$RETIRED_RE" "$f" 2>/dev/null); then
      hits="${hits}${line_hits}"$'\n'
    else
      hits="${hits}${f}: (phrase wrapped across a line break)"$'\n'
    fi
  done < <(
    for p in "$@"; do
      [ -e "$p" ] || continue
      find "$p" -type f ! -path '*/node_modules/*' ! -path '*/build/*' -print0
    done
  )

  hits="$(printf '%s' "$hits" | grep -c . >/dev/null 2>&1 && printf '%s' "$hits" || printf '')"
  [ -z "$hits" ] && return 0
  printf '%s' "$hits"
  return 1
}

# ── self-test ────────────────────────────────────────────────────────────────

self_test() {
  local tmp status=0
  tmp="$(mktemp -d)"
  # shellcheck disable=SC2064  # expand $tmp now, not at trap time
  trap "rm -rf '$tmp'" EXIT

  mkdir -p "$tmp/surface"
  cat >"$tmp/surface/retired.md" <<'EOF'
Bowrain is the shared brand memory for your team and your agents.
The brand-first story led with voice.
Bowrain — Localization platform
Kapi drafts, Bowrain governs.
It converges it for your whole team, all the time.
EOF

  # The wrapped case: the phrase straddles a line break, which is how it
  # actually appears in prose that has been through a formatter.
  cat >"$tmp/surface/wrapped.md" <<'EOF'
Bowrain holds the shared brand
memory that every project draws on.
EOF

  cat >"$tmp/surface/clean.md" <<'EOF'
Bowrain is the context graph your people and agents plug into.
Content memory keeps its name: the store of approved wording.
Bowrain governs and stewards multilingual content.
An SEO page may say localization; the product may not call itself one.
Brand is one coordinate beside audience, surface, register and market.
The brand voice profile is one axis of a profile.
EOF

  local out n
  if out=$(scan_paths "$tmp/surface/retired.md"); then
    echo "✖ self-test: the matcher did NOT flag planted retired framing"
    status=1
  else
    n=$(printf '%s\n' "$out" | grep -c . || true)
    if [ "$n" -ne 5 ]; then
      echo "✖ self-test: expected 5 hits in the planted file, got ${n}:"
      printf '%s\n' "$out"
      status=1
    else
      echo "✓ self-test: flags all five retired phrases (5 hits)"
    fi
  fi

  if out=$(scan_paths "$tmp/surface/wrapped.md"); then
    echo "✖ self-test: the matcher did NOT flag a phrase wrapped across lines"
    status=1
  else
    echo "✓ self-test: flags a retired phrase split across a line break"
  fi

  if out=$(scan_paths "$tmp/surface/clean.md"); then
    echo "✓ self-test: settled vocabulary passes (content memory, brand as a coordinate)"
  else
    echo "✖ self-test: the matcher flagged settled vocabulary:"
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

# Run the self-test first: a matcher that silently stops matching is worse than
# no check at all, and it costs milliseconds.
if ! self_test >/dev/null; then
  echo "✖ check-vocabulary.sh: self-test failed — the matcher is broken."
  self_test || true
  exit 1
fi

if hits=$(scan_paths "${SWEPT_SURFACES[@]}"); then
  echo "✓ no retired framing in the swept surfaces"
  # An empty pending list is the completion signal for the whole sweep, so say
  # so rather than printing a bare heading over nothing. (Also: bash 3.2, which
  # macOS ships, treats "${arr[@]}" on an empty array as unbound under set -u.)
  if [ "${#PENDING_SURFACES[@]}" -gt 0 ]; then
    echo ""
    echo "Not yet scanned — each is swept by the worklist item named:"
    printf '  %s\n' "${PENDING_SURFACES[@]}"
  else
    echo "  every file-backed surface is in scope — the sweep is complete."
    echo "  CLI help text is not file-backed: it is assembled from Go string"
    echo "  literals, and is held to the same vocabulary by"
    echo "  TestConvention_HelpTextUsesCurrentVocabulary in cli/."
  fi
  exit 0
fi

echo "✖ retired framing found in user-facing prose:"
printf '%s\n' "$hits"
echo ""
echo "These phrases were retired by R12 (context-first positioning):"
echo ""
echo "  brand memory          → the context graph. Brand is ONE coordinate,"
echo "                          beside audience, surface, register, market and"
echo "                          validity. ('content memory' is a different"
echo "                          object and keeps its name.)"
echo "  brand-first           → context-first."
echo "  Kapi drafts, …        → kapi holds the context graph for one project;"
echo "                          Bowrain holds the same graph across projects."
echo "                          Reach, not capability."
echo "  for your whole team   → team is one angle, not the venue."
  echo "  localization platform → the context graph. (l10n stays as SEO"
  echo "                          vocabulary; the product does not call"
  echo "                          itself a localization platform.)"
echo ""
echo "See web/docs/contribute/implementation/positioning.md for the canon."
exit 1
