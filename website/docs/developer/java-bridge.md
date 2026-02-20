---
sidebar_position: 6
title: Java Bridge
---

# Java Bridge

The Java bridge provides access to all 40+ Okapi Framework filters without rewriting them in Go. It works by running a JVM subprocess that hosts an adapter translating between gokapi's Part model and Okapi's Event model.

## How It Works

Communication uses synchronous NDJSON over stdin/stdout:

- **Commands:** `open`, `read`, `write`, `close`, `info`, `list_filters`
- **Responses:** `{status, data}` or `{status, error}`
- **Content:** base64-encoded in both directions

Bridge-backed formats are registered into the standard `FormatRegistry` and are indistinguishable from native formats at the API level.

## Global Bridge Pool

A single process-wide `BridgePool` manages all JVM instances. The pool is keyed by JAR path, but the total number of running JVMs never exceeds `maxSize` (default: `runtime.NumCPU()`).

```go
type BridgePool struct {
    mu      sync.Mutex
    cond    *sync.Cond
    maxSize int
    active  int                       // total running (idle + in-use)
    idle    map[string][]*JavaBridge   // keyed by JARPath
}
```

### Acquire Logic

1. Return idle bridge for requested JAR (LIFO for cache warmth)
2. If capacity available, create new bridge
3. If at capacity but idle bridges exist for a different JAR, evict one
4. If all bridges are in-use with none idle, block until one is released

## Bridge Descriptor

Each plugin version includes a `*.bridge.json` descriptor:

```json
{
  "jar": "gokapi-okapi-bridge.jar",
  "jvmArgs": ["-Xmx512m"],
  "filters": ["html", "xml", "docx", "xlsx", "epub"],
  "timeout": "30s"
}
```

## Available Filters

Through the Java bridge, gokapi can access Okapi's full filter library including DOCX, XLSX, EPUB, IDML, PDF, DITA, FrameMaker, InDesign, and many more.

See [AD-007](/docs/ad/007-plugin-system) for the complete design rationale.
