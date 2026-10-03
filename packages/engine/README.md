# @neokapi/engine

The [kapi](https://github.com/neokapi/neokapi) content engine — format-aware
parsing, processing flows, and pluggable tools — compiled to WebAssembly and
wrapped as a typed, dependency-light npm package.

The package owns:

- **Boot** — `bootKapiRuntime(wasmExecUrl, wasmUrl)`: installs an in-memory
  filesystem, loads SQLite (`@sqlite.org/sqlite-wasm`) and installs the bridge
  the engine's database driver calls, loads Go's `wasm_exec.js`, instantiates
  the engine, and resolves once the engine signals ready. Idempotent; one warm
  instance per page.
- **`KapiRuntime`** — the facade over the engine's global function set:
  `run` (any browser-safe kapi CLI command), `preview`, `inspect`,
  `inspectAnnotated`, `kbf`, `segment`, `segmentEngines`, `runWithTrace`,
  `reset` (start a directory over, its projects, databases and files) and
  `removeDatabase`, plus the in-memory volume (`vol`), `cwd`/`chdir`, and
  `setSinks` for stdout/stderr routing. `read`, `apply` and `describe` edit
  content through the change contract (`kapi.change/v1`), the one `kapi apply`
  and the agent tools use.
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

Payload types (the `ContentTree` returned by `inspect`, run shapes, overlays,
the change set, its result and the read page) come from
`@neokapi/contract-types`, generated from the Go structs, so they cannot drift
from the engine.

The package deliberately has **no UI or ML dependencies** (no xterm, monaco,
pdfium, onnxruntime). Higher-level kits — terminals, modals, plugin bridges —
build on top of it (see `@neokapi/kapi-playground` in the neokapi repo).

## The wasm assets are not bundled

The engine binary (`kapi-cli.wasm`) is ~90 MB raw / ~20 MB gzipped, so it is
**not** shipped in this package. You pass its URL (and the matching
`wasm_exec.js`) to `bootKapiRuntime`.

The engine's stores (content memory, terms, the workspace and its operation
log, the block cache) are SQL, and in the browser they run on SQLite's own
WebAssembly build. Boot loads `sqlite3.wasm` from beside the engine binary,
preferring a precompressed `sqlite3.wasm.gz`, and `make web-wasm-cli` stages
both there. It must be the release of `@sqlite.org/sqlite-wasm` this package
depends on (the package exports it as `@sqlite.org/sqlite-wasm/sqlite3.wasm`).
To serve it elsewhere, pass its URL:

```ts
await bootKapiRuntime(wasmExecUrl, wasmUrl, { sqliteWasmUrl: "/assets/sqlite3.wasm" });
```

The databases live in memory for the life of the page. Two patterns for the
engine asset:

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
# in the neokapi repo — outputs kapi-cli.wasm(.gz), sqlite3.wasm(.gz), wasm_exec.js
make web-wasm-cli
```

Copy the files into your static assets (keep each `.gz` next to its `.wasm`
to get the small download), then:

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

## Editing content

`read`, `apply` and `describe` carry the change contract to the same change
service `kapi inspect` and `kapi apply` use. A read gives each block the
reference to copy into an operation's `at` and the revision to send as its
`if_match`; an apply of an operation built from them lands through the format's
writer, so everything around the text stays byte for byte, and is refused as
`stale` when the block changed since the read.

```ts
import { ChangeRefused } from "@neokapi/engine";

const page = await rt.read({ doc: "/project/app.json" });
const hello = page.blocks.find((b) => b.text === "world")!;

const result = await rt.apply({
  ops: [{ op: "set_content", at: hello.ref, if_match: hello.rev, text: "everyone" }],
});
if (result.status !== "applied") {
  console.warn(result.error?.message ?? result.ops.find((op) => op.error)?.error?.message);
}

const html = await rt.describe({ format: "html" }); // which operations HTML supports

try {
  await rt.read({ doc: "/project/missing.json" });
} catch (e) {
  if (e instanceof ChangeRefused) console.warn(e.result.error?.code); // "not_found"
}
```

Each call takes options as a second argument: `project` names the project the
call acts in (its `kapi.yaml`, its root or a path inside it; omitted, the one
discovery finds from the working directory), and an apply's `actor`
(`{ kind: "person" | "agent", name, session }`) names who sends it, a person
when omitted. In a project, an applied change set is recorded in the engine's
workspace log, as `kapi apply` records one. An `apply` resolves to its
`kapi.change-result/v1` whatever the status; a `read` or `describe` the
service refuses throws `ChangeRefused`, which carries that result. The engine
runs these calls, the commands `run` starts and `reset` one at a time, so a
call never sees a command reconfigure the engine under it.

## Host capabilities (reverse bridges)

Some engine features call back into globals the page may provide — PDF
extraction, an on-device LLM, on-device NER, and the platform Translator API.
Each degrades with an actionable error when absent. The typed contracts live
in `@neokapi/engine/capabilities`; `detectCapabilities()` reports what the
current page provides. Reference implementations ship with the neokapi repo's
playground and lab kits.

## License

Apache-2.0
