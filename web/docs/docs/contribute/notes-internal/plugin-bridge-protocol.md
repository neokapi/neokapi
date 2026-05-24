---
sidebar_position: 3
title: "Okapi Bridge Protocol"
description: Implementation note for AD-007 — the gRPC bridge protocol between neokapi and the Okapi Java subprocess, covering the BridgeService RPCs (Process, ProcessStep, Shutdown) and the Part-to-Event translation.
keywords: [bridge protocol, gRPC, BridgeService, Okapi bridge, Process RPC, Part model, Event model, implementation note]
---

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

```
┌─ Reader Thread (filterPool, bounded) ─────────────────────────┐
│  filter.open(doc)                                             │
│  writer = filter.createFilterWriter()  ← same filter instance │
│  while filter.hasNext():                                      │
│    event = filter.next()                                      │
│    eventQueue.put(event) ─────────────→ Writer Thread          │
│    if subscribed(event):                                      │
│      sendBatch.add(toContentBlock(event))                     │
│      if batch full: respObserver.onNext(ContentBlockBatch)    │
│  eventQueue.put(END_OF_EVENTS)                                │
│  writerFuture.get()                                           │
└───────────────────────────────────────────────────────────────┘

┌─ Writer Thread (writerPool, unbounded) ───────────────────────┐
│  while (event = eventQueue.poll()) != END_OF_EVENTS:          │
│    modified = applier.applyTranslations(event)                │
│    writer.handleEvent(modified)                               │
└───────────────────────────────────────────────────────────────┘
```

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

## Bridge Registry

A single process-wide `BridgeRegistry` manages bridge instances:

```go
type BridgeRegistry struct {
    bridges   map[string]*managedBridge  // keyed by config hash
    global    chan struct{}              // global concurrency semaphore
    maxPerJVM int                       // per-JVM concurrency limit
    daemon    bool                      // persist JVMs across invocations
}
```

### Concurrency Control

- **Global semaphore** (`maxTotal`, default `NumCPU`): bounds total concurrent streams across all JVMs
- **Per-JVM semaphore** (`maxPerJVM`, default 8): bounds concurrent streams on each JVM
- `Acquire(cfg)` returns a bridge + release function; blocks if at capacity

### Daemon Mode

When `KAPI_BRIDGE_DAEMON=1`:

- JVMs persist after kapi exits (no Shutdown RPC sent)
- Discovered via address files in `~/.cache/neokapi/bridge/`
- `KAPI_BRIDGE_IDLE_TIMEOUT` controls JVM auto-shutdown (default 30s)
- Eliminates JVM startup cost for subsequent invocations

### Warmup

`WarmupBridges()` eagerly starts one JVM per bridge configuration before concurrent file processing begins, amortizing the ~1.3s JVM startup cost.

## gRPC Performance Tuning

Both Go client and Java server are tuned for localhost throughput:

### Go Client

| Setting           | Value  | Purpose                              |
| ----------------- | ------ | ------------------------------------ |
| Write buffer      | 256 KB | Coalesce small writes (default 32KB) |
| Read buffer       | 256 KB | Reduce read syscalls                 |
| Stream window     | 4 MB   | Per-stream flow control headroom     |
| Connection window | 8 MB   | Per-connection flow control          |
| Max recv msg      | 64 MB  | Large ContentBlockBatch messages     |
| readParts channel | 4096   | Absorb large batch unpacks           |

### Java Server (Netty)

| Setting                | Value | Purpose                           |
| ---------------------- | ----- | --------------------------------- |
| Flow control window    | 4 MB  | Match Go client window            |
| Max inbound msg        | 64 MB | Large batch messages              |
| ContentBlockBatch size | 1024  | Blocks per gRPC message (Java→Go) |

### Go→Java Send Strategy

Processed parts are sent back individually (not batched) from Go to Java.
Batching from Go→Java causes a deadlock: the final partial batch is held until
the `processedParts` channel closes, which requires `ReadDone` from Java, which
requires translations from Go — circular dependency. Individual sends avoid this
because each part is delivered immediately.

Java→Go batching (ContentBlockBatch of 1024) is safe because Java sends all
blocks before waiting for translations.

## Plugin Parameters

Plugin parameters are described by JSON Schema files bundled in the `schemas/` directory of each plugin version. The `FormatSchema` type (`core/format/schema/schema.go`) loads these schemas, which define available configuration options per filter.

Parameters are passed as `map<string, string>` in `ProcessHeader.filter_params`. The Java bridge supports:

- **Flat parameters**: `key=value` pairs applied directly to the Okapi filter
- **Envelope config**: `kind: Okf{Format}FilterConfig` + `spec: {params}` for structured config
- **Config files**: `configFile` path or `fprmContent` inline for native Okapi `.fprm`/YAML config
- **Schema validation**: warnings logged for invalid parameters
- **Parameter flattening**: hierarchical JSON config flattened to Okapi parameter names via `x-flattenPath` schema annotations

## Multi-Version Directory Layout

```
~/.local/share/kapi/plugins/
  okapi/
    2.17.0/
      manifest.json
      schemas/
      neokapi-bridge-jar-with-dependencies.jar
    2.18.0/
      manifest.json
      schemas/
      neokapi-bridge-jar-with-dependencies.jar
```

Each version directory contains a `manifest.json` with capabilities and command configuration. The plugin loader registers capabilities with versioned names (`okf_html@2.17.0`) and bare aliases (`okf_html` → latest).

## Plugin Loader

The `PluginLoader` (`plugin/loader/`) ties discovery together:

- **`ScanMetadata`**: reads `manifest.json` and `schemas/` from disk — no JVM needed
- **`LoadBridges`**: creates the shared `BridgeRegistry`, registers reader factories
- **`WarmupBridges`**: eagerly starts JVMs before concurrent processing
- **`Shutdown`**: stops all bridges and plugins
