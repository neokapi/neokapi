---
sidebar_position: 7
title: Extract, Translate, Compile Pipeline
description: The three-phase kapi-react pipeline — extract JSX to a KLF archive, translate it with kapi (AI, MT, or TM), compile locales back into runtime JSON. Includes an optional split phase for code-split apps.
keywords: [extract, translate, compile, KLF, kapi-react pipeline, code splitting, localization pipeline]
---

import { PhaseFlow } from "@neokapi/docs-shared";

# The extract → translate → compile pipeline

Three phases, one contract: the KLF directory archive. A fourth optional phase — **split** — slices the compiled output along bundler chunk lines so code-split apps can lazy-load translations per route.

<PhaseFlow
  nodes={[
    { label: "Your source code" },
    {
      label: "i18n/",
      sub: "KLF archive",
      role: "io",
      edge: "kapi-react extract",
      loop: ["kapi ai-translate / pseudo-translate / qa / ai-review", "accumulate target locales in place"],
    },
    {
      label: "public/translations/{locale}.json",
      sub: "loaded at runtime by your app",
      edge: "kapi-react compile",
    },
    {
      label: "dist/translations/{locale}/{chunk}.json",
      sub: "lazy-loaded per route",
      edge: "kapi-react split (optional)",
    },
  ]}
/>

The same `i18n/` is the source-of-truth artifact through the whole round-trip. Translation tools read it, append the target locale they're producing, and write back to the same file — so you accumulate locales rather than juggling per-run output files. One file in the repo, one file to ship to translators, one file to compile.

Each phase has a single tool; none of them are coupled to the others. You can swap out the translator step for any process that preserves the KLF contract — human translators working in a CAT tool, AI translation, pre-existing TMS.

## Phase 1: extract

The extractor walks every `.jsx` / `.tsx` file in your project and produces translatable blocks. Two output modes:

- **Default** — per-file `.klf` under `--out` (default `i18n/`). Human-readable, git-diffable.
- **`--stream`** — NDJSON block records on stdout. File discovery happens via `--src` glob when stdin is a terminal; kapi's exec format can pipe NUL-separated paths to stdin for batch-controlled extraction.

```bash
# Default: write .klf files for inspection / commit.
vp kapi-react extract \
  --src "src/**/*.{tsx,jsx}" \
  --out i18n \
  --source-locale en \
  --target-locale fr \
  --target-locale de \
  --target-locale ja

# Or stream NDJSON blocks to stdout for piping:
vp kapi-react extract --stream > i18n/blocks.ndjson
```

Flags:

| Flag              | Default              | Purpose                                                     |
| ----------------- | -------------------- | ----------------------------------------------------------- |
| `--src`           | `src/**/*.{tsx,jsx}` | Glob of source files to scan.                               |
| `--out`           | `i18n`               | Output directory for `.klf` files.                          |
| `--stream`        | off                  | Emit NDJSON blocks on stdout instead of writing `.klf`.     |
| `--strict`        | off                  | Exit non-zero if any warning was recorded (CI enforcement). |
| `--config`        | —                    | Path to a JSON config file (componentMap, rules).           |
| `--project`       | `app`                | Project id stamped into `.klf.project`.                     |
| `--source-locale` | `en`                 | Source locale in file metadata.                             |
| `--target-locale` | —                    | Declared target locale (repeatable).                        |

The extractor also **prints warnings** for unmapped React components, so you know which ones to add to `componentMap` for hash stability:

```text
Scanning 186 files...
[neokapi] src/components/Settings.tsx:19: <TabsTrigger> is an unmapped component with translatable text — extracted. Add a componentMap entry: { TabsTrigger: 'button' }.
Extracted 1007 blocks from 186 files → i18n/
```

Wire it into your package scripts and CI:

```json title="package.json"
{
  "scripts": {
    "extract": "vp kapi-react extract",
    "extract:ci": "vp kapi-react extract --strict",
    "pack": "vp kapi-react extract --stream > i18n/blocks.ndjson"
  }
}
```

For full authoring-time coverage, pair this with [`@neokapi/kapi-react-lint`](./linting) — editor squigglies for `t(variable)`, `<img alt={'Logo ' + x} />`, and the other patterns the build-time transform can't catch.

### What's in the KLF directory

A directory of per-file `.klf` JSON documents, mirroring your source tree
(e.g. `src/App.tsx` → `i18n/src/App.klf`). Each `.klf` is a self-contained KLF
`File` carrying:

- `project` — id, source locale, declared target locales.
- `documents` — one document for the source file, holding its `Block`s.
- Optional targets / skeleton / annotation overlays (added by translators).

See [AD-008](/contribute/architecture/008-project-model) for the full schema.

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

The `.klf` is the translator's deliverable. Three common paths:

### Path A: AI translation

Run a full translation pass with `kapi ai-translate`:

```bash
kapi ai-translate i18n/ --target-lang fr
kapi ai-translate i18n/ --target-lang de
kapi ai-translate i18n/ --target-lang ja
```

Each run **accumulates** a target locale into the same `.klf`. The writer is locale-additive by design — existing targets stay put, the requested locale is added or updated in place. No `-o` needed unless you want to redirect output.

`kapi` supports Anthropic, OpenAI, Azure OpenAI, Google Gemini, and Ollama. It preserves placeholders, inline element tokens, and plural/select structure — AI providers that mangle them are automatically wrapped with recovery logic.

### Path B: Pseudo-translate

For UI-layout QA, pseudo-translation generates visibly-altered strings without any real translation:

```bash
kapi pseudo-translate i18n/
```

`Welcome` becomes `[Ŵéḷçőḿé]`, padded and accented. Missing translations stand out instantly, and strings that wrap too aggressively (or too narrowly) show up in layout testing.

### Path C: CAT tools / TMS / human translators

The `.klf` is the exchange format. A translator's workflow might be:

1. Open the `i18n/` archive (or the individual `.klf` files) in their CAT tool.
2. Translate every block, leveraging their existing TM.
3. Save back to the same `i18n/`.

Structural context (the `jsxPath`, the translator note, the inline element tokens) renders as rich context in modern CAT tools.

### In-place default vs. explicit redirect

`kapi` tool commands default to in-place for KLF inputs — `kapi pseudo-translate i18n/` reads and writes the same files, since the KLF writer is locale-additive (it adds or updates the requested locale, leaving the others intact). Pass `-o other-dir/` to redirect without touching the originals.

Non-KLF formats (JSON, XLIFF, …) aren't locale-additive, so they write a new file in a locale-aware location: if the input path carries the source locale it is swapped for the target (`src/locales/en/app.json → src/locales/fr/app.json`), otherwise the output lands under a `{lang}/` directory beside the input (`messages.json → fr/messages.json`). Use `-o` for an explicit path or template, or `--output-dir DIR` to root outputs at `DIR/{lang}/`.

### Multiple locales in one `i18n/`

A single `i18n/` tree with N target locales on each block is the default and recommended layout — simpler to version, all translations stay together. See [AD-008](/contribute/architecture/008-project-model) for the project model.

### Project-driven flow with `.kapi`

If you already use a [`.kapi` project file](/contribute/architecture/008-project-model) to define your workflow, declare each archive-backed collection with an `exec` format pointing at kapi-react (or any other extractor):

```yaml title="translation.kapi"
version: v1
name: MyApp
defaults:
  source_language: en
  target_languages: [fr, de, ja]
content:
  - name: ui
    # Block state lives in the project cache (gitignored, regenerable).
    items:
      - path: "src/**/*.tsx"
        format:
          name: exec
          config:
            command: "vp kapi-react extract --stream"
```

```bash
# 1. Extract — kapi runs the declared command for each collection,
#    streams NDJSON blocks into the collection's block store.
kapi extract -p translation.kapi

# 2. Translate — run a composed flow over the project for each target language.
kapi run ai-translate-qa -p translation.kapi
```

The `command` string picks the package manager — `vp`, `pnpm`, `npm`, `yarn`, or a direct binary path — so the project declares its preferences explicitly without kapi making assumptions. `kapi run` then executes the named [flow](/framework/flows) against the project's extracted blocks for each target language.

### Standalone pipe (no `.kapi`)

For ad-hoc projects, skip `.kapi` entirely and compose with Unix pipes:

```bash
vp kapi-react extract --stream > i18n/blocks.ndjson
kapi pseudo-translate i18n/
vp kapi-react compile i18n/ --out public/translations
```

Same underlying wire format (NDJSON on the extract stage, KLF from there on) — the declarative `.kapi` form just factors the pipe into the project file.

## Phase 3: compile

`kapi-react compile` reads the translated `.klf` and emits one JSON dict per locale:

```bash
kapi-react compile i18n/ \
  --out public/translations
```

Output:

```
Compiled 1007 entries → public/translations/fr.json
Compiled 1007 entries → public/translations/de.json
Compiled 1007 entries → public/translations/ja.json
```

Each JSON file is a flat `{hash: renderedText}` map. The runtime `__t(hash, fallback, params)` looks up the hash; the renderer picks the plural / select form.

## Phase 4: split (optional)

For code-split apps, the compiled `{locale}.json` is one file per locale — the user downloads every string even for routes they never visit. The plugin + `kapi-react split` divide that catalog along bundler chunk boundaries so each chunk lands its own translation subset alongside its JS.

Two inputs:

- **`translations-manifest.json`** — emitted by the Vite/Rollup plugin's `generateBundle` hook when `mode: "runtime"`. Maps each output chunk to the set of hashes its modules reference.
- **`public/translations/{locale}.json`** — the compiled master dict from Phase 3.

```bash
vite build                                       # emits dist/translations-manifest.json
kapi-react compile i18n/ --out public/translations
kapi-react split \
  --manifest dist/translations-manifest.json \
  --locales  public/translations \
  --out      dist/translations
```

Output:

```
dist/translations/
├── manifest.json                   ← copy of the chunk → hashes map
└── {locale}/
    ├── index.json                  ← hashes used by the main chunk
    ├── SettingsPage.json
    └── FlowEditor.json
```

Hashes shared across chunks are duplicated into each subset so every chunk file is independently loadable. Runtime wiring is a one-line addition to each lazy route:

```tsx
import { loadTranslationChunk } from "@neokapi/kapi-react/runtime";

const routes = [
  {
    path: "/settings",
    lazy: async () => {
      const [mod] = await Promise.all([
        import("./SettingsPage"),
        loadTranslationChunk(locale, `/translations/${locale}/SettingsPage.json`),
      ]);
      return { Component: mod.default };
    },
  },
];
```

`loadTranslationChunk` merges the subset into the active dict; concurrent calls for the same `(locale, url)` share a single fetch. Missing hashes fall back to the source text baked into each `__t` / `__tx` call at build time — a late-arriving chunk never breaks render. See [Runtime mode → Lazy loading per route](./modes#lazy-loading-per-route-code-splitting) for the full runtime contract.

Apps that ship a single bundle don't need this phase at all — keep using `loadTranslations(locale, url)` against the compiled master dict.

## Round-trip in one diagram

```
src/App.tsx        <h1>Welcome</h1>
                          │
                          │ extract (source only)
                          ▼
i18n/      Block { hash: "aB3", source: [{text: "Welcome"}] }
                          │
                          │ kapi ai-translate --target-lang fr
                          ▼
i18n/      Block { hash: "aB3", source: […], targets: { fr: […] } }
                          │
                          │ kapi ai-translate --target-lang de (additive)
                          ▼
i18n/      Block { hash: "aB3", source: […], targets: { fr, de } }
                          │
                          │ compile
                          ▼
public/translations/
  fr.json          { "aB3": "Bienvenue" }
  de.json          { "aB3": "Willkommen" }
                          │
                          ├─── loadTranslations("fr", "/translations/fr.json")
                          │     (single bundle — one fetch, all strings)
                          │
                          │  kapi-react split  (optional)
                          ▼
dist/translations/fr/
  index.json       { "aB3": "Bienvenue" }        ← main chunk
  Settings.json    { …subset used by Settings }  ← lazy chunk
                          │
                          │  loadTranslationChunk("fr", "/translations/fr/Settings.json")
                          │  (fires when React.lazy() resolves the Settings route)
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
    git diff --exit-code i18n/ || {
      echo "::error::i18n/ drifted. Re-extract locally and commit."
      exit 1
    }
```

For apps with a translation backend, you'd instead push the archive to that backend and wait for translated deliverables — but the principle is the same: extract on every change, don't let translations drift from source.

## Incremental extracts

The extractor is stateless — it always produces the same `.klf` for the same source + config. For an incremental pipeline (only translate what changed), diff two archives on the translation side. Each block's hash tells you whether its source shifted.

## Next

- [Runtime vs. inline modes](./modes) — shipping one bundle with OTA dicts vs. one bundle per locale.
- [Translating with kapi](./translating-with-kapi) — pseudo-translation, AI translation, QA.
- [Configuration](./configuration) — componentMap, rules, Storybook, custom warning handlers.
