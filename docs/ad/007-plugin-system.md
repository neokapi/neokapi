---
id: 007-plugin-system
sidebar_position: 7
title: "AD-007: Plugin System and Okapi Bridge"
---
# AD-007: Plugin System and Okapi Bridge

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
architecture ([AD-005](./005-connector-system.md)). Connector plugins implement
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

A Go-managed JVM subprocess hosts an adapter that translates between gokapi's Part model and Okapi's Event model. Communication uses synchronous NDJSON over stdin/stdout with base64-encoded content. The adapter layer wraps the bridge protocol behind standard `DataFormatReader` / `DataFormatWriter` interfaces.

### Global Bridge Pool

A single process-wide `BridgePool` manages all JVM instances, keyed by JARPath, with a global capacity limit of `runtime.NumCPU()`. The pool uses LIFO reuse, cross-JAR eviction to prevent deadlocks, and `sync.Cond` with `Broadcast()` for fair waiting.

See [Plugin Bridge Protocol](/docs/notes/plugin-bridge-protocol) for bridge commands/responses, BridgePool acquire algorithm, and bridge descriptor format.

### Plugin Configuration

Plugins declare their configuration parameters via the `Info()` RPC response using `ParameterDescriptor` structs. Configuration is namespaced by plugin type and name in the project config file, integrated with the Viper-based layered configuration system ([AD-001](./001-vision.md)). On the CLI, plugin parameters become namespaced flags.

See [Plugin Bridge Protocol](/docs/notes/plugin-bridge-protocol) for the ParameterDescriptor struct and configuration examples.

### Plugin Governance

**Protocol stability.** The v1 protocol (defined in `plugin/proto/v1/gokapi.proto`)
is frozen. New fields can be added (protobuf is forward-compatible), but existing
fields cannot be changed or removed. New capabilities require a new protocol
version (v2, v3, etc.). The host supports at least two protocol versions
simultaneously, giving plugin authors time to migrate.

**Quality tiers.** Not all plugins carry the same trust level:

| Tier | Source | Trust | Installation |
|------|--------|-------|-------------|
| **Built-in** | `formats/`, `tools/` | Full | Ships with binary |
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

KAZ manifests ([AD-003](./003-content-store.md)) record plugin dependencies
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
  ([AD-005](./005-connector-system.md))
- Plugin configuration integrates cleanly with the Viper config system
  ([AD-001](./001-vision.md)); namespaced flags prevent collisions
- KAZ manifests record plugin dependencies for project portability
  ([AD-003](./003-content-store.md))
- Plugin scope is enforced by the RPC boundary -- plugins cannot modify core
  behavior
- The built-in vs plugin split keeps the out-of-box experience complete while
  allowing specialization through plugins
