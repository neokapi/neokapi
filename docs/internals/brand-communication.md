# Brand & Communication Guideline

Guidance for writing user-facing text in this repository: the documentation
site (`web/docs`), the marketing landing pages (`web/landing`,
`bowrain/web/landing`), READMEs, release notes, CLI help, and UI copy. Claude
and human contributors should follow it whenever they add or edit prose.

The goal: an **academic, precise register** — explain what something is and what
it does, and let the facts stand without selling. The reader is a developer or
localization engineer, not a prospect.

## Voice

- State capabilities plainly. "kapi reads and writes localization, document, and
  data formats" — not "kapi delivers powerful, seamless format support."
- Lead with the problem and the mechanism, not adjectives. Prefer a
  problem→solution sentence over a brochure bullet list of buzzwords.
- Be specific and verifiable. Every claim should be checkable against the code
  or a generated artifact.
- One idea per sentence; short sentences over long ones.

## Avoid

- **Marketing superlatives and hype:** powerful, seamless, effortless, blazing,
  production-proven, game-changing, cutting-edge, revolutionary, supercharge,
  unleash, magic, "just point and go", "localize at scale", "everything you
  need". If a sentence still means the same thing with the adjective removed,
  remove it.
- **Emoji** in documentation and committed prose.
- **Inconsistent casing.** Use sentence case for headings and UI titles
  ("AI-native translation", not "AI-Native Translation").
- **Brochure framing.** Don't restate the same feature list as a hero, a card
  grid, and a bullet list. Say it once, in the right place.

## Source work and localization — don't frame everything as translation

neokapi does two jobs: **source-language work** (brand guardrails, terminology,
QA) and **localization** (translation across languages and formats). Wording
that frames the shared engine as translation undersells the source-language
half — the reader doing brand work shouldn't feel the docs are about something
else.

- ✗ "the **translatable** text is segmented into blocks", "kapi extracts
  **translatable** text", "only the **translatable** text changes".
- ✓ Use neutral terms for the shared mechanism — *text*, *content*, *block*,
  *extract*: "the text is **extracted** into blocks", "kapi extracts the text",
  "only the text changes".
- Reserve *translatable* / *translation* for genuinely localization-specific
  contexts (a translation-job estimate, a target-language round-trip, the Okapi
  heritage mapping).
- **Extraction — not segmentation — produces blocks.** Reading a file
  *extracts* its text into blocks; segmentation is a separate, opt-in overlay
  *within* a block. Don't write "segmented into blocks".
- Exception: the content model's **Block = translatable content** vs. a
  **non-translatable skeleton** is a precise technical distinction — keep it in
  the Framework / content-model docs, but don't carry that framing onto the
  front page or everyday Kapi pages.

## Never hardcode counts that the code controls

Do not write numbers that change whenever the codebase changes — they rot and
create a maintenance tax on every PR:

- ✗ "42 built-in formats", "15+ formats", "40+ Okapi filters", "5 MT
  providers", "80+ tools", "~30 tools".
- ✓ Name the *categories* ("localization, document, data, subtitle, and office
  formats") and link to the live, generated reference (e.g. the `/formats`
  page, built from `formats.json` via `make generate-format-docs`).
- If a count genuinely helps on an MDX page, derive it from the generated data
  at render time — never type the literal.

The same rule covers tool counts, provider counts, filter counts, and
"X languages supported".

## Don't duplicate

- One authoritative page per topic. When two pages overlap, either merge them
  or split them by a clear audience boundary (concept/usage vs. API), and
  cross-link instead of repeating prose.
- When a page moves or merges, add a redirect for the old URL
  (`@docusaurus/plugin-client-redirects` in the docs site) so links don't break.

## Documentation must match the code

Verify every CLI command, flag, import path, package name, flow name, and
config key against the source before publishing. Prefer generated artifacts as
the source of truth. Specifics that have bitten us:

- **Import paths:** `github.com/neokapi/neokapi/core/model` (not `.../model`);
  top-level `sievepen/` and `termbase/` hold both in-memory and SQLite backends
  (not `core/sievepen`, not `cli/storage/sievepen`); LLM/MT backends are
  `providers/ai` (package `aiprovider`) and `providers/mt` (package
  `mtprovider`); pipeline tools are `core/ai/tools`, `core/mt/tools`; brand
  voice is `core/brand`.
- **Built-in flows** are `ai-translate`, `ai-translate-qa`, `pseudo-translate`,
  `qa-check`. There is **no** `translate` flow — `kapi run translate` only works
  with a project file that defines one.
- **`--target-lang` is single-valued** for `run` and tool commands; only
  `extract` accepts a comma-separated list. Don't show `--target-lang fr,de,ja`
  fanning out to multiple files.
- **`kapi termbase`** uses `-s`/`--source-locale` and `-t`/`--target-locale`
  (not `--source-lang`).
- **`--json`** is the output-format flag (a global persistent flag); `--format`
  / `-f` overrides *input* format detection — don't use `--format json` for
  output.
- **Homebrew formula** is `neokapi/tap/kapi-cli` (CLI) and
  `neokapi/tap/kapi` (cask).
- Format families: DOCX/XLSX/PPTX/ODF/EPUB/PDF/IDML are **native**, not
  bridge-only. `TBX` is not a format (only `tmx`); `RESX` is an XML preset, not
  a standalone format.

## Product names

- **neokapi** — the project and Go framework (lowercase).
- **kapi** — the standalone CLI.
- **kapi-desktop** — the desktop GUI companion.
- **bowrain** — the full-stack platform. The standalone `bowrain` binary is
  retired; bowrain commands run via `kapi <command>` once the plugin is
  installed.

## Navigation & information architecture (docs sites)

- Surface top-level sections directly in the navbar — don't hide everything
  behind a single "Documentation" entry.
- Use one sidebar per context (Get Started, CLI, React, Desktop, Framework,
  Reference) and organize within each context by Diátaxis (tutorial, how-to,
  reference, explanation).

## Pre-publish checklist

1. No superlatives or hype words; no emoji.
2. Shared-mechanism wording is neutral (text/content/extract), not
   "translatable", unless the context is specifically about translation.
3. No hardcoded format/tool/provider/filter/language counts.
4. Each topic stated once; overlapping pages merged or cross-linked; redirects
   added for moved URLs.
5. Every command, flag, import path, and flow name verified against the code.
6. Headings in sentence case; product names spelled per the list above.
7. Build is clean — `tsc` and the site build pass with no new warnings.

> If machine enforcement is wanted later, this guideline can also be encoded as
> a `core/brand` VoiceProfile (tone/style/vocabulary rules + examples) and run
> through the `brand-voice-check` / `brand-vocab-filter` tools.
