---
sidebar_position: 3
title: "Okapi Bridge Protocol"
description: Implementation note for AD-007 — the gRPC bridge protocol between neokapi and the Okapi Java subprocess, covering the BridgeService RPCs (Process, ProcessStep, Shutdown) and the Part-to-Event translation.
keywords: [bridge protocol, gRPC, BridgeService, Okapi bridge, Process RPC, Part model, Event model, implementation note]
---

import { LanesDiagram } from "@site/src/components/diagram";

# Bridge Protocol

This note provides implementation details for [AD-007](/contribute/architecture/007-plugin-system).

## gRPC Bridge Protocol

The Okapi bridge is a Mode-C plugin daemon that hosts Okapi Framework filters and exposes them via a gRPC service. The host side (`cli/pluginhost/`, with the format client in `format_client.go`) manages the daemon lifecycle, connects as a gRPC client, and translates between neokapi's Part model and Okapi's Event model via `core/plugin/protoconvert`.

The protocol is defined in `core/plugin/proto/v2/neokapi_bridge.proto`:

```protobuf
service BridgeService {
  // Process performs a complete document processing cycle using bidirectional
  // streaming. Supports read-only, read-write, and single-pass modes.
  rpc Process(stream ProcessRequest) returns (stream ProcessResponse);

  // ProcessStep runs a single Okapi pipeline step over a stream of parts.
  rpc ProcessStep(stream StepRequest) returns (stream StepResponse);

  // Shutdown gracefully shuts down the bridge server.
  rpc Shutdown(ShutdownRequest) returns (ShutdownResponse);
}
```

### Process RPC Protocol

A single `Process` stream handles the full document lifecycle:

1. **Go sends `ProcessHeader`** — filter class, input document (path or inline bytes), source/target locale, encoding, output destination, `subscribe_parts` filter
2. **Java reads the filter** — iterates Okapi events, converts subscribed events to `ContentBlock` messages, batches into `ContentBlockBatch` (up to 1024 blocks per message), streams to Go
3. **Go processes blocks** — pipes through flow tool chain (e.g., pseudo-translate, AI translate), sends processed blocks back individually via `ProcessRequest.part`
4. **Java applies translations** — writer thread applies translations from a queue, writes events to the Okapi filter writer
5. **Java sends `ReadDone`** — signals all events have been read and written
6. **Go sends `CloseSend`** — signals no more processed parts
7. **Java sends `ProcessComplete`** — output path or inline bytes

### Wire Format

Two lightweight message types reduce gRPC overhead:

```protobuf
// ContentBlock — lightweight block for gRPC transfer (~10x smaller than BlockMessage).
// Omits skeleton and is_referent which stay on the Java side.
message ContentBlock {
  string id = 1;
  string name = 2;
  string type = 3;
  string mime_type = 4;
  bool translatable = 5;
  repeated SegmentMessage source = 6;
  repeated TargetEntry targets = 7;
  map<string, string> properties = 8;
  bool preserve_whitespace = 9;
  map<string, AnnotationEntry> annotations = 10;
  DisplayHintMessage display_hint = 11;
}

// ContentBlockBatch — batched content blocks (up to 1024 per message).
message ContentBlockBatch {
  repeated ContentBlock blocks = 1;
}
```

The `subscribe_parts` field in `ProcessHeader` controls which event types cross gRPC:

```protobuf
message ProcessHeader {
  ...
  repeated int32 subscribe_parts = 10;
  // Empty = all events cross gRPC (backward compatible).
  // [4] = Block only — structural events (Layer, Data, Group) are
  // written directly by Java without gRPC round-trips.
}
```

Setting `subscribe_parts = [4]` reduces message count from ~570K to ~157K for a large XLSX file, since only translatable Block events need Go-side processing.

### Content Transfer

Content can be referenced in three ways via `ContentRef`:

```protobuf
message ContentRef {
  oneof location {
    bytes  inline = 1;  // Inline bytes
    string path   = 2;  // Local filesystem path
    string uri    = 3;  // Remote/local URI
  }
}

message OutputRef {
  oneof destination {
    string path = 1;
    string uri  = 2;
  }
}
```

File paths are preferred over inline bytes — they allow Java to resolve relative references (ITS linked rules, XLIFF standoff, companion files) and avoid byte transfer overhead.

### Daemon Startup and Transport

In daemon mode (`kapi-okapi-bridge daemon`) the bridge self-allocates a per-PID
socket path under the JVM temp dir (`kapi-okapi-bridge-<pid>.sock`), unless
`NEOKAPI_BRIDGE_SOCKET` overrides it. It binds that Unix-domain socket using
Netty's **native transports — kqueue on macOS, epoll on Linux** — for
kernel-level throughput, then prints the canonical handshake line on stdout:

```json
{"socket":"/tmp/kapi-okapi-bridge-12345.sock","version":"..."}
```

The host (`cli/pluginhost/daemon.go`) reads that line and dials the Unix socket
as a gRPC client. The host dials Unix sockets only; it does not connect over TCP.

The server also has a TCP fallback: when no socket path is set (a legacy,
non-daemon path used by the old Go shim and by tests), it binds a localhost gRPC
port and reports `tcp://localhost:<port>` instead. `createUnixSocketServer`
throws `UnsupportedOperationException` on any OS that is neither macOS nor
Linux, so the daemon transport is effectively POSIX-only today.

## Java Pipeline Architecture

The Java `BridgeServiceImpl` uses a two-thread single-pass design:

<LanesDiagram
  handoff="eventQueue"
  lanes={[
    {
      title: "Reader Thread",
      sub: "filterPool, bounded",
      role: "io",
      steps: [
        "filter.open(doc)",
        "writer = filter.createFilterWriter()   // same filter instance",
        "while filter.hasNext():",
        "  event = filter.next()",
        "  eventQueue.put(event)              → Writer Thread",
        "  if subscribed(event):",
        "    sendBatch.add(toContentBlock(event))",
        "    if batch full: respObserver.onNext(ContentBlockBatch)",
        "eventQueue.put(END_OF_EVENTS)",
        "writerFuture.get()",
      ],
    },
    {
      title: "Writer Thread",
      sub: "writerPool, unbounded",
      role: "translate",
      steps: [
        "while (event = eventQueue.poll()) != END_OF_EVENTS:",
        "  modified = applier.applyTranslations(event)",
        "  writer.handleEvent(modified)",
      ],
    },
  ]}
/>

Key design choices:

- **Single filter read** — writer is created before iteration, same filter instance as reader. No double I/O (unlike the two-phase approach).
- **Decoupled threads** — reader never blocks on translations, writer never blocks on gRPC sends. Prevents the circular deadlock that occurs when a single thread handles both gRPC sends and translation queue draining.
- **Bounded event queue** (`ArrayBlockingQueue`, capacity 8192) — provides back-pressure without deadlock.
- **Pipeline semaphore** — rejects excess concurrent streams with `RESOURCE_EXHAUSTED`.
- **Separate thread pools** — `filterPool` (bounded, `--concurrency N`) for reader pipelines, `writerPool` (unbounded) for writer threads. Prevents thread starvation when all filterPool threads are busy reading.

### Configuration

| Flag                | Default                 | Description                              |
| ------------------- | ----------------------- | ---------------------------------------- |
| `--concurrency N`   | `availableProcessors()` | Max concurrent filter pipelines          |
| `--idle-timeout N`  | 0 (no timeout)          | Shut down after N seconds idle           |
| `--stuck-timeout N` | 120                     | Translation queue poll timeout (seconds) |

### Heartbeat and Auto-Close

- **Parent heartbeat** (subprocess mode): checks if parent process is alive every 5 seconds; exits if parent dies
- **Idle timeout** (daemon mode): shuts down after N seconds with no active streams
- **Stuck timeout**: aborts pipeline if translation queue poll exceeds timeout

## Host-Side Discovery and Daemon Pool

The Go side of the bridge lives in `cli/pluginhost/` — the host-side runtime
for kapi's unified plugin model (#438). It discovers plugins from on-disk
manifests, builds dispatch tables, and (for Mode-C plugins like the Okapi
bridge) manages a pool of long-lived daemon subprocesses connected over
Unix-socket gRPC. There is no `BridgeRegistry`, `managedBridge`, or
`PluginLoader`; those names belong to a retired Go layer.

### Discovery (`cli/pluginhost/discover.go`)

Discovery is pure filesystem reads — **no subprocess is launched to enumerate
plugins**. `Discover` walks each plugin root, reads `manifest.json` from each
sub-directory, parses it with `manifest.Parse`, and verifies the declared
`plugin` name matches the install directory name. Manifests that fail to parse
or validate are skipped with a warning; missing directories are silently
ignored. Each surviving manifest becomes a `*Plugin` carrying its install
`Dir`, `Source`, parsed `Manifest`, and resolved `BinaryPath`.

Roots are scanned in precedence order (lower `Source.Order` wins on a name
conflict), assembled by `assembleRoots`:

| Order | Root                                        | Source                                    |
| ----- | ------------------------------------------- | ----------------------------------------- |
| 1     | `$KAPI_PLUGINS_DIR` (os-path-list, may be multiple) | env override                      |
| 2     | `$XDG_DATA_HOME/kapi/plugins` (→ `~/.local/share/kapi/plugins`) | per-user install       |
| 3     | system dirs (`/opt/homebrew/share/kapi/plugins`, `/usr/local/share/kapi/plugins`, `/usr/share/kapi/plugins`) | system install |

`NewHost` (`cli/pluginhost/host.go`) folds the discovered plugins into
dispatch tables for commands, MCP tools, formats, and recipe schema
extensions. When two plugins claim the same capability name the conflicting
entry is dropped from the table and a conflict message is emitted, so an
ambiguous capability simply does not dispatch until one plugin is removed.

### Discovery cache (`cli/pluginhost/cache.go`)

To avoid re-reading every manifest on each invocation, discovery results are
cached as JSON. `CacheLocation` resolves to `$KAPI_PLUGIN_CACHE`, else
`$XDG_CACHE_HOME/kapi/plugins-cache.json` (→ `~/.cache/kapi/...`). The cache
records each root's directory mtime; `IsFresh` rejects the cache when the
binary's `cacheVersion` changed, `GOOS`/`GOARCH` differ, the set of roots
changed, or any root's current mtime is newer than the recorded one. A miss
triggers a full rescan + rebuild via `BuildCache`. The cache is rebuilt as a
side effect of install/update/remove.

### Daemon pool (`cli/pluginhost/daemon.go`)

Mode-C plugins are served by a `DaemonPool` owned by the kapi process. The
pool lazily spawns one daemon subprocess per plugin in `Acquire`, reuses a
healthy daemon on subsequent calls, and tears every daemon down on
`Shutdown()`:

- **Lazy spawn + reuse** — the first `Acquire(plugin)` runs
  `plugin.BinaryPath daemon`, reads the daemon's first stdout line as the
  `Handshake` JSON (`{"socket": "...", "version": "..."}`), and dials that
  Unix socket as a gRPC client. Later calls return the cached, healthy
  `DaemonClient` (gRPC `ClientConn` is concurrency-safe, so one client serves
  parallel RPCs).
- **Per-plugin spawn lock** — concurrent first-callers for the same plugin
  serialize on a per-plugin mutex (`spawnLockFor`) so only one JVM is started;
  the losers re-check the cache and reuse the winner's client.
- **Bounded pool, LRU eviction** — `MaxDaemons` caps concurrent daemons. When
  zero it resolves from `$KAPI_MAX_DAEMONS`, falling back to
  `defaultMaxDaemons` (8). Exceeding the cap evicts the least-recently-used
  daemon (`lruLocked`) before spawning a new one.
- **Per-daemon idle timeout** — `idleTimeoutFor` prefers an explicit
  `DaemonPoolOptions.IdleTimeout`, then the manifest's
  `daemon.idle_timeout_seconds`, then `defaultIdleTimeout` (5 minutes). A
  `watchIdle` goroutine terminates a daemon that sits unused past that
  window. (Startup is bounded the same way via `startup_timeout_seconds`,
  default 30s.)
- **External attach** — `$KAPI_DAEMON_SOCKET_<PLUGIN>` (e.g.
  `KAPI_DAEMON_SOCKET_OKAPI_BRIDGE`) points the pool at a pre-started daemon's
  socket, skipping `exec` entirely. `pseudobench` uses this to measure a
  long-lived daemon's per-call cost without paying JVM startup each
  invocation.

Mode C is POSIX-only today: `spawn` returns an error on Windows, since the
transport is Unix sockets.

## Transport and Throughput

The host dials the daemon's Unix socket with `grpc.NewClient(...)` and
insecure transport (`cli/pluginhost/daemon.go`, `dialUnixSocket`) — insecure
is safe because the socket lives under `$TMPDIR` with 0600 mode, owned by the
same user. `waitReady` actively probes the connection to `READY` (or fails on
the startup deadline) so the pool fails fast when a daemon isn't serving.

Throughput on the wire comes from two structural choices rather than tuned
buffer knobs on the Go side:

- **Batched Java→Go transfer** — the Java reader packs subscribed events into
  `ContentBlockBatch` messages of up to 1024 `ContentBlock`s (see the Wire
  Format section above), amortizing gRPC framing over many blocks. This is
  safe because Java sends all blocks before waiting for any translation back.
- **Individual Go→Java sends** — processed parts are sent back one at a time
  via `ProcessRequest.part`, not batched. Batching the return path would
  deadlock: the final partial batch would be held until the processed-parts
  stream closes, which needs `ReadDone` from Java, which needs translations
  from Go — a circular dependency. Per-part delivery breaks the cycle.

The `subscribe_parts = [4]` (Block-only) optimization — letting Java write
structural events directly without a gRPC round-trip — does far more for large
documents than any buffer sizing, cutting message counts by roughly 3-4×.

## Plugin Parameters

Plugin parameters are described by JSON Schema files bundled in the `schemas/` directory of each plugin version. The `FormatSchema` type (`core/format/schema/schema.go`) loads these schemas, which define available configuration options per filter.

Parameters are passed as `map<string, string>` in `ProcessHeader.filter_params`. The Java bridge supports:

- **Flat parameters**: `key=value` pairs applied directly to the Okapi filter
- **Envelope config**: `kind: Okf{Format}FilterConfig` + `spec: {params}` for structured config
- **Config files**: `configFile` path or `fprmContent` inline for native Okapi `.fprm`/YAML config
- **Schema validation**: warnings logged for invalid parameters
- **Parameter flattening**: hierarchical JSON config flattened to Okapi parameter names via `x-flattenPath` schema annotations

## Install Layout

A plugin installs into a single directory named for the plugin, directly
under a discovery root. `InstallFromRegistry` (`cli/pluginhost/install.go`)
writes `<root>/<plugin-name>/`, downloads + verifies the platform asset, and
records provenance in `installed.json`:

```
~/.local/share/kapi/plugins/
  okapi-bridge/
    manifest.json            # plugin, version, binary, daemon, capabilities
    installed.json           # version + registry provenance
    Contents/MacOS/kapi-okapi-bridge   # the daemon binary (manifest.binary)
```

`manifest.json` declares `plugin`, `version` (semver, e.g. `1.48.0`),
`binary` (the executable to exec), an optional `daemon` block
(`idle_timeout_seconds`, `startup_timeout_seconds`, handshake fields), and a
`capabilities` block listing the formats, commands, MCP tools, and schema
extensions the plugin provides. Formats are registered under their bare names
(`okf_html`, `okf_archive`, …) — there is no `@version` aliasing or `→ latest`
resolution in the current model; one directory holds one version of a plugin,
and a duplicate plugin name across roots is resolved by source precedence
(see Discovery above).

The discovery + dispatch surface replaces the retired `PluginLoader` /
`ScanMetadata` / `LoadBridges` / `WarmupBridges` layer entirely:

- **Discovery** — `pluginhost.Discover` reads manifests from disk; no JVM or
  subprocess is launched to enumerate capabilities.
- **Dispatch** — `pluginhost.NewHost` builds the command/MCP/format/schema
  tables and surfaces conflicts.
- **Daemon lifecycle** — `pluginhost.DaemonPool` spawns Mode-C daemons lazily
  on first `Acquire`, reuses them, and tears them down on `Shutdown`. There is
  no eager warmup step; the per-plugin spawn lock keeps the first concurrent
  burst from starting redundant JVMs.
