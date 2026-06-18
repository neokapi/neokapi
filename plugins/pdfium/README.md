# kapi-pdfium — PDFium-backed PDF reader plugin

A Mode-C (daemon over a Unix socket, gRPC `BridgeService`) kapi plugin that reads
PDFs with Google's **PDFium** via [go-pdfium](https://github.com/klippa-app/go-pdfium)
(cgo). On native builds there is no in-core PDF reader, so this plugin supplies the
`.pdf` format outright; the browser uses PDFium compiled to WebAssembly instead
(`core/formats/pdf/wasm_bridge.go`). See [AD-028](../../web/docs/contribute/architecture/028-pdf-reader-plugin.md)
for the full subsystem design.

- **Correct text**, including CID/Type0 fonts and CJK.
- **Geometry** (`geometry=true` config) → one block per positioned text run with a
  `GeometryAnnotation`; with `glyphs=true` (implies geometry) each block also
  carries per-character boxes. The default **fast path** emits one block per page
  (best for `kgrep`/`kcat`/`kconv` batch scans). Per-rect fragments are merged into
  line-level runs by the shared `core/formats/pdf.GroupRuns`, so the native and
  browser readers group text identically.
- **Structure** — in geometry mode the reader recovers headings, paragraphs, and
  tables through two tiers:
  - **Tier 1 (tagged PDFs)** — reads the PDF's own logical structure tree
    (`internal/pdfreader/structtree.go`). This uses PDFium's *experimental*
    marked-content APIs, which go-pdfium wires only under the
    `pdfium_experimental` build tag, so the shipped plugin is built with it. The
    code needs no build tag — without the experimental API at runtime it returns
    "no tagged structure" and the reader falls back to tier 2.
  - **Tier 2 (geometric)** — `core/structure.Analyze` infers structure from block
    positions. Shared with the browser reader.
- **Crash-isolated**: a malformed-PDF segfault dies with the subprocess, not kapi.
- Heavy dependency (PDFium) stays out of the core binary.

## Build

Needs `libpdfium` on `PKG_CONFIG_PATH` (a `pdfium.pc`). The Makefile targets build
with `-tags pdfium_experimental` so tier-1 tagged-structure extraction is active:

```
PKG_CONFIG_PATH=<dir with pdfium.pc> make build-pdfium-plugin
PKG_CONFIG_PATH=<...> DYLD_LIBRARY_PATH=<lib dir> make test-pdfium-plugin   # dynamic dev
```

The tier-1 tests are gated to `//go:build pdfium_experimental`; the rest run in
either configuration.

## Protocol

Speaks the same `core/plugin/proto/v2` `BridgeService.Process` (read mode) that
okapi-bridge uses, so the host needs no new client code. Parts (incl. geometry)
cross the wire through the shared payload registry (`protoconvert`).

## Distribution

`scripts/package-pdfium-plugin.sh` builds the per-platform tarball with
`-tags pdfium_experimental` and **bundles the PDFium shared library** at `lib/`
beside the binary, found via an rpath baked into the binary (`@loader_path/lib` on
macOS, `$ORIGIN/lib` on Linux, same-dir on Windows) — not a static link. Tarballs
are cosign-signed and indexed in the registry; the release lane
(`.github/workflows/release-pdfium.yml`) mirrors kapi-sat. Bundled with kapi-cli
(Homebrew `kapi-cli` depends on `kapi-pdfium`) and installed on demand by the
desktop the first time a PDF is opened.
