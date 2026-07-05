# @neokapi/engine

The [kapi](https://github.com/neokapi/neokapi) content engine — format-aware
parsing, processing flows, and localization tools — compiled to WebAssembly and
wrapped as a typed, dependency-light npm package.

The package owns:

- **Boot** — `bootKapiRuntime(wasmExecUrl, wasmUrl)`: installs an in-memory
  filesystem, loads Go's `wasm_exec.js`, instantiates the engine, and resolves
  once the engine signals ready. Idempotent; one warm instance per page.
- **`KapiRuntime`** — the facade over the engine's global function set:
  `run` (any browser-safe kapi CLI command), `preview`, `inspect`,
  `inspectAnnotated`, `klf`, `segment`, `segmentEngines`, `runWithTrace`,
  plus the in-memory volume (`vol`), `cwd`/`chdir`, and `setSinks` for
  stdout/stderr routing.
- **Versioned ABI** — `engineABI()` reads the engine's `kapiEngineABI()`
  descriptor (`{abi, version, functions}`) for feature detection;
  `hasEngineFunction(name)` probes individual entry points (with a fallback
  for pre-ABI builds). Within one `abi` value the contract is strictly
  additive.
- **Ambient typings** — every engine global (`kapiRun`, `labInspect`, …) is
  typed on `globalThis`, so no call site needs an `as any` cast.
- **Typed host capabilities** — the optional reverse bridges a page may
  provide (`__kapiPdfium`, `kapiLocalGenerate`, `kapiLocalNER`,
  `kapiBrowserTranslate`) as documented interfaces, plus
  `detectCapabilities()`.

Payload types (the `ContentTree` returned by `inspect`, run shapes, overlays)
come from `@neokapi/contract-types`, generated from the Go structs — they
cannot drift from the engine.

The package deliberately has **no UI or ML dependencies** (no xterm, monaco,
pdfium, onnxruntime). Higher-level kits — terminals, modals, plugin bridges —
build on top of it (see `@neokapi/kapi-playground` in the neokapi repo).

## The wasm asset is not bundled

The engine binary (`kapi-cli.wasm`) is ~64 MB raw / ~13 MB gzipped, so it is
**not** shipped in this package. You pass its URL (and the matching
`wasm_exec.js`) to `bootKapiRuntime`. Two patterns:

### 1. CDN-hosted asset

Point at a hosted engine build. The loader prefers a precompressed
`<wasmUrl>.gz` sibling and inflates it with `DecompressionStream`, so the
asset works from static hosts that don't set `Content-Encoding`:

```ts
import { bootKapiRuntime } from "@neokapi/engine";

const base = "https://cdn.example.com/kapi/wasm/1.2.0";
const runtime = await bootKapiRuntime(`${base}/wasm_exec.js`, `${base}/kapi-cli.wasm`);
```

### 2. Self-hosted asset

Build the engine from the neokapi repo and serve it with your app:

```bash
# in the neokapi repo — outputs kapi-cli.wasm, kapi-cli.wasm.gz, wasm_exec.js
make web-wasm-cli
```

Copy the three files into your static assets (keep the `.gz` next to the
`.wasm` to get the small download), then:

```ts
const runtime = await bootKapiRuntime("/assets/wasm_exec.js", "/assets/kapi-cli.wasm");
```

Either way, show progress while the asset downloads:

```ts
import { onBootProgress } from "@neokapi/engine";

onBootProgress(({ loaded, total, done }) => {
  if (!done) render(loaded, total); // total is null when Content-Length is absent
});
```

## Using the runtime

```ts
import { bootKapiRuntime, engineABI } from "@neokapi/engine";

const rt = await bootKapiRuntime(wasmExecUrl, wasmUrl);

// Feature-detect the booted engine.
console.log(engineABI()); // { abi: 1, version: "1.2.0", functions: ["kapiRun", …] }

// Files live in the engine's in-memory volume.
rt.vol.writeFile("/project/app.json", new TextEncoder().encode(`{"hello":"world"}`));

// Run any browser-safe kapi command; stdout/stderr flow through setSinks.
rt.setSinks(
  (s) => console.log(s),
  (s) => console.error(s),
);
await rt.run(["pseudo-translate", "/project/app.json", "-o", "/project/out.json"]);

// Or use the typed endpoints.
const { tree } = await rt.inspect("/project/app.json"); // ContentTree (@neokapi/contract-types)
```

## Host capabilities (reverse bridges)

Some engine features call back into globals the page may provide — PDF
extraction, an on-device LLM, on-device NER, and the platform Translator API.
Each degrades with an actionable error when absent. The typed contracts live
in `@neokapi/engine/capabilities`; `detectCapabilities()` reports what the
current page provides. Reference implementations ship with the neokapi repo's
playground and lab kits.

## License

Apache-2.0
