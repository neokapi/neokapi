# `hello` — kapi plugin reference example

A minimal Go plugin demonstrating the kapi plugin protocol v1
([spec](../../../docs/internals/plugin-protocol-v1.md)).

The plugin declares two capabilities:

- One Mode-A command (`kapi hello [name]`)
- One Mode-B MCP tool (`say_hello`)

## Build & install (development)

```sh
# from the neokapi repo root
go build -o examples/plugins/hello/kapi-hello ./examples/plugins/hello

# point kapi at the examples dir
export KAPI_PLUGINS_DIR=$PWD/examples/plugins
kapi --help        # shows `hello` under [hello]
kapi hello world   # → "Hello, world!"
```

The reference plugin is unsigned; `KAPI_PLUGINS_DIR` is the only
discovery root that accepts unsigned plugins without `--unsafe`.

## What this demonstrates

| File | Role |
|---|---|
| `manifest.json` | Capability declarations consumed by kapi at startup |
| `main.go` | Subprocess entry — routes `command`, `mcp-server`, `version` |

A real plugin would also include `schemas/<key>.json` files for any
recipe schema extensions it owns, and would use a full MCP SDK rather
than the hand-rolled JSON-RPC stub here.
