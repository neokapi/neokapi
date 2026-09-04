---
sidebar_position: 7
title: Extract, Translate, Compile Pipeline
description: "The three-phase neokapi-i18n pipeline: extract JSX to a KBF archive, translate it with kapi (AI or content memory), compile locales back into runtime JSON. Includes an optional split phase for code-split apps."
keywords: [extract, translate, compile, KBF, neokapi-i18n pipeline, code splitting, translation pipeline]
---

import { PhaseFlow } from "@neokapi/docs-shared";

# The extract → translate → compile pipeline

Three phases, one contract: the KBF directory archive. A fourth optional phase, **split**, slices the compiled output along bundler chunk lines so code-split apps can lazy-load translations per route.

<PhaseFlow
  nodes={[
    { label: "Your source code" },
    {
      label: "i18n/",
      sub: "KBF archive",
      role: "io",
      edge: "neokapi-i18n extract",
      loop: ["kapi translate / pseudo-translate / qa / review", "accumulate target locales in place"],
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

The same `i18n/` is the source-of-truth artifact through the whole round-trip. Translation tools read it, append the target locale they're producing, and write back to the same file, so you accumulate locales rather than juggling per-run output files. One file in the repo, one file to ship to translators, one file to compile.

Each phase has a single tool; none of them are coupled to the others. You can swap out the translator step for any process that preserves the KBF contract: human translators working in a CAT tool, AI translation, a pre-existing TMS.

## Phase 0: explain (optional)

Before you extract anything, you can ask the extractor what it *would* do and why:

```bash
vp neokapi-i18n explain src/components/Settings.tsx
```

```text
L3    <div>          [container] skipped — has block-level children (they extract separately)
L4    <h1>           [translatable] extracted  hash=cYEMc2v3JVx
L6    <code>         [non-translatable] skipped — classified non-translatable
L7    <input>        [container] skipped — no translator-editable text
        ↳ placeholder [attribute] extracted  hash=i42kuGUFbb4
```

Each line is the element's W3C ITS classification, the gate that decided its fate, and the hash it received. "Zero-config extraction" is only trustworthy if you can audit it; this is the audit. Add `--extracted` to list only what made the catalog.

## Phase 1: extract

The extractor walks every `.jsx` / `.tsx` file in your project and produces translatable blocks. Plain `.ts` (and `.mts` / `.cts`) modules are parsed as TypeScript too, and their `t()` calls are extracted when the glob includes them (`--src "src/**/*.{ts,tsx,jsx}"`). A file that cannot be parsed is reported with a warning naming it, so nothing goes missing silently. Two output modes:

- **Default**: per-file `.kbf.json` under `--out` (default `i18n/`). Human-readable, git-diffable.
- **`--stream`**: NDJSON block records on stdout. File discovery happens via `--src` glob when stdin is a terminal; kapi's exec format can pipe NUL-separated paths to stdin for batch-controlled extraction.

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

Flags:

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
    "extract": "vp neokapi-i18n extract",
    "extract:ci": "vp neokapi-i18n extract --strict",
    "pack": "vp neokapi-i18n extract --stream > i18n/blocks.ndjson"
  }
}
```

For full authoring-time coverage, pair this with [`@neokapi/i18n-react-lint`](./linting): editor squigglies for `t(variable)`, `<img alt={'Logo ' + x} />`, and the other patterns the build-time transform can't catch.

### What's in the KBF directory

A directory of per-file `.kbf.json` documents, mirroring your source tree
(e.g. `src/App.tsx` → `i18n/src/App.kbf.json`). The mirror is kept on every
run: a catalog for a source that has been deleted, renamed or excluded from
`--src` is removed, while per-locale targets under `i18n/<lang>/` are left
alone. Each catalog is a self-contained KBF `File` carrying:

- `project`: id, source locale, declared target locales.
- `documents`: one document for the source file, holding its `Block`s.
- Optional targets / skeleton / annotation overlays (added by translators).

See [C-01](/contribute/architecture/context/c-01-project-model) for the full schema.

### One block per

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

The `.kbf.json` is the translator's deliverable. Three common paths:

### Path A: AI translation

Run a full translation pass with `kapi translate`:

```bash
kapi translate i18n/ --target-lang fr
kapi translate i18n/ --target-lang de
kapi translate i18n/ --target-lang ja
```

Each run **accumulates** a target locale into the same `.kbf.json`. The writer is locale-additive by design: existing targets stay put, the requested locale is added or updated in place. No `-o` needed unless you want to redirect output.

`kapi` supports Anthropic, OpenAI, Azure OpenAI, Google Gemini, and Ollama. It preserves placeholders, inline element tokens, and plural/select structure; AI providers that mangle them are automatically wrapped with recovery logic.

### Path B: Pseudo-translate

For UI-layout checks, pseudo-translation generates visibly-altered strings without any real translation:

```bash
kapi pseudo-translate i18n/
```

`Welcome` becomes `▒ Ŵéḷçőḿé ▒`, padded and accented. Missing translations stand out instantly, and strings that wrap too aggressively (or too narrowly) show up in layout testing.

### Path C: CAT tools / TMS / human translators

The `.kbf.json` is the exchange format. A translator's workflow might be:

1. Open the `i18n/` archive (or the individual `.kbf.json` files) in their CAT tool.
2. Translate every block, leveraging their existing content memory.
3. Save back to the same `i18n/`.

The context a block carries (its file and line, its element, the translator note, the inline element tokens) renders as rich context in modern CAT tools.

### In-place default vs. explicit redirect

`kapi` tool commands default to in-place for KBF inputs: `kapi pseudo-translate i18n/` reads and writes the same files, since the KBF writer is locale-additive (it adds or updates the requested locale, leaving the others intact). Pass `-o other-dir/` to redirect without touching the originals.

Non-KBF formats (JSON, XLIFF, …) aren't locale-additive, so they write a new file in a locale-aware location: if the input path carries the source locale it is swapped for the target (`src/locales/en/app.json → src/locales/fr/app.json`), otherwise the output lands under a `{lang}/` directory beside the input (`messages.json → fr/messages.json`). Use `-o` for an explicit path or template, or `--output-dir DIR` to root outputs at `DIR/{lang}/`.

### Layout: one tree, or a subdir per locale

Two layouts, both clean:

- **Locale-additive**: one `i18n/` tree where each block carries every target locale, filled in place (the default for `kapi translate i18n/ --target-lang …`). Simplest to version; all translations stay together.
- **Recipe-driven per-locale files**: the source catalogs live under `i18n/src/` and kapi writes a separate file per locale under `i18n/{lang}/`, mapped by a `kapi.yaml` content entry (`path: i18n/src/**/*.kbf.json` → `target: i18n/{lang}/{path}.kbf.json`). This is what `kapi init --framework neokapi-i18n` scaffolds.

Both keep everything under one `i18n/` directory. Because the source lives under `i18n/src/`, the source glob never matches the generated `i18n/{lang}/` targets, so there is no need for sibling `i18n-<lang>/` trees. See [C-01](/contribute/architecture/context/c-01-project-model) for the project model and [Drive it from a project](./translating-with-kapi#drive-it-from-a-project) for the recipe.

### Project-driven flow with `kapi.yaml`

If you already use a [`kapi.yaml` project file](/contribute/architecture/context/c-01-project-model) to define your workflow, declare each archive-backed collection with an `exec` format pointing at neokapi-i18n (or any other extractor):

```yaml title="kapi.yaml"
version: v1
name: MyApp
defaults:
  source_language: en
  target_languages: [fr, de, ja]
collections:
  - name: ui
    # Block state lives in the project cache (gitignored, regenerable).
    content:
      - path: "src/**/*.tsx"
        format:
          name: exec
          config:
            command: "vp neokapi-i18n extract --stream"
```

```bash
# 1. Extract: kapi runs the declared command for each collection and
#    streams NDJSON blocks into the collection's block store.
kapi extract -p kapi.yaml

# 2. Translate: run a composed flow over the project for each target language.
kapi run translate-qa -p kapi.yaml
```

The `command` string picks the package manager (`vp`, `pnpm`, `npm`, `yarn`, or a direct binary path), so the project declares its preferences explicitly without kapi making assumptions. `kapi run` then executes the named [flow](/framework/flows) against the project's extracted blocks for each target language.

### Standalone pipe (no `kapi.yaml`)

For ad-hoc projects, skip `kapi.yaml` entirely and compose with Unix pipes:

```bash
vp neokapi-i18n extract --stream > i18n/blocks.ndjson
kapi pseudo-translate i18n/
vp neokapi-i18n compile i18n/ --out public/translations
```

Same underlying wire format (NDJSON on the extract stage, KBF from there on); the declarative `kapi.yaml` form factors the pipe into the project file.

## Phase 3: compile

`neokapi-i18n compile` reads the translated `.kbf.json` and emits one JSON dict per locale:

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

Each JSON file is a flat `{hash: renderedText}` map. The runtime `__t(hash, fallback, params)` looks up the hash; the renderer picks the plural / select form.

A target that does not carry every placeholder its source has is left out of the dict, so the runtime falls back to the source text for that entry rather than rendering a sentence missing its count, name or link. The next translation pass fills it in once the target is sound.

## Phase 4: split (optional)

For code-split apps, the compiled `{locale}.json` is one file per locale; the user downloads every string even for routes they never visit. The plugin + `neokapi-i18n split` divide that catalog along bundler chunk boundaries so each chunk lands its own translation subset alongside its JS.

Two inputs:

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

Hashes shared across chunks are duplicated into each subset so every chunk file is independently loadable. Runtime wiring is a one-line addition to each lazy route:

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

`loadTranslationChunk` merges the subset into the active dict; concurrent calls for the same `(locale, url)` share a single fetch. Missing hashes fall back to the source text baked into each `__t` / `__tx` call at build time, so a late-arriving chunk never breaks render. See [Runtime mode → Lazy loading per route](./modes#lazy-loading-per-route-code-splitting) for the full runtime contract.

Apps that ship a single bundle don't need this phase at all; keep using `loadTranslations(locale, url)` against the compiled master dict.

## Round-trip in one diagram

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

For apps with a translation backend, you'd instead push the archive to that backend and wait for translated deliverables. The principle is the same: extract on every change, and keep translations from drifting away from source.

## Incremental extracts

The extractor is stateless: it always produces the same `.kbf.json` for the same source + config. For an incremental pipeline (only translate what changed), diff two archives on the translation side. Each block's hash tells you whether its source shifted.

## Next

- [Runtime vs. inline modes](./modes): shipping one bundle with OTA dicts vs. one bundle per locale.
- [Translating with kapi](./translating-with-kapi): pseudo-translation, AI translation, checks.
- [Configuration](./configuration): componentMap, rules, Storybook, custom warning handlers.
