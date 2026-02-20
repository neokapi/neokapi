---
sidebar_position: 3
title: "Plugin Bridge Protocol"
---
# Plugin Bridge Protocol

This note provides implementation details for [AD-007](/docs/ad/007-plugin-system).

## Java Bridge Protocol

A Go-managed JVM subprocess hosts an adapter that translates between gokapi's Part model and Okapi's Event model. Communication uses synchronous NDJSON over stdin/stdout.

**Commands:** `open`, `read`, `write`, `close`, `info`, `list_filters`
**Responses:** `\{status, data\}` or `\{status, error\}`
**Content:** base64-encoded in both directions

Bridge descriptor files (`*.bridge.json`) describe which JAR to use, JVM arguments, and timeouts. They are discovered by the plugin loader from versioned directories.

The adapter layer (`plugin/bridge/adapter.go`) wraps the bridge protocol behind standard `DataFormatReader` / `DataFormatWriter` interfaces. Bridge-backed formats are indistinguishable from native formats at the registry level. Each adapter carries a `BridgeConfig` identifying its JAR and passes it to the pool on acquire.

## Global Bridge Pool

A single process-wide `BridgePool` manages all JVM instances. The pool is keyed internally by JARPath: idle bridges are bucketed by JAR, but the total number of running JVMs (across all JARs) never exceeds `maxSize` (default: `runtime.NumCPU()`).

```go
type BridgePool struct {
    mu      sync.Mutex
    cond    *sync.Cond
    maxSize int
    active  int                       // total running (idle + in-use)
    idle    map[string][]*JavaBridge   // keyed by JARPath
}
```

### Acquire Algorithm

1. Return idle bridge for requested JAR (LIFO for cache warmth)
2. If capacity available, create new bridge
3. If at capacity but idle bridges exist for a different JAR, **evict one** (stop it, start a new one for the requested JAR)
4. If all bridges are in-use with none idle, block until one is released

The eviction step is critical: without it, the pool deadlocks when all capacity is consumed by one JAR and a request arrives for a different JAR. `sync.Cond` with `Broadcast()` is used because waiters may be for different JARs.

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

## Plugin Loader

The `PluginLoader` (`plugin/loader/`) ties discovery together:

- Scans the plugin directory for versioned subdirectories
- Loads Go binary plugins via `host.PluginManager`
- Loads Java bridge plugins via bridge descriptors (`*.bridge.json`)
- Registers all discovered formats, tools, connectors, and providers into the core registries
- Manages the shared bridge pool and plugin lifecycle
