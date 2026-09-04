#!/usr/bin/env bash
#
# Guard: retired framing and retired vocabulary must not reappear in user-facing
# prose, in shipped product strings, or in the metadata a build stamps into an
# installer and a signed binary.
#
# Four rule sets, each with its own scope:
#
#   FRAMING     — the positioning phrases the R12 canon retires, listed with
#                 their replacements in this script's failure output. Scope:
#                 SWEPT_SURFACES.
#   VOCABULARY  — the fixed vocabulary of docs/internals/brand-communication.md
#                 (content memory, terms, voice profile). Scope: VOCAB_SURFACES,
#                 a narrower set, because a Go or TSX file names identifiers as
#                 well as prose and the two cannot be told apart by a grep.
#   CASED       — retired vocabulary that is a word in prose and an identifier
#                 in code, told apart by case. Scope: SWEPT_SURFACES, matched
#                 case-sensitively. "QA" is the whole rule today: the product's
#                 family is checks, while `qa` stays the registered tool name,
#                 the flow id, the overlay type and the gate id.
#   IDENT       — the same word inside a compound identifier (`run-qa-btn`,
#                 `qa_issues`, `qaReport`, `FileQAResult`), which the CASED rule
#                 cannot see because a camel-cased or hyphenated name carries no
#                 word boundary around the initialism. Scope: IDENT_SURFACES,
#                 the source trees. Bare `qa` never matches, and the boundary
#                 compounds are deleted before the match: see #2316.
#
# Prose drifts back to a retired phrase the way an absolute home path creeps
# into a Makefile: nobody decides to, and nothing fails. It re-enters through a
# copied paragraph, a stale doc, or a fresh session that read the code first and
# re-derived the old frame from package names. This makes that a build failure
# rather than something a reader notices months later.
#
# What fails under FRAMING, and why it is retired:
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
# What fails under VOCABULARY: termbase, glossary, localize in any inflection,
# l10n, translation memory, and "brand voice" as the name of the mechanism. The
# CLI's own help text is held to the same list by
# TestConvention_HelpTextUsesCurrentVocabulary in cli/, which walks the built
# command tree rather than grepping Go source.
#
# What fails under CASED: the standalone word QA, and "quality assurance". A
# check reads content and reports findings; the families are the rule-based
# checks, terminology checks and voice checks. Case is what separates the word
# from the name: `qa` names the tool a recipe invokes, `translate-qa` a flow,
# "qa" the overlay type and the gate id, and every one of those is a wire or
# persisted value a rename may not touch. The same split is asserted on help
# text by TestConvention_HelpTextUsesCurrentVocabulary.
#
# What fails under IDENT: `qa` welded to another word. The sweep that renamed
# the checks family left the wire alone, so what a rename may not touch is a
# short, closed list — the tool name, the routes, the overlay kind, the block
# properties, the flow ids, the proto field — and everything else that spells
# `qa` inside a name is a leftover. Listing the boundary rather than the
# offenders is what makes this re-runnable: a new compound fails until someone
# either renames it or writes down which wire it belongs to.
#
# What this guard is NOT for: prose kapi can read AND CI gates. Both halves of
# the rule now live in .kapi/voice.yaml — the vocabulary as forbidden_terms, the
# retired framing as prohibited_patterns — and three make targets run them
# through kapi on every PR: check-docs-prose (the docs and the READMEs),
# check-governed-prose (packaging, the Windows build metadata, the Homebrew
# cask) and check-reference-prose (the authored dossiers).
#
# What is left here is the complement, and it is meant to keep shrinking:
#
#   * .ts and .tsx source, which has no reader. Note this is NOT a candidate for
#     a grammar: prose in a component is an externalization defect, and
#     @neokapi/i18n-react-lint is the check that belongs there. The catalogs are
#     the governed artefact and kapi already reads them.
#   * the two docusaurus.config.ts files and the landing HTML shell, which hold
#     a tagline and a social card in formats with no reader.
#   * build metadata and Storybook fixtures under apps/, plus the CLI module.
#     CLI help text specifically is held by a Go test that walks the built cobra
#     tree (cli/conventions_test.go), which is exact where a file scan is not.
#
# See strategy/source-vocabulary-checking.md for the boundary and the plan to
# delete this script.
#
# What this guard CANNOT catch: a hero that leads with translation. That is a
# judgement about emphasis, not a phrase, and it stays a review question.
#
# Usage:
#     ./scripts/check-vocabulary.sh              # scan swept surfaces
#     ./scripts/check-vocabulary.sh --self-test  # prove the matchers both ways
#
# Wired into `make check-vocabulary` (part of `make lint`), `make pre-push`, and
# the repo-guards job in .github/workflows/ci.yml.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"

# ── the matchers ─────────────────────────────────────────────────────────────
#
# Each pattern carries its own case flag, because CASED needs to tell a word
# from an identifier and the other two do not. The leading (?i) is what makes
# FRAMING and VOCABULARY case-insensitive; drop it and a rule matches exactly.
#
# All three are tolerant of the line wrapping and markdown emphasis that prose
# actually contains: "brand **memory**" and a "brand\nmemory" split across two
# lines both count. Matching is per-line, so the wrap case is handled by
# normalising whitespace before the scan rather than by the pattern.
readonly RETIRED_RE='(?i)brand[[:space:]*_]+memor(y|ies)|brand-first|Kapi[[:space:]*_]+drafts|for your whole team|locali[sz]ation[[:space:]*_]+platform'

# The inflections are the point: a noun-only search misses "localized", which is
# how most of these survive a sweep. TMX and TBX are deliberately absent — they
# are external file-format standards we do not own, and naming them is correct.
# This is the same pattern the CLI help-text convention test carries; keep the
# two in step.
readonly RETIRED_VOCAB_RE='(?i)termbases?|glossar(y|ies)|locali[sz](e|es|ed|ing|ation|ations)|l10n|translation memor(y|ies)|brand[[:space:]*_-]+voice'

# Rename-boundary identifiers, deleted from a line before the VOCABULARY match
# runs. Each names something a rename may not touch, so the retired word inside
# it is a boundary rather than prose drifting back:
#
#   "termbases"  the desktop's navigation view id. It is persisted in the app's
#                view state and pinned to its historical spelling by the id
#                union in apps/kapi-desktop/frontend/src/types/api.ts.
#
# Quoted deliberately: the bare word `termbase` in prose stays a failure.
readonly VOCAB_BOUNDARY_RE='"termbases"'

# The cased rule. \bQA\b matches the word and nothing else: the boundaries fail
# on either side of the initialism inside a camel-cased identifier, so a symbol
# is never reported here as prose. Lowercase `qa` never matches, and that is the
# point: it names the tool, the flow id, the overlay type and the gate id.
readonly RETIRED_CASED_RE='\bQA\b|[Qq]uality [Aa]ssurance'

# Deleted from a line before the CASED match runs:
#
#   QA: "634"    ISO 3166-1 alpha-2 for Qatar, in the country-code table the
#                locale-demand view reads. The code is the standard's, not ours.
readonly CASED_BOUNDARY_RE='QA: "634"'

# The identifier rule. Four shapes, and bare `qa` is none of them:
#
#   foo-qa, foo_QA     a compound ending in the initialism
#   qa-foo, QA_foo     a compound starting with it
#   qaReport           lowerCamel starting with it
#   FileQAResult       camel carrying it, and QAIssue starting with it
#
# The last two require a lowercase letter after the capitalised word, which is
# what keeps a base64 digest in a lockfile from reading as an identifier.
readonly RETIRED_IDENT_RE='[A-Za-z0-9]+[-_][Qq][Aa](?![A-Za-z0-9])|(?<![A-Za-z0-9])[Qq][Aa][-_][A-Za-z0-9]|(?<![A-Za-z0-9])qa[A-Z][a-z]|[a-z](?:QA|Qa)[A-Z][a-z]|(?<![A-Za-z0-9])QA[A-Z][a-z]'

# Deleted from a line before the IDENT match runs. Every entry is a value a
# rename may not touch, grouped by what it is. Longest alternative first, so a
# prefix never swallows the compound that contains it.
#
#   flow ids           the built-in and sample flows a user types after
#                      `kapi run`, and one edge id inside translate-qa
#   routes             POST /:ws/:id/actions/:ref/qa-check[-block]
#   tool + quota       the AI tool name, which is also the billing operation
#                      the usage table maps and the quota row it writes
#   block properties   written onto the block by the AI check tool
#   schema ids         the two members of the `qa` tool group, named after the
#                      registered tool they belong to
#   proto              a sync field and the message it would carry
#   reference page     the generated page for `kapi exec qa`
#   walkthrough        the walkthrough id and its scene ids, which name the
#                      video files on the docs CDN
#   ITS fixture        locQualityRatingProfileRef values captured from upstream
#                      Okapi's XLIFF tests
readonly IDENT_BOUNDARY_RE='translate-qa-parallel|translate-and-qa|translate-qa|parallel-qa|pseudo-qa|qa-only|qa-xliff|qa-then-pseudo|qa-check-block|qa-check|qa_check|qa-checks|qa-provider|qa-rules|qa-ai|qa_results|SyncQAResult|exec-qa|kapi-terminology-qa|terminology-qa|termbase-qa|terms-qa|qaModel'

# ── scope ────────────────────────────────────────────────────────────────────
#
# The surfaces swept to R12. Adding a surface here is how a sweep gets locked in.
readonly SWEPT_SURFACES=(
  # NOT here any more, and deliberately: README.md, bowrain/README.md, web/docs
  # and bowrain/web/docs/docs are gated per-PR by `make check-docs-prose`, which
  # runs the rule through kapi under the project's voice profile. The framing
  # patterns this script used to own for them now live in
  # .kapi/voice.yaml:style.prohibited_patterns. A surface kapi can read and CI
  # gates leaves this list; what stays below is the complement.
  web/src
  bowrain/web/docs/src
  # The Docusaurus tagline is the fallback <meta description> for every doc
  # page that carries no frontmatter of its own, and the HTML shell holds the
  # SPA/landing <title> and social card — the surfaces that read the retired
  # framing to a search engine long after the prose was reframed.
  web/docusaurus.config.ts
  bowrain/web/docs/docusaurus.config.ts
  bowrain/web/landing/index.html
  # The CLI module, which carries the agent skill an assistant reads on every
  # run — the highest-leverage prose in the repo and the easiest to forget
  # (P-4) — alongside the help strings and the fixtures that quote the product
  # back to itself.
  cli
  # The desktop app: its window chrome, its first-run copy, and the build
  # metadata below.
  apps
  # Distribution metadata (packaging, deploy/homebrew) is gated by `make
  # check-governed-prose` through kapi. Its vocabulary half moved when those
  # collections were declared; the framing half moved with the patterns.
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

# The surfaces held to the fixed vocabulary as well as the framing. Narrower on
# purpose: these carry product strings and shipped metadata, and no identifier
# that a rename boundary keeps (bar the one in VOCAB_BOUNDARY_RE).
readonly VOCAB_SURFACES=(
  # Every string the desktop build stamps into an installer, a .app bundle, and
  # a signed Windows binary's file properties.
  apps/kapi-desktop/build
  # The desktop's own components — the window a user meets before any docs.
  apps/kapi-desktop/frontend/src/components
  # NOT here any more, and deliberately: packaging/nfpm.yaml,
  # apps/kapi-desktop/build/windows/info.json and deploy/homebrew/*.rb are
  # declared as collections in kapi.yaml and gated by `make
  # check-governed-prose`, which runs the rule through kapi. A surface that
  # moves under a collection LEAVES this list — one rule, one enforcer per
  # surface. What stays below is what kapi still cannot open.
  # The landing page's own components, and the head sidecar that carries its
  # per-locale <title> and social description. Both hold prose and nothing else;
  # the six-section rebuild left no retired spelling behind in either.
  bowrain/web/landing/src
  bowrain/web/landing/locale-meta.json
  # The desktop's Storybook fixtures and the recording-only demo app. Fixtures
  # are read as "this is what kapi is for", and the demo app is literally what
  # the walkthrough videos show, so both are product prose whatever the file
  # extension says.
  apps/kapi-desktop/frontend/src/stories
  apps/kapi-desktop/frontend/src/demo
)

# The surfaces held to the identifier rule: the source trees, both languages,
# plus the two docs trees and the repo's own internals. Wider than the prose
# lists because an identifier is not prose — a name is a name in a Go file, a
# story fixture, a CSS custom property and a recipe example alike, and #2316
# renamed all four.
readonly IDENT_SURFACES=(
  core
  host
  cli
  kapi
  providers
  plugins
  kpz
  packages
  apps
  bowrain
  web/src
  web/docs
  web/sidebars.ts
  web/docusaurus.config.ts
  docs/internals
  harness/src
  scripts
)

# Surfaces deliberately NOT scanned yet, each with the worklist item that will
# sweep it and add it above. Printed on every run: an unscanned surface must be
# visible, not silently absent, or a green check reads as "all prose is clean"
# when it means "the prose we swept is clean".
readonly PENDING_SURFACES=(
  "cli/skills (vocabulary) — the i18n playbooks name third-party libraries (@angular/localize, expo-localization) and the eval table quotes user prompts verbatim, and the skill description is intent-matching vocabulary: it must contain the words a user types. No sweep is scheduled; deciding what a matching surface owes the vocabulary rule comes first."
)

# Files inside a swept surface that legitimately spell a retired phrase. Each
# needs a reason that is about the file, not about the effort of fixing it.
readonly ALLOWED_FILES=(
  # okapi-bridge's own tool ids and descriptions, captured verbatim. Renaming
  # another project's identifiers in a fixture would make the fixture lie about
  # what the bridge reports.
  apps/kapi-desktop/frontend/src/stories/fixtures/tools-metadata.json
  # TMX's name expands to Translation Memory eXchange. It is an external
  # standard we do not own, and the Storybook entry names the format.
  apps/kapi-desktop/frontend/src/stories/formats/tmx.stories.tsx
  # The recording-only demo app mirrors the real sidebar, whose view ids
  # ("termbases", "memories") are PERSISTED in settings.json and deliberately
  # keep their historical spellings — see frontend/src/types/api.ts. The labels
  # a user reads say Terms and Content Memory; only the ids are old.
  apps/kapi-desktop/frontend/src/demo/DemoApp.tsx
  # Prototype v1: a record of a design the product did not take, the fork
  # between a "content project" and a "localization project". Prototype v2
  # replaced it with one project shape. The vocabulary is the rejected design's
  # own; each file says so at the top and the Storybook group reads
  # "Prototype v1 (superseded)".
  apps/kapi-desktop/frontend/src/stories/prototype/AdaptiveSidebar.stories.tsx
  apps/kapi-desktop/frontend/src/stories/prototype/ConfiguredFlows.stories.tsx
  apps/kapi-desktop/frontend/src/stories/prototype/Launcher.stories.tsx
  apps/kapi-desktop/frontend/src/stories/prototype/NewProject.stories.tsx
  apps/kapi-desktop/frontend/src/stories/prototype/QuickTools.stories.tsx
  apps/kapi-desktop/frontend/src/stories/prototype/StepUpToLocalization.stories.tsx
  apps/kapi-desktop/frontend/src/stories/prototype/_shared.tsx
  # v2's own stories describe what v1 got wrong, which means naming it.
  apps/kapi-desktop/frontend/src/stories/prototype/v2/Sidebar.stories.tsx
  apps/kapi-desktop/frontend/src/stories/prototype/v2/Flows.stories.tsx
  apps/kapi-desktop/frontend/src/stories/prototype/v2/ProjectLanguages.stories.tsx
)

# Files the CASED rule alone excuses, on top of ALLOWED_FILES. Kept separate so
# a file excused one word is still held to the other two rule sets.
readonly CASED_ALLOWED_FILES=(
  # A captured dataset: the transcripts an assistant produced against this
  # repo, stored verbatim as the evidence a skill eval reports on. Editing the
  # words in it would make the record say something that was never said.
  web/src/pages/skill-eval/_skilleval.json
  # A target-locale catalog. Translations are written by the loop and land here
  # compiled; a source string that changes orphans its entry until the next
  # convergence, which is the normal drift and never a build failure.
  apps/kapi-desktop/frontend/public/translations/nb.json
)

# Files the IDENT rule alone excuses. Entries are anchored regexes rather than
# literal paths, because the artefacts they name repeat per module and per
# locale and a literal list would go stale on the next module.
readonly IDENT_ALLOWED_FILES=(
  # Module checksum files and pnpm lockfiles. A hit inside a base64 digest is a
  # byte coincidence: nothing in either file is a name this repo chose.
  '.*/go\.sum'
  '.*/pnpm-lock\.yaml'
  # Catalogs and content-memory bundles, written by the convergence loop. A
  # source string that changes orphans its entry until the next run, so a
  # retired spelling here is drift the loop clears rather than a name to fix.
  '.*/i18n/catalogs/.*\.json'
  '.*/translations/[a-z-]+\.json'
  '\.kapi/memory/.*\.json'
  # Generated from the built command tree and the tool registry; the tool name
  # `qa` is what they are reporting.
  'packages/reference-data/data/.*\.json'
  'host/i18n/commands\.json'
  'web/src/components/KapiPlayground/embeds/.*'
  # A vendored MDX parity corpus: copies of published pages, captured as they
  # were. Editing one makes the fixture lie about what it parsed.
  'core/formats/mdx/testdata/.*'
  'core/formats/mdx/corpus\.yaml'
  # Base64 office documents, carried inline as playground samples.
  'packages/kapi-playground/src/samples\.ts'
  # Fixtures captured verbatim from another project: okapi-bridge's tool
  # metadata, and upstream Okapi's XLIFF tests.
  'apps/kapi-desktop/frontend/src/stories/fixtures/tools-metadata\.json'
  'cli/parity/formats/fixtures_xliff_generated\.go'
  # Prototype v1, a record of a design the product did not take. Its step names
  # are that design's, and the Storybook group reads "Prototype v1 (superseded)".
  'apps/kapi-desktop/frontend/src/stories/prototype/.*'
  # The two files that plant retired spellings so a matcher that stops matching
  # is caught: this script's own self-test, and the CLI convention test.
  'scripts/check-vocabulary\.sh'
  'cli/conventions_test\.go'
)

# list_files prints NUL-separated paths for one surface.
#
# A relative path is a surface inside this repo, enumerated with git so the
# scan sees exactly the files the repo owns: node_modules, compiled site output
# and generated catalogs are ignored and therefore skipped, while a tracked
# build/ directory of hand-written metadata is not. An absolute path is the
# self-test's temp directory, outside any work tree, so it falls back to a walk.
list_files() {
  local p
  for p in "$@"; do
    [ -e "$p" ] || continue
    case "$p" in
      /*) find "$p" -type f -print0 ;;
      *) git ls-files -z --cached --others --exclude-standard -- "$p" ;;
    esac
  done
}

# allow_re joins an allowlist of paths into the anchored alternation scan_paths
# takes, and prints nothing for an empty list.
#
# The ${a[@]+"${a[@]}"} form: under `set -u` an empty array expansion is an
# unbound-variable error on bash 3.2, which is what /bin/bash still is on macOS.
allow_re() {
  printf '%s\n' ${@+"$@"} | paste -sd'|' -
}

# scan_paths prints "file:line:match" for every hit of regex $1 under the paths
# from $4 on, and "file: (phrase wrapped across a line break)" for a file that
# is dirty only once its newlines are flattened. $2 is the boundary pattern
# deleted before matching (empty: nothing is deleted). $3 is an alternation of
# paths this rule excuses, passed per rule so a file excused one word is still
# scanned for the others. Returns 1 if it printed anything.
#
# Two matches per file rather than one: a per-line match gives exact line
# numbers for the ordinary case, and a flattened whole-file match catches a
# phrase that prose wrapping split in two. Reporting the wrapped case at file
# level — rather than guessing a line — keeps each occurrence to exactly one hit.
#
# One perl process for the whole scan, not a grep pipeline per file: the swept
# set is several thousand files, and three subprocesses each put this guard at
# twenty seconds inside `make lint`.
#
# $PREFILTER, when set by the caller, is a cheap pattern every hit must contain.
# A file that fails it is skipped before the boundary substitution and the
# per-line loop, which is what keeps a rule scoped to the whole source tree
# affordable: nearly every file matches none of these words at all, and the
# alternations are an order of magnitude dearer than one substring scan.
scan_paths() {
  local re="$1" boundary="$2" exclude_re="$3"
  shift 3
  local hits

  hits="$(list_files "$@" | RE="$re" BOUNDARY="$boundary" ALLOWED="$exclude_re" PREFILTER="${PREFILTER:-}" perl -0 -ne '
    BEGIN {
      $re      = qr/$ENV{RE}/;
      $bound   = $ENV{BOUNDARY} ne "" ? qr/$ENV{BOUNDARY}/ : undef;
      $allowed = $ENV{ALLOWED}  ne "" ? qr/^(?:$ENV{ALLOWED})$/ : undef;
      $pre     = $ENV{PREFILTER} ne "" ? qr/$ENV{PREFILTER}/ : undef;
    }
    chomp;
    my $f = $_;
    next unless -f $f;
    # Icons, fonts and screenshots are in the tracked set too; a match inside
    # one would be a byte coincidence, not prose.
    next if -B $f;
    next if $allowed && $f =~ $allowed;
    my $text = do { local $/; open(my $fh, "<", $f) or next; <$fh> };
    next unless defined $text;
    next if $pre && $text !~ $pre;
    $text =~ s/$bound//g if $bound;
    my @lines = split /\n/, $text, -1;
    my $found = 0;
    for my $i (0 .. $#lines) {
      my $line = $lines[$i];
      while ($line =~ /$re/g) { print "$f:", $i + 1, ":$&\n"; $found = 1 }
    }
    next if $found;
    (my $flat = $text) =~ s/\n/ /g;
    print "$f: (phrase wrapped across a line break)\n" if $flat =~ /$re/;
  ')"

  [ -z "$hits" ] && return 0
  printf '%s\n' "$hits"
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

  # The vocabulary rule set, which the framing rule set deliberately lets pass.
  cat >"$tmp/surface/vocab.md" <<'EOF'
Kapi — Localization Toolkit
Import your termbase and your glossary.
Useful for localization QA and for localized builds.
The translation memory holds approved wording.
Score a draft against its brand voice.
EOF

  cat >"$tmp/surface/vocab-clean.md" <<'EOF'
Kapi — the context-aware content workbench
Import your terms store; the content memory holds approved wording.
Useful for multilingual checks and for translated builds.
Score a draft against its voice profile.
The desktop's view id "termbases" is persisted and keeps its spelling.
EOF

  # The cased rule set, which both other rule sets deliberately let pass.
  cat >"$tmp/surface/cased.md" <<'EOF'
Run QA over the target before you ship.
Quality assurance is what the gate does.
The QA report lists every finding.
EOF

  cat >"$tmp/surface/cased-clean.md" <<'EOF'
Run the checks over the target before you ship.
The rule-based checks report every finding.
`kapi exec qa` runs the tool, and `translate-qa` is the flow it sits in.
The overlay type is "qa" and so is the gate id.
QAFoo and BarQABaz are identifiers, and a rename takes them together.
QA: "634" is Qatar in the ISO 3166-1 table.
EOF

  # The identifier rule set, which all three others deliberately let pass.
  cat >"$tmp/surface/ident.txt" <<'EOF'
data-testid="run-qa-btn"
const qaReport = collect()
type FileQAResult struct{}
QAIssue carries the rule id.
kind: qa_issues
EOF

  cat >"$tmp/surface/ident-clean.txt" <<'EOF'
kapi run translate-qa -i app.xliff --target-lang fr
POST /:ws/:id/actions/:ref/qa-check-block
Operation: "qa_check"
v.SetProperty("qa-provider", name)
schema.ToolMeta{ID: "qa-rules"}
// Future: repeated SyncQAResult qa_results = 14;
The overlay type is "qa" and so is the gate id.
data-testid="run-check-btn"
EOF

  local out n
  if out=$(scan_paths "$RETIRED_RE" "" "" "$tmp/surface/retired.md"); then
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

  if out=$(scan_paths "$RETIRED_RE" "" "" "$tmp/surface/wrapped.md"); then
    echo "✖ self-test: the matcher did NOT flag a phrase wrapped across lines"
    status=1
  else
    echo "✓ self-test: flags a retired phrase split across a line break"
  fi

  if out=$(scan_paths "$RETIRED_RE" "" "" "$tmp/surface/clean.md"); then
    echo "✓ self-test: settled vocabulary passes (content memory, brand as a coordinate)"
  else
    echo "✖ self-test: the matcher flagged settled vocabulary:"
    printf '%s\n' "$out"
    status=1
  fi

  if out=$(scan_paths "$RETIRED_VOCAB_RE" "$VOCAB_BOUNDARY_RE" "" "$tmp/surface/vocab.md"); then
    echo "✖ self-test: the vocabulary matcher did NOT flag planted retired vocabulary"
    status=1
  else
    n=$(printf '%s\n' "$out" | grep -c . || true)
    if [ "$n" -lt 6 ]; then
      echo "✖ self-test: expected every planted retired word to be flagged, got ${n}:"
      printf '%s\n' "$out"
      status=1
    else
      echo "✓ self-test: flags termbase, glossary, localization, localized, translation memory, brand voice"
    fi
  fi

  if out=$(scan_paths "$RETIRED_VOCAB_RE" "$VOCAB_BOUNDARY_RE" "" "$tmp/surface/vocab-clean.md"); then
    echo "✓ self-test: settled vocabulary and the persisted \"termbases\" view id pass"
  else
    echo "✖ self-test: the vocabulary matcher flagged settled vocabulary:"
    printf '%s\n' "$out"
    status=1
  fi

  if out=$(scan_paths "$RETIRED_CASED_RE" "$CASED_BOUNDARY_RE" "" "$tmp/surface/cased.md"); then
    echo "✖ self-test: the cased matcher did NOT flag planted QA"
    status=1
  else
    n=$(printf '%s\n' "$out" | grep -c . || true)
    if [ "$n" -ne 3 ]; then
      echo "✖ self-test: expected 3 hits in the planted file, got ${n}:"
      printf '%s\n' "$out"
      status=1
    else
      echo "✓ self-test: flags QA and quality assurance (3 hits)"
    fi
  fi

  # The case split is the whole rule, so assert it rather than assuming it: a
  # matcher that also caught the tool name would fail every recipe in the docs.
  if out=$(scan_paths "$RETIRED_CASED_RE" "$CASED_BOUNDARY_RE" "" "$tmp/surface/cased-clean.md"); then
    echo "✓ self-test: the qa tool name, camel-cased identifiers and the Qatar code pass"
  else
    echo "✖ self-test: the cased matcher flagged a boundary:"
    printf '%s\n' "$out"
    status=1
  fi

  if out=$(scan_paths "$RETIRED_IDENT_RE" "$IDENT_BOUNDARY_RE" "" "$tmp/surface/ident.txt"); then
    echo "✖ self-test: the identifier matcher did NOT flag planted compounds"
    status=1
  else
    n=$(printf '%s\n' "$out" | grep -c . || true)
    if [ "$n" -ne 5 ]; then
      echo "✖ self-test: expected 5 hits in the planted file, got ${n}:"
      printf '%s\n' "$out"
      status=1
    else
      echo "✓ self-test: flags qa in hyphenated, snake and camel compounds (5 hits)"
    fi
  fi

  # The boundary list is the whole rule, so assert it: a matcher that also
  # caught the flow id would fail every recipe and every route in the tree.
  if out=$(scan_paths "$RETIRED_IDENT_RE" "$IDENT_BOUNDARY_RE" "" "$tmp/surface/ident-clean.txt"); then
    echo "✓ self-test: the flow ids, routes, properties and proto names pass"
  else
    echo "✖ self-test: the identifier matcher flagged a boundary:"
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
  echo "✖ check-vocabulary.sh: self-test failed — a matcher is broken."
  self_test || true
  exit 1
fi

status=0

if hits=$(scan_paths "$RETIRED_RE" "" "$(allow_re ${ALLOWED_FILES[@]+"${ALLOWED_FILES[@]}"})" "${SWEPT_SURFACES[@]}"); then
  echo "✓ no retired framing in the swept surfaces"
else
  status=1
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
fi

if hits=$(scan_paths "$RETIRED_VOCAB_RE" "$VOCAB_BOUNDARY_RE" "$(allow_re ${ALLOWED_FILES[@]+"${ALLOWED_FILES[@]}"})" "${VOCAB_SURFACES[@]}"); then
  echo "✓ no retired vocabulary in the product strings and build metadata"
else
  status=1
  echo "✖ retired vocabulary found in product strings or build metadata:"
  printf '%s\n' "$hits"
  echo ""
  echo "The fixed vocabulary is docs/internals/brand-communication.md:"
  echo ""
  echo "  termbase, glossary    → the terms store."
  echo "  translation memory    → content memory (recycling is the verb)."
  echo "  localization, l10n    → multilingual content, or language. Recast the"
  echo "  localize, localized     sentence rather than substituting one word."
  echo "  brand voice           → the voice profile. Brand is the label most"
  echo "                          workspaces give their default profile, not the"
  echo "                          name of the mechanism."
  echo ""
  echo "An identifier a rename may not touch — a persisted id, a wire field, an"
  echo "analytics id — belongs in VOCAB_BOUNDARY_RE with the reason it is one."
  echo ""
fi

cased_allowed="$(allow_re \
  ${ALLOWED_FILES[@]+"${ALLOWED_FILES[@]}"} \
  ${CASED_ALLOWED_FILES[@]+"${CASED_ALLOWED_FILES[@]}"})"
if hits=$(scan_paths "$RETIRED_CASED_RE" "$CASED_BOUNDARY_RE" "$cased_allowed" "${SWEPT_SURFACES[@]}"); then
  echo "✓ no QA in the swept prose"
else
  status=1
  echo "✖ QA found in user-facing prose:"
  printf '%s\n' "$hits"
  echo ""
  echo "The product family is checks. A check reads content and reports"
  echo "findings without modifying it, and kapi check is the verb:"
  echo ""
  echo "  QA (checks in general)  → the checks. Run QA → run the checks."
  echo "  QA results, QA issues   → findings."
  echo "  a QA check              → a check."
  echo "  QA (the rule-based one, → rule-based checks, as against the"
  echo "  against voice or terms)   terminology checks and the voice checks."
  echo "  quality assurance       → checks, or verification. Recast the"
  echo "                            sentence rather than swapping one word."
  echo ""
  echo "Lowercase qa is a different thing and never matches here: it names the"
  echo "registered tool, the flow id, the overlay type and the gate id, and"
  echo "every one of those is a value a rename may not touch. A Go or TS"
  echo "identifier does not match either: a camel-cased name carries no word"
  echo "boundary on either side of the initialism. The IDENT rule below is"
  echo "what holds identifiers to the same vocabulary."
  echo ""
fi

ident_allowed="$(allow_re ${IDENT_ALLOWED_FILES[@]+"${IDENT_ALLOWED_FILES[@]}"})"
if hits=$(PREFILTER='[Qq][Aa]' scan_paths "$RETIRED_IDENT_RE" "$IDENT_BOUNDARY_RE" "$ident_allowed" "${IDENT_SURFACES[@]}"); then
  echo "✓ no qa compounds outside the rename boundary"
else
  status=1
  echo "✖ qa found inside an identifier:"
  printf '%s\n' "$hits"
  echo ""
  echo "The family is checks and the result is a finding, in a name as much as"
  echo "in a sentence:"
  echo ""
  echo "  qa-issues-list, qaIssue → the findings. check-findings-list."
  echo "  run-qa-btn, inspector-qa → run-check-btn, inspector-check."
  echo "  qaReport, QAConfig       → checkReport, CheckConfig."
  echo "  qa-report.html           → check-report.html."
  echo ""
  echo "If the name is a value a rename may not touch — a route path, a wire or"
  echo "persisted field, an analytics id, a registered tool or flow id — add it"
  echo "to IDENT_BOUNDARY_RE with the reason, not to the allowlist. The"
  echo "allowlist is for whole files that are generated, vendored or captured."
  echo ""
fi

if [ "$status" -ne 0 ]; then
  exit 1
fi

# An empty pending list is the completion signal for the whole sweep, so say
# so rather than printing a bare heading over nothing. (Also: bash 3.2, which
# macOS ships, treats "${arr[@]}" on an empty array as unbound under set -u.)
if [ "${#PENDING_SURFACES[@]}" -gt 0 ]; then
  echo ""
  echo "Not yet held to the fixed vocabulary — each is swept by the item named:"
  printf '  %s\n' "${PENDING_SURFACES[@]}"
else
  echo "  every file-backed surface is in scope — the sweep is complete."
fi
echo ""
echo "  CLI help text is not file-backed: it is assembled from Go string"
echo "  literals, and is held to the same vocabulary by"
echo "  TestConvention_HelpTextUsesCurrentVocabulary in cli/."
exit 0
