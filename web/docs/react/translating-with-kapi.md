---
sidebar_position: 9
title: Translating kapi-react Projects with kapi
description: How to use the kapi CLI to translate a kapi-react KLF archive — pseudo-translation for QA, AI translation with Claude or GPT-4, and review and QA flows that write results back to the archive in place.
keywords: [kapi, translate, KLF, pseudo-translate, AI translation, kapi-react, localization workflow]
---

# Translating with `kapi`

`kapi-react` produces a KLF directory archive. The `kapi` CLI translates it. This page walks through the three most useful flows.

## Pseudo-translation for UI QA

Pseudo-translation is the fastest way to see what's been picked up for translation — and what hasn't.

```bash
kapi pseudo-translate i18n/
```

No `-o` — the default for KLF inputs is in-place: the `qps` target is added to the same archive. Run again with `--target-lang fr` to add another locale; the writer is locale-additive and existing targets stay put.

Source `Welcome to Acme` becomes `[Ŵéḷçőḿé tő Âçmé]`:

- **Accented characters** make translated strings visually distinct. Untranslated strings (bugs) stand out immediately in the UI.
- **Brackets** mark start/end, so truncation is obvious (`Welcome to Ac…` never wraps to `[Ŵéḷç…`).
- **Expansion** (adding 30–50% more characters) mimics German / French / Russian string growth so layout bugs surface before you ship.

Then `kapi-react compile` it + load as any other locale:

```bash
kapi-react compile i18n/ --out public/translations
```

Your dev server now has a `qps` locale — wire a language picker and ship pseudo-translated screenshots to design review.

### Pseudo-translation in CI

Add it to CI as a UI-layout smoke test:

```yaml title=".github/workflows/ui-qa.yml"
- run: vp kapi-react extract
- run: kapi pseudo-translate i18n/
- run: vp kapi-react compile i18n/ --out public/translations
- run: npm run test:e2e # runs against ?locale=qps
```

## AI translation

For actual translations, `kapi translate` feeds the KLF directory through an LLM. It preserves placeholders, inline element tokens, and plural / select structure:

```bash
kapi translate i18n/ --target-lang fr
kapi translate i18n/ --target-lang de
kapi translate i18n/ --target-lang ja
```

Each call accumulates a target locale in place. To redirect output to a different file, pass `-o target-dir/` — the input stays untouched.

kapi supports Anthropic, OpenAI, Google Gemini, Azure OpenAI, and local Ollama models. Select the provider (and optionally the model) with flags — or in a flow's step config:

```bash
kapi translate i18n/ --target-lang fr --provider anthropic
```

API keys are never written into the committed recipe. Supply one, in precedence order, with `--api-key`, a saved keychain credential (`kapi credentials add`, then `--credential <name>`), or the provider's standard environment variable (`ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, `GEMINI_API_KEY`, …). Local Ollama needs no key.

See [AI translation](/framework/ai-translation) for the full provider and configuration surface.

### Context carries through

Every block in the KLF directory carries its `jsxPath` (e.g. `"div > button"`), its component name, and any `data-i18n-note` annotation. The AI translator gets that context as part of the prompt — so a `<Button>Close</Button>` in a dialog gets a different translation than a `<a>Close</a>` in a list-item's delete action.

### Translate a subset

For incremental translations (only the strings that changed), re-extract into the same archive and translate only the untranslated blocks. `kapi-react extract` is locale-additive, so re-running it adds new source blocks without disturbing existing targets; `translate --skip-matched` then skips any block that already has a target for the locale:

```bash
vp kapi-react extract                          # refresh i18n/ with new/changed blocks
kapi translate i18n/ --target-lang fr --skip-matched
```

Only the blocks added since the last pass are sent to the LLM; everything already translated is left as-is.

## Quality assurance

`kapi qa` runs placeholder, inline-code, whitespace, and length checks against a translated archive; `kapi term-check` enforces a glossary:

```bash
kapi qa i18n/ --target-lang fr                                # placeholder, code, length, consistency
kapi term-check i18n/ --target-lang fr --termbase fr-termbase.csv   # terminology
```

`qa` covers:

- **Placeholder & inline-code integrity** — every `{name}` and inline-element token (`{=m0}`) in the source appears in the target.
- **Length bounds** — flag targets that grow or shrink beyond configurable percentages of the source (useful for fixed-width UI containers).
- **Consistency** — double spaces, doubled words, leading/trailing whitespace, target-identical-to-source, and more (each individually toggleable).

`term-check` flags targets that violate the glossary — e.g. a brand term that must stay untranslated. (`kapi qa --check-terminology` folds the project termbase into the QA pass instead of running a separate command.)

QA results can fail your build — a common CI pattern is `extract → translate → qa`, exiting non-zero on any category you gate on.

## Translation memory leverage

`kapi` has a built-in SQLite translation memory. Feed past translations in:

```bash
kapi tm import historical-translations.xliff -s en -t fr
```

Then pre-fill matches before the AI pass: `kapi recycle` writes exact and high-scoring fuzzy matches into the target, and `translate --skip-matched` translates only what's left:

```bash
kapi recycle i18n/ --target-lang fr            # fill targets from the TM (defaults to the project TM)
kapi translate i18n/ --target-lang fr --skip-matched
```

Pass `--tm <name-or-path>` to leverage a specific TM. See [Translation memory](/framework/translation-memory) for the match and fill thresholds.

## Terminology consistency

For apps with a large product vocabulary, keep terms rendered consistently with a termbase. Import the glossary, then gate translations with `kapi term-check` so any target that diverges from an approved term is flagged:

```bash
kapi termbase import product-terms.csv -s en -t fr
kapi term-check i18n/ --target-lang fr --termbase product-terms.csv
```

To feed terminology into the translation step itself rather than only checking it afterward, compose a [flow](/framework/flows) that runs term lookup before `translate` — the matched terms become glossary context in the prompt.

See [Terminology](/framework/terminology).

## Putting it together

A complete Makefile / package-scripts setup for a multi-locale app:

```json title="package.json"
{
  "scripts": {
    "i18n:extract": "vp kapi-react extract",
    "i18n:pseudo": "kapi pseudo-translate i18n/",
    "i18n:ai": "for lang in fr de ja; do kapi translate i18n/ --target-lang $lang; done",
    "i18n:compile": "vp kapi-react compile i18n/ --out public/translations"
  }
}
```

## Drive it from a project

The commands above are ad-hoc — flags on every call, fine for a quick run. For an
app you translate every release, a [`.kapi` project file](/contribute/architecture/008-project-model)
is the working model worth adopting: it captures the content patterns, target
languages, flows, and defaults once, so you drive everything through named flows
instead of repeating flags, and the project store accumulates translation memory
across releases. Define a `translate` flow in the recipe (for example
`recycle` → `translate` → `qa`), then:

```json title="package.json"
{
  "scripts": {
    "i18n:extract": "vp kapi-react extract",
    "i18n:translate": "kapi run translate",
    "i18n:compile": "vp kapi-react compile i18n/ --out public/translations"
  }
}
```

`kapi run` discovers the nearest `.kapi` recipe (or pass `-p translation.kapi`) and executes the flow with the project's declared source and target languages and defaults. See [Flows](/framework/flows) and the [project model](/contribute/architecture/008-project-model).

## Next

- [Configuration](./configuration) — componentMap, rules, Storybook, warnings.
