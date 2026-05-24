---
sidebar_position: 6
title: Okapi Bridge
description: The Okapi bridge exposes the Okapi Framework's Java filters to neokapi as a Mode-C plugin daemon — a JVM subprocess that adapts between neokapi's Part model and Okapi's Event model over the gRPC BridgeService.
keywords: [Okapi bridge, Java filters, gRPC, JVM, BridgeService, Process RPC, daemon, Part model, Event model, neokapi]
---

# Okapi Bridge

The Okapi bridge provides access to the Okapi Framework's filters without
rewriting them in Go. It is the canonical [Mode-C plugin](/contribute/plugins#three-transport-modes):
a long-lived daemon subprocess that hosts an adapter translating between
neokapi's `Part` model and Okapi's `Event` model. The current implementation
runs a JVM, but the bridge protocol is gRPC-based and language-agnostic.

## How it works

The bridge is installed like any other plugin (`kapi plugin install okapi-bridge`)
and declares its filters as `formats` in its `manifest.json`. Because formats are
a Mode-C capability, kapi launches the bridge as a daemon:

1. kapi runs `<binary> daemon`. The JVM starts once per kapi session.
2. The daemon binds a Unix-domain socket and prints a one-line JSON handshake on
   stdout: `{"socket":"…","version":"…"}`. The socket is served with Netty's
   native transports — **kqueue on macOS, epoll on Linux** — for kernel-level
   throughput. If no socket path is configured (a legacy, non-daemon fallback
   used by tests), the bridge instead serves gRPC on a localhost TCP port and
   reports it as `tcp://…`.
3. kapi opens a gRPC client to that Unix socket and dispatches
   document-processing requests against the `BridgeService` defined in
   [`core/plugin/proto/v2/neokapi_bridge.proto`](https://github.com/neokapi/neokapi/blob/main/core/plugin/proto/v2/neokapi_bridge.proto).
   The host (`cli/pluginhost/daemon.go`) dials Unix sockets only.

Bridge-backed formats are registered into the standard `FormatRegistry`
(see `cli/pluginhost/format_factory.go`) and are indistinguishable from native
formats at the API level — flows and commands reference `okapi-html`, `okapi-xml`,
and so on without knowing they come from a subprocess.

## BridgeService

A single bidirectional-streaming `Process` RPC handles the whole document
lifecycle, replacing the per-step `Open`/`Read`/`Write`/`Close` RPCs of the v1
protocol:

| RPC           | Shape                | Purpose                                                        |
| ------------- | -------------------- | -------------------------------------------------------------- |
| `Process`     | Bidirectional stream | Full read / read-write / write-only document cycle (see below) |
| `ProcessStep` | Bidirectional stream | Run a single Okapi pipeline step over a stream of parts        |
| `Shutdown`    | Unary                | Gracefully stop the bridge daemon                              |

The client opens a `Process` stream and sends a `ProcessHeader` first, which
selects the mode:

- **Read-only** (no output ref in the header) — Java reads the document and
  streams `Part`s back; Go closes the send side when done receiving.
- **Read-write** (output ref present) — Java reads and retains its `Event`s while
  streaming parts; Go sends processed parts back concurrently; Java's write thread
  applies translations to the retained `Event`s and writes the result.
- **Write-only** — same as read-write, but Go ignores the read-phase parts and
  drives the output entirely from the parts it sends.

Streaming means content flows incrementally without buffering the whole document
in memory — critical for large files (e.g. XLSX, IDML). The host-side client that
drives these RPCs lives in
[`cli/pluginhost/format_client.go`](https://github.com/neokapi/neokapi/blob/main/cli/pluginhost/format_client.go);
`Part` ↔ proto conversion lives in
[`core/plugin/protoconvert/`](https://github.com/neokapi/neokapi/tree/main/core/plugin/protoconvert).

## Daemon reuse and capacity

kapi's daemon pool (`cli/pluginhost/daemon.go`) keeps the JVM warm across
successive operations within a session, so the bridge's startup cost is paid once
rather than per file. Idle daemons are shut down after the manifest's
`idle_timeout_seconds` (default 5 min), and the number of concurrent daemons is
capped via `KAPI_MAX_DAEMONS` (default 8) with LRU eviction.

## Packaging

The Okapi bridge is built with `jpackage` (no Go shim): each release produces a
native launcher plus a bundled JRE per platform, cosign-signed via GitHub Actions
keyless OIDC. Multiple versions can be installed side by side and pinned per
recipe (`requires: { okapi-bridge: ">=1.47.0" }`).

## Available filters

Through the Okapi bridge, neokapi reaches Okapi's full filter library — including
DOCX, XLSX, EPUB, IDML, DITA, FrameMaker, and many more. The authoritative list is
the bridge's published `manifest.json`; see the
[neokapi/okapi-bridge](https://github.com/neokapi/okapi-bridge) repository.

## See also

- [Plugin System](/contribute/plugins) — how kapi discovers and dispatches to Mode-C daemons
- [AD-007: Plugin System and Okapi Bridge](/contribute/architecture/007-plugin-system) — design rationale
- [Okapi Bridge Protocol note](/contribute/notes-internal/plugin-bridge-protocol) — `Process` RPC details and Part↔Event translation
