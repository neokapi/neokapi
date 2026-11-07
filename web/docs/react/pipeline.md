---
sidebar_position: 7
title: Extract, Translate, Compile Pipeline
description: "The three-phase neokapi-i18n pipeline: extract JSX to a KBF archive, translate it with kapi (AI or content memory), compile locales back into runtime JSON. Includes an optional split phase for code-split apps."
keywords: [extract, translate, compile, KBF, neokapi-i18n pipeline, code splitting, translation pipeline]
---

import { PhaseFlow } from "@neokapi/docs-shared";

# The extract → translate → compile pipeline

Your JSX becomes a catalog of blocks, the catalog collects a translation for each target language, and the translations come back as one JSON file per locale that your app loads. One file format, the KBF directory archive, carries the content through every phase.

An optional fourth phase, **split**, cuts the compiled output along your bundler's chunks, so a code-split app loads the translations for a route together with its code.

<PhaseFlow
  nodes={[
    { label: "Your source code" },
    {
      label: "i18n/",
      sub: "KBF archive",
      role: "io",
      edge: "neokapi-i18n extract",
      loop: ["kapi translate / pseudo-translate / check", "accumulate target locales in place"],
    },
    {
      label: "public/translations/{locale}.json",
      sub: "loaded at runtime by your app",
      edge: "neokapi-i18n compile",
    },
    {
      label: "dist/translations/{locale}/{chunk}.json",
      sub: "lazy-loaded per route",
      edge: "neokapi-i18n split (optional)",
    },
  ]}
/>

## One archive through the whole round trip

The `i18n/` directory is the source of truth from extraction to compile. A translation tool reads it, adds the target locale it produces, and writes the result back to the same files. Locales accumulate in one place, so you have one archive to commit, one to hand to translators and one to compile.

Each phase is a separate command, and none depends on how another one ran. Any process that reads and writes KBF can take the translation step:

- human translators working in a CAT tool
- AI translation with `kapi`
- an existing translation management system

## Phase 0: explain (optional)

Before you extract, you can ask the extractor what it would do with a file, and why:

```bash
vp neokapi-i18n explain src/components/Settings.tsx
```

```text
L3    <div>          [container] skipped: has block-level children (they extract separately)
L4    <h1>           [translatable] extracted  hash=cYEMc2v3JVx
L6    <code>         [non-translatable] skipped: classified non-translatable
L7    <input>        [container] skipped: no translator-editable text
        ↳ placeholder [attribute] extracted  hash=i42kuGUFbb4
```

Each line gives the element's W3C ITS classification, the rule that decided whether it extracts, and the hash it received. Use it to audit zero-config extraction. Add `--extracted` to list only the elements that reached the catalog.

## Phase 1: extract

The extractor walks every `.jsx` and `.tsx` file in your project and produces translatable blocks. It parses plain `.ts`, `.mts` and `.cts` modules as TypeScript too, and extracts their `t()` calls when the glob includes them (`--src "src/**/*.{ts,tsx,jsx}"`). A file that fails to parse produces a warning that names it.

There are two output modes:

- **Default**: one `.kbf.json` per source file under `--out` (default `i18n/`). You can read and diff these files.
- **`--stream`**: NDJSON block records on stdout, for a consumer you write yourself. When stdin is a terminal, `--src` finds the files; otherwise a caller can pipe NUL-separated paths to stdin. kapi reads the `.kbf.json` files and ignores the stream.

```bash
# Default: write .kbf.json files for inspection / commit.
vp neokapi-i18n extract \
  --src "src/**/*.{tsx,jsx}" \
  --out i18n \
  --source-locale en \
  --target-locale fr \
  --target-locale de \
  --target-locale ja

# Or stream NDJSON blocks to stdout for piping:
vp neokapi-i18n extract --stream > i18n/blocks.ndjson
```

### Extract flags

| Flag              | Default              | Purpose                                                     |
| ----------------- | -------------------- | ----------------------------------------------------------- |
| `--src`           | `src/**/*.{tsx,jsx}` | Glob of source files to scan (repeatable; add `.ts` to extract `t()` calls from plain modules). |
| `--out`           | `i18n`               | Output directory for `.kbf.json` files.                          |
| `--stream`        | off                  | Emit NDJSON blocks on stdout instead of writing `.kbf.json`.     |
| `--ignore`        | (none)               | Glob to exclude (repeatable): fixtures, stories, tests.     |
| `--strict`        | off                  | Exit non-zero if any warning was recorded (CI enforcement). |
| `--config`        | (none)               | Path to a JSON config file (componentMap, rules).           |
| `--project`       | `app`                | Project id stamped into the file's `project` field.         |
| `--source-locale` | `en`                 | Source locale in file metadata.                             |
| `--target-locale` | (none)               | Declared target locale (repeatable).                        |

### Warnings and scripts

The extractor prints a warning for each unmapped React component, with the `componentMap` entry that keeps its hash stable:

```text
Scanning 186 files...
[neokapi] src/components/Settings.tsx:19: <TabsTrigger> is an unmapped component with translatable text: extracted. Add a componentMap entry: { TabsTrigger: 'button' }.
Extracted 1007 blocks from 186 files → i18n/
```

Add the commands to your package scripts and CI:

```json title="package.json"
{
  "scripts": {
    "extract": "vp neokapi-i18n extract",
    "extract:ci": "vp neokapi-i18n extract --strict",
    "pack": "vp neokapi-i18n extract --stream > i18n/blocks.ndjson"
  }
}
```

For coverage while you write code, add [`@neokapi/i18n-react-lint`](./linting). It marks `t(variable)`, `<img alt={'Logo ' + x} />` and the other patterns the build-time transform can't catch in your editor.

### What's in the KBF directory

The directory holds one `.kbf.json` document per source file and mirrors your source tree, so `src/App.tsx` becomes `i18n/src/App.kbf.json`. Every run keeps the mirror current. It removes the catalog of a source file you deleted, renamed or excluded from `--src`, and leaves the per-locale targets under `i18n/<lang>/` in place.

Each catalog is a self-contained KBF `File` with:

- `project`: id, source locale, declared target locales.
- `documents`: one document for the source file, holding its `Block`s.
- Optional targets / skeleton / annotation overlays (added by translators).

See [C-01](/contribute/architecture/context/c-01-project-model) for the full schema.

### What becomes a block

The extractor writes one block for each:

- Translatable JSX element (`<h1>`, `<p>`, `<button>`, auto-promoted `<div>`, unmapped components).
- Translatable attribute (`title`, `placeholder`, `alt`, `aria-label`, …).
- User-facing `t(...)` call.
- `<Plural>` / `<Select>` construct.

Each block carries:

- `hash`: stable id computed from the source text + the element's own tag.
- `source`: typed runs (text, placeholders, inline element tokens, plural/select wrappers).
- `placeholders`: metadata about each `{name}` / `{=mN}` in the source.
- `properties`: file + line + component name + `element` (the resolved tag) + optional translator note.

## Phase 2: translate

The `.kbf.json` files are what a translator works on. There are three common ways to fill them.

### Path A: AI translation

Run a full translation pass with `kapi translate`:

```bash
kapi translate i18n/ --target-lang fr
kapi translate i18n/ --target-lang de
kapi translate i18n/ --target-lang ja
```

Each run adds one target locale to the same `.kbf.json` files. The KBF writer is locale-additive: it adds or updates the requested locale and keeps the others. You need `-o` only to send the output somewhere else.

`kapi` works with the AI providers listed under [Translation](/framework/translation). It keeps placeholders, inline element tokens and plural/select structure intact, and wraps a provider that damages them with recovery logic.

### Path B: Pseudo-translate

To check layout, pseudo-translation produces visibly altered strings with no real translation:

```bash
kapi pseudo-translate i18n/
```

`Welcome` becomes `▒ Ŵéḷçőḿé ▒`, padded and accented. A string that is still English stands out at once, and layout testing shows the strings that wrap too early or too late.

### Path C: CAT tools, a TMS or human translators

The `.kbf.json` file is the exchange format. A translator's workflow might be:

1. Open the `i18n/` archive (or the individual `.kbf.json` files) in their CAT tool.
2. Translate every block, with matches from their existing content memory.
3. Save back to the same `i18n/`.

A modern CAT tool shows the context each block carries: its file and line, its element, the translator note and the inline element tokens.

### In place, or redirected

For KBF inputs, `kapi` tool commands write in place: `kapi pseudo-translate i18n/` reads and writes the same files, because the KBF writer adds or updates the requested locale and keeps the others. Pass `-o other-dir/` to write elsewhere and leave the originals untouched.

Other formats (JSON, XLIFF, …) hold one locale per file, so kapi writes a new file in a locale-aware location:

- If the input path contains the source locale, kapi swaps it for the target: `src/locales/en/app.json → src/locales/fr/app.json`.
- Otherwise the output goes to a `{lang}/` directory beside the input: `messages.json → fr/messages.json`.
- `-o` takes an explicit path or template, and `--output-dir DIR` roots the outputs at `DIR/{lang}/`.

A run with no target language, such as `kapi exec redact`, rewrites each input in place, and `--output-dir DIR` writes the outputs under `DIR/`.

### Layout: one tree, or a subdirectory per locale

There are two layouts:

- **Locale-additive**: one `i18n/` tree where each block carries every target locale, filled in place. This is what `kapi translate i18n/ --target-lang …` produces. It is the simplest to version, because all translations stay together.
- **Recipe-driven per-locale files**: the source catalogs live under `i18n/src/`, and kapi writes a separate file per locale under `i18n/{lang}/`. A `kapi.yaml` content entry maps one to the other (`path: i18n/src/**/*.kbf.json` → `target: i18n/{lang}/{path}.kbf.json`). `kapi init --framework neokapi-i18n` scaffolds this layout.

Both keep everything under one `i18n/` directory. Because the source lives under `i18n/src/`, the source glob never matches the generated `i18n/{lang}/` targets, and you need no sibling `i18n-<lang>/` trees. See [C-01](/contribute/architecture/context/c-01-project-model) for the project model and [Drive it from a project](./translating-with-kapi#drive-it-from-a-project) for the recipe.

### Project-driven flow with `kapi.yaml`

For an app you translate every release, a [`kapi.yaml` recipe](/contribute/architecture/context/c-01-project-model) records the languages, the flow and the gates once, so you stop passing flags on every call. `kapi init --framework neokapi-i18n` writes one. Its collection reads the bundler's catalogs under `i18n/src/` and maps each to a per-locale target under `i18n/{lang}/`:

```yaml title="kapi.yaml"
version: v1
name: MyApp
defaults:
  source_language: en
  target_languages: [fr, de, ja]
collections:
  - path: "i18n/src/**/*.kbf.json"
    format: kbf
    target: "i18n/{lang}/{path}.kbf.json"
```

The bundler plugin still extracts, and kapi reads what it wrote:

```bash
# 1. Extract: the bundler plugin writes one catalog per source file under i18n/src/.
vp neokapi-i18n extract

# 2. Converge: run the project's flow over every target language in the recipe,
#    looping until each locale clears its ship gate or waits for review.
kapi up

# 3. Materialize the per-locale catalogs the recipe maps, then compile them.
kapi merge
vp neokapi-i18n compile i18n/ --out public/translations
```

### What `kapi up` does

- It re-reads the collection before each pass, so it picks up a changed catalog without a separate import step.
- A recipe with no flow of its own runs the built-in default: content memory reuse, then AI translation, then the project's bound checks.
- It reports a locale that is behind as pending. A target language that lags the source never makes it fail.
- With `defaults.materialize: on-converge` in the recipe, or `kapi up --materialize`, it writes the target files itself for the locales whose gates are met, and you can drop the `kapi merge` step.

`kapi status` shows the coverage per locale without running anything. `kapi check --ship` enforces the gates, for example before a release tag. `kapi run <flow>` runs one named [flow](/framework/flows) for a single pass over the same recipe.

### Ad-hoc commands (no `kapi.yaml`)

For a quick run, skip the recipe and point the commands at the archive:

```bash
vp neokapi-i18n extract
kapi pseudo-translate i18n/
vp neokapi-i18n compile i18n/ --out public/translations
```

Each command reads and writes the same `.kbf.json` files. The recipe records the languages, the flow and the gates, so that `kapi up` can repeat the run.

## Phase 3: compile

`neokapi-i18n compile` reads the translated `.kbf.json` files and writes one JSON dict per locale:

```bash
neokapi-i18n compile i18n/ \
  --out public/translations
```

Output:

```
Compiled 1007 entries → public/translations/fr.json
Compiled 1007 entries → public/translations/de.json
Compiled 1007 entries → public/translations/ja.json
```

Each JSON file is a flat `{hash: renderedText}` map. The runtime `__t(hash, fallback, params)` looks up the hash, and the renderer picks the plural or select form.

Compile leaves out a target that lacks any placeholder its source has. The runtime then shows the source text for that entry, with its count, name or link intact. The next translation pass fills the entry in once the target is sound.

## Phase 4: split (optional)

In a code-split app, a single `{locale}.json` per locale makes the user download every string, including those for routes they never visit. The bundler plugin and `neokapi-i18n split` divide the catalog along chunk boundaries, so each chunk gets its own translation subset beside its JS.

Split takes two inputs:

- **`translations-manifest.json`**: emitted when `mode: "runtime"` by Vite, Rollup, webpack, Rspack, and esbuild (esbuild needs `metafile: true`). Maps each output chunk to the set of hashes its modules reference.
- **`public/translations/{locale}.json`**: the compiled master dict from Phase 3.

```bash
vite build                                       # emits dist/translations-manifest.json
neokapi-i18n compile i18n/ --out public/translations
neokapi-i18n split \
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

### Load a chunk with its route

Split copies a hash that several chunks use into each subset, so every chunk file loads on its own. Each lazy route needs one more line:

```tsx
import { loadTranslationChunk } from "@neokapi/i18n-react/runtime";

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

`loadTranslationChunk` merges the subset into the active dict, and concurrent calls for the same `(locale, url)` share a single fetch. A hash that has not arrived yet renders the source text baked into its `__t` / `__tx` call at build time, so a late chunk never breaks the render. See [Runtime mode → Lazy loading per route](./modes#lazy-loading-per-route-code-splitting) for the full runtime contract.

An app that ships a single bundle skips this phase and calls `loadTranslations(locale, url)` with the compiled master dict.

## Round trip in one diagram

<PhaseFlow
  nodes={[
    { label: "src/App.tsx", sub: "<h1>Welcome</h1>" },
    {
      label: "i18n/ Block",
      sub: 'hash "aB3" · source + targets',
      edge: "neokapi-i18n extract (source only)",
      role: "io",
      loop: ["kapi translate --target-lang fr", "then de … (additive, in place)"],
    },
    {
      label: "public/translations/{locale}.json",
      sub: '{ "aB3": "Bienvenue" }',
      edge: "neokapi-i18n compile",
      role: "io",
      loop: ["loadTranslations(locale, url)", "single bundle → app renders"],
    },
    {
      label: "dist/translations/{locale}/",
      sub: "index.json + lazy chunks",
      edge: "neokapi-i18n split (optional)",
      role: "io",
    },
    {
      label: 'Your app renders "Bienvenue"',
      edge: "loadTranslationChunk per route",
      role: "io",
    },
  ]}
/>

## CI: re-extract on every build, fail on drift

The same source and config always extract to the same archive. CI can therefore re-extract and fail when the committed `i18n/` differs:

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

An app with a translation backend pushes the archive to that backend instead and waits for the translated files. Either way, you extract on every change, so the translations follow the source.

## Incremental extracts

The extractor keeps no state between runs: the same source and config always produce the same `.kbf.json`. To translate only what changed, compare two archives on the translation side. A block whose hash changed has new source text.

## Related

- [Runtime vs. inline modes](./modes): shipping one bundle with OTA dicts vs. one bundle per locale.
- [Translating with kapi](./translating-with-kapi): pseudo-translation, AI translation, checks.
- [Configuration](./configuration): componentMap, rules, Storybook, custom warning handlers.
