# Dogfood localization of neokapi's own surfaces — feasibility, effort, cost

Status: investigation (2026-06-10), **implemented** for the kapi-side text
surfaces on the same date — see "Implementation status" at the end. Video
production (recording/narration/rendering) remains to be run on a machine
with the recording stack; everything else below the plan headings landed. Scope: localize the kapi-side product
surfaces (desktop app, docs site, framework data, CLI, landing page) and their
visual assets (videos, screenshots, light + dark) using kapi itself — brand
voice, termbase, and TM in the loop. First locale: Norwegian Bokmål (`nb`),
chosen so the maintainer can review personally. Bowrain surfaces follow later
with the same machinery.

## Verdict

Feasible, and closer than expected. Every load-bearing capability already
exists and is proven end-to-end by the `qps` pseudo-locale pipeline:
extraction formats (JSON/KLF, MDX, Markdown, YAML, PO, MO), `ai-translate`
with brand-voice prompt injection and glossary, `tm-leverage` (exact + fuzzy),
termbase with TBX import and forbidden/preferred term statuses, `term-check`
QA, and gettext MO catalogs consumed at runtime by the CLI and desktop app.
`nb` normalizes correctly in `core/locale`.

The genuine gaps are wiring, not capability:

1. **No root dogfood recipe.** CLAUDE.md describes a repo-root `*.kapi`, but
   none exists; only the minimal `core/i18n/i18n.kapi` (builtin metadata
   extraction). A root recipe declaring all surfaces is the keystone.
2. **The harness has zero locale awareness.** Narration text is inline
   English in `demo.yaml`, captions are baked into rendered frames, TTS is
   voice-picked but not language-parameterized, publish routing has no locale
   dimension.
3. **Library packages are hardcoded.** `packages/ui` (~100 components) and
   `packages/flow-editor` carry English strings outside the kapi-react
   extraction path (though `packages/ui/i18n-manifest.json` already encodes
   the intended translatability model).
4. **Cobra help and `cli/output` are not catalog-wired.** The `core/i18n`
   Translator localizes registry metadata but not the ~153 Short/Long/Example
   entries (~1.1k words) or table headers/status messages.
5. **No composed preset flow** chaining tm-leverage → ai-translate
   (+ glossary/brand voice) → term-check → qa-check; flows must be authored
   in the recipe (acceptable for dogfooding — it *is* the dogfood).

## Surface inventory and current readiness

| Surface | Volume (approx.) | Localization state |
| --- | --- | --- |
| kapi-desktop frontend | 473 strings (~2–3k words), 37 screens | **Ready.** `@neokapi/kapi-react` build-time extraction + OTA loading; `qps` catalog live; runtime language switching wired (`useNeokapi()`) |
| Framework data (tool/format/preset metadata, schema titles) | ~500 strings (~5k words), `core/i18n/builtins/metadata.json` | **Ready.** Generated catalog, gettext MO runtime, `qps.mo` proves the loop; surfaces in CLI lists and desktop schemas |
| Docs site `web/docs` | 111 MD/MDX files, ~141k raw words (~90–100k translatable prose) | Docusaurus i18n configured but `en`-only; kapi's MDX format handles frontmatter + JSX preservation |
| Walkthrough videos (kapi) | 10 published pairs (light + dark); 6 of 10 are desktop-UI demos | No locale dimension in harness; captions baked into frames |
| Screenshots | 5 hand-made logos in `web/docs/static/img`; all other visuals are per-demo artifacts inside videos | Subsumed by video re-capture — no separate screenshot program needed |
| `packages/ui`, `packages/flow-editor` | ~100 + 26 components, hundreds of labels | Hardcoded; `i18n-manifest.json` defines the intended model |
| CLI help + output | 153 help entries (~1.1k words) + ~139 fmt-call headers/messages | Hardcoded; no catalog wiring |
| Landing home (folded into `web/docs`) | ~1–2k words | Localizes through the docs-site path |
| Go error strings | ~1.4k, mostly developer-facing | Out of scope for v1 (recommend: leave English) |

Total kapi-side translatable volume: **~110–120k words, ~85% of it docs.**

## Phased plan

### Phase 0 — Dogfood foundation (3–5 dev-days)

- Author the repo-root `neokapi.kapi` recipe declaring all content sets:
  desktop KLF catalogs, `core/i18n/builtins/*.json`, docs MDX tree, landing
  strings, harness narration. `defaults.target_languages: [nb]`.
- Author the `VoiceProfile` from `docs/internals/brand-communication.md`
  (academic register, no superlatives, neutral terminology) — feeds
  `ai-translate` via `RenderVoiceGuideCompact`.
- Seed the termbase: product names (kapi, Bowrain, sievepen, KLF, flow,
  tool, recipe, …) with `nb` decisions and forbidden variants; import as
  TBX/CSV into `.kapi/termbase.db`.
- Initialize the project TM (`.kapi/tm.db`); it grows as the first surfaces
  are translated and reviewed, making every later surface cheaper.
- Compose the recipe flow: tm-leverage → ai-translate (glossary + brand
  voice) → term-check → qa-check.
- Honor the in-repo isolation contract: the dogfood recipe at the root is the
  *one* invocation that may bind to it; everything else keeps `KAPI_NO_PROJECT=1`.

### Phase 1 — Framework data + desktop UI in `nb` (4–7 dev-days + ~1–2 review-days)

- Run the flow over desktop KLF + builtin metadata (~7–8k words). Compile
  `nb.mo` and `nb.json` catalogs; verify with the existing language switcher.
- Externalize `packages/ui` and `packages/flow-editor` strings through the
  kapi-react path per `i18n-manifest.json` — the bulk of this phase's
  engineering (3–6 days inside the range above).
- Review burden (maintainer, nb-fluent): 1–2 days; UI strings are short and
  high-leverage — this also stocks the TM and settles terminology before the
  docs run.

### Phase 2 — Docs site in `nb` (2–4 dev-days + tiered review)

- Enable `nb` in `docusaurus.config.ts`; `write-translations` for theme
  strings (JSON — kapi json format handles it).
- Pipeline: kapi MDX translate `web/docs/docs/**` → `i18n/nb/.../current/`
  tree. Code blocks, CLI output, and JSX stay untouched by format config;
  TM session caching makes re-runs incremental as docs drift.
- Review: a full pass over ~100k words is 70–140 hours at 1–2k words/hour —
  not recommended. Tiered instead: full review of the ~20 top-traffic pages
  (~15–25 hours), automated term-check + qa-check across the rest, spot
  checks. Corrections feed the TM, so quality compounds.

### Phase 3 — Localized videos, light + dark (4–7 dev-days + ~1 review-day)

Engineering (one-time, reused for every future locale and for bowrain):

- Add a `locale` dimension to `demo.yaml` narration (per-locale scene text),
  TTS language/voice selection, caption generation, render output naming
  (`{publishAs}-{locale}-{theme}.webm`), publish routing, and locale-aware
  `ThemedVideo` embeds in the docs.
- Desktop walkthroughs (`record-desktop.ts`): set the app UI language before
  recording — depends on Phase 1.

Per-locale production:

- Translate 10 narration scripts (~3–5k words) through the same recipe flow;
  maintainer review ~half a day.
- Terminal demos (4 of 10): narration + captions only — single capture is
  reused, 2 theme re-renders each.
- Desktop demos (6 of 10): re-record light + dark against the nb UI, then
  render. Capture automation exists; this is compute time (a few hours), not
  person time.
- TTS: Gemini TTS (default backend) and ElevenLabs Multilingual v2 both
  cover Norwegian; **verify nb voice quality early** — this is the one
  external dependency that could force a backend switch. Inter/JetBrains
  Mono with latin-ext cover æ/ø/å.
- `docs-assets` release grows ~2× per locale (20 → 40 webm for kapi);
  acceptable, but watch release size as locales accumulate.

### Phase 4 — CLI help, output, landing (5–8 dev-days + ~1 review-day)

- Extend the `core/i18n` Translator to Cobra `Short`/`Long`/`Example` and
  `cli/output` headers (scope-keyed lookups, generator parity with the
  builtin-metadata pipeline, `--lang`/`KAPI_LANG` already resolve). ~3–5 days.
- Externalize landing-page copy and run it through the flow. ~2–3 days.
- Translation volume is small (~2–3k words); review under a day.

### Later — Bowrain

Everything from Phases 0 and 3 carries over directly: 8 published video
pairs, the web SPA, desktop app, and server strings. The web SPA and bowrain
desktop have no kapi-react wiring today, so expect a Phase-1-shaped
externalization effort there; the harness locale machinery, recipe pattern,
TM, and termbase are already paid for.

## Cost summary (per locale `nb`, kapi side)

| Cost | Estimate |
| --- | --- |
| Engineering (one-time, reusable for all locales) | **18–31 dev-days** across Phases 0–4; Phases 0–2 alone (text surfaces live): 9–16 days |
| AI translation API (~120k words incl. QA passes, batching, brand-voice prompt overhead) | **< $50** on Sonnet-class; low hundreds on Opus-class. Noise relative to people time |
| TTS (10 demos, ~30 min audio) | **< $30** (Gemini trivial; ElevenLabs at the top of that) |
| Compute (renders, re-records) | A few hours wall-clock per full pass; the existing staged harness flow applies |
| Maintainer review (nb) | UI + metadata + CLI + narration: **3–5 days**. Docs: 15–25 hours tiered (70–140 hours if exhaustive) |
| Ongoing maintenance | Marginal: TM leverage + session caching make incremental re-runs cheap; each later locale pays only translation + review + render, no engineering |

## Risks and open questions

- **nb TTS quality** (Gemini default voice in Norwegian) — verify before
  committing to the video phase; backend is swappable per `.env`.
- **Docs drift** — translation lags source; mitigate with the TM-driven
  incremental flow in CI and a "translated as of <commit>" banner per page.
- **MDX edge cases at scale** — 111 real files will surface extraction-rule
  tuning (admonitions, tabs, diagram-kit props). Budgeted inside Phase 2 but
  the long tail lives here.
- **packages/ui externalization** is the largest single engineering unknown;
  the i18n-manifest suggests the design exists but the implementation does not.
- **Review bottleneck is the maintainer.** The plan deliberately front-loads
  small, high-leverage surfaces (UI, terminology) so the docs run inherits a
  settled termbase and a warm TM.

## Implementation status (2026-06-10)

All text surfaces shipped in `nb`; the model is: reviewed translations live
as committed TMX seeds under `l10n/tm/` (~2,080 pairs across six seeds),
`make l10n-seed` rebuilds the gitignored `.kapi/` termbase + TM from them,
and `make l10n` reproduces every localized artifact via `kapi tm-leverage`
— generated catalogs only ever contain reviewed strings. Review happens in
the TMX seeds and `l10n/termbase.csv` (~57 terminology decisions).

| Surface | Mechanism | State |
| --- | --- | --- |
| Framework data | `l10n-builtins` → `core/i18n/catalogs/nb.mo` (embedded) | 667 pairs; `kapi --lang nb tools/formats` localized |
| CLI help + output | `cli/i18n` generator + embedded catalogs (`l10n-cli`) | `KAPI_LANG=nb kapi --help` localized; multi-line Longs fall back pending the sievepen line-structure fix |
| Desktop UI + shared libraries | kapi-react extraction (now spanning `packages/ui` + `flow-editor`) + `l10n-desktop` | 1,034-entry `nb.json`; picker entry "Norsk (bokmål)" |
| Docs site | Docusaurus `nb` + bilingual `kapi extract`/`merge` loop | Theme strings + nine priority kapi pages; remaining pages fall back to English |
| Landing page | kapi-react runtime + footer switcher (`l10n-landing`) | 100% leveraged; demo/terminal content `translate="no"` |
| Videos | Harness `--locale` dimension + nb narration for the nine published kapi demos | Engineering + scripts done; record/narrate/render on the recording machine (`pnpm run demo published --locale=nb --only=narrate,render,publish --theme=both`) |

Dogfooding paid for itself immediately — five framework bugs found and
fixed by localizing our own content: merge staleness compared unlike run
renderings (inline-markup blocks always stale); content-target templates
lacked `**`/`{path}` support; `ResolveContent` used `filepath.Glob` (no
`**`); the TMX importer decoded BOM-less UTF-8 as windows-1252; and
sievepen's plain-text normalization drops line structure (open follow-up,
guarded in CLI help).

Known follow-ups: per-item format config plumbing into extract/merge
(frontmatter title/description), the sievepen line-structure fix, the
remaining ~100 docs pages (the pipeline scales by re-running the loop
per page), bowrain surfaces, and the nb video production run.
