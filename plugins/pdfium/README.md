# kapi-pdfium — PDFium-backed PDF reader plugin

A Mode-C (daemon over a Unix socket, gRPC `BridgeService`) kapi plugin that reads
PDFs with Google's **PDFium** via [go-pdfium](https://github.com/klippa-app/go-pdfium)
(cgo). It replaces the core hand-rolled pure-Go reader for `.pdf` when installed:

- **Correct text**, including CID/Type0 fonts and CJK (the hand-rolled reader
  garbles these and corrupts non-ASCII glyphs).
- **Per-segment geometry** (`geometry=true` filter param) → `GeometryAnnotation`
  for the visual editor; default **fast path** emits one block per page (best for
  `kgrep`/`kcat`/`kconv` batch scans).
- **Crash-isolated**: a malformed-PDF segfault dies with the subprocess, not kapi.
- Heavy dependency (PDFium) stays out of the core binary.

## Build

Needs `libpdfium` on `PKG_CONFIG_PATH` (a `pdfium.pc`). For distribution, link a
**static** archive (single self-contained binary, like static ICU) via
`scripts/gen-pdfium-static-pc.sh`; for local dev a dynamic libpdfium works if it's
on the loader path.

```
PKG_CONFIG_PATH=<dir with pdfium.pc> make build-pdfium-plugin
PKG_CONFIG_PATH=<...> DYLD_LIBRARY_PATH=<lib dir> make test-pdfium-plugin   # dynamic dev
```

## Protocol

Speaks the same `core/plugin/proto/v2` `BridgeService.Process` (read mode) that
okapi-bridge uses, so the host needs no new client code. Parts (incl. geometry)
cross the wire through the shared payload registry (`protoconvert`).

## Distribution

Bundled with kapi-cli and kapi-desktop (installed into a discovered plugins root,
e.g. `/opt/homebrew/share/kapi/plugins/pdfium/`), not a separate download.
Per-platform build + signing mirrors the kapi-sat release lane.
