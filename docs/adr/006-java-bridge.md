# ADR-006: Java bridge and global bridge pool

## Context

Okapi provides 40+ production-proven format filters (DOCX, XLSX, EPUB, IDML,
PDF, etc.) that represent years of development. Rewriting them all in Go is
impractical in the near term. We need a mechanism to access these filters from
Go with minimal overhead.

Each Java bridge is a JVM subprocess with stateful filter state. The JVM
filter lifecycle is: Open (set active filter) -> Read (extract Parts) -> Close.
A bridge can only serve one document at a time. Starting a new JVM per document
is prohibitively expensive (~2-5s startup), and with multi-version plugin
support (ADR-005), per-version pools would allow JVM counts to grow
unboundedly.

## Decision

### Bridge Protocol

Implement a Java bridge: a Go-managed JVM subprocess that hosts an adapter
translating between gokapi's Part model and Okapi's Event model. Communication
uses synchronous NDJSON over stdin/stdout.

**Commands:** `open`, `read`, `write`, `close`, `info`, `list_filters`
**Responses:** `{status, data}` or `{status, error}`
**Content:** base64-encoded in both directions

Bridge descriptor files (`*.bridge.json`) describe which JAR to use, JVM
arguments, and timeouts. They are discovered by the plugin loader from
versioned directories.

### Format Adapters

The adapter layer (`plugin/bridge/adapter.go`) wraps the bridge protocol
behind standard `DataFormatReader` / `DataFormatWriter` interfaces.
Bridge-backed formats are indistinguishable from native formats at the
registry level. Each adapter carries a `BridgeConfig` identifying its JAR
and passes it to the pool on acquire.

### Global Bridge Pool

A single process-wide `BridgePool` manages all JVM instances. The pool is
keyed internally by JARPath: idle bridges are bucketed by JAR, but the total
number of running JVMs (across all JARs) never exceeds `maxSize` (default:
`runtime.NumCPU()`).

```go
type BridgePool struct {
    mu      sync.Mutex
    cond    *sync.Cond
    maxSize int
    active  int                       // total running (idle + in-use)
    idle    map[string][]*JavaBridge   // keyed by JARPath
}
```

Acquire logic:

1. Return idle bridge for requested JAR (LIFO for cache warmth)
2. If capacity available, create new bridge
3. If at capacity but idle bridges exist for a different JAR, **evict one**
   (stop it, start a new one for the requested JAR)
4. If all bridges are in-use with none idle, block until one is released

The eviction step is critical: without it, the pool deadlocks when all
capacity is consumed by one JAR and a request arrives for a different JAR.

`sync.Cond` with `Broadcast()` is used instead of channels because a
channel-based pool cannot route Acquire to per-JAR idle buckets without
complex select logic. `Broadcast` is used over `Signal` because waiters may
be for different JARs.

The first bridge for each descriptor is seeded during plugin loading (needed
for `ListFilters` discovery). The `PluginLoader` creates one pool and shares
it across all bridge descriptors and versions.

### Design Details

- **JARPath as routing key**: different JARs = different filter
  implementations. Two configs with the same JAR are interchangeable.
- **No TTL or reaper**: JVM startup is slow, so keeping idle bridges warm is
  the right default.
- **`PoolStats`**: cheap snapshot of pool state (active, in-use, idle-by-JAR)
  for debugging and monitoring.

## Alternatives Considered

- **JNI / CGo**: tight coupling; crash propagation; complex build.
- **gRPC bridge**: would work but NDJSON over stdio is simpler for the
  synchronous command-response pattern of filter operations.
- **HTTP bridge**: requires port allocation; more complex lifecycle.
- **Per-descriptor pools**: simpler but allows JVM count to scale linearly
  with plugin versions (e.g., 2 versions x NumCPU = 2x JVMs).
- **Channel-based pool with per-JAR sub-channels**: complex select logic
  for multi-bucket routing; hard to implement eviction.
- **Separate pools with a global semaphore**: two layers of synchronization;
  deadlock risk.
- **No eviction (fail instead of evict)**: simpler but fragile; users would
  hit errors when mixing plugin versions.

## Consequences

- All 40+ Okapi filters are available without Go rewrites
- JVM startup cost is amortized: pay once per bridge, reuse across documents
- Total JVM count is bounded by one `maxSize` regardless of plugin versions
- Different JARs share capacity fairly via eviction
- LIFO idle selection keeps recently-used JVMs warm
- Base64 encoding of document content adds ~33% transfer overhead; acceptable
  for the typical document sizes in localization
- Bridge descriptors enable per-version JVM configuration (memory, timeouts)
- Format adapters are transparent: callers see standard DataFormatReader /
  DataFormatWriter interfaces
