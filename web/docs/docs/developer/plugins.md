---
sidebar_position: 5
title: Plugin System
---

# Plugin System

neokapi uses [HashiCorp go-plugin](https://github.com/hashicorp/go-plugin) with gRPC transport for out-of-process plugins. Each plugin is a separate executable that communicates with the host over stdin/stdout.

## Plugin Types

Plugins are classified into three main types:

- **Bundle** — a collection of formats and/or tools distributed as a single unit (e.g., the Okapi bridge with its Java format filters)
- **Format** — a standalone format reader/writer plugin implementing `DataFormatReaderPlugin` and/or `DataFormatWriterPlugin` gRPC services
- **Tool** — a standalone tool plugin implementing `ToolPlugin` gRPC service

### Bundles

A bundle packages multiple formats and tools into one installable plugin. The Okapi bridge is the canonical example — it provides many format filters (DOCX, XLSX, EPUB, HTML, etc.) and processing tools via a single bridge subprocess.

Bundles are declared with `plugin_type: "bundle"` in the registry manifest and list their capabilities explicitly. This allows the CLI to search and filter by contained capability type:

```bash
kapi plugins search --bundle         # list all bundles
kapi plugins search --format         # formats (including those inside bundles)
kapi plugins search --tool           # tools (including those inside bundles)
kapi plugins search --bundle --tool  # bundles that contain tool capabilities
```

When a bundle is installed, its individual capabilities (formats, tools) are registered separately into the core registries. This means flows and commands can reference individual formats from a bundle (e.g., `okapi-html`) without knowing they came from a bundle.

## Plugin Discovery

Plugins are discovered by scanning a directory for executables matching the naming convention:

- `neokapi-format-*` — format reader/writer plugins
- `neokapi-tool-*` — tool plugins

The host launches each plugin, performs a version handshake, queries capabilities via `Info()`, and registers into the appropriate registry. Bundles (like bridge plugins) are discovered via `*.bridge.json` descriptors and may register many capabilities at once.

## Multi-Version Support

Multiple versions of the same plugin (or bundle) can be installed side-by-side:

```
~/.config/kapi/plugins/
  okapi/
    1.46.0/
      version.json
      okapi.bridge.json
      neokapi-okapi-bridge.jar
    1.47.0/
      version.json
      okapi.bridge.json
      neokapi-okapi-bridge.jar
```

Formats register with versioned names (`okapi-html@1.46.0`) and bare aliases (`okapi-html`) pointing to the latest version.

## Writing a Format Plugin

A format plugin is a Go binary that serves one or more gRPC services:

```go
package main

import (
    "github.com/hashicorp/go-plugin"
    gp "github.com/neokapi/neokapi/plugin"
)

func main() {
    plugin.Serve(&plugin.ServeConfig{
        HandshakeConfig: gp.Handshake,
        Plugins: map[string]plugin.Plugin{
            "format_reader": &gp.DataFormatReaderGRPCPlugin{
                Impl: NewMyFormatReader(),
            },
        },
        GRPCServer: plugin.DefaultGRPCServer,
    })
}
```

## Writing a Bundle Plugin

A bundle is typically distributed as a bridge (`.bridge.json` + JAR or other executable) but can also be a Go binary that registers multiple capabilities. Bridge-based bundles communicate with the host via gRPC. See [Bridge Protocol](/notes-internal/plugin-bridge-protocol) for details.

For Go-based bundles, register multiple `format_reader`, `format_writer`, and `tool` services in the `ServeConfig.Plugins` map.

## gRPC Protocol

```protobuf
service DataFormatReaderPlugin {
    rpc Open(OpenRequest) returns (OpenResponse);
    rpc Read(ReadRequest) returns (stream PartMessage);
    rpc Close(CloseRequest) returns (CloseResponse);
    rpc Info(InfoRequest) returns (FormatInfo);
}

service ToolPlugin {
    rpc Process(stream PartMessage) returns (stream PartMessage);
    rpc Info(InfoRequest) returns (ToolInfo);
}
```

See [AD-007](/architecture/007-plugin-system) for the full design rationale.
