# Curated result views (R8 · #677)

Framework-first docs widgets. kapi is a _framework_; the CLI is one front-end.
These components show what the **framework produced** — the content model,
before/after transforms, and a dual CLI ⇄ result view — rather than only a
terminal.

All three run the **real kapi CLI compiled to WebAssembly** in the browser,
reusing `@neokapi/kapi-playground` (the kit). They are **lazy + client-only**:
each wraps its body in `BrowserOnly` + a dynamic `import()` of the heavy kit, so
a docs page that never mounts a curated view ships **zero wasm**. They honor
light/dark and match the playground styling by reusing the kit's `--kpg-*`
design tokens (defined on `:root` by the kit's `styles.css`).

## How they reuse the kit

- **Boot** — `useCuratedRuntime()` dynamically imports the kit and calls
  `bootKapiRuntime(wasmExecUrl, wasmUrl)` (URLs resolved through the existing
  Docusaurus adapter `useKapiPlaygroundConfig`). Boot is idempotent kit-side, so
  several curated views on one page share the single warm runtime instance.
- **`BlockPreview`** uses `KapiRuntime.preview(path)` →
  `{ ok, format, blocks:[{id,text}], total, bytes }`.
- **`BeforeAfter`** / **`DualExample`** use `KapiRuntime.run(argv)` and read the
  output file back via `runtime.vol.readFile(path)`.
- Seeding (`seed.ts`) writes bundled fixtures (`getFixture`) or inline samples
  into the runtime's in-memory volume, mirroring the kit's gap-filling behavior.
- `DualExample`'s "Terminal" affordance hands off to the kit's shared modal via
  `openKapi` (imported from the SSR-clean `/store` subpath).

No kit source was modified — the curated views compose the kit's public API.

## Components

### `<BlockPreview>` — the "kapi reader"

Parse a sample file and render the content model (blocks / ids / source text,
with inline-span markers shown as chips) in a clean table. The single best
framework demo: _"here's how kapi sees your file."_ Use on the content-model /
formats / inline-formatting concept pages and as the **format → parse/blocks**
reference view.

| Prop      | Type                     | Notes                                                            |
| --------- | ------------------------ | ---------------------------------------------------------------- |
| `sample`  | `string \| InlineSample` | A bundled fixture name (see `fixtureNames`) or `{name,content}`. |
| `title`   | `string?`                | Card header; defaults to the file name.                          |
| `caption` | `string?`                | Sub-line under the title.                                        |

```tsx
<BlockPreview sample="strings.xml" caption="How kapi parses an Android strings file" />
<BlockPreview sample={{ name: "demo.html", content: "<p>Hi <b>there</b></p>" }} />
```

### `<BeforeAfter>` — source → transformed result

Write the pristine source, run a tool/command, read the output, show input vs
output side by side. Use for **tool** reference entries (pseudo-translate,
search-replace, case-transform, redaction, …) and guides.

| Prop          | Type                     | Notes                                                                      |
| ------------- | ------------------------ | -------------------------------------------------------------------------- |
| `sample`      | `string \| InlineSample` | Pristine source, rewritten fresh before each run.                          |
| `command`     | `string`                 | e.g. `kapi pseudo-translate messages.json -o out.json`.                    |
| `outputPath`  | `string?`                | File to read for the "after" pane. Defaults to the source name (in-place). |
| `beforeLabel` | `string?`                | Defaults to `"Source"`.                                                    |
| `afterLabel`  | `string?`                | Defaults to `"Result"`.                                                    |
| `caption`     | `string?`                | Caption under the command strip.                                           |
| `autoRun`     | `boolean?`               | Auto-run on mount (default `true`).                                        |

```tsx
<BeforeAfter
  sample="messages.json"
  command="kapi pseudo-translate messages.json -o out.json"
  outputPath="out.json"
/>
```

### `<DualExample>` — CLI command ⇄ curated result

The old playground's "file in → parsed blocks out" feel, generalized: a CLI
command (+ its captured stdout/stderr in the Catppuccin palette) beside the
curated result. Layouts: `"split"` (side by side, default) or `"tabs"`. Use in
guides and wherever both the command ergonomics _and_ the framework output
matter.

| Prop      | Type                          | Notes                                                     |
| --------- | ----------------------------- | --------------------------------------------------------- |
| `command` | `string`                      | Command run in the terminal pane; its output is captured. |
| `seed`    | `(string \| InlineSample)[]?` | Fixtures/inline samples to seed before the command runs.  |
| `result`  | `DualResult`                  | The curated result (see below).                           |
| `layout`  | `"split" \| "tabs"`           | Defaults to `"split"`.                                    |
| `caption` | `string?`                     | Caption above the example.                                |

`DualResult` is either a block view or a before/after view:

```tsx
// blocks result
<DualExample
  command="kapi word-count messages.json"
  seed={["messages.json"]}
  result={{ kind: "blocks", sample: "messages.json", title: "messages.json (parsed)" }}
/>

// before/after result
<DualExample
  command="kapi pseudo-translate messages.json -o out.json"
  seed={["messages.json"]}
  result={{
    kind: "before-after",
    sample: "messages.json",
    command: "kapi pseudo-translate messages.json -o out.json",
    outputPath: "out.json",
  }}
/>
```

## When to use which

| Goal                                            | Component      |
| ----------------------------------------------- | -------------- |
| Show how kapi _parses_ a format (content model) | `BlockPreview` |
| Show a tool's effect (source → result)          | `BeforeAfter`  |
| Show the command _and_ the framework output     | `DualExample`  |

## Files

- `useCuratedRuntime.ts` — lazy boot hook (one warm runtime per page).
- `seed.ts` — fixture/inline seeding + `parseCommand` + `readText`.
- `curated.css` — `kapi-cur-*` styles reusing the kit's `--kpg-*` tokens.
