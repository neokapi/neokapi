---
sidebar_position: 3
title: "Bridge Protocol"
---
# Bridge Protocol

This note provides implementation details for [AD-007](/docs/ad/007-plugin-system).

## gRPC Bridge Protocol

The Okapi bridge is a subprocess that hosts Okapi Framework filters and exposes them via a gRPC service. The Go side (`core/plugin/bridge/`) manages the subprocess lifecycle, connects as a gRPC client, and translates between gokapi's Part model and Okapi's Event model.

The protocol is defined in `core/plugin/proto/v2/gokapi_bridge.proto`:

```protobuf
service BridgeService {
  rpc Info(InfoRequest) returns (InfoResponse);
  rpc ListFilters(ListFiltersRequest) returns (ListFiltersResponse);
  rpc Open(OpenRequest) returns (OpenResponse);
  rpc Read(ReadRequest) returns (stream PartMessage);
  rpc Write(stream WriteChunk) returns (WriteResponse);
  rpc Close(CloseRequest) returns (CloseResponse);
  rpc Shutdown(ShutdownRequest) returns (ShutdownResponse);
}
```

### RPC Lifecycle

A typical read cycle:

1. **ListFilters** — called once during plugin discovery to register available formats
2. **Open** — sends the document content (or a file path) and filter configuration
3. **Read** — streams `PartMessage` values (Blocks, Layers, Data, Groups, Media) as the filter extracts them
4. **Close** — releases filter resources; the bridge becomes available for the next document

A write cycle:

1. **Open** — opens the filter with the original document for skeleton reconstruction
2. **Write** — client-streams a `WriteHeader` (filter class, locale, original content) followed by `PartMessage` values; returns the reconstructed document bytes
3. **Close** — releases resources

### Subprocess Startup

On launch, the bridge subprocess starts a gRPC server on a random port and prints the socket address to stdout. The Go side reads this address, connects as a gRPC client, and the bridge is ready. The `BridgeConfig.StartupTimeout` controls how long to wait.

### Content Transfer

For small files, document content is sent inline in the `OpenRequest.content` or `WriteHeader.original_content` fields (protobuf `bytes`). For large files or formats that require auxiliary file resolution (e.g., XLIFF with external ITS references), the `source_path` field passes an absolute file path and the bridge reads directly from disk.

The `Read` and `Write` RPCs use gRPC streaming, so Parts flow incrementally without buffering the entire document in memory.

## Global Bridge Pool

A single process-wide `BridgePool` manages all bridge subprocess instances. The pool is keyed by process configuration (command + args): idle bridges are bucketed by key, but the total number of running subprocesses (across all keys) never exceeds `maxSize` (default: `runtime.NumCPU()`).

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

### Acquire Algorithm

1. Return idle bridge for requested key (LIFO for cache warmth)
2. If capacity available, create new bridge
3. If at capacity but idle bridges exist for a different key, **evict one** (stop it, start a new one for the requested key)
4. If all bridges are in-use with none idle, block until one is released

The eviction step is critical: without it, the pool deadlocks when all capacity is consumed by one key and a request arrives for a different key. `sync.Cond` with `Broadcast()` is used because waiters may be for different keys.

### Health Checks

On release back to the pool, bridges undergo a health check to verify the gRPC connection is still alive. Unhealthy bridges are discarded rather than returned to the idle set, preventing stale subprocesses from causing hangs on subsequent use.

### Seeding

The first bridge for each descriptor is seeded during plugin loading (needed for `ListFilters` discovery). The `PluginLoader` creates one pool and shares it across all bridge descriptors and versions.

## ParameterDescriptor

Plugins declare their configuration parameters as part of the `Info()` RPC response:

```go
type ParameterDescriptor struct {
    Name         string      `json:"name"`
    Type         string      `json:"type"`          // string, int, float, bool, []string, file
    Default      interface{} `json:"default"`
    Description  string      `json:"description"`
    Required     bool        `json:"required"`
    Choices      []string    `json:"choices,omitempty"`
    Sensitive    bool        `json:"sensitive"`          // API keys, passwords
}
```

Plugin configuration lives under a namespace in the project configuration file, scoped by plugin type and name:

```yaml
# kapi.yaml
formats:
  docx:
    extract_comments: true
    track_changes: accept

tools:
  terminology:
    glossary: ./glossaries/corporate.tbx
    match_threshold: 85

providers:
  custom-llm:
    endpoint: https://llm.internal.company.com/v1
    model: company-translate-7b
```

The host reads plugin configuration from the Viper config tree and passes it to the plugin via the `Open` or `Configure` RPC. Plugins never read `kapi.yaml` directly.

On the CLI, plugin parameters become namespaced flags to avoid collisions:

```bash
kapi convert -i report.docx -o report.xliff \
  --docx.extract-comments \
  --docx.track-changes=reject
```

## Multi-Version Directory Layout

Multiple versions of the same plugin can be installed side-by-side:

```
~/.config/gokapi/plugins/
  okapi/
    1.46.0/
      version.json
      okapi.bridge.json
      gokapi-okapi-bridge.jar
    1.47.0/
      version.json
      okapi.bridge.json
      gokapi-okapi-bridge.jar
```

Each version directory contains a `version.json` manifest with name, version, and install type. The plugin loader scans all version directories and registers capabilities with versioned names:

- `okapi-html@1.46.0` -- specific version
- `okapi-html` -- bare alias pointing to the latest installed version

Semver comparison determines "latest". Bare-name aliases are registered only if no explicit bare-name registration exists, preventing conflicts.

## Bundles

A **bundle** is a plugin that provides multiple formats and/or tools as a single
distributable unit. In the remote registry, bundles are declared with
`plugin_type: "bundle"` and list their capabilities explicitly:

```json
{
  "name": "okapi",
  "version": "1.47.0",
  "plugin_type": "bundle",
  "install_type": "bridge",
  "capabilities": [
    {"type": "format", "name": "html", "display_name": "HTML", "mime_types": ["text/html"]},
    {"type": "format", "name": "openxml", "display_name": "Microsoft Office (OpenXML)", "extensions": [".docx", ".xlsx", ".pptx"]},
    {"type": "tool", "name": "segmentation", "display_name": "SRX Segmentation"}
  ]
}
```

The Okapi bridge is the canonical bundle example. Bridge-backed bundles use the
same `*.bridge.json` descriptor and gRPC subprocess protocol described above.
Go binary bundles can also exist — they simply register multiple format readers,
writers, and/or tools via the go-plugin handshake.

### CLI Search and Filtering

The `kapi plugins search` command provides flags for filtering by plugin kind:

| Flag | Effect |
|------|--------|
| `--bundle` | Only show bundles |
| `--format` | Only show plugins providing format capabilities (includes bundles with formats) |
| `--tool` | Only show plugins providing tool capabilities (includes bundles with tools) |

These flags combine with AND logic alongside `--type`, `--mime`, and `--ext`. For
example, `--bundle --format` returns only bundles that contain format capabilities.

## Plugin Loader

The `PluginLoader` (`plugin/loader/`) ties discovery together:

- Scans the plugin directory for versioned subdirectories
- Loads Go binary plugins via `host.PluginManager`
- Loads bridge plugins via bridge descriptors (`*.bridge.json`)
- Registers all discovered formats, tools, connectors, and providers into the core registries
- Manages the shared bridge pool and plugin lifecycle
- Bundles are loaded the same way as standalone plugins; their individual capabilities are registered separately into format and tool registries
