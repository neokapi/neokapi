---
sidebar_position: 5
title: Plugin System
---

# Plugin System

gokapi uses [HashiCorp go-plugin](https://github.com/hashicorp/go-plugin) with gRPC transport for out-of-process plugins. Each plugin is a separate executable that communicates with the host over stdin/stdout.

## Plugin Types

- **Format Reader Plugin** — implements `DataFormatReaderPlugin` gRPC service
- **Format Writer Plugin** — implements `DataFormatWriterPlugin` gRPC service
- **Tool Plugin** — implements `ToolPlugin` gRPC service

## Plugin Discovery

Plugins are discovered by scanning a directory for executables matching the naming convention:

- `gokapi-format-*` — format reader/writer plugins
- `gokapi-tool-*` — tool plugins

The host launches each plugin, performs a version handshake, queries capabilities via `Info()`, and registers into the appropriate registry.

## Multi-Version Support

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

Formats register with versioned names (`okapi-html@1.46.0`) and bare aliases (`okapi-html`) pointing to the latest version.

## Writing a Plugin

A plugin is a Go binary that serves one or more gRPC services:

```go
package main

import (
    "github.com/hashicorp/go-plugin"
    gp "github.com/gokapi/gokapi/plugin"
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

See [AD-007](/docs/ad/007-plugin-system) for the full design rationale.
