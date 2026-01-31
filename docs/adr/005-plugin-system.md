# ADR-005: Plugin system

**Status:** Accepted

## Context

A plugin system is needed for third-party format readers, writers, and tools.
Plugins evolve independently of the core framework and of each other. The key
requirements are:

- Crash isolation: a buggy plugin must not crash the host process
- Language agnosticism: plugins could be written in Go, Java, Python, Rust
- Versioned protocol: host and plugin can negotiate capabilities
- Multi-version support: different projects may need different plugin versions
- Simple discovery: scan a directory for executables

Go's standard `plugin` package loads shared objects into the same process,
offering no crash isolation and limited platform support.

## Decision

### Out-of-Process Plugins via go-plugin

Use [HashiCorp go-plugin](https://github.com/hashicorp/go-plugin) with gRPC
transport. Each plugin is a separate executable that communicates with the host
over stdin/stdout using gRPC. Protocol buffers define the service contract.

Plugin discovery scans a directory for executables matching the naming
convention `gokapi-format-*` or `gokapi-tool-*`. The host launches each
plugin, performs a version handshake, queries capabilities via the `Info()` RPC,
and registers the plugin's formats/tools into the appropriate registry.

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
formats with versioned names:

- `okapi-html@1.46.0` -- specific version
- `okapi-html` -- bare alias pointing to the latest installed version

Semver comparison determines "latest". Bare-name aliases are registered only
if no explicit bare-name registration exists, preventing conflicts.

### Plugin Registry

The plugin registry (`plugin/registry/`) provides:

- `ListAllInstalled(dir)` -- scan for all versions of all plugins
- `CompareSemver(a, b)` -- semantic version comparison
- Version resolution for CLI commands (`kapi plugins list`,
  `kapi plugins install`)

### Plugin Loader

The `PluginLoader` (`plugin/loader/`) ties discovery together:

- Scans the plugin directory for versioned subdirectories
- Loads Go binary plugins via `host.PluginManager`
- Loads Java bridge plugins via bridge descriptors (`*.bridge.json`)
- Registers all discovered formats and tools into the core registries
- Manages the shared bridge pool (see ADR-006) and plugin lifecycle

## Alternatives Considered

- **Go `plugin` package**: in-process; no crash isolation; Linux/macOS only;
  version skew issues with Go runtime.
- **HTTP/REST plugins**: heavier protocol; requires port allocation; less
  natural for streaming.
- **WASM plugins**: promising but immature Go WASM host support; limited
  system access.
- **Single version only**: simpler but forces all projects onto the same
  plugin version; no gradual rollouts.
- **Docker-based isolation**: heavyweight; doesn't work for desktop app.

## Consequences

- Plugins run out-of-process: crashes do not affect the host
- Any language that can implement a gRPC server works as a plugin
- Protocol versioning in the handshake prevents incompatible plugins from
  loading
- Multiple versions of the same plugin coexist without conflict
- Users can pin specific versions per project in `gokapi.yaml`
- Bare-name aliases provide a convenient default (latest version)
- Plugin install/upgrade/remove operations are directory-level: copy or
  delete a version directory
- Plugin startup adds latency (~100-500ms per plugin); acceptable for
  long-running pipelines
- Proto definitions in `plugin/proto/` define the contract between host
  and plugins
