---
sidebar_position: 7
title: Engine service (gRPC)
description: "kapi engine serve exposes the content engine as a local gRPC API (extract documents into the canonical content model, process the part stream through tools or flows, and merge back to bytes) from Python, Node, or any gRPC-capable language."
keywords: [kapi engine serve, gRPC, EngineService, polyglot, Python, Node, unix socket, stdio, content model]
---

# Engine service (gRPC)

`kapi engine serve` exposes the neokapi content engine as a local gRPC service, so any gRPC-capable language can drive it: extract a document into the canonical content-model part stream, process that stream through tools or flows, and merge it back to document bytes with the format's skeleton round-trip. It is the plugin protocol flipped outbound: plugins serve kapi over this transport shape; here kapi serves you.

`engine` is a plumbing verb. It is absent from `kapi --help` and from the generated [command reference](/commands), because it is a transport for other programs rather than a command a person runs day to day; this page is its reference.

The `.proto` files are the contract:

- `core/proto/engine/v1/engine.proto`: the `EngineService` RPCs and envelopes
- `core/proto/content/v1/content.proto`: the canonical content-model schema (`PartMessage`, `BlockMessage`, the `RunMessage` inline union, overlays, skeleton)

Both follow the same compatibility policy: field numbers are frozen, fields are never renamed, and new fields append. The contract is locked in CI by example clients in Python and Node ([`examples/engine-client-python`](https://github.com/neokapi/neokapi/tree/main/examples/engine-client-python), [`examples/engine-client-node`](https://github.com/neokapi/neokapi/tree/main/examples/engine-client-node)) that perform a byte-exact extract → pseudo-translate → merge round trip and compare the result against the CLI.

## Trust model

The service is for **trusted local peers only**. It listens on a Unix socket (created inside a per-user, `0700` directory by default) and has no authentication in v1: the socket, or in `--stdio` mode the process pipes, is the security boundary, the same trust model as kapi's plugin daemon sockets. Do not expose the socket over the network, proxy it, or place it in a shared directory.

## Starting the server

```bash
kapi engine serve
kapi engine serve --socket /tmp/my-engine.sock
```

On startup the command prints a one-line JSON handshake on stdout, mirroring the plugin daemon convention, then serves until interrupted:

```json
{"socket":"/run/user/1000/kapi/engine-4242.sock","version":"1.2.0","pid":4242}
```

Spawn-and-parse: start the process, read the first stdout line, dial the socket (`unix://<path>` works with the standard gRPC clients in most languages). The default socket lives under `$XDG_RUNTIME_DIR/kapi/`, falling back to the user cache directory. Formats contributed by installed plugins are served transparently; the engine routes them through their plugin daemons.

## Stdio transport (spawn-per-session)

```bash
kapi engine serve --stdio
```

With `--stdio` the server serves exactly **one** gRPC connection over the process's stdin/stdout instead of a socket. Spawn-per-session callers (an editor extension, a language SDK that execs kapi per workspace) get a private engine with no socket lifecycle to manage: spawn the process, speak gRPC over its pipes, and close its stdin to shut it down (stdin EOF ends the connection and the process exits cleanly). `--stdio` and `--socket` are mutually exclusive.

In stdio mode **stdout carries nothing but the gRPC byte stream**. The handshake moves to stderr (`{"transport":"stdio","version":"1.2.0","pid":4242}`) and any logging goes to stderr as well; it is informational only, since the caller already holds both pipe ends.

Client support: gRPC is HTTP/2 framing, and most client libraries only dial network addresses. Go's `grpc-go` supports the pipe transport directly; pass a custom dialer that returns a `net.Conn` wrapping the child's pipes:

```go
cmd := exec.Command("kapi", "engine", "serve", "--stdio")
stdin, _ := cmd.StdinPipe()   // our writes → the server's stdin
stdout, _ := cmd.StdoutPipe() // the server's stdout → our reads
cmd.Stderr = os.Stderr        // handshake + logs
_ = cmd.Start()

conn, _ := grpc.NewClient("passthrough:///stdio",
	grpc.WithTransportCredentials(insecure.NewCredentials()),
	grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
		return &pipeConn{in: stdout, out: stdin}, nil // net.Conn over the pipes; no-op deadlines
	}),
)
client := enginev1.NewEngineServiceClient(conn)
// … RPCs …
_ = stdin.Close() // stdin EOF: the server exits cleanly
```

(`pipeConn` is a small `net.Conn` adapter: `Read` from the child's stdout, `Write` to its stdin, `Close` closes stdin, and the `Set*Deadline` methods return nil; gRPC's own keepalive polices the established connection.)

`@grpc/grpc-js` and Python's `grpcio` do not expose custom byte-stream transports, so from those languages prefer the Unix socket mode. Stdio mode targets integrations that can drive HTTP/2 over pipes: Go clients, editor hosts, and SDKs that embed their own gRPC transport.

## RPCs

Streaming calls are header-first: the client's first message is a header, followed by payload messages, then a half-close; the server streams results and finishes with a summary message. Parts travel in `PartBatch` frames and documents in `DocumentChunk` frames, so no single message grows with the input. Failures are gRPC status errors (`INVALID_ARGUMENT` for bad headers or unknown names, `INTERNAL` for engine failures) rather than in-band strings.

| RPC | Shape | Purpose |
| --- | --- | --- |
| `Extract` | bidi stream | Document bytes in (chunks, or a `ContentRef` path in the header) → content-model `PartMessage` stream out. The header carries the format id (empty = detect from name + bytes), locales, encoding, and format-reader config. |
| `Process` | bidi stream | Parts in → parts out, through an ordered tool chain (`tools`, each with a JSON config) or a named built-in flow (`flow`), the same pipeline executor the CLI uses, one concurrent stage per tool. |
| `Merge` | bidi stream | Parts in → document bytes out via the format writer's skeleton round-trip. The header's `original` document is the skeleton reference. |
| `Detect` | unary | Format detection from a file name and optional content sample. |
| `ListFormats` / `ListTools` / `ListFlows` | unary | The registered formats, tools, and built-in flows. |

`Process` runs linear tool chains only: a flow with parallel branches (fan-out or merge-join) is rejected with `INVALID_ARGUMENT`, matching the linear pipeline execution used product-wide, including by the CLI.

## A minimal Node session

```js
import grpc from "@grpc/grpc-js";
import protoLoader from "@grpc/proto-loader";

const def = protoLoader.loadSync("core/proto/engine/v1/engine.proto", {
  includeDirs: [repoRoot], oneofs: true, defaults: true,
});
const EngineService =
  grpc.loadPackageDefinition(def).neokapi.engine.v1.EngineService;
const client = new EngineService(
  `unix://${socket}`, grpc.credentials.createInsecure());

// Extract: header first, then the document, then half-close.
const call = client.extract();
call.write({ header: { name: "messages.json", sourceLocale: "en" } });
call.write({ chunk: { data: bytes } });
call.end();
call.on("data", (resp) => { if (resp.parts) parts.push(...resp.parts.parts); });
```

The example clients show the full loop, including `Process` with `{ tools: [{ tool: "pseudo-translate" }], targetLocale: "qps" }` and the byte-exact merge; run both with `make engine-examples`.

## When to use which surface

- **Engine service**: long-lived, warm, typed; many documents from a foreign-language process, with the full content model on the wire.
- **[CLI JSON contract](/reference/cli-contract)**: spawn-per-task scripting; structured results, error envelope, NDJSON progress.
- **[MCP server](/reference/mcp)**: AI assistants and agent frameworks.

The desktop app and the CLI itself stay in-process; this service is an outbound API for external callers.
