---
sidebar_position: 3
title: "WASM Engine ABI"
description: "The stable JS contract between the browser wasm build of kapi and the @neokapi/engine npm package: the global function set, the change contract's entry points, the kapiEngineABI feature-detection descriptor, the engine's Worker and the workspace it keeps, and the optional host-provided reverse bridges."
keywords: [wasm, WebAssembly, engine ABI, kapiEngineABI, kapiApply, kapiRead, "@neokapi/engine", browser engine, change contract, reverse bridge, Worker, OPFS, opfs-sahpool, implementation note]
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
  forever so the globals stay callable. In a browser the host is the
  engine's Worker ([The Worker](#the-worker)), so the globals live there and
  the facade reaches them by message.
- **Reverse bridges.** Some features call back into optional host-provided
  globals: `__kapiPdfium` (PDF text + geometry), `kapiLocalGenerate`
  (on-device LLM), `kapiLocalNER` (on-device NER), and `kapiBrowserTranslate`
  (with the platform `Translator` API). Each degrades with an actionable
  error when its bridge is absent. The typed interfaces live in
  `packages/engine/src/capabilities.ts`. A page installs them on its own
  `globalThis` whether the engine runs there or in a Worker.

## The Worker

`bootKapiRuntime` starts the engine in a dedicated Worker
(`packages/engine/src/worker.ts`) wherever `Worker` exists: Go, SQLite and the
engine's file system run there, off the page's thread. `{ worker: false }`, or
an environment without `Worker` (Node, a test), runs it on the calling thread
as before, with the workspace in memory.

- **Calls by message.** The facade sends each call as a message naming the
  engine global and its arguments (`packages/engine/src/protocol.ts`), and the
  Worker answers with the value. Calls that are not engine globals
  (`removeDatabase`, `reset`, `syncContext`, and reading back a trace) run beside them in the
  Worker (`packages/engine/src/local.ts`). `kbf`, `segment`,
  `segmentEngines` and `removeDatabase` answer Promises on the facade, in both
  modes.
- **The file system has a mirror on the page.** `runtime.vol` stays
  synchronous: the page holds a copy of the engine's file system, a write on
  the page reaches the Worker before the next call (messages keep their
  order), and the Worker sends the changes a call made before the call's
  answer, so a page that reads `vol` after a call sees them. `memfs` reports
  each changed path (`onChange`) for this.
- **The ABI descriptor.** The Worker sends `kapiEngineABI()` with its ready
  message, and the facade installs it on the page, so `engineABI()` and
  `hasEngineFunction()` answer there.
- **Reverse bridges stay on the page.** With each call the facade sends which
  bridges the page has, and the Worker installs a stand-in for each that asks
  the page and waits for its answer. A stand-in answers with a Promise, so the
  engine waits for it on a goroutine: `labSegmentAsync` is `labSegment`
  answered as a Promise for that reason, and the ICU4X engine
  (`core/segment/icu4xjs`) accepts a Promise from its bridge. The Worker
  answers `kapiIntlSentenceBreaks` itself, since `Intl.Segmenter` exists
  there, and mirrors the presence of the platform `Translator` API.
- **Loading `wasm_exec.js`.** The Worker imports it as a module, falling back
  to `importScripts` in a classic Worker. A host on another origin serves it
  with CORS, as it already does for the engine binary.

## Storage

Every store in the browser build is the native code: the workspace and its
operation log, the projector, the content memory, the terms store, the voice
store, the decision ledger, the context graph and the block cache. They are SQL
behind `core/storage`, and the browser build's driver runs that SQL on the
official SQLite WebAssembly build, `@sqlite.org/sqlite-wasm`.

- **Where the workspace is kept.** In the Worker, the engine keeps its
  databases in SQLite's `opfs-sahpool` VFS, in the origin private file system,
  and the files of its file system in a database of its own in the same pool
  (`/.kapi-engine/volume.db`, outside the driver's namespace), written after
  each call. Both survive a reload and a browser restart, and need no COOP or
  COEP headers, which GitHub Pages cannot set. `runtime.storage` says which
  applies: `{ kind: "opfs" }`, or `{ kind: "memory", reason }` with `reason`
  one of `disabled`, `main-thread`, `unsupported`, `another-tab`, `taken` or
  `failed`, and `describeStorage()` turns it into a sentence for a page.
  `runtime.onStorageChange()` reports each change. The pool is
  named by a key (`BootOptions.persist`, default `"kapi"`); the docs key it by
  their base URL, since the stable docs, the preview channel and each pull
  request preview share an origin and serve different engines.
- **One tab owns the pool.** The pool holds a synchronous handle on every file,
  which only one context may hold, so the Worker first takes a Web Lock named
  for the pool and holds it for its life. Every tab waits a moment for the lock
  (a reload releases it as the old page closes). `BootOptions.whenHeld` says
  what a tab does when another tab still holds it: `"memory"` (the default)
  runs in memory with `reason: "another-tab"`, `"wait"` waits until the owner
  lets go, and `"take"` takes the workspace over. Before installing the pool
  the Worker checks that no other context still holds its files and waits for
  one that is closing: a failed install in this release of `sqlite-wasm`
  removes the pool's directory. The pool grows between calls, since growing it
  waits on the file system, keeping spare slots for the databases and journals
  a call creates.
- **Handing the workspace over.** `runtime.takeOver()` moves the workspace to
  the tab that calls it. The calls in flight finish first; then the facade
  starts a second Worker that requests the lock with `steal: true`. Stealing
  rejects the owner's held lock request, which is how the owner learns it lost
  the workspace: its Worker keeps the file changes not yet kept, stops using
  the pool and sends `lost`, and its page fails the calls in flight, stops that
  Worker (which releases the pool's file handles) and starts another in memory
  with the page's files, so `storage` becomes `{ kind: "memory", reason:
  "taken" }`. The new owner waits for the handles to be free (up to 15
  seconds), installs the pool and replaces the tab's files with the kept ones;
  the facade moves onto that Worker and stops the old one. When the pool
  cannot be opened, `takeOver()` rejects and the tab stays as it was. A
  Worker that loses the lock before it is ready is replaced by one in memory.
  The playground's files panel offers **Use the workspace here** while
  another tab holds the workspace.
- **Locks between connections.** The pool's own locks only remember the level
  asked for. The engine opens several connections to one database, so the
  bridge installs a lock table over the pool's methods
  (`packages/engine/src/locks.ts`) that keeps SQLite's lock levels per
  database, as `memdb` and `os_unix.c` do within one process: a write beside
  another connection's transaction reports `database is locked` at once, as it
  does in memory.
- **The workspace is a cache.** The browser may evict what a site keeps
  (Safari removes data a script wrote after seven days without interaction),
  so the workspace in a browser is a cache of a log that should also live
  elsewhere. `kapiExportWorkspace()` packs the engine's files and the context
  of every project among them into a workspace package (`kpz.KindWorkspace`,
  `kapi-workspace`): the files under `files/`, and per project a context
  package under `contexts/`, the operation log `kapi store export` writes.
  `kapiImportWorkspace(bytes)` writes the files and merges each log, as
  `kapi store import` does, which rebuilds the project's stores. A terms
  store outside every project (one `kapi terms import --file` wrote, say) has
  no log, so it travels as a terms bundle under `termstores/`, which the
  manifest maps to the store's path, and an import adds its concepts and
  relations to a store at that path. The stores in a project's `.kapi/` are
  projections of its log and stay out; so do the data root, `/tmp` and the
  lab's own directory (`/.lab`). The facade's `exportWorkspace()` and
  `importWorkspace()` wrap them, and the playground's files panel offers both.
- **A reset, then an import.** A reset forgets each project in the workspace
  (`workspace.Forget`), which records `project.forget` in the log and keeps
  the operations before it for history; a store starts after the latest
  removal. The workspace treats an operation held only before its project's
  removal as not held: `Record` writes it again after the removal, a pull
  counts it as new, a push leaves out every operation before the removal, and
  the removal clears what the workspace knew about the project's remotes. So
  importing the package exported before a reset, in the same browser, merges
  the project's log again and rebuilds its stores.
- **Sync through a folder.** `kapiSyncContext(remote, options)` pulls a
  project's context from a remote the page holds and pushes what the engine
  recorded (`App.SyncProjectContextWith`, the same pull and push
  `kapi context sync` runs), and resolves to `{pull, push}`. The remote is an object
  with `list`, `get` and `put` (`kapi/cmd/kapi-wasm-cli/contextremote.go`);
  `folderRemote(handle)` (`packages/engine/src/folderremote.ts`) builds one
  over a `FileSystemDirectoryHandle`, from `showDirectoryPicker()` or the
  origin private file system. The folder keeps the layout `FileRemote` keeps
  on disk, one file per object, each written under a dot-name and moved into
  place, so a recipe with `context: {backend: file, path: <folder>}` on the
  same computer shares the context. The facade's `syncContext(folder, opts)`
  sends the handle to the Worker, where the remote is built, and the files
  panel offers **Sync context with a folder** where `showDirectoryPicker`
  exists (Chromium). The other backends stay native: a `git` backend runs the
  `git` executable, which a page cannot start, and the `s3` backend reads the
  AWS credential chain, which a page has no safe place to hold.

- **The bridge.** `packages/engine/src/sqlite.ts` loads the module and installs
  one object on `globalThis`, `__kapiSQL`, before Go starts. The Go driver
  (`core/storage/driver_js.go`, `core/storage/sqlitejs_js.go`) registers with
  `database/sql` and calls the bridge synchronously through `syscall/js`, so Go
  and SQLite share the page's main thread. Arguments and result rows cross in
  one packed byte buffer per call. A failure comes back as SQLite's own message
  and extended result code.
- **Where the databases live.** Named by absolute path, in the pool or, in
  memory, in SQLite's `memdb` VFS. Every connection to one name shares the
  database, and `ATTACH` by path reaches it, since the bridge makes its VFS the
  default. In memory the bridge holds a connection of its own on each name, so
  a database outlives the pools that open and close it between commands. The
  engine names its data root `/.kapi-data` (`KAPI_DATA_DIR`), where the
  workspace lives.
- **The driver's profile.** `storage.DriverProfile()` reports one connection per
  file, no WAL and no cross-process lock, and durability when the bridge says
  its databases outlive the page (`__kapiSQL.durable`). The busy timeout is
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
- **Starting a directory over.** `kapiReset(dir)` forgets every project
  checked out at or below `dir` in the workspace (`App.ForgetProjectsUnder`,
  which closes the stores it holds there, reads the registry for the projects a
  previous page registered there, and calls `workspace.Forget` for each), then
  removes every database there outside the workspace's own. A project seeded
  there again begins with an empty context and an empty record, because the
  projector replays, and the context ledger reports, only what the log holds
  after a project's latest removal. A project is keyed by
  its recipe's `id:` or `name:`, so the same sample seeded in two directories
  is one project with one context, and a reset under either directory forgets
  it whole: the learning labs reset their whole `/learn` tree on Play for that
  reason. `KapiRuntime.reset(dir)` calls it and then clears the directory's
  files; the playground's Reset button and the terminal's `rm` (through
  `KapiRuntime.removeDatabase`) use them.
- **Tested in Node.** `make test-wasm-stores` runs the store suites under
  `GOOS=js` in Node over the same bridge (`scripts/wasm-stores/`), and the
  change service's suites beside them ([The change
  contract](#the-change-contract)). A test that needs a subprocess (git, or a
  second writer process), directory permissions that reach a database, or
  preemption skips there and names which.

## The change contract

A page that edits content sends the change contract
([E-09](/contribute/architecture/engine/e-09-the-change-contract)) to three
entry points, rather than a command line to `kapiRun`. Each takes the
contract's JSON as a string and an optional second string, the call's options,
and returns a Promise of a JSON string.

| Entry point | Takes | Resolves to |
| --- | --- | --- |
| `kapiRead(requestJSON, optionsJSON?)` | a read request: `{doc, blocks, editions, cursor, limit}` | a read page: each block's `ref` and `rev`, its text in placeholder form, its codes, plurals and selects, its other editions and the operations it accepts, and `next` for the following page |
| `kapiApply(changeSetJSON, optionsJSON?)` | a `kapi.change/v1` change set: an object with `ops`, JSONL, or an array of operations | the `kapi.change-result/v1` result |
| `kapiDescribe(requestJSON, optionsJSON?)` | `{format}` or `{doc}` | what the format supports of each operation |

- **Options.** `{project, actor}`. `project` names the project the call acts
  in: its `kapi.yaml`, its root, or a path inside it. Without one, the call
  acts in the project discovery finds from the engine's working directory, as a
  command given no `-p` does, and outside any project a document is a path
  under that directory. `actor`, which only an apply takes, is
  `{kind, name, session}` with `kind` either `person` or `agent`. Without
  one, the sender is the one `kapi apply` records in the same environment: a
  person, unless `KAPI_ACTOR` or an agent host's marker says otherwise. An
  agent's term, memory and recipe operations, its decisions other than a
  pre-review (`advise`), an `if_match` of `*` and the `report` gate are
  refused as `not_permitted`, as they are over MCP.
- **One service.** The calls build the change service through
  `host.ReadChangesJSON`, `host.ApplyChangesJSON` and
  `host.DescribeChangesJSON`, from the same function the MCP edit tools use
  (`callChangeService`), with the commit check, the policy and the recorder
  `kapi apply` runs. A reference and a revision that `kapiRead` returns are the
  ones `kapi inspect` prints for the same file, and `kapiApply` takes either.
  A bilingual file whose reader has to be told the language of the translation
  it holds, such as a PO catalog's `msgstr`, is read in the one language other
  than the source that a read's `editions` or an apply's operations name, as
  `kapi apply` reads one.
- **Answers.** A request the service refuses, a change set that does not
  decode included, resolves to a `kapi.change-result/v1` whose `error` says
  why. The Promise rejects only for a failure the contract has no code for,
  such as a `project` that names no file. On the facade, `read` and
  `describe` throw `ChangeRefused`, which carries that result, and `apply`
  resolves to its result whatever the status.
- **The record.** In a project, an applied change set is recorded in the
  workspace's log under `/.kapi-data` as one `content.edit` operation per
  document, the record `kapi apply` writes natively, and the projector writes
  it into the block history. A later read shows a translation's basis from
  there. Outside a project nothing is recorded. Every edit is kept with the
  rest of the workspace ([Storage](#storage)).
- **One at a time.** The calls, the commands `kapiRun` runs (a completion
  included) and `kapiReset` share the engine's App, so the engine runs them in
  turn: a command started while a call runs waits for it, and the other way
  round. A command replaces the App's configuration and a reset removes
  databases, so neither may run while a call waits on the file system. A call
  takes its project from its options and its source language from that
  project, not from the flags a command was given. Two writers of one file in
  the page still take turns at the commit: the file home's lock
  (`core/storage/filelock`) is a mutex of the process where the platform has no
  lock between processes.
- **Tested.** `make test-wasm-stores` runs, under `GOOS=js` over the browser's
  SQLite driver, `core/change` and its file home with the conformance suite
  (`core/change/changetest`), the lock, the host's service for a project
  through the same suite and its JSON entry points, and the engine's own test
  of the order its entry points take turns in.
  `make change-wasm-smoke` (`scripts/verify-snippets/change-smoke.ts`) boots
  the real engine in Node and drives the three calls through the facade: a read
  that matches `kapi inspect`, an edit that lands byte for byte and is
  recorded, a stale replay, a translation's basis read back, and the refusals.

## Command surface

`kapiRun(argv)` executes the ordinary kapi CLI, so the browser build has a
second contract alongside the global function set: which verbs it answers.
`kapi apply` and `kapi inspect` run there as they do natively, over the same
service as the change contract's entry points.

- **One declaration.** `cli.BrowserCommandSet` is the browser build's command
  set, mirroring `cli.KapiCommandSet` (the native binary's) verb for verb.
  `kapi/cmd/kapi-wasm-cli` registers it wholesale and declares nothing itself.
- **No missing verbs.** A verb the browser cannot run (one needing a
  subprocess, the OS keychain, the network, or a socket) is recorded in
  `cli.browserGaps` with the facility it needs, and registers a command that
  reports it. `unknown command` therefore means the verb does not exist in kapi
  at all, never that the browser omitted it. `--help` still works on those
  verbs; their help text carries the limitation.
- **No MCP server.** `kapi mcp` is a recorded gap, and the build leaves the
  server out entirely: the host and cli files that import the MCP SDK are built
  with `//go:build !js`, so neither the SDK nor a tool it would serve reaches
  the engine. `host/mcp_js.go` and `cli/mcp_js.go` stand in for the few names
  the rest of the build uses. `make test-wasm-stores` compiles the host's tests
  for the browser, so a host test that drives an MCP tool sits in a file built
  with `!js`: the `mcp_*_test.go` files, and the `*_mcp_test.go` files beside
  the tests they belong with.
- **No plugin host.** `kapi plugin` is a recorded gap: a plugin is a separate
  executable that kapi starts as a subprocess and reaches over gRPC, and a page
  can do neither. The `host/pluginhost` files that start a plugin or talk to
  one (the daemon pool, the gRPC clients for formats, segmenters, comments and
  source connectors, the subprocess launches) are built with `//go:build !js`,
  so gRPC and the plugin protobuf stay out of the engine. Discovery and the
  manifest-driven host build for the browser as well, and
  `host/pluginhost/runtime_js.go` stands in for the launches the rest of the
  build names, each answering that the browser runs no plugin. A test that
  drives the plugin wire sits in a file built with `!js`.
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
| The engine's Worker and its messages | `packages/engine/src/worker.ts`, `packages/engine/src/protocol.ts` |
| Where the workspace is kept (pool, Web Lock, file store) | `packages/engine/src/storage.ts` |
| Handing the workspace between tabs (`takeOver`, `lost`) | `packages/engine/src/runtime.ts`, `packages/engine/src/worker.ts` |
| Locks between connections on the pool | `packages/engine/src/locks.ts` |
| Workspace export and import (`kapiExportWorkspace`, `kapiImportWorkspace`) | `kapi/cmd/kapi-wasm-cli/workspace.go`, `kpz/workspacepkg.go` |
| An operation held before its project's removal | `core/workspace/local.go` (`Record`), `core/workspace/sync.go` |
| Sync through a folder (`kapiSyncContext`) | `kapi/cmd/kapi-wasm-cli/contextremote.go`, `packages/engine/src/folderremote.ts`, `host/contextsync.go` |
| Persistence smoke (reload, restart, a takeover, a waiting tab, a reset and re-import, a folder sync, an export) | `scripts/wasm-persist/`, `make wasm-persist-smoke` |
| Browser database driver + profile + namespace | `core/storage/driver_js.go`, `core/storage/sqlitejs_js.go` |
| Store suites under `GOOS=js` | `scripts/wasm-stores/`, `make test-wasm-stores` |
| Lab project for the annotators | `kapi/cmd/kapi-wasm-cli/labproject.go` |
| Starting a directory over (`kapiReset`) | `kapi/cmd/kapi-wasm-cli/reset.go`, `App.ForgetProjectsUnder` in `host/projectstore.go` |
| The change contract (`kapiRead`, `kapiApply`, `kapiDescribe`) | `kapi/cmd/kapi-wasm-cli/changes.go`, `host/changes_call.go` |
| The change contract smoke | `scripts/verify-snippets/change-smoke.ts`, `make change-wasm-smoke` |
| Reverse-bridge capability types | `packages/engine/src/capabilities.ts` |
| Payload types (ContentTree, runs, the change contract) | `@neokapi/contract-types` (generated) |

When adding an entry point: add it to `engineExports`, type it in
`globals.ts`, surface it on the facade if user-facing, and leave the `abi`
value alone (additions don't bump it).
