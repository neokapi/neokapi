---
sidebar_position: 9
title: Translating neokapi-i18n Projects with kapi
description: "How to use the kapi CLI to translate a neokapi-i18n KBF archive: pseudo-translation for QA, AI translation with the provider of your choice, and review and QA flows that write results back to the archive in place."
keywords: [kapi, translate, KBF, pseudo-translate, AI translation, neokapi-i18n, translation workflow]
---

# Translating with `kapi`

`neokapi-i18n` produces a KBF directory archive. The `kapi` CLI translates it. This page walks through the three most useful flows.

## Pseudo-translation for UI QA

Pseudo-translation is the fastest way to see what has been picked up for translation, and what has not.

```bash
kapi pseudo-translate i18n/
```

With no `-o`, the default for KBF inputs is in-place: the `qps` target is added to the same archive. Run again with `--target-lang fr` to add another locale; the writer is locale-additive and existing targets stay put.

Source `Welcome to Acme` becomes `▒ Ŵéḷçőḿé tő Âçmé ▒`:

- **Accented characters** make translated strings visually distinct. Untranslated strings (bugs) stand out immediately in the UI.
- **Markers** (`▒`) mark start and end, so truncation is obvious (`Welcome to Ac…` never wraps to `▒ Ŵéḷç…`).
- **Expansion** (adding 30–50% more characters) mimics German, French and Russian string growth so layout bugs surface before you ship.

Then `neokapi-i18n compile` it and load it as any other locale:

```bash
neokapi-i18n compile i18n/ --out public/translations
```

Your dev server now has a `qps` locale. Wire a language picker and ship pseudo-translated screenshots to design review.

### Pseudo-translation in CI

Add it to CI as a UI-layout smoke test:

```yaml title=".github/workflows/ui-qa.yml"
- run: vp neokapi-i18n extract
- run: kapi pseudo-translate i18n/
- run: vp neokapi-i18n compile i18n/ --out public/translations
- run: npm run test:e2e # runs against ?locale=qps
```

## AI translation

For actual translations, `kapi translate` feeds the KBF directory through an LLM. It preserves placeholders, inline element tokens, and plural / select structure:

```bash
kapi translate i18n/ --target-lang fr
kapi translate i18n/ --target-lang de
kapi translate i18n/ --target-lang ja
```

Each call accumulates a target locale in place. To redirect output to a different file, pass `-o target-dir/`; the input stays untouched.

kapi supports Anthropic, OpenAI, Google Gemini, Azure OpenAI, and local Ollama models. Select the provider (and optionally the model) with flags, or in a flow's step config:

```bash
kapi translate i18n/ --target-lang fr --provider anthropic
```

API keys are never written into the committed recipe. Supply one, in precedence order, with `--api-key`, a saved keychain credential (`kapi credentials add`, then `--credential <name>`), or the provider's standard environment variable (`ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, `GEMINI_API_KEY`, …). Local Ollama needs no key.

See [Translation](/framework/translation) for the full provider and configuration surface.

### Context carries through

Every block in the KBF directory carries its element (`"button"`, `"p"`, …), its file and line, its component name, and any `data-i18n-note` annotation. The AI translator gets that context as part of the prompt, so a `<button>Close</button>` gets a different translation than an `<a>Close</a>` in a list-item's delete action.

Where the element alone is too little to disambiguate (two buttons both reading "Open", one a verb and one a state), add the note explicitly with `data-i18n-note` or `t(text, context)`. That note is part of the key, so the two strings separate for the translator and stay separate.

### Translate a subset

For incremental translations (only the strings that changed), re-extract into the same archive and translate only the untranslated blocks. `neokapi-i18n extract` is locale-additive, so re-running it adds new source blocks without disturbing existing targets; `kapi exec translate --skip-matched` then skips any block that already has a target for the locale:

```bash
vp neokapi-i18n extract                          # refresh i18n/ with new/changed blocks
kapi exec translate i18n/ --target-lang fr --skip-matched
```

Only the blocks added since the last pass are sent to the LLM; everything already translated is left as-is.

## Quality assurance

`kapi exec qa` runs placeholder, inline-code, whitespace, and length checks against a translated archive; `kapi exec term-check` enforces approved terminology:

```bash
kapi exec qa i18n/ --target-lang fr                              # placeholder, code, length, consistency
kapi exec term-check i18n/ --target-lang fr --termstore terms/fr.db   # terminology
```

`qa` covers:

- **Placeholder and inline-code integrity**: every `{name}` and inline-element token (`{=m0}`) in the source appears in the target.
- **Length bounds**: flag targets that grow or shrink beyond configurable percentages of the source (useful for fixed-width UI containers).
- **Consistency**: double spaces, doubled words, leading/trailing whitespace, target-identical-to-source, and more (each individually toggleable).

`term-check` flags targets that diverge from an approved term, for example a product name that must stay untranslated. (`kapi exec qa --check-terminology` folds the project terms store into the QA pass instead of running a separate command.)

QA results can fail your build. A common CI pattern is `extract → translate → qa`, exiting non-zero on any category you gate on.

## Content memory leverage

`kapi` has a built-in SQLite content memory. Feed past translations in:

```bash
kapi memory import historical-translations.tmx -s en -t fr
```

Then pre-fill matches before the AI pass: `kapi exec recycle` writes exact and high-scoring fuzzy matches into the target, and `kapi exec translate --skip-matched` translates only what's left:

```bash
kapi exec recycle i18n/ --target-lang fr   # fill targets from the project content memory
kapi exec translate i18n/ --target-lang fr --skip-matched
```

Pass `--memory <name-or-path>` to `kapi exec recycle` to draw on a specific content memory. See [Content memory](/framework/content-memory) for the match and fill thresholds.

## Terminology consistency

For apps with a large product vocabulary, keep terms rendered consistently with a terms store. Import the term list into a named store, then gate translations with `kapi exec term-check` so any target that diverges from an approved term is flagged:

```bash
kapi terms import product-terms.csv -s en -t fr --name product-terms
kapi exec term-check i18n/ --target-lang fr --termstore product-terms
```

To feed terminology into the translation step itself rather than only checking it afterward, bind the store in the recipe (`defaults.terms_source`) or list rules under `term_rules:` in the step's config. `translate` reads the matching rules itself and sends them in the prompt's terminology section.

See [Terminology](/framework/terminology).

## Putting it together

A complete Makefile / package-scripts setup for a multi-locale app:

```json title="package.json"
{
  "scripts": {
    "i18n:extract": "vp neokapi-i18n extract",
    "i18n:pseudo": "kapi pseudo-translate i18n/",
    "i18n:ai": "for lang in fr de ja; do kapi translate i18n/ --target-lang $lang; done",
    "i18n:compile": "vp neokapi-i18n compile i18n/ --out public/translations"
  }
}
```

## Drive it from a project

The commands above are ad-hoc: flags on every call, fine for a quick run. For an
app you translate every release, a [`kapi.yaml` project file](/contribute/architecture/context/c-01-project-model)
is the working model worth adopting: it captures the content patterns, target
languages, flows, and defaults once, so you drive everything through named flows
instead of repeating flags, and the project store accumulates content memory
and terminology across releases.

`kapi init --framework neokapi-i18n` scaffolds the recommended layout, in which
everything the stack authors sits under one `i18n/` directory. The bundler writes
source catalogs to `i18n/src/`; kapi writes per-locale targets to `i18n/{lang}/`.
Source living under `i18n/src/` (rather than flat under `i18n/`) is what lets the source
glob stay clear of the generated targets, with no sibling `i18n-<lang>/` trees:

```yaml title="kapi.yaml"
version: v1
name: MyApp
defaults:
  source_language: en
  target_languages: [de, fr, ja, nb]
  # The terms store and the voice profile are git-tracked sources under i18n/.
  voice:
    profile_file: i18n/voice.yaml
  terms_source: i18n/terms.json
collections:
  - path: "i18n/src/**/*.kbf.json"
    format: kbf
    target: "i18n/{lang}/{path}.kbf.json"
```

```
i18n/
├── src/                    source KBF catalogs (from `neokapi-i18n extract`)
├── de/ fr/ ja/ nb/         per-locale targets (from kapi)
├── terms.json              terms store (git source)
└── voice.yaml              voice profile (git source)
```

The content memory's local store and the pseudo-locale output are rebuildable
state, so they live under `.kapi/work/` (gitignored); the memory bundle the
recipe names in `defaults.memory_source` is committed with the rest of `.kapi/`.
Define a `translate` flow in the recipe (for example `recycle` → `translate` → `qa`), then:

```json title="package.json"
{
  "scripts": {
    "i18n:extract": "vp neokapi-i18n extract",
    "i18n:translate": "kapi run translate",
    "i18n:compile": "vp neokapi-i18n compile i18n/ --out public/translations"
  }
}
```

`kapi run` discovers the nearest `kapi.yaml` recipe (or pass `-p kapi.yaml`) and executes the flow with the project's declared source and target languages and defaults. See [Flows](/framework/flows) and the [project model](/contribute/architecture/context/c-01-project-model).

## Drive it from Claude

The same flows run from your AI assistant. With the [kapi MCP server](/reference/mcp)
connected, point Claude at your extracted strings and ask it to translate and check
them. It calls `kapi` to translate the KBF archive, runs the QA checks, and fixes
anything that breaks, the same author → check → revise loop you'd run by hand:

> "Translate the strings in `i18n/` to French and German, keep the placeholders and
> inline elements intact, then run QA and fix anything that fails."

Claude translates in place (locale-additive), runs `kapi exec qa` / `kapi exec term-check`, and
loops on the findings until the archive passes; you `neokapi-i18n compile` the result as
usual. Your components stay as they are; only the catalogue changes.

See [Use with Claude](/kapi/get-started/use-with-claude) for the MCP setup and the
broader agent loop.

## Next

- [Configuration](./configuration): componentMap, rules, Storybook, warnings.
