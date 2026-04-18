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
   myproject.klz ─── (send to translators / AI / TMS) ───┐
      ▲                                                  │
      │                                                  ▼
      │  kapi ai-translate / pseudo-translate / qa / ai-review
      │  accumulate target locales in place
      │                                                  │
      └──────────────────────────────────────────────────┘
      │
      │  kapi-react compile
      ▼
public/translations/{locale}.json  ────►  loaded at runtime by your app
```

The same `myproject.klz` is the source-of-truth artifact through the whole round-trip. Translation tools read it, append the target locale they're producing, and write back to the same file — so you accumulate locales rather than juggling per-run output files. One file in the repo, one file to ship to translators, one file to compile.

Each phase has a single tool; none of them are coupled to the others. You can swap out the translator step for any process that preserves the KLZ contract — human translators working in a CAT tool, AI translation, pre-existing TMS.

## Phase 1: extract

The extractor walks every `.jsx` / `.tsx` file in your project and produces translatable blocks. Two output modes:

- **Default** — per-file `.klf` under `--out` (default `i18n/`). Human-readable, git-diffable.
- **`--stream`** — NDJSON block records on stdout, reads NUL-separated paths on stdin. For pipes.

```bash
# Default: write .klf files for inspection / commit.
vp kapi-react extract \
  --src "src/**/*.{tsx,jsx}" \
  --out i18n \
  --source-locale en \
  --target-locale fr \
  --target-locale de \
  --target-locale ja

# Or pipe straight into a .klz:
vp kapi-react extract --stream | kapi pack --out i18n/myproject.klz
```

Flags:

| Flag | Default | Purpose |
|---|---|---|
| `--src` | `src/**/*.{tsx,jsx}` | Glob of source files to scan. |
| `--out` | `i18n` | Output directory for `.klf` files. |
| `--stream` | off | Emit NDJSON blocks on stdout instead of writing `.klf`. |
| `--config` | — | Path to a JSON config file (componentMap, rules). |
| `--project` | `app` | Project id stamped into `.klf.project`. |
| `--source-locale` | `en` | Source locale in file metadata. |
| `--target-locale` | — | Declared target locale (repeatable). |

The extractor also **prints warnings** for auto-promoted containers and unmapped components so you can decide which to add to `componentMap`:

```text
Scanning 186 files...
[neokapi] src/components/AppHome.tsx:61: <div> contains translatable text — extracted. …
[neokapi] src/components/Settings.tsx:19: <TabsTrigger> is an unmapped component with translatable text — extracted. Add a componentMap entry: { TabsTrigger: 'button' }.
Extracted 1007 blocks from 186 files → i18n/
```

Wire it into your package scripts and CI:

```json title="package.json"
{
  "scripts": {
    "extract": "vp kapi-react extract",
    "pack":    "vp kapi-react extract --stream | kapi pack --out i18n/myproject.klz"
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
kapi ai-translate i18n/myproject.klz --target-lang fr
kapi ai-translate i18n/myproject.klz --target-lang de
kapi ai-translate i18n/myproject.klz --target-lang ja
```

Each run **accumulates** a target locale into the same `.klz`. The writer is locale-additive by design — existing targets stay put, the requested locale is added or updated in place. No `-o` needed unless you want to redirect output.

`kapi` supports Anthropic, OpenAI, Azure OpenAI, Google Gemini, and Ollama. It preserves placeholders, inline element tokens, and plural/select structure — AI providers that mangle them are automatically wrapped with recovery logic.

### Path B: Pseudo-translate

For UI-layout QA, pseudo-translation generates visibly-altered strings without any real translation:

```bash
kapi pseudo-translate i18n/myproject.klz --target-lang qps
```

`Welcome` becomes `[Ŵéḷçőḿé]`, padded and accented. Missing translations stand out instantly, and strings that wrap too aggressively (or too narrowly) show up in layout testing.

### Path C: CAT tools / TMS / human translators

The `.klz` is the exchange format. A translator's workflow might be:

1. Open the `.klz` in a CAT tool (Phrase, Smartcat, Trados, Bowrain's web editor, …).
2. Translate every block, leveraging their existing TM.
3. Save back to the same `myproject.klz`.

Structural context (the `jsxPath`, the translator note, the inline element tokens) renders as rich context in modern CAT tools.

For apps built on [Bowrain](/bowrain/introduction), this is transparent — the dashboard ingests `myproject.klz`, shows translators a React-shaped view of each block, and writes the updated archive back when they're done.

### In-place default vs. explicit redirect

`kapi` tool commands default to in-place for KLZ inputs — `kapi pseudo-translate proj.klz --target-lang qps` reads and writes the same file. Pass `-o other.klz` to redirect without touching the original. Non-KLZ formats (JSON, XLIFF, …) still default to `./out/{name}.{ext}` since they're not locale-additive.

### Multilingual vs. per-locale files

A single `myproject.klz` with N target locales is the default and recommended layout — simpler to version, one file to ship, all translations stay together. Per-locale files (`myproject.fr.klz`, `myproject.de.klz`) are supported when you want parallel translator workflows without merge conflicts; use them sparingly. See [AD-045](/docs/ad/045-klf-klz-spec#file-naming-conventions) for the full convention.

### Project-driven flow with `.kapi`

If you already use a [`.kapi` project file](/docs/ad/041-kapi-desktop) to define your workflow, declare each archive-backed collection with an `exec` format pointing at kapi-react (or any other extractor):

```yaml title="translation.kapi"
version: v1
name: MyApp
defaults:
  source_language: en
  target_languages: [fr, de, ja]
content:
  - name: ui
    archive: i18n/myapp.klz
    items:
      - path: "src/**/*.tsx"
        format:
          name: exec
          config:
            command: "vp kapi-react extract --stream"
```

```bash
# 1. Extract — kapi runs the declared command for each collection,
#    streams NDJSON blocks into the collection's .klz.
kapi extract -p translation.kapi

# 2. Status — read-only coverage report.
kapi status -p translation.kapi

# 3. Sync — top up every (archive, missing-locale) pair.
kapi sync -p translation.kapi --tool ai-translate
```

The `command` string picks the package manager — `vp`, `pnpm`, `npm`, `yarn`, or a direct binary path — so the project declares its preferences explicitly without kapi making assumptions. `kapi sync` then runs the named translation tool against each archive for each incomplete locale.

### Standalone pipe (no `.kapi`)

For ad-hoc projects, skip `.kapi` entirely and compose with Unix pipes:

```bash
vp kapi-react extract --stream | kapi pack --out i18n/ui.klz
kapi pseudo-translate i18n/ui.klz --target-lang qps
vp kapi-react compile i18n/ui.klz --out public/translations
```

Same underlying wire format (NDJSON on the extract stage, KLZ from there on) — the declarative `.kapi` form just factors the pipe into the project file.

## Phase 3: compile

`kapi-react compile` reads the translated `.klz` and emits one JSON dict per locale:

```bash
kapi-react compile i18n/myproject.klz \
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
src/App.tsx        <h1>Welcome</h1>
                          │
                          │ extract (source only)
                          ▼
myproject.klz      Block { hash: "aB3", source: [{text: "Welcome"}] }
                          │
                          │ kapi ai-translate --target-lang fr
                          ▼
myproject.klz      Block { hash: "aB3", source: […], targets: { fr: […] } }
                          │
                          │ kapi ai-translate --target-lang de (additive)
                          ▼
myproject.klz      Block { hash: "aB3", source: […], targets: { fr, de } }
                          │
                          │ compile
                          ▼
public/translations/
  fr.json          { "aB3": "Bienvenue" }
  de.json          { "aB3": "Willkommen" }
                          │
                          │ loadTranslations("fr", "/translations/fr.json")
                          ▼
Your app           <h1>Welcome</h1>  renders as  "Bienvenue"
```

## CI: re-extract every build, fail on drift

The extract is deterministic, so CI can use the archive hash as a contract:

```yaml title=".github/workflows/ci.yml"
- name: Extract translatable content
  run: npm run extract

- name: Fail if translators need to re-open the file
  run: |
    git diff --exit-code i18n/myproject.klz || {
      echo "::error::i18n/myproject.klz drifted. Re-extract locally and commit."
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
