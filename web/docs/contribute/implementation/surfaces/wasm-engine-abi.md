---
sidebar_position: 3
title: "WASM Engine ABI"
description: "The stable JS contract between the browser wasm build of kapi and the @neokapi/engine npm package: the global function set, the kapiEngineABI feature-detection descriptor, and the optional host-provided reverse bridges."
keywords: [wasm, WebAssembly, engine ABI, kapiEngineABI, "@neokapi/engine", browser engine, reverse bridge, implementation note]
---

# WASM Engine ABI

The browser build of kapi (`kapi/cmd/kapi-wasm-cli`) registers a set of global
JS functions at boot. That set is a versioned contract, the ABI consumed by
the `@neokapi/engine` npm package (`packages/engine`), which wraps it in the
typed `KapiRuntime` facade.

## Contract

- **Single registration point.** Every entry point is declared in the
  `engineExports` table in `kapi/cmd/kapi-wasm-cli/main.go` and installed on
  `globalThis` from there. Nothing else registers engine globals.
- **Feature detection.** `kapiEngineABI()` returns
  `{abi, version, functions}`: the ABI major version, the kapi build version,
  and the list of registered global names. A build without `kapiEngineABI`
  predates the descriptor and is treated as abi 0 (probe individual globals).
- **Additive within a version.** For one `abi` value, changes are strictly
  additive: new functions may appear in `functions`, but existing names,
  signatures, and payload shapes never change or disappear. Renaming or
  removing an entry point, or changing a signature incompatibly, bumps the
  version.
- **Boot handshake.** The host installs its `fs`/`process` shims (the
  `wasm_exec.js` environment) and the SQLite bridge (`__kapiSQL`, see
  [Storage](#storage)) before instantiation; after registering the globals,
  the engine invokes the host's `__kapiCliReady()` callback and then blocks
  forever so the globals stay callable.
- **Reverse bridges.** Some features call back into optional host-provided
  globals: `__kapiPdfium` (PDF text + geometry), `kapiLocalGenerate`
  (on-device LLM), `kapiLocalNER` (on-device NER), and `kapiBrowserTranslate`
  (with the platform `Translator` API). Each degrades with an actionable
  error when its bridge is absent. The typed interfaces live in
  `packages/engine/src/capabilities.ts`.

## Storage

Every store in the browser build is the native code: the workspace and its
operation log, the projector, the content memory, the terms store, the voice
store, the decision ledger, the context graph and the block cache. They are SQL
behind `core/storage`, and the browser build's driver runs that SQL on the
official SQLite WebAssembly build, `@sqlite.org/sqlite-wasm`.

- **The bridge.** `packages/engine/src/sqlite.ts` loads the module and installs
  one object on `globalThis`, `__kapiSQL`, before Go starts. The Go driver
  (`core/storage/driver_js.go`, `core/storage/sqlitejs_js.go`) registers with
  `database/sql` and calls the bridge synchronously through `syscall/js`, so Go
  and SQLite share the page's main thread. Arguments and result rows cross in
  one packed byte buffer per call. A failure comes back as SQLite's own message
  and extended result code.
- **Where the databases live.** In SQLite's `memdb` VFS, named by absolute path.
  Every connection to one name shares the database, and `ATTACH` by path reaches
  it. The bridge holds a connection of its own on each name, so a database
  outlives the pools that open and close it between commands. The engine names
  its data root `/.kapi-data` (`KAPI_DATA_DIR`), where the workspace lives.
  Nothing outlives the tab.
- **The driver's profile.** `storage.DriverProfile()` reports one connection per
  file, no WAL, no cross-process lock and no durability. The busy timeout is
  zero: a second connection's wait would spin the only thread, so a lock
  conflict reports `database is locked` at once. Code on these pools never holds
  a transaction or open rows and then waits for a second session on the same
  pool from the same goroutine. The block store's iterators read a page at a
  time and release the connection before yielding, so a caller may write while
  it iterates. A failed `COMMIT` is rolled back, as the native driver does, so
  the pool's one connection never stays inside a transaction.
  `make test-stores-oneconn` runs the store suites natively with every pool held
  to one connection and WAL off, to find such code without a browser: a call
  that waits 20 seconds for the pool panics with every goroutine's stack, and a
  read on one pool beside a write the same code holds on another waits out the
  busy timeout and fails with `database is locked`, which the browser reports at
  once. CI runs it beside `make test-wasm-stores`, and on push runs the host
  suite the same way (`make test-host-oneconn`).
- **Database files belong to the driver.** The page's file system never sees a
  database, so code that asks whether one exists, removes, renames or lists them
  calls `storage.Exists`, `storage.Remove`, `storage.Rename`, `storage.List` and
  `storage.RemoveAll` rather than `os`. A database's directory still has to
  exist in the engine's file system, as it does natively.
- **A database file a page adds.** A SQLite file in the engine's file system
  (one a reader uploads, say) is read into memory the first time a store opens
  its path, through `sqlite3_deserialize`, and writes go to the copy in memory.
  The namespace answers for such files too, and `storage.Remove` deletes the
  file with the database, so a removed database stays removed. A
  file that is not a database fails to open with `file is not a database`, as
  it does natively; one with an unflushed write-ahead log beside it fails with a
  message that says so.
- **Row batches.** A query's rows cross 256 at a time, or about 1 MiB at a
  time when they are large.
- **Word search.** The module carries SQLite's built-in FTS5 tokenizers and no
  ICU, so `storage.FTSWordTokenizer` is `unicode61`.
- **The asset.** `sqlite3.wasm` is served beside the engine binary, where
  `make web-wasm-cli` stages it (with `sqlite3.wasm.gz`) from the pinned
  package, and the boot prefers the precompressed copy.
  `bootKapiRuntime(wasmExecUrl, wasmUrl, { sqliteWasmUrl })` takes another
  location. Measured with gzip -9, the asset is about 0.40 MB and the bundled
  JavaScript (the module's glue and the bridge, minified) about 0.07 MB.
- **The lab project.** The read-only annotators behind `labInspectAnnotated`
  look terms up in a project the engine creates at `/.lab/project` on first use,
  and opens again if a reset closed it. Its terms are the bundle in
  `kapi/cmd/kapi-wasm-cli/fixtures/terms.json`, imported through the projector,
  so they are operations in the workspace log as they would be natively.
  Commands typed in a lab resolve stores as natively: inside a project they use
  the project's, and outside one the standalone store a flag or the working
  directory names.
- **Starting a directory over.** `kapiReset(dir)` forgets every project at or
  below `dir` in the workspace (`App.ForgetProjectsUnder`, which closes its
  stores and calls `workspace.Forget`), then removes every database there
  outside the workspace's own. A project seeded there again begins with an
  empty context, because the projector replays only what the log holds after a
  project's latest removal. `KapiRuntime.reset(dir)` calls it and then clears
  the directory's files; the playground's Reset button and the terminal's `rm`
  (through `KapiRuntime.removeDatabase`) use them.
- **Tested in Node.** `make test-wasm-stores` runs the store suites under
  `GOOS=js` in Node over the same bridge (`scripts/wasm-stores/`). A test that
  needs a git subprocess, directory permissions that reach a database, or
  preemption skips there and names which.

## Command surface

`kapiRun(argv)` executes the ordinary kapi CLI, so the browser build has a
second contract alongside the global function set: which verbs it answers.

- **One declaration.** `cli.BrowserCommandSet` is the browser build's command
  set, mirroring `cli.KapiCommandSet` (the native binary's) verb for verb.
  `kapi/cmd/kapi-wasm-cli` registers it wholesale and declares nothing itself.
- **No missing verbs.** A verb the browser cannot run (one needing a
  subprocess, the OS keychain, the network, or a socket) is recorded in
  `cli.browserGaps` with the facility it needs, and registers a command that
  reports it. `unknown command` therefore means the verb does not exist in kapi
  at all, never that the browser omitted it. `--help` still works on those
  verbs; their help text carries the limitation.
- **Drift is a test failure.** `cli.TestBrowserCommandSurface` compares the two
  sets and fails when a verb appears in one and not the other, or when a gap's
  help metadata drifts from the command it stands in for. Adding a verb to
  `KapiCommandSet` is a decision: wire it up for the browser, or record why it
  cannot run there.
- **Runtime guard.** `make wasm-surface-smoke`
  (`scripts/verify-snippets/command-surface-smoke.ts`) boots the real wasm in
  Node, sweeps every reachable verb for `unknown command`, asserts each gap's
  message, and replays the argv the lab explorers themselves use. It runs in the
  docs snippet-verification workflow (`docs-verify-snippets.yml`).
- **Capture strips ANSI.** The engine boots with `CLICOLOR_FORCE=1` so the
  playground terminal renders kapi's real styling; `runCapture` hands output to
  program code instead, and strips the escapes so a `--json` payload parses.

The same derivation feeds the docs: `scripts/gen-refs` reads
`cli.BrowserUnavailableReason` for each command's runnable-in-browser badge, so
the Command Reference cannot claim a verb runs in the lab when it does not.

## Where the pieces live

| Concern | Location |
| --- | --- |
| Registration table + `kapiEngineABI()` | `kapi/cmd/kapi-wasm-cli/main.go` |
| Browser command set + recorded gaps | `cli/browsercmds.go` |
| Surface drift guard | `cli/browsercmds_test.go` |
| Runtime surface smoke | `scripts/verify-snippets/command-surface-smoke.ts` |
| Ambient TS typings for the globals | `packages/engine/src/globals.ts` |
| Wire shapes + `engineABI()` helper | `packages/engine/src/abi.ts` |
| `KapiRuntime` facade + boot | `packages/engine/src/runtime.ts` |
| SQLite bridge (`__kapiSQL`) | `packages/engine/src/sqlite.ts` |
| Browser database driver + profile + namespace | `core/storage/driver_js.go`, `core/storage/sqlitejs_js.go` |
| Store suites under `GOOS=js` | `scripts/wasm-stores/`, `make test-wasm-stores` |
| Lab project for the annotators | `kapi/cmd/kapi-wasm-cli/labproject.go` |
| Starting a directory over (`kapiReset`) | `kapi/cmd/kapi-wasm-cli/reset.go`, `App.ForgetProjectsUnder` in `host/projectstore.go` |
| Reverse-bridge capability types | `packages/engine/src/capabilities.ts` |
| Payload types (ContentTree, runs) | `@neokapi/contract-types` (generated) |

When adding an entry point: add it to `engineExports`, type it in
`globals.ts`, surface it on the facade if user-facing, and leave the `abi`
value alone (additions don't bump it).
