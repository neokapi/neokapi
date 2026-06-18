# kapi-vision — document-vision plugin (OCR, layout, tables)

A kapi plugin that runs document-vision ONNX models in-process and speaks a
binary-framed stdin/stdout protocol. Phase 1 provides **OCR** (RapidOCR /
PP-OCRv5 detection + recognition); later phases add ML layout and table
structure. The heavy `onnxruntime` dependency lives here, never in the portable
`kapi` binary — the same isolation as `kapi-sat`.

The host-side `vision` engine (`cli/vision_plugin.go`) discovers this plugin,
spawns `kapi-vision serve`, and drives it; the framework's image format reader
(`core/formats/image`) and, later, the PDF tier-3 path consume the results
through `core/vision`. See the design in `strategy/kapi-vision/plan.md`.

## Protocol — `visionproto`

Binary-framed on stdin/stdout (not line-JSON: image payloads are MB-scale).
Each message is `[uint32 headerLen][header JSON][uint32 payloadLen][payload]`,
big-endian. Ops: `ping`, `info`, `ocr` (image bytes in the payload frame). The
`visionproto` package is pure Go (stdlib only) so a host could speak it without
the native build; per the SaT precedent the CLI instead mirrors the small wire
format to avoid importing the plugin module.

## Builds

Two configurations, like `kapi-sat`:

- **Default build** (`go build ./...`) — links no native library; the engine is
  a stub that answers `ping`/`info` and returns `ErrNoONNX` from `ocr`. The
  protocol, model-cache, and algorithm tests run here with no native dependency.
- **ONNX build** (`-tags onnx`, `CGO_ENABLED=1`) — links onnxruntime (loaded at
  runtime from `KAPI_VISION_ORT_LIB` or a copy bundled beside the binary) and
  runs the real detection + recognition pipeline. This is the release build.

```
GOWORK=off go test ./...                                  # default: protocol + models + algorithms
GOWORK=off CGO_ENABLED=1 go build -tags onnx ./...        # compile the ONNX engine
KAPI_VISION_ORT_LIB=<libonnxruntime> KAPI_VISION_MODELS_DIR=<dir> \
  GOWORK=off CGO_ENABLED=1 go test -tags onnx ./...       # full inference (needs lib + models)
```

## Models

PP-OCRv5 *mobile* ONNX assets (detection DBNet + recognition CRNN+CTC) plus the
PP-OCRv5 character dictionary, mirrored on the `vision-models-v1` neokapi release
for reproducible downloads. They download to an XDG cache on first use
(content-hash verified), are bundled beside the binary in the release tarball, or
are taken from `KAPI_VISION_MODELS_DIR` for offline/dev use. All Apache-2.0.
PP-OCRv5 is the current recommended PP-OCR generation; onnxruntime is pinned to
1.26.0 to match the `yalue/onnxruntime_go` C API.

## Status

Phase 1 is complete and validated end-to-end: the protocol, stub engine, model
cache, plugin process, host engine + discovery, the pure-Go OCR algorithms, and
the ONNX engine. `TestOCRSmoke` (gated to `-tags onnx` + `KAPI_VISION_ORT_LIB`
+ `KAPI_VISION_MODELS_DIR`) reads the committed `hello.png` through the real
PP-OCRv5 mobile models. The host (kapi) passes a file **path** to the plugin and
never loads the image bytes itself; the plugin opens and decodes the file.

Known v1 limitations (improvable later): axis-aligned detection boxes (not
rotated polygons) and occasional dropped inter-word spaces from the mobile
recognizer. Remaining for the release: bundle the ORT lib + dictionary in the
tarball (release lane mirroring `release-sat`), then registry + homebrew.
