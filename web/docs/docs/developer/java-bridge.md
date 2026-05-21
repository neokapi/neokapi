---
sidebar_position: 6
title: Okapi Bridge
---

# Okapi Bridge

The Okapi bridge provides access to the Okapi Framework filters without rewriting them in Go. It works by running a subprocess that hosts an adapter translating between neokapi's Part model and Okapi's Event model. The current implementation runs a JVM, but the bridge protocol is gRPC-based and language-agnostic.

## How It Works

The bridge subprocess starts a gRPC server and prints its socket address to stdout. The Go side connects as a client and communicates via the `BridgeService` defined in `core/plugin/proto/v2/neokapi_bridge.proto`:

| RPC           | Direction        | Purpose                                  |
| ------------- | ---------------- | ---------------------------------------- |
| `ListFilters` | Unary            | Discover available filters at startup    |
| `Info`        | Unary            | Get metadata for a specific filter       |
| `Open`        | Unary            | Open a document with a filter            |
| `Read`        | Server-streaming | Stream extracted Parts from the document |
| `Write`       | Client-streaming | Send Parts to reconstruct the document   |
| `Close`       | Unary            | Release filter resources                 |
| `Shutdown`    | Unary            | Gracefully stop the bridge               |

`Read` and `Write` use gRPC streaming — content flows incrementally without buffering the entire document in memory, which is critical for large files (e.g., XLSX).

Bridge-backed formats are registered into the standard `FormatRegistry` and are indistinguishable from native formats at the API level.

## Global Bridge Pool

A single process-wide `BridgePool` manages all bridge subprocess instances. The pool is keyed by process configuration (command + args), but the total number of running subprocesses never exceeds `maxSize` (default: `runtime.NumCPU()`).

```go
type BridgePool struct {
    mu      sync.Mutex
    cond    *sync.Cond
    maxSize int
    active  int                       // total running (idle + in-use)
    closed  bool
    logger  *log.Logger
    idle    map[string][]*JavaBridge   // keyed by PoolKey
}
```

### Acquire Logic

1. Return idle bridge for requested key (LIFO for cache warmth)
2. If capacity available, create new bridge
3. If at capacity but idle bridges exist for a different key, evict one
4. If all bridges are in-use with none idle, block until one is released

Bridges are health-checked on release to prevent stale subprocesses from causing hangs.

## Bridge Descriptor

Each plugin version includes a `*.bridge.json` descriptor:

```json
{
  "jar": "neokapi-okapi-bridge.jar",
  "jvmArgs": ["-Xmx512m"],
  "filters": ["html", "xml", "docx", "xlsx", "epub"],
  "timeout": "30s"
}
```

## Available Filters

Through the Okapi bridge, neokapi can access Okapi's full filter library including DOCX, XLSX, EPUB, IDML, PDF, DITA, FrameMaker, InDesign, and many more.

See [AD-007](/architecture/007-plugin-system) for the complete design rationale.
