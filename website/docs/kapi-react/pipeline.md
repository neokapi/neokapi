---
sidebar_position: 6
title: Extract → translate → compile
---

# The extract → translate → compile pipeline

Three phases, one contract: the `.klz` archive.

```
Your source code
      │
      │  kapi-react extract
      ▼
  extracted.klz  ──────────────►  (send to translators / AI / TMS)
                                         │
                                         │  translators, kapi ai-translate,
                                         │  kapi pseudo-translate, etc.
                                         ▼
                                   translated.klz
      ┌──────────────────────────────────┘
      │
      │  kapi-react compile
      ▼
public/translations/{locale}.json  ────►  loaded at runtime by your app
```

Each phase has a single tool; none of them are coupled to the others. You can swap out the translator step for any process that preserves the KLZ contract — human translators working in a CAT tool, AI translation, pre-existing TMS.

## Phase 1: extract

The extractor walks every `.jsx` / `.tsx` file in your project, emits one `Document` per file, and bundles them all into a single `.klz` archive.

```bash
kapi-react extract \
  --src "src/**/*.{tsx,jsx}" \
  --out i18n/extracted.klz \
  --source-locale en \
  --target-locale fr \
  --target-locale de \
  --target-locale ja
```

Flags:

| Flag | Default | Purpose |
|---|---|---|
| `--src` | `src/**/*.{tsx,jsx}` | Glob of source files to scan. |
| `--out` | `i18n/extracted.klz` | Output archive path. |
| `--config` | — | Path to a JSON config file (componentMap, rules). |
| `--project` | `app` | Project id stamped into the archive's manifest. |
| `--source-locale` | `en` | Source locale in the archive metadata. |
| `--target-locale` | — | Declared target locale (repeatable). |

The extractor also **prints warnings** for auto-promoted containers and unmapped components so you can decide which to add to `componentMap`:

```text
Scanning 186 files...
[neokapi] src/components/AppHome.tsx:61: <div> contains translatable text — extracted. …
[neokapi] src/components/Settings.tsx:19: <TabsTrigger> is an unmapped component with translatable text — extracted. Add a componentMap entry: { TabsTrigger: 'button' }.
Extracted 1007 blocks from 186 files → i18n/extracted.klz
```

Wire it into your package scripts and CI:

```json title="package.json"
{
  "scripts": {
    "extract": "kapi-react extract",
    "check:i18n": "kapi-react extract --out /tmp/check.klz && diff -q i18n/extracted.klz /tmp/check.klz"
  }
}
```

### What's in the `.klz`

A ZIP archive with:

- `manifest.json` — project, source/target locales, SHA-256 of each part.
- `documents/<slug>.klf` — one file per source file, each carrying its `Block`s.
- Optional targets / skeleton / annotation sidecars (added by translators).

See [AD-045](/docs/ad/045-klf-klz-spec) for the full schema.

### One block per

- Translatable JSX element (`<h1>`, `<p>`, `<button>`, auto-promoted `<div>`, unmapped components).
- Translatable attribute (`title`, `placeholder`, `alt`, `aria-label`, …).
- User-facing `t(...)` call.
- `<Plural>` / `<Select>` construct.

Each block carries:

- `hash` — stable id computed from source text + structural context.
- `source` — typed runs (text, placeholders, inline element tokens, plural/select wrappers).
- `placeholders` — metadata about each `{name}` / `{=mN}` in the source.
- `properties` — file + line + component name + `jsxPath` + optional translator note.

## Phase 2: translate

The `.klz` is the translator's deliverable. Three common paths:

### Path A: AI translation

Run a full translation pass with `kapi ai-translate`:

```bash
kapi ai-translate i18n/extracted.klz \
  --source-lang en \
  --target-lang fr \
  -o i18n/translated.klz
```

`kapi` supports Anthropic, OpenAI, Azure OpenAI, Google Gemini, and Ollama. It preserves placeholders, inline element tokens, and plural/select structure — AI providers that mangle them are automatically wrapped with recovery logic.

### Path B: Pseudo-translate

For UI-layout QA, pseudo-translation generates visibly-altered strings without any real translation:

```bash
kapi pseudo-translate i18n/extracted.klz \
  --target-lang qps \
  -o i18n/translated.klz
```

`Welcome` becomes `[Ŵéḷçőḿé]`, padded and accented. Missing translations stand out instantly, and strings that wrap too aggressively (or too narrowly) show up in layout testing.

### Path C: CAT tools / TMS / human translators

The `.klz` is the exchange format. A translator's workflow might be:

1. Open the `.klz` in a CAT tool (Phrase, Smartcat, Trados, Bowrain's web editor, …).
2. Translate every block, leveraging their existing TM.
3. Save back to `translated.klz`.

Structural context (the `jsxPath`, the translator note, the inline element tokens) renders as rich context in modern CAT tools.

For apps built on [Bowrain](/bowrain/introduction), this is transparent — the dashboard ingests `extracted.klz`, shows translators a React-shaped view of each block, and writes `translated.klz` back when they're done.

## Phase 3: compile

`kapi-react compile` reads the translated `.klz` and emits one JSON dict per locale:

```bash
kapi-react compile i18n/translated.klz \
  --out public/translations
```

Output:

```
Compiled 1007 entries → public/translations/fr.json
Compiled 1007 entries → public/translations/de.json
Compiled 1007 entries → public/translations/ja.json
```

Each JSON file is a flat `{hash: renderedText}` map. The runtime `__t(hash, fallback, params)` looks up the hash; the renderer picks the plural / select form.

## Round-trip in one diagram

```
src/App.tsx      <h1>Welcome</h1>
                        │
                        │ extract
                        ▼
extracted.klz    Block { hash: "aB3", source: [{text: "Welcome"}], … }
                        │
                        │ translate (human / AI / pseudo)
                        ▼
translated.klz   Block { hash: "aB3", source: […], targets: { fr: […] } }
                        │
                        │ compile
                        ▼
fr.json          { "aB3": "Bienvenue" }
                        │
                        │ loadTranslations("fr", "/translations/fr.json")
                        ▼
Your app         <h1>Welcome</h1>  renders as  "Bienvenue"
```

## CI: re-extract every build, fail on drift

The extract is deterministic, so CI can use the archive hash as a contract:

```yaml title=".github/workflows/ci.yml"
- name: Extract translatable content
  run: npm run extract

- name: Fail if translators need to re-open the file
  run: |
    git diff --exit-code i18n/extracted.klz || {
      echo "::error::i18n/extracted.klz drifted. Re-extract locally and commit."
      exit 1
    }
```

For apps with a translation backend, you'd instead push the archive to that backend and wait for translated deliverables — but the principle is the same: extract on every change, don't let translations drift from source.

## Incremental extracts

The extractor is stateless — it always produces the same `.klz` for the same source + config. For an incremental pipeline (only translate what changed), diff two archives on the translation side. Each block's hash tells you whether its source shifted.

## Next

- [Runtime vs. inline modes](./modes) — shipping one bundle with OTA dicts vs. one bundle per locale.
- [Translating with kapi](./translating-with-kapi) — pseudo-translation, AI translation, QA.
- [Configuration](./configuration) — componentMap, rules, Storybook, custom warning handlers.
