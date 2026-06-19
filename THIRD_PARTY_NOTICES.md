# Third-Party Notices

neokapi (the framework and CLIs) is Apache-2.0 (see [LICENSE](LICENSE)); the
bowrain platform code is AGPL-3.0. This file documents **third-party software
bundled into or invoked by neokapi plugins and binaries**, and the policy for
doing so compliantly.

> This is engineering documentation, not legal advice. Before a release ships a
> bundled native dependency, the exact build configuration and notices below
> should get a compliance review.

## Policy for bundling native dependencies in plugins

Heavy native helpers (ML runtimes, codecs, models) ship **inside the per-platform
plugin** that needs them — `kapi-vision`, `kapi-pdfium`, `kapi-sat`, `kapi-asr`,
`kapi-av` — distributed via the plugin registry, never linked into the portable
`kapi` binary. When a plugin bundles a third-party dependency it must:

1. **Prefer a separate executable invoked over a process boundary** (e.g. the
   plugin `exec`s the bundled tool) rather than static-linking it into the plugin
   binary. This isolates licenses across the process boundary and satisfies
   "replaceable component" obligations (notably ffmpeg's LGPL) trivially.
2. **Match the license's redistribution terms** — for copyleft (LGPL) deps that
   means a compatible build configuration (below), shipping the license text, and
   making the corresponding source available.
3. **Carry a `NOTICE` file in the plugin bundle** listing each bundled dependency,
   its version, its license, and a source pointer.

## Bundled / invoked dependencies

| Dependency | Used by | License | How shipped | Notes |
|---|---|---|---|---|
| **FFmpeg** | `kapi-av` (video demux) | **LGPL-2.1+** (LGPL build) | separate bundled executable, `exec`'d | GPL components **excluded** — see the FFmpeg checklist below |
| **whisper.cpp** + **ggml** | `kapi-asr` (speech recognition) | MIT | in the plugin binary | permissive; attribution only |
| **Whisper models** (OpenAI, ggml format) | `kapi-asr` | MIT | downloaded on demand to cache | permissive; attribution only |
| **ONNX Runtime** | `kapi-vision`, `kapi-pdfium` (where ONNX) | MIT | bundled / resolved native lib | permissive; attribution only |
| **PP-OCRv5 / PP-DocLayoutV3 models** | `kapi-vision` | Apache-2.0 (PaddleOCR/PaddleDetection) | bundled / download-on-demand | attribution + NOTICE |
| **PDFium** | `kapi-pdfium` | BSD-3-Clause (+ Apache-2.0 parts) | bundled native lib | attribution |
| **wtpsplit / SaT models** | `kapi-sat` | see upstream `UPSTREAM-LICENSE` | download-on-demand | verify per-model terms |

In-browser model use (the docs Labs via onnxruntime-web / transformers.js — e.g.
PP-OCR, TrOCR) downloads models client-side from their upstream hosts and bundles
no third-party binaries into this repository.

## FFmpeg — LGPL compliance checklist (`kapi-av`)

`kapi-av` decodes a video container to extract the audio track (to PCM/WAV) and
sample frames (to PNG). This needs only demuxers, native decoders, and the
`wav` / `image2` muxers — **no GPL components**. To stay LGPL:

- [ ] Build FFmpeg with **`--disable-gpl --disable-nonfree`**; do **not** enable
      `--enable-gpl`. Exclude GPL-only components (x264, x265, libpostproc, and
      GPL-only filters). We do not re-encode to patented codecs — we output WAV
      and PNG — so these are not needed.
- [ ] Ship FFmpeg as a **separate executable** the plugin `exec`s (the current
      `core/av` model), **not** statically linked into the plugin binary.
- [ ] Include the **LGPL-2.1 license text** and FFmpeg's attribution in the
      `kapi-av` bundle `NOTICE`.
- [ ] Provide **corresponding source**: pin the exact FFmpeg version and the
      `configure` flags used, and link the source (or include a written offer to
      provide it). The build script that produces the bundled FFmpeg per platform
      records this.
- [ ] Keep the bundled FFmpeg **replaceable** — because it is a separate
      executable, a user can substitute their own LGPL-compatible build.

Per-platform builds (macOS arm64, Linux amd64/arm64, Windows amd64/arm64) are
produced by the release matrix with the same flags, so the LGPL configuration is
identical across targets.

## whisper.cpp — MIT (`kapi-asr`)

whisper.cpp and ggml are MIT-licensed; the OpenAI Whisper model weights (ggml
`.bin`) are MIT. Bundling requires only that the MIT license text and copyright
notice travel in the `kapi-asr` bundle `NOTICE`. No copyleft obligations.

## Source availability

For any copyleft dependency (currently only FFmpeg, LGPL), the corresponding
source for the exact bundled version is made available via the dependency's
upstream release tag recorded in the producing plugin's build script, alongside
the `configure`/build flags used. Contact the maintainers for a source archive
of any bundled LGPL component if the upstream link is unavailable.
