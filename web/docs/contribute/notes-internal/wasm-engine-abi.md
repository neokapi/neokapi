---
sidebar_position: 31
title: "WASM Engine ABI"
description: The stable JS contract between the browser wasm build of kapi and the @neokapi/engine npm package — the global function set, the kapiEngineABI feature-detection descriptor, and the optional host-provided reverse bridges.
keywords: [wasm, WebAssembly, engine ABI, kapiEngineABI, "@neokapi/engine", browser engine, reverse bridge, implementation note]
---

# WASM Engine ABI

The browser build of kapi (`kapi/cmd/kapi-wasm-cli`) registers a set of global
JS functions at boot. That set is a versioned contract — the ABI consumed by
the `@neokapi/engine` npm package (`packages/engine`), which wraps it in the
typed `KapiRuntime` facade.

## Contract

- **Single registration point.** Every entry point is declared in the
  `engineExports` table in `kapi/cmd/kapi-wasm-cli/main.go` and installed on
  `globalThis` from there. Nothing else registers engine globals.
- **Feature detection.** `kapiEngineABI()` returns
  `{abi, version, functions}` — the ABI major version, the kapi build version,
  and the list of registered global names. A build without `kapiEngineABI`
  predates the descriptor and is treated as abi 0 (probe individual globals).
- **Additive within a version.** For one `abi` value, changes are strictly
  additive: new functions may appear in `functions`, but existing names,
  signatures, and payload shapes never change or disappear. Renaming or
  removing an entry point, or changing a signature incompatibly, bumps the
  version.
- **Boot handshake.** The host installs its `fs`/`process` shims (the
  `wasm_exec.js` environment) before instantiation; after registering the
  globals, the engine invokes the host's `__kapiCliReady()` callback and then
  blocks forever so the globals stay callable.
- **Reverse bridges.** Some features call back into optional host-provided
  globals: `__kapiPdfium` (PDF text + geometry), `kapiLocalGenerate`
  (on-device LLM), `kapiLocalNER` (on-device NER), and `kapiBrowserTranslate`
  (with the platform `Translator` API). Each degrades with an actionable
  error when its bridge is absent. The typed interfaces live in
  `packages/engine/src/capabilities.ts`.

## Where the pieces live

| Concern | Location |
| --- | --- |
| Registration table + `kapiEngineABI()` | `kapi/cmd/kapi-wasm-cli/main.go` |
| Ambient TS typings for the globals | `packages/engine/src/globals.ts` |
| Wire shapes + `engineABI()` helper | `packages/engine/src/abi.ts` |
| `KapiRuntime` facade + boot | `packages/engine/src/runtime.ts` |
| Reverse-bridge capability types | `packages/engine/src/capabilities.ts` |
| Payload types (ContentTree, runs) | `@neokapi/contract-types` (generated) |

When adding an entry point: add it to `engineExports`, type it in
`globals.ts`, surface it on the facade if user-facing, and leave the `abi`
value alone (additions don't bump it).
