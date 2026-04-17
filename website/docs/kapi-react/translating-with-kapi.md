---
sidebar_position: 8
title: Translating with kapi
---

# Translating with `kapi`

`kapi-react` produces a `.klz` archive. The `kapi` CLI translates it. This page walks through the three most useful flows.

## Pseudo-translation for UI QA

Pseudo-translation is the fastest way to see what's been picked up for translation — and what hasn't.

```bash
kapi pseudo-translate i18n/extracted.klz \
  --target-lang qps \
  -o i18n/translated.klz
```

Source `Welcome to Acme` becomes `[Ŵéḷçőḿé tő Âçmé]`:

- **Accented characters** make translated strings visually distinct. Untranslated strings (bugs) stand out immediately in the UI.
- **Brackets** mark start/end, so truncation is obvious (`Welcome to Ac…` never wraps to `[Ŵéḷç…`).
- **Expansion** (adding 30–50% more characters) mimics German / French / Russian string growth so layout bugs surface before you ship.

Then `kapi-react compile` it + load as any other locale:

```bash
kapi-react compile i18n/translated.klz --out public/translations
```

Your dev server now has a `qps` locale — wire a language picker and ship pseudo-translated screenshots to design review.

### Pseudo-translation in CI

Add it to CI as a UI-layout smoke test:

```yaml title=".github/workflows/ui-qa.yml"
- run: npm run extract
- run: kapi pseudo-translate i18n/extracted.klz --target-lang qps -o i18n/pseudo.klz
- run: kapi-react compile i18n/pseudo.klz --out public/translations
- run: npm run test:e2e     # runs against ?locale=qps
```

## AI translation

For actual translations, `kapi ai-translate` feeds the `.klz` through an LLM. It preserves placeholders, inline element tokens, and plural / select structure:

```bash
kapi ai-translate i18n/extracted.klz \
  --source-lang en \
  --target-lang fr \
  -o i18n/translated-fr.klz
```

Supported providers (configured in `kapi.yaml` or via flags):

- **Anthropic** — Claude models. Recommended for quality.
- **OpenAI** — GPT models.
- **Google Gemini** — streaming + live thinking progress.
- **Azure OpenAI** — Azure-hosted GPT models.
- **Ollama** — local models, no API key.

Configuration example:

```yaml title="kapi.yaml"
tools:
  ai-translation:
    provider: anthropic
    model: claude-sonnet-4-6
    apiKey: ${ANTHROPIC_API_KEY}
```

See [AI translation](/docs/features/ai-translation) for the full configuration surface.

### Context carries through

Every block in the `.klz` carries its `jsxPath` (e.g. `"div > button"`), its component name, and any `data-i18n-note` annotation. The AI translator gets that context as part of the prompt — so a `<Button>Close</Button>` in a dialog gets a different translation than a `<a>Close</a>` in a list-item's delete action.

### Translate a subset

For incremental translations (only the strings that changed), extract and diff against the current `translated.klz`:

```bash
kapi-react extract --out i18n/new.klz
kapi diff i18n/new.klz i18n/translated.klz --only-new -o i18n/delta.klz
kapi ai-translate i18n/delta.klz --target-lang fr -o i18n/delta-fr.klz
kapi merge i18n/translated-fr.klz i18n/delta-fr.klz -o i18n/translated-fr.klz
```

## Quality assurance

`kapi qa` runs terminology, placeholder, and consistency checks against a translated `.klz`:

```bash
kapi qa i18n/translated-fr.klz \
  --termbase fr-termbase.csv \
  --target-lang fr
```

Checks:

- **Placeholder integrity** — every `{name}` in the source appears in the target.
- **Terminology adherence** — if a termbase says `kapi` must stay `kapi`, flag targets that translated it.
- **Tag consistency** — `{=m0}` in the source must appear in the target; `<strong>` ↔ `<strong>` preserved.
- **Length bounds** — optional per-block character limits (useful for UI strings with fixed-width containers).

QA results can fail your build — a common CI pattern is `extract → ai-translate → qa`, failing on any category you care about.

## Translation memory leverage

`kapi` has a built-in SQLite translation memory. Feed past translations in:

```bash
kapi tm import historical-translations.xliff --source-lang en --target-lang fr
```

Then when you translate a new `.klz`:

```bash
kapi ai-translate i18n/extracted.klz --target-lang fr --tm-leverage
```

Segments that match (or fuzzy-match) past translations come through pre-populated — the AI only translates what's new.

See [Translation memory](/docs/features/translation-memory) for more.

## Terminology pre-translation

For apps with a large product vocabulary, pre-translate terminology before the AI pass so terms are consistently rendered:

```bash
kapi tools terminology-pretranslation i18n/extracted.klz \
  --termbase product-terms.csv \
  --target-lang fr \
  -o i18n/pre-translated.klz
```

Then pass `pre-translated.klz` to `kapi ai-translate`, which respects the pre-translated segments.

See [Terminology](/docs/features/terminology).

## Round-trip with Bowrain

For apps with a translation platform behind them, Bowrain ingests the extracted `.klz` directly:

```bash
bowrain push i18n/extracted.klz    # upload to Bowrain backend
# …translators work in the web editor…
bowrain pull -o i18n/translated.klz   # download finished translations
```

See [Bowrain CLI](/bowrain/cli/overview) for the project-level workflow.

## Putting it together

A complete Makefile / package-scripts setup for a multi-locale app:

```json title="package.json"
{
  "scripts": {
    "i18n:extract": "kapi-react extract --out i18n/extracted.klz",
    "i18n:pseudo":  "kapi pseudo-translate i18n/extracted.klz --target-lang qps -o i18n/pseudo.klz",
    "i18n:ai":      "for lang in fr de ja; do kapi ai-translate i18n/extracted.klz --target-lang $lang -o i18n/translated-$lang.klz; done",
    "i18n:compile": "kapi-react compile i18n/translated.klz --out public/translations"
  }
}
```

## Next

- [Configuration](./configuration) — componentMap, rules, Storybook, warnings.
