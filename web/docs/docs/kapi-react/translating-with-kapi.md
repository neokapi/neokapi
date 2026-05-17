---
sidebar_position: 9
title: Translating with kapi
---

# Translating with `kapi`

`kapi-react` produces a KLF directory archive. The `kapi` CLI translates it. This page walks through the three most useful flows.

## Pseudo-translation for UI QA

Pseudo-translation is the fastest way to see what's been picked up for translation — and what hasn't.

```bash
kapi pseudo-translate i18n/ --target-lang qps
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
- run: kapi pseudo-translate i18n/ --target-lang qps
- run: vp kapi-react compile i18n/ --out public/translations
- run: npm run test:e2e # runs against ?locale=qps
```

## AI translation

For actual translations, `kapi ai-translate` feeds the KLF directory through an LLM. It preserves placeholders, inline element tokens, and plural / select structure:

```bash
kapi ai-translate i18n/ --target-lang fr
kapi ai-translate i18n/ --target-lang de
kapi ai-translate i18n/ --target-lang ja
```

Each call accumulates a target locale in place. To redirect output to a different file, pass `-o target-dir/` — the input stays untouched.

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

See [AI translation](/features/ai-translation) for the full configuration surface.

### Context carries through

Every block in the KLF directory carries its `jsxPath` (e.g. `"div > button"`), its component name, and any `data-i18n-note` annotation. The AI translator gets that context as part of the prompt — so a `<Button>Close</Button>` in a dialog gets a different translation than a `<a>Close</a>` in a list-item's delete action.

### Translate a subset

For incremental translations (only the strings that changed), extract and diff against the existing archive:

```bash
vp kapi-react extract --out i18n-new/
kapi diff i18n-new/ i18n/ --only-new -o i18n-delta/
kapi ai-translate i18n-delta/ --target-lang fr -o i18n-delta-fr/
kapi merge i18n/ i18n-delta-fr/ -o i18n/
```

## Quality assurance

`kapi qa` runs terminology, placeholder, and consistency checks against a translated `.klf`:

```bash
kapi qa i18n/ \
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

Then when you translate a new `.klf`:

```bash
kapi ai-translate i18n/ --target-lang fr --tm-leverage
```

Segments that match (or fuzzy-match) past translations come through pre-populated — the AI only translates what's new.

See [Translation memory](/features/translation-memory) for more.

## Terminology pre-translation

For apps with a large product vocabulary, pre-translate terminology before the AI pass so terms are consistently rendered:

```bash
kapi tools terminology-pretranslation i18n/ \
  --termbase product-terms.csv \
  --target-lang fr \
  -o i18n-pre/
```

Then pass `i18n-pre/` to `kapi ai-translate`, which respects the pre-translated segments.

See [Terminology](/features/terminology).

klf` directly:

```bash
# …translators work in the web editor…
```

## Putting it together

A complete Makefile / package-scripts setup for a multi-locale app:

```json title="package.json"
{
  "scripts": {
    "i18n:extract": "vp kapi-react extract",
    "i18n:pseudo": "kapi pseudo-translate i18n/ --target-lang qps",
    "i18n:ai": "for lang in fr de ja; do kapi ai-translate i18n/ --target-lang $lang; done",
    "i18n:compile": "vp kapi-react compile i18n/ --out public/translations"
  }
}
```

## Project-driven alternative

If you have a [`.kapi` project file](/architecture/008-project-model) with the collection's `archive:` pointing at `i18n/`, the scripts collapse to one sync call:

```json title="package.json"
{
  "scripts": {
    "i18n:extract": "kapi extract -p translation.kapi",
    "i18n:sync": "kapi sync -p translation.kapi --tool ai-translate",
    "i18n:compile": "vp kapi-react compile i18n/ --out public/translations"
  }
}
```

`kapi sync` iterates every (archive, missing-locale) pair from the project's declared target languages and runs the given tool. `kapi status -p translation.kapi` gives a quick coverage check without running anything.

## Next

- [Configuration](./configuration) — componentMap, rules, Storybook, warnings.
