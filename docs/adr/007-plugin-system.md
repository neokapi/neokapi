---
id: 007-plugin-system
sidebar_position: 7
title: "ADR-007: Plugin System and Okapi Bridge"
---
# ADR-007: Plugin System and Okapi Bridge

## Context

Plugins enable third-party formats, tools, connectors, and providers that evolve
independently of the core framework and of each other. The key requirements are:

- **Crash isolation** — a buggy plugin must not crash the host process
- **Language agnosticism** — plugins could be written in Go, Java, Python, Rust
- **Versioned protocol** — host and plugin can negotiate capabilities
- **Multi-version support** — different projects may need different plugin versions
- **Simple discovery** — scan a directory for executables matching naming conventions

Okapi provides 40+ production-proven format filters (DOCX, XLSX, EPUB, IDML, PDF,
etc.) that represent years of development. Rewriting them all in Go is impractical
in the near term. These filters need to be accessible from Go with minimal overhead.

Go's standard `plugin` package loads shared objects into the same process, offering
no crash isolation and limited platform support (Linux/macOS only).

## Decision

### Out-of-Process Plugins via go-plugin

Use [HashiCorp go-plugin](https://github.com/hashicorp/go-plugin) with gRPC
transport. Each plugin is a separate executable that communicates with the host
over stdin/stdout using gRPC. Protocol buffers define the service contract.

Plugin discovery scans a directory for executables matching the naming convention
for each plugin type. The host launches each plugin, performs a version handshake,
queries capabilities via the `Info()` RPC, and registers the plugin's capabilities
into the appropriate registry.

### Plugin Types

| Type | What it adds | Discovery | Example |
|------|-------------|-----------|---------|
| **Format** | Reader + Writer for a file format | `gokapi-format-*` | `gokapi-format-docx` |
| **Tool** | Processing step in a flow | `gokapi-tool-*` | `gokapi-tool-terminology` |
| **Connector** | Bidirectional system integration | `gokapi-connector-*` | `gokapi-connector-contentful` |
| **Provider** | AI/LLM or MT backend | `gokapi-provider-*` | `gokapi-provider-deepl-v2` |

The **connector** type enables community-built connectors for CMS platforms,
design tools, and other systems that integrate with gokapi's connector-first
architecture ([ADR-005](./005-connector-system.md)). Connector plugins implement
the same bidirectional sync interface as built-in connectors, pulling content into
the store and pushing translations back.

### Multi-Version Support

Multiple versions of the same plugin can be installed side-by-side using a
versioned directory layout:

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

Each version directory contains a `version.json` manifest with name, version,
and install type. The plugin loader scans all version directories and registers
capabilities with versioned names:

- `okapi-html@1.46.0` -- specific version
- `okapi-html` -- bare alias pointing to the latest installed version

Semver comparison determines "latest". Bare-name aliases are registered only
if no explicit bare-name registration exists, preventing conflicts.

### Plugin Loader

The `PluginLoader` (`plugin/loader/`) ties discovery together:

- Scans the plugin directory for versioned subdirectories
- Loads Go binary plugins via `host.PluginManager`
- Loads Java bridge plugins via bridge descriptors (`*.bridge.json`)
- Registers all discovered formats, tools, connectors, and providers into the
  core registries
- Manages the shared bridge pool and plugin lifecycle

### Java Bridge

A Go-managed JVM subprocess hosts an adapter that translates between gokapi's
Part model and Okapi's Event model. Communication uses synchronous NDJSON over
stdin/stdout.

**Commands:** `open`, `read`, `write`, `close`, `info`, `list_filters`
**Responses:** `{status, data}` or `{status, error}`
**Content:** base64-encoded in both directions

Bridge descriptor files (`*.bridge.json`) describe which JAR to use, JVM
arguments, and timeouts. They are discovered by the plugin loader from versioned
directories.

The adapter layer (`plugin/bridge/adapter.go`) wraps the bridge protocol behind
standard `DataFormatReader` / `DataFormatWriter` interfaces. Bridge-backed formats
are indistinguishable from native formats at the registry level. Each adapter
carries a `BridgeConfig` identifying its JAR and passes it to the pool on acquire.

### Global Bridge Pool

A single process-wide `BridgePool` manages all JVM instances. The pool is keyed
internally by JARPath: idle bridges are bucketed by JAR, but the total number of
running JVMs (across all JARs) never exceeds `maxSize` (default:
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

The eviction step is critical: without it, the pool deadlocks when all capacity
is consumed by one JAR and a request arrives for a different JAR. `sync.Cond`
with `Broadcast()` is used because waiters may be for different JARs.

The first bridge for each descriptor is seeded during plugin loading (needed
for `ListFilters` discovery). The `PluginLoader` creates one pool and shares
it across all bridge descriptors and versions.

### Plugin Configuration

Plugins declare their configuration parameters as part of the `Info()` RPC
response. Each parameter has a name, type, default value, description, and
required/optional status:

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

Plugin configuration lives under a namespace in the project configuration file,
scoped by plugin type and name. This integrates with the Viper-based layered
configuration system ([ADR-001](./001-vision.md)):

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

The host reads plugin configuration from the Viper config tree and passes it to
the plugin via the `Open` or `Configure` RPC. Plugins never read `kapi.yaml`
directly.

On the CLI, plugin parameters become namespaced flags to avoid collisions:

```bash
kapi convert -i report.docx -o report.xliff \
  --docx.extract-comments \
  --docx.track-changes=reject
```

### Plugin Governance

**Protocol stability.** The v1 protocol (defined in `plugin/proto/v1/gokapi.proto`)
is frozen. New fields can be added (protobuf is forward-compatible), but existing
fields cannot be changed or removed. New capabilities require a new protocol
version (v2, v3, etc.). The host supports at least two protocol versions
simultaneously, giving plugin authors time to migrate.

**Quality tiers.** Not all plugins carry the same trust level:

| Tier | Source | Trust | Installation |
|------|--------|-------|-------------|
| **Built-in** | `formats/`, `lib/tools/` | Full | Ships with binary |
| **Official** | gokapi org registry | High | `kapi plugins install` |
| **Community** | Third-party registry | Medium | `kapi plugins install --source <url>` |
| **Local** | User-built | User's risk | Copy to plugin directory |

**Built-in vs plugin split.** Formats with broad usage (HTML, XML, JSON, YAML,
XLIFF, PO, Markdown, CSV, SRT, VTT) are built-in. Specialized or proprietary
formats (DOCX, IDML, InDesign, MIF, DITA) are plugins. Core tools (pseudo-translate,
QA check, word count, segmentation, TM leverage) are built-in. Integration tools
(specific TMS connectors, custom MT engines) are plugins. AI providers (Anthropic,
OpenAI, Ollama) are built-in because AI translation is a core value proposition.

The test: if removing it makes gokapi feel incomplete for the common case, it
should be built-in.

### Project Portability

KAZ manifests ([ADR-003](./003-content-store.md)) record plugin dependencies
to prevent "works on my machine" problems:

```yaml
# manifest.yaml inside project.kaz
plugins:
  formats:
    - name: gokapi-format-docx
      version: "1.2.0"
      min_version: "1.1.0"
      required: true
  tools:
    - name: gokapi-tool-terminology
      version: "1.0.3"
      min_version: "1.0.0"
      required: false           # Flow works without it, skips term annotation
```

When a project is opened, gokapi validates plugin availability:

```
$ kapi merge -i project.kaz
Error: project requires plugins not available locally:
  X gokapi-format-docx >= 1.1.0 (not installed)
  OK gokapi-tool-terminology >= 1.0.0 (installed: 1.0.3)

Install missing plugins with:
  kapi plugins install gokapi-format-docx
```

When an optional plugin is missing, the flow skips that tool and logs a warning
rather than failing. This allows projects to degrade gracefully when shared
across environments with different plugin sets.

## Alternatives Considered

- **Go `plugin` package** — in-process; no crash isolation; Linux/macOS only;
  version skew issues with Go runtime.
- **HTTP/REST plugins** — heavier protocol; requires port allocation; less
  natural for streaming.
- **WASM plugins** — promising but immature Go WASM host support; limited
  system access.
- **JNI/CGo for Java bridge** — tight coupling; crash propagation; complex build.
  NDJSON over stdio is simpler for the synchronous command-response pattern of
  filter operations.
- **gRPC for bridge** — would work but NDJSON over stdio is simpler for the
  synchronous command-response pattern of filter operations.
- **Per-descriptor bridge pools** — simpler but allows JVM count to scale
  linearly with plugin versions (e.g., 2 versions x NumCPU = 2x JVMs).
- **Docker-based isolation** — heavyweight; doesn't work for desktop app.

## Consequences

- Plugins run out-of-process: crashes do not affect the host
- Any language that can implement a gRPC server works as a plugin
- Protocol versioning in the handshake prevents incompatible plugins from loading
- Multiple versions of the same plugin coexist without conflict
- All 40+ Okapi filters are accessible without Go rewrites via the Java bridge
- JVM count is bounded by one `maxSize` regardless of plugin versions; different
  JARs share capacity fairly via eviction
- Connector plugins extend gokapi's integration capabilities, enabling
  community-built connectors for CMS platforms and design tools
  ([ADR-005](./005-connector-system.md))
- Plugin configuration integrates cleanly with the Viper config system
  ([ADR-001](./001-vision.md)); namespaced flags prevent collisions
- KAZ manifests record plugin dependencies for project portability
  ([ADR-003](./003-content-store.md))
- Plugin scope is enforced by the RPC boundary -- plugins cannot modify core
  behavior
- The built-in vs plugin split keeps the out-of-box experience complete while
  allowing specialization through plugins
