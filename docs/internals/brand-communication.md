# Brand & Communication Guideline

Guidance for writing user-facing text in this repository: the documentation
site and landing home (`web`), the bowrain landing page
(`bowrain/web/landing`), READMEs, release notes, CLI help, and UI copy. Claude
and human contributors should follow it whenever they add or edit prose.

The goal: an **academic, precise register**. Explain what something is and what
it does, and let the facts stand without selling. The reader is a developer or a
content engineer, not a prospect.

## Voice

- State capabilities plainly. "kapi reads and writes document, data, and
  bilingual interchange formats" rather than "kapi delivers powerful, seamless
  format support."
- Lead with the problem and the mechanism, not adjectives. Prefer a
  problem→solution sentence over a brochure bullet list of buzzwords.
- Be specific and verifiable. Every claim should be checkable against the code
  or a generated artifact.
- One idea per sentence; short sentences over long ones.

## Avoid

- **Marketing superlatives and hype:** powerful, seamless, effortless, blazing,
  production-proven, game-changing, cutting-edge, revolutionary, supercharge,
  unleash, "just point and go", "translate at scale", "everything you
  need". If a sentence still means the same thing with the adjective removed,
  remove it.
- **"magic" as a claim about the product:** "it just works, like magic", "pure
  magic", "magical", "where the magic happens". State the mechanism instead.
  The word also has a technical sense (magic bytes, a magic number, the magic
  string on a document root) which names a file signature and stays.
- **Emoji** in documentation and committed prose.
- **Inconsistent casing.** Use sentence case for headings and UI titles
  ("AI-native translation", not "AI-Native Translation").
- **Brochure framing.** Don't restate the same feature list as a hero, a card
  grid, and a bullet list. Say it once, in the right place.

## The vocabulary

neokapi is a **content and language intelligence** framework: it parses any
format into one content model, edits and checks the content inside it, and writes
it back faithfully. Multilingual work is one of the things it does; the frame is
the content. The vocabulary below is authoritative; use the right-hand column.

| Don't write | Write |
| --- | --- |
| translation memory, TM | **content memory** |
| termbase, glossary (the store) | **terms**, **terms store** |
| localization, l10n, localisation | **multilingual content**, or **language** |
| localize, localized, localizing, localizable | adapt, translate, or recast the sentence |
| localization framework / engine | **content framework** / **content engine** |

Words that are **not** jargon and stay: *translate*, *translation* (the act),
*locale*, *language*, *terminology*, *parity*, *bilingual*, *i18n*.

Recast rather than substitute. "The page being localized" becomes "the page being
translated". "A localization pipeline" becomes "the pipeline", or "the
translation pipeline" when the sentence is genuinely about translating. "A
localization engineer" becomes "a content engineer", or just "an engineer" when
the qualifier carries nothing.

Three things are **not** ours to rename, and must stay factual:

- **TMX** is "Translation Memory eXchange" (LISA/OSCAR) and **TBX** is "TermBase
  eXchange" (ISO 30042). Never rewrite those expansions.
- **The Okapi Framework** is a localization framework, and the Java bridge wraps
  it. Say so; accurate provenance is not something to sanitize. The Okapi
  terminology mapping (Filter → DataFormat, Step → Tool, …) stays as recorded.
- **Retained identifiers.** SQL tables and migration bookkeeping (`tm_entries`,
  `tm_variant_trigram`, `sievepen_migrations`, `termbase_migrations`), `l10n-*`
  make targets, `l10n/` as a project directory a user chose, HTTP routes
  (`/translation-memory`, `/tm-matches`) and analytics ids (`glossary_saved`,
  `method: "tm"`) all keep their spelling; renaming them
  breaks a wire, disk, or history boundary. Where prose documents one, describe
  the **new concept** and quote the identifier verbatim: "the content memory's
  entry table (`tm_entries`)".

  The names a user actually types or sees on disk are *not* on that list, and
  match the concepts: recipe keys are `memory:`, `terms_source:` and
  `memory_source:`; flags are `--memory` and `--termstore`; a project keeps one
  database at `.kapi/work/store.db` with its committed sources beside it; named
  stores live under `<ConfigDir>/terms/`. Finding `tm` or `termbase` in one of
  those is a leftover, not a boundary.

## Don't frame the shared engine as translation

Source-language work (brand guardrails, terminology, QA) and translation share
one engine. Wording that frames the shared mechanism as translation undersells
the source-language half: the reader doing brand work shouldn't feel the docs
are about something else.

- ✗ "the **translatable** text is segmented into blocks", "kapi extracts
  **translatable** text", "only the **translatable** text changes".
- ✓ Use neutral terms for the shared mechanism: *text*, *content*, *block*,
  *extract*: "the text is **extracted** into blocks", "kapi extracts the text",
  "only the text changes".
- Reserve *translatable* / *translation* for genuinely translation-specific
  contexts (a translation-job estimate, a target-language round-trip, the Okapi
  heritage mapping).
- **Extraction produces blocks; segmentation does not.** Reading a file
  *extracts* its text into blocks; segmentation is a separate, opt-in overlay
  *within* a block. Don't write "segmented into blocks".
- Exception: the content model's **Block = translatable content** vs. a
  **non-translatable skeleton** is a precise technical distinction; keep it in
  the Framework / content-model docs, but don't carry that framing onto the
  front page or everyday Kapi pages.
- **Lead examples with source-content formats, not interchange formats.** The
  framework's everyday surface is reading the content people author: JSON,
  DOCX, Markdown, HTML, YAML, mobile catalogs (`strings.xml`, `.xcstrings`), and
  the like. Classic bilingual interchange formats (**XLIFF, PO, TMX, Qt TS**)
  are for the translator handoff; feature them in the bilingual-workflow /
  interchange context, not as the headline example or the first item in a
  formats list. We support them, but they aren't the point.

## Never hardcode counts that the code controls

Do not write numbers that change whenever the codebase changes; they rot and
create a maintenance tax on every PR:

- ✗ "42 built-in formats", "15+ formats", "40+ Okapi filters", "5 MT
  providers", "80+ tools", "~30 tools".
- ✓ Name the *categories* ("document, data, subtitle, office, and bilingual
  interchange formats") and link to the live, generated reference (e.g. the
  `/formats` page, rendered from the reference dataset that
  `make generate-reference-docs` compiles).
- If a count genuinely helps on an MDX page, derive it from the generated data
  at render time; never type the literal.

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
  top-level `memory/` and `terms/` hold both in-memory and SQLite backends
  (not `core/memory`, not `cli/storage/memory`); LLM/MT backends are
  `providers/ai` (package `aiprovider`) and `providers/mt` (package
  `mtprovider`); pipeline tools are `core/ai/tools`, `core/mt/tools`; brand
  voice is `core/profile`.
- **Built-in flows** include `translate`, `translate-qa`, `pseudo-translate`,
  `qa` and `recycle` (`host/flowdef/builtin.go`). `kapi run translate` runs the
  built-in `translate` flow, which is memory reuse, then translate, then the
  checks; a project file can define additional named flows, and a built-in name
  wins over a project flow of the same name.
- **`--target-lang` is single-valued** for `run` and tool commands; only
  `extract` accepts a comma-separated list. Don't show `--target-lang fr,de,ja`
  fanning out to multiple files.
- **The store verbs are `kapi memory` and `kapi terms`**; `kapi tm` and
  `kapi termbase` were retired with no aliases. Both take the locale with
  `-s`/`--source-locale` and `-t`/`--target-locale`, not `--source-lang`.
- **The user config home is platform-dependent.** kapi resolves
  `os.UserConfigDir()`: `~/.config/kapi` on Linux, `~/Library/Application
  Support/kapi` on macOS. Don't write `~/.config/kapi` unqualified; name both
  platforms, or point at `kapi config path`. (`$HOME/.config/kapi/kapi.yaml` is
  still read as a lower-precedence legacy layer everywhere, so an existing
  hand-written file keeps working.)
- **`--json`** is the output-format flag (a global persistent flag); `--format`
  / `-f` overrides *input* format detection; don't use `--format json` for
  output.
- **Homebrew formula** is `neokapi/tap/kapi-cli` (CLI) and
  `neokapi/tap/kapi` (cask).
- Format families: DOCX/XLSX/PPTX/ODF/EPUB/IDML are **native**, not
  bridge-only. PDF is read by the `kapi-pdfium` plugin (and its wasm build in
  the browser); the framework carries only its configuration. `TBX` is not a
  format (only `tmx`); `RESX` is a standalone native format
  (`core/formats/resx/`), not just an XML preset.

## Product names

- **neokapi**: the project and Go framework (lowercase).
- **kapi**: the standalone CLI.
- **kapi-desktop**: the desktop GUI companion.
- **bowrain**: the full-stack platform. The standalone `bowrain` binary is
  retired; bowrain commands run via `kapi <command>` once the plugin is
  installed.

## Navigation & information architecture (docs sites)

- Surface top-level sections directly in the navbar; don't hide everything
  behind a single "Documentation" entry.
- Use one sidebar per context (the sidebars in `web/sidebars.ts`: Kapi,
  Framework, Reference, Toolbox, React) and organize within each context by
  Diátaxis (tutorial, how-to, reference, explanation).

## Pre-publish checklist

1. No superlatives or hype words; no emoji.
2. Vocabulary matches [The vocabulary](#the-vocabulary): no *translation memory*,
   *termbase*, *glossary*, *localization*, *l10n* or *localize* in any inflection.
   Check with `grep -niE 'localiz|localis|l10n|termbase|glossar|translation memor'`;
   that pattern catches the inflections a noun-only search misses.
   `scripts/check-vocabulary.sh` runs the same sweep over the user-facing
   surfaces in `make lint` and `make pre-push`.
3. Shared-mechanism wording is neutral (text/content/extract), not
   "translatable", unless the context is specifically about translation.
4. No hardcoded format/tool/provider/filter/language counts.
5. Each topic stated once; overlapping pages merged or cross-linked; redirects
   added for moved URLs.
6. Every command, flag, import path, and flow name verified against the code.
7. Headings in sentence case; product names spelled per the list above.
8. Build is clean: `tsc` and the site build pass with no new warnings.

## Machine enforcement

This guideline is encoded as the project's voice profile, `.kapi/voice.yaml`,
bound project-wide by `defaults.voice` in the root `kapi.yaml`; keep the two in
step. `make check-docs-prose` runs `kapi check` over both documentation sites,
the READMEs and the site taglines under that profile, and
`make check-governed-prose` does the same for the package and installer
descriptions; both run in the `reference-data-drift.yml` workflow.
`scripts/check-vocabulary.sh` sweeps the retired vocabulary in `make lint`,
`make pre-push` and the *Repo guards* CI job.
