---
sidebar_position: 9
id: plugin-protocol-v1
title: "Plugin protocol v1"
description: "The versioned, language-neutral specification for kapi plugins: the manifest model, the three transport modes, the Mode-C gRPC surface and wire format, and the conformance suite an out-of-tree plugin repository runs against a released kapi to self-report conformance."
keywords:
  [
    plugin protocol,
    protocol v1,
    manifest,
    conformance,
    gRPC,
    BridgeService,
    Mode A,
    Mode B,
    Mode C,
    stdio handshake,
    Unix socket,
    specification,
  ]
---

import { LanesDiagram } from "@neokapi/docs-shared";

# Plugin protocol v1

This is the **versioned, language-neutral specification** for plugins targeting
kapi. It is a contract, not a tour: a plugin author in any language implements
against this page and verifies the result with the
[conformance suite](#conformance-suite), which runs against a *released* kapi
module, so a plugin repository never needs to live inside this one.

The architectural rationale is [E-05](/contribute/architecture/engine/e-05-plugin-system);
the in-process Go registries a Go plugin binary uses internally are
[Note: Plugin model](plugin-model.md). This page is the wire.

## Versioning

The protocol is versioned as a whole, independently of any kapi release:

- **`conformance.ProtocolVersion`** (`core/plugin/conformance`) names the
  revision this repository implements. It is `1`.
- **`manifest.CurrentVersion`** / **`manifest.SupportedVersions`**
  (`core/plugin/manifest`) name the manifest-document revisions a kapi binary
  accepts. A manifest declares its own with `manifest_version`.

Within a protocol version the rules below are additive: capabilities, manifest
fields, and conformance checks may be **added**, but an existing rule is not
changed and a conformance check ID is not renamed or repurposed. A change that
would break a conforming plugin is a new protocol version.

A plugin declares which revision it targets implicitly, by passing that
revision's conformance suite. There is no protocol-version handshake on the
wire: the manifest is read before any process is launched, so incompatibility is
detected from disk rather than negotiated at runtime.

## Manifest

Every plugin installs into a single directory named for the plugin, directly
under a discovery root, with a `manifest.json` at its root:

```
~/.local/share/kapi/plugins/
  myplugin/
    manifest.json            # identity + capabilities + daemon config
    installed.json           # version + registry provenance (written by kapi)
    kapi-myplugin            # the executable named by manifest.binary
    schemas/
      server.json            # any JSON Schemas the manifest references
```

```json
{
  "manifest_version": "1",
  "plugin": "myplugin",
  "version": "1.4.0",
  "binary": "kapi-myplugin",
  "license": "Apache-2.0",
  "min_kapi_version": "1.0.0",
  "capabilities": {
    "commands": [],
    "mcp_tools": [],
    "formats": [],
    "tools": [],
    "segmenters": [],
    "source_connectors": [],
    "schema_extensions": [],
    "command_contributions": [],
    "config_namespaces": [],
    "selfcheck": true
  },
  "daemon": {
    "idle_timeout_seconds": 300,
    "startup_timeout_seconds": 30,
    "handshake": { "type": "stdio-handshake", "fields": ["socket", "version"] }
  },
  "models": []
}
```

The canonical Go types are `core/plugin/manifest/manifest.go`; the JSON Schema
is embedded at `core/plugin/manifest/schema.json` and returned by
`manifest.SchemaJSON()`, so a plugin's own build can validate its manifest
against the exact document kapi validates against.

### Manifest rules

These are the rules the host enforces at discovery time. A manifest that breaks
any of them means the plugin does not register, usually silently, which is why
the conformance suite checks each one by name.

| Rule                                                                                                        | Why it matters                                                                                |
| ----------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------- |
| `manifest_version` is in `manifest.SupportedVersions`                                                       | An unsupported revision is rejected outright.                                                  |
| No keys outside the schema (`additionalProperties: false`)                                                  | A misspelled key is silently ignored rather than taking effect.                                 |
| `plugin` matches the install directory's name, and matches `[a-z0-9][a-z0-9-]*`                              | Discovery verifies the two agree and drops the plugin when they do not.                         |
| `binary` is relative to the plugin directory, resolves inside it, and is executable                         | Every transport execs this path.                                                               |
| At least one capability section is populated                                                                | A plugin with no capabilities cannot be dispatched to.                                          |
| `daemon` is present **exactly** when a `formats`, `tools`, `segmenters`, or `source_connectors` is declared    | The host reads the daemon's timeouts and handshake shape before it execs it.                     |
| Every referenced JSON Schema resolves inside the plugin directory and parses                                | An unreadable schema degrades recipe validation to a structural-only check.                      |
| Every non-bundled `models[].files[]` pins a 64-hex lowercase `sha256` and an `https` URL                     | The signed manifest is the only trust root for model bytes; the host refuses an unpinned one.     |
| At most one model asset is `default`                                                                        | Otherwise "the default model" is ambiguous.                                                    |

Capability names must not collide **between plugins**: kapi drops the
conflicting entry from its dispatch table and reports the conflict, so an
ambiguous capability does not dispatch until one plugin is removed. A
collision with a *built-in* is resolved in the plugin's favour: installing a
plugin for a format is an explicit signal to prefer it.

### Discovery

Discovery is pure filesystem reads: **no subprocess is launched to enumerate
capabilities.** Roots are scanned in precedence order (lower order wins a name
conflict):

| Order       | Root                                                                                             | Source             |
| ----------- | ------------------------------------------------------------------------------------------------ | ------------------ |
| 1 (highest) | `$KAPI_PLUGINS_DIR` (OS path list, `:` separated, `;` on Windows)                                 | dev / CI / sandbox |
| 2           | `$XDG_DATA_HOME/kapi/plugins` (default `~/.local/share/kapi/plugins`)                              | per-user install   |
| 3           | `/opt/homebrew/share/kapi/plugins`, `/usr/local/share/kapi/plugins`, `/usr/share/kapi/plugins`      | system install     |

`$KAPI_PLUGINS_DIR_ONLY=1` restricts discovery to root 1, which is how this
repository's own tests and scripts stay isolated from a developer's installed
plugins. kapi never consults `$PATH`.

Results are cached as JSON at `$KAPI_PLUGIN_CACHE`, else
`$XDG_CACHE_HOME/kapi/plugins-cache.json`. The cache records each root's
directory mtime and is rejected when the binary's cache version changes,
`GOOS`/`GOARCH` differ, the set of roots changes, or a root's mtime is newer
than recorded.

## Standard verbs

Every plugin binary answers these, regardless of which transports it declares:

```
<binary> version           # print the plugin's version, exit 0
<binary> doctor            # self-check, exit 0 when healthy (only if selfcheck)
```

`version` is used to confirm an installed binary matches its manifest, and to
warm first exec on macOS. `doctor` backs `kapi plugins doctor`; a plugin that
bundles native libraries, models, or in-process engines declares
`capabilities.selfcheck` and confirms those resolve.

An **unrecognised** first argument must exit non-zero. A catch-all dispatcher
silently absorbs verbs added by a later protocol revision instead of failing
visibly, so this is a required rule rather than a style preference.

## The three transports

A manifest's capability sections determine which transports kapi uses. A plugin
may declare any subset.

| Capability section      | Transport | Invocation                              |
| ----------------------- | --------- | --------------------------------------- |
| `commands`              | Mode A    | `<binary> command <name> [args...]`     |
| `command_contributions` | Mode A    | `<binary> command <handler> [flags...]` |
| `mcp_tools`             | Mode B    | `<binary> mcp-server`                   |
| `formats`               | Mode C    | `<binary> daemon`                       |
| `tools`                 | Mode C    | `<binary> daemon`                       |
| `segmenters`            | Mode C    | `<binary> daemon`                       |
| `source_connectors`     | Mode C    | `<binary> daemon`                       |
| `schema_extensions`     | none      | validated in-process by kapi            |

### Mode A: one-shot subprocess

```
<binary> command <name> [sub...] [args/flags...]
```

- stdin / stdout / stderr are inherited from kapi. (A data-returning contribution
  is the exception: kapi captures stdout and discards stderr.)
- The environment carries `KAPI_PLUGIN_DIR`, `KAPI_PLUGIN_NAME`, and
  `KAPI_PLUGIN_VERSION` on top of kapi's own environment, **less the provider
  API-key variables** (`ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, `GEMINI_API_KEY`,
  `GOOGLE_API_KEY`, `AZURE_OPENAI_API_KEY`). A plugin is separate software the
  user installed; kapi's model credentials are not part of what installing it
  granted. Everything else is inherited: `PATH`, `HOME`, `TMPDIR`,
  `XDG_CACHE_HOME`, and any variables the plugin's own `models` declarations
  resolve to. A plugin that genuinely needs to reach a model provider should
  say so in its manifest rather than read the host's key by inheritance. The
  conformance harness (`core/plugin/conformance`) scrubs the same variables, so
  a plugin that reads one fails under `kapi plugin conform` the way it fails
  under kapi; the list has one home, `core/credentials/providerenv`.
- The **exit code is propagated** to kapi's caller.
- The process exits after each command; no state carries over.
- A command name the manifest does not declare must exit non-zero.

Command attachment follows the **no-shadowing rule**: installing a plugin never
changes what an existing kapi verb means. Every plugin command also attaches
under `kapi <group> <verb>`, and a command whose name collides with a built-in
attaches under the group *only*. A plugin that needs to participate in a core
verb uses a `command_contribution` rather than a same-name command. See
[E-05](/contribute/architecture/engine/e-05-plugin-system).

### Mode B: session subprocess (MCP over stdio)

```
<binary> mcp-server
```

- One long-lived process per `kapi mcp` session.
- The transport is **newline-delimited JSON-RPC 2.0** on stdin/stdout (the MCP
  stdio transport). **stdout carries nothing else**: a stray log line corrupts
  the stream and kills the session mid-call. Diagnostics go to stderr.
- The server answers `initialize` with a `protocolVersion` and a
  `serverInfo.name`, then serves `tools/list` and `tools/call`.
- `tools/list` must include **every** tool the manifest declares; a declared
  tool the server does not serve is a dead entry in kapi's dispatch table.
- **Closing stdin ends the session**: the process must exit, or kapi leaks one
  process per session.

### Mode C: daemon over a Unix socket

```
<binary> daemon
   ↓ first stdout line, within daemon.startup_timeout_seconds (default 30)
{"socket":"/tmp/kapi-daemon-myplugin-12345.sock","version":"1.4.0","pid":12345}
```

- The daemon binds a **Unix-domain socket**, prints exactly one JSON line, the
  **handshake**, then keeps stdout open. Everything after that first line is
  log output, which kapi forwards to its own stderr.
- The handshake's `socket` is required and must be an **absolute path**. kapi
  dials `unix://<socket>` and only that; it never connects over TCP.
- The socket should be **owner-only** (mode `0600`, under `$TMPDIR`). kapi dials
  it with insecure transport precisely because it is user-private.
- kapi drives the connection to `READY` before dispatching, so a daemon that
  bound its socket but never served fails fast rather than on first use.
- The daemon stays alive until kapi exits or it hits `idle_timeout_seconds`
  (default 300). Concurrent daemons are capped by `$KAPI_MAX_DAEMONS`
  (default 8) with LRU eviction; a per-plugin spawn lock keeps a concurrent
  first-use burst from starting redundant processes.
- The daemon must **exit on SIGTERM** and release its socket file. Eviction,
  idle timeout, and kapi's own exit all tear daemons down with SIGTERM, and a
  stale socket blocks the next bind at that path.
- `$KAPI_DAEMON_SOCKET_<PLUGIN>` (e.g. `KAPI_DAEMON_SOCKET_MYPLUGIN`) points
  kapi at an already-running daemon's socket and skips `exec` entirely, which is how a
  benchmark measures per-call cost without paying startup each time.

Mode C is POSIX-only today: the transport is a Unix socket, and the host returns
an error on Windows.

## Mode-C gRPC surface

A Mode-C daemon serves `BridgeService`, defined in
`core/plugin/proto/v2/neokapi_bridge.proto`:

```protobuf
service BridgeService {
  // Full document processing cycle. Bidirectional streaming; supports
  // read-only, read-write, and single-pass modes.
  rpc Process(stream ProcessRequest) returns (stream ProcessResponse);

  // Run a single pipeline step over a stream of parts.
  rpc ProcessStep(stream StepRequest) returns (stream StepResponse);

  // Segment one run of text with a declared segmentation engine.
  rpc Segment(SegmentRequest) returns (SegmentResponse);

  // Shut down gracefully.
  rpc Shutdown(ShutdownRequest) returns (ShutdownResponse);
}
```

Only the RPCs backing declared capabilities need real implementations: a
formats-only plugin may leave `Segment` unimplemented, and a segmenter-only
plugin may leave `Process` unimplemented. `Shutdown` is recommended but optional;
the host falls back to SIGTERM.

A daemon that serves gRPC without registering `BridgeService` answers every call
with `Unimplemented`, which is indistinguishable from a plugin that provides
nothing. The conformance suite probes for this explicitly.

### Process lifecycle

One `Process` stream handles a full document:

1. **kapi sends `ProcessHeader`**: format name (`filter_class`), input document
   (`ContentRef`: path, inline bytes, or URI), source/target locale, encoding,
   MIME type, parameters, an optional output destination, and a
   `subscribe_parts` filter.
2. **The plugin reads** its document, converts subscribed events to
   `ContentBlock`s, batches them into `ContentBlockBatch` (up to 1024 blocks per
   message), and streams them back.
3. **kapi processes blocks** through the flow's tool chain and sends each
   processed block back individually.
4. **The plugin applies** the returned translations and writes its output.
5. **The plugin sends `ReadDone`** once everything has been read and written.
6. **kapi closes its send side** to signal no more processed parts.
7. **The plugin sends `ProcessComplete`** with an output path or inline bytes.

A **read-only** run omits both the output reference and the output locale: kapi
closes its send side immediately after the header, and the plugin streams blocks
without a writer.

### Wire format

Two lightweight message types keep gRPC framing cheap. `ContentBlock` is a
stripped block (roughly a tenth the size of a full `BlockMessage`) omitting the
skeleton and referent flags, which stay on the plugin's side, and
`ContentBlockBatch` amortizes framing over up to 1024 of them:

```protobuf
message ContentBlockBatch {
  repeated neokapi.content.v1.ContentBlock blocks = 1;
}
```

`ProcessHeader.subscribe_parts` controls which part types cross the wire at all.
Empty means all of them (backwards compatible); `[5]` means blocks only, letting
the plugin write structural events directly without a round-trip. On a large
spreadsheet that cuts message counts by roughly 3–4×, far more than any buffer
tuning.

Direction matters for batching:

- **Plugin → kapi is batched.** The plugin sends all its blocks before waiting
  for any translation back, so a partial batch is never stranded.
- **kapi → plugin is not.** Batching the return path would deadlock: the final
  partial batch would be held until the processed-part stream closes, which needs
  `ReadDone` from the plugin, which needs the translations. Per-part delivery
  breaks the cycle.

Content is referenced three ways:

```protobuf
message ContentRef {
  oneof location {
    bytes  inline = 1;
    string path   = 2;
    string uri    = 3;
  }
}
```

**Prefer `path`.** A real filesystem path lets the plugin resolve relative
references, linked rule files, stand-off annotations, companion assets, and
avoids moving bytes over the socket. The host raises the per-message ceiling to
256 MB precisely because plugins that use `inline` stream whole documents.

### Concurrency inside a daemon

A daemon serves concurrent RPCs (a gRPC connection is multiplexed), so its
internal design is its own business, but two patterns must hold,
because getting them wrong deadlocks the protocol rather than merely slowing it
down.

<LanesDiagram
  handoff="internal queue"
  lanes={[
    {
      title: "Reader side",
      sub: "bounded pool",
      role: "io",
      steps: [
        "open the document",
        "create the writer BEFORE iterating",
        "for each event:",
        "  if subscribed: send to kapi FIRST",
        "  then enqueue for the writer",
        "enqueue end-of-events",
      ],
    },
    {
      title: "Writer side",
      sub: "unbounded pool",
      role: "translate",
      steps: [
        "dequeue until end-of-events:",
        "  apply returned translations",
        "  write the event",
      ],
    },
  ]}
  caption="Convert and send a subscribed event before enqueuing it: applying translations and writing mutate the event in place, so reading it first avoids a race. Decoupling the two sides prevents the circular deadlock a single thread hits when it must both send on the stream and drain the translation queue. Bound the queue for back-pressure; keep the writer pool unbounded so a busy reader pool cannot starve it. A read-only run needs no writer side at all."
/>

Reject excess concurrent streams with `RESOURCE_EXHAUSTED` rather than queueing
without limit, and bound the translation-queue poll so a stuck stream aborts
instead of pinning a worker forever.

### Parameters

Format, tool, and segmenter parameters arrive as `map<string, string>`
(`ProcessHeader.filter_params` and `SegmentRequest.params`), described by the JSON
Schema the manifest points at (`capabilities.formats[].schema`,
`capabilities.tools[].schema`, `capabilities.segmenters[].schema`). The host
loads those schemas through `core/format/schema` for CLI introspection and UI
forms; validating a parameter's *value* is the plugin's own responsibility, since
only the plugin knows what it accepts.

### Recipe schema extensions

`schema_extensions` never crosses a process boundary. Each entry binds a recipe
YAML key at a scope (`project`, `defaults`, `collection`, `item`) to a JSON
Schema file in the plugin directory:

```json
{
  "name": "server",
  "scope": "project",
  "group": "myplugin",
  "json_schema": "schemas/server.json"
}
```

At register time kapi compiles that schema and validates the recipe's payload
under that key at parse time. A schema that cannot be read, parsed, or compiled
degrades to a structural-only check with a warning: one plugin's broken schema
never stops a recipe from loading, which is exactly why the conformance suite
treats an unreadable schema as a failure on the plugin's side.

## Model assets

A plugin declares large data dependencies rather than fetching them itself, so
downloads are uniform with the rest of kapi (one cache, one progress bar, one
integrity check) and the plugin binary stays a pure compute engine:

```json
"models": [
  {
    "id": "my-model", "version": "1", "default": true,
    "license": "Apache-2.0",
    "files": [
      { "path": "weights.onnx", "url": "https://…/resolve/<commit>/weights.onnx",
        "sha256": "<64 lowercase hex>", "size": 123456 }
    ]
  }
]
```

The host stages a model under
`$XDG_CACHE_HOME/kapi/models/<plugin>/<id>/<version>/` and passes the plugin that
directory; the plugin never reaches the network. Every non-bundled file **must**
pin a lowercase 64-hex `sha256`, and should pin an immutable upstream revision in
its URL rather than a moving branch. There is no unsafe escape hatch for a
declared asset: the manifest ships inside the signed tarball, so those digests
are the trust root. A `bundled: true` asset ships inside the tarball and needs
only a `path`.

## Distribution

A release tarball contains a single top-level directory named for the plugin, so
a user can install without kapi at all:

```
kapi-myplugin-1.4.0-darwin-arm64.tar.gz
└── myplugin/
    ├── manifest.json
    ├── LICENSE
    ├── kapi-myplugin
    └── schemas/server.json
```

The plugin's `LICENSE` text ships inside the tarball, so the archive carries the
terms of the work it contains; a plugin that bundles a third-party runtime ships
that runtime's licence text beside it. A plugin's GitHub release is published as
not-latest, so a tool that resolves the repository's latest release still finds
kapi rather than the plugin.

```bash
tar -xzf kapi-myplugin-*.tar.gz -C ~/.local/share/kapi/plugins/
```

A registry is a JSON index over HTTPS mapping plugin → version → per-platform
tarball URL, SHA-256, Sigstore bundle URL, and the pinned cosign certificate
identity plus OIDC issuer. `kapi plugin install` verifies the SHA-256 against the
registry-pinned hash, then the bundle's signing certificate against the pinned
identity using [`sigstore-go`](https://github.com/sigstore/sigstore-go),
mirroring `cosign verify-blob` keyless defaults (SCT, transparency-log, and
observer-timestamp thresholds of 1). A registry entry missing `signature`,
`cert_identity`, or `cert_oidc_issuer` is rejected unless `--unsafe` is passed,
there is no silent unsigned install path.

Because tarballs are fetched with Go's HTTP client rather than a browser, the
extracted binary never carries the macOS quarantine attribute, so plugins need no
Apple notarization or Authenticode signature. The supply-chain layer (cosign +
SHA-256) is the meaningful guarantee, and it is enforced on every platform. See
[E-05](/contribute/architecture/engine/e-05-plugin-system).

## Conformance suite

`core/plugin/conformance` is the executable form of this specification: a
black-box driver that exercises the same contract the host relies on, against an
installed plugin directory. It reads no plugin source, so the plugin may be
written in any language.

It lives in the **framework** module (`github.com/neokapi/neokapi`), so an
out-of-tree plugin repository depends on a released kapi version and imports
nothing else: no CLI, no cobra, no platform code.

```go
import "github.com/neokapi/neokapi/core/plugin/conformance"

func TestProtocolConformance(t *testing.T) {
    conformance.RunT(t, conformance.Suite{Dir: pluginInstallDir})
}
```

`RunT` logs the full transcript and fails the test once per failing required
check. For a non-test caller (a release gate, a `doctor`-style command), use
`Run` and inspect the report:

```go
report, err := conformance.Run(ctx, conformance.Suite{Dir: dir})
if err != nil {           // the suite could not run at all
    return err
}
fmt.Print(report.Text())  // human transcript
data, _ := report.JSON()  // stable machine artifact for CI
if !report.OK() {
    return report.Err()   // every required failure, summarised
}
```

A failing check is never an `error` from `Run`; the error return is reserved for
a suite that could not start. Callers gate on `report.OK()`.

### What it checks

Checks are grouped and namespaced `<group>.<name>`. The authoritative list is
`conformance.Checks()`, which returns every check (ID, title, transport, whether
it is required, and a one-line statement of what breaks in kapi when it fails),
without running anything, so CI can diff coverage over time.

| Group      | Covers                                                                                                                                                                                                                    |
| ---------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `manifest` | Every rule in [Manifest rules](#manifest-rules): presence, parse + validation, supported version, no unknown keys, name/directory agreement, binary resolution, capability/daemon consistency, schema files, model pinning |
| `binary`   | The [standard verbs](#standard-verbs): `version` succeeds and agrees with the manifest, `doctor` succeeds when `selfcheck` is declared, an unknown verb is rejected                                                        |
| `modeA`    | An undeclared command is rejected; optionally, one declared command is invoked and its exit code and output asserted                                                                                                       |
| `modeB`    | The MCP handshake completes, `tools/list` covers every declared tool, stdout carries only JSON-RPC, and closing stdin ends the process                                                                                     |
| `modeC`    | The handshake line, the bound socket and its permissions, gRPC reaching `READY`, `BridgeService` being registered, `Segment` answering for declared segmenters, `Shutdown`, and SIGTERM termination with the socket released |

A check is **required** or **advisory**. An advisory failure is reported but does
not clear `report.OK()`: it names something the host tolerates today that a
well-behaved plugin should still fix. Two are advisory: a `version` verb that
disagrees with the manifest, and a socket reachable by other users.

A check that does not apply is **skipped**, never failed: a plugin declaring no
MCP tools skips the whole `modeB` group, Mode C skips on Windows, and each
opt-in probe skips when not configured. A skip is not a deficiency.

When a prerequisite fails, dependent checks skip with a pointer to the root cause
rather than restating it: a daemon that never printed its handshake produces
exactly one failure, not nine.

### Opt-in probes

The suite never invokes a plugin capability that could have side effects on its
own initiative. It runs only the read-only standard verbs plus deliberately
invalid arguments whose job is to prove the plugin rejects them. Doing real work
is opt-in, because only the plugin author knows which capabilities are safe to
exercise unattended:

```go
conformance.Suite{
    Dir: dir,
    // Invoke one declared command and assert its result.
    CommandProbe: &conformance.CommandProbe{
        Command: "status", WantExit: 0, WantStdout: "ok",
    },
    // Read a real document through a declared format. The suite stages the
    // bytes on disk and hands the daemon the path, as the host would.
    FormatProbe: &conformance.FormatProbe{
        Format: "myfmt", Input: fixture, Filename: "sample.myfmt", MinBlocks: 3,
    },
    // Call a declared segmentation engine.
    SegmentProbe: &conformance.SegmentProbe{
        Engine: "myseg", Text: "One. Two.", MinBoundaries: 1,
    },
}
```

`Suite` also carries `Env` (extra environment for every spawned process),
`Only` / `Skip` (check IDs or group names), and `Logf` (a trace line per check).
`Only` restricts which checks are *reported*; it does not disable the suite's own
setup, so `Only: []string{"modeC"}` still resolves the manifest those checks
need.

### Three budgets, not one

A run has three independent budgets, because the suite spends time in three
different ways and one knob cannot govern them all:

| Field | Bounds | Default |
| --- | --- | --- |
| `Timeout` | the work a single check does | `DefaultTimeout` (60s) |
| `StartupTimeout` | how long a Mode-C daemon may take to print its handshake | the manifest's `daemon.startup_timeout_seconds`, else `DefaultStartupTimeout` (30s) |
| `ShutdownGrace` | how long a daemon may take to exit after `Shutdown` or SIGTERM | `DefaultShutdownGrace` (5s) |

The two daemon-lifecycle budgets are waits on the *plugin's* process, not work
the suite does, so neither is clamped by `Timeout`; a check that owns one is
given its `Timeout` plus that wait. Keeping them separate is what lets a caller
shorten the one wait a healthy plugin never uses in full (the teardown grace) on
a case written to prove a daemon ignores SIGTERM, without also starving startup.
Collapsing them makes a loaded machine report every negative Mode-C case as the
same startup timeout instead of the protocol violation it was written to catch.

A startup timeout is reported as its own failure mode: the detail names the
startup budget, the dependent checks skip citing it rather than a generic "the
daemon did not start", and `Result.Err` wraps `ErrStartupTimeout`. It is the one
Mode-C outcome an overloaded machine can produce on its own, so it is worth
telling apart from a protocol violation, which is a claim about the plugin.

### The reference plugins

Two plugins in this repository are checked by the suite's own tests, so the
specification and its examples cannot drift apart:

- **`examples/plugins/hello/`**: a minimal Mode A + B plugin with no
  third-party dependencies. If it ever stops conforming, the suite says so.
- **`core/plugin/conformance/testdata/probedaemon/`**: a Mode A + C fixture that
  speaks the whole protocol and carries environment switches making it violate
  one rule at a time (no handshake, non-JSON handshake, missing socket field,
  relative socket, unregistered service, permissive socket, missing `Shutdown`,
  ignored SIGTERM, leaked socket). Every negative path in the suite is verified
  against a real subprocess rather than a mock.

Out of tree, [neokapi/okapi-bridge](https://github.com/neokapi/okapi-bridge) is
the reference implementation in a non-Go language: a JVM daemon exposing the
Okapi Framework's Java filters over Mode C. Its role is to keep this protocol
honest from the outside: it consumes released kapi versions and reports
conformance on a schedule, never gating this repository.
