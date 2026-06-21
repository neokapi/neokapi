# `kapi-sat` — SaT (Segment any Text) ML segmenter plugin

`kapi-sat` is a standalone kapi plugin binary that runs the
[SaT / wtpsplit](https://github.com/segment-any-text/wtpsplit) sentence
segmentation models in-process and exposes them to a host process over a tiny
line-delimited JSON protocol on stdin/stdout.

The heavy native dependencies (ONNX Runtime + the XLM-RoBERTa tokenizer) live
**only in this plugin** — they are never linked into the portable `kapi`
binary. The host-side `sat` segment engine spawns `kapi-sat serve` and drives
it with the cgo-free [`satproto`](satproto/) client.

## Module layout

```
plugins/sat/
├── go.mod                      module github.com/neokapi/neokapi/plugins/sat
├── manifest.json               plugin manifest (registry distribution)
├── cmd/kapi-sat/main.go        entry point: serve | version | command
├── satproto/                   PURE-GO protocol (host imports this; no cgo)
│   ├── satproto.go             Request/Response, Serve loop, Client driver
│   └── satproto_test.go
└── internal/
    ├── model/                  PURE-GO model+tokenizer download & XDG cache
    │   ├── model.go
    │   └── model_test.go
    └── sat/                    inference
        ├── algo.go             PURE-GO: blocks, recombine, sigmoid, rune mapping
        ├── algo_test.go
        ├── float16.go          PURE-GO: IEEE-754 half-precision <-> float32
        ├── float16_test.go
        ├── engine.go           Engine interface + ErrNoONNX
        ├── engine_stub.go      //go:build !onnx  (default; no native deps)
        ├── engine_onnx.go      //go:build onnx   (cgo: onnxruntime + tokenizer)
        ├── ort_onnx.go         //go:build onnx   (runtime init + KAPI_SAT_ORT_LIB)
        └── engine_smoke_test.go //go:build onnx && satmodel  (real-model e2e)
```

The `satproto` package and everything reachable from the default build have **no
cgo dependency**, so `go build ./...` and `go test ./...` work on any machine.
The ONNX backend is compiled in only with `-tags onnx`.

## Protocol (host integration)

The host imports `github.com/neokapi/neokapi/plugins/sat/satproto` and uses
`satproto.NewClient(stdin, stdout)`:

```go
cmd := exec.Command(pluginBinary, "serve")
cmd.Env = append(os.Environ(), "KAPI_SAT_ORT_LIB=/path/to/libonnxruntime.dylib")
stdin, _ := cmd.StdinPipe()
stdout, _ := cmd.StdoutPipe()
cmd.Stderr = os.Stderr // plugin logs (download progress, model load) go here
_ = cmd.Start()

client := satproto.NewClient(stdin, stdout)
version, _ := client.Ping()
bounds, _ := client.Segment("Hello world. How are you?", "sat-3l-sm", "en", 0.25)
// bounds == []int{13}  -> interior sentence boundaries as RUNE offsets

stdin.Close() // signals the plugin's read loop to exit
_ = cmd.Wait()
```

Wire format — one JSON object per line, both directions:

| Request | Response |
|---|---|
| `{"id":1,"op":"segment","text":"…","lang":"en","model":"sat-3l-sm","threshold":0.25}` | `{"id":1,"boundaries":[13,40,…]}` |
| `{"op":"ping"}` | `{"ok":true,"version":"…"}` |
| `{"op":"info"}` | `{"version":"…","models":[{"name":"sat-3l-sm","loaded":true,"default":true},…]}` |
| any failure | `{"id":N,"error":"…"}` |

`boundaries` are **interior sentence boundaries as rune offsets** into the exact
request text (a boundary at rune `i` means a new sentence begins at `text[i]`).
Offsets `0` and `len([]rune(text))` are never emitted. The process stays alive
across requests; models load lazily on first use and are cached in memory.

## Building

### Default build (no ONNX; CI-safe)

```sh
make build-sat-plugin           # -> bin/kapi-sat (pure Go)
make test-sat-plugin            # protocol + algorithm + cache tests
```

In this build, `segment` requests return an error advising a rebuild with
`-tags onnx`; `ping` and `info` still work (useful for host capability probing).

### ONNX build (real in-process segmenter)

Requires two native libraries:

1. **ONNX Runtime shared library** — download the prebuilt archive for your
   platform from the [microsoft/onnxruntime releases](https://github.com/microsoft/onnxruntime/releases)
   (e.g. `onnxruntime-osx-arm64-<ver>.tgz`) and extract it. The binary is
   pointed at the `.dylib`/`.so` **at runtime** via `KAPI_SAT_ORT_LIB`.

2. **Tokenizer static library `libtokenizers.a`** — download from the
   [daulet/tokenizers releases](https://github.com/daulet/tokenizers/releases)
   (e.g. `libtokenizers.darwin-arm64.tar.gz`) or build it from that repo's Rust
   source with `make build`. It is linked **at build time** via `CGO_LDFLAGS`.

```sh
# directory containing libtokenizers.a
export SAT_TOKENIZERS_LIB=/path/to/libtokenizers

make build-sat-plugin-onnx      # -> bin/kapi-sat (with ONNX backend)

# run: point at the onnxruntime shared lib
export KAPI_SAT_ORT_LIB=/path/to/onnxruntime/lib/libonnxruntime.<ver>.dylib
bin/kapi-sat serve
```

Equivalent raw command:

```sh
cd plugins/sat && GOWORK=off CGO_ENABLED=1 \
  CGO_LDFLAGS="-L$SAT_TOKENIZERS_LIB" \
  go build -tags onnx -o ../../bin/kapi-sat ./cmd/kapi-sat
```

### Real-model end-to-end test

Gated behind `onnx && satmodel`; downloads the model on first run:

```sh
cd plugins/sat && GOWORK=off CGO_ENABLED=1 CGO_LDFLAGS="-L$SAT_TOKENIZERS_LIB" \
  KAPI_SAT_ORT_LIB=/path/to/libonnxruntime.dylib \
  go test -tags "onnx satmodel" -run TestEngineSegmentEndToEnd -v ./internal/sat/
```

## Distribution

The plugin is distributed as signed, per-platform release tarballs indexed in
the [`neokapi/registry`](https://github.com/neokapi/registry). Install it with:

```sh
kapi plugins install sat
```

### Tarball layout

Each release artifact `kapi-sat_<version>_<os>_<arch>.tar.gz` contains:

```
kapi-sat[.exe]                the plugin binary (at the tarball root)
manifest.json                 the plugin manifest (at the tarball root)
lib/libonnxruntime.dylib      onnxruntime shared library (darwin)
  | libonnxruntime.so         (linux)
  | onnxruntime.dll           (windows)
```

### Bundled onnxruntime — auto-resolution

The onnxruntime shared library is **bundled** at `lib/<name>` beside the binary.
When `KAPI_SAT_ORT_LIB` is unset, `resolveORTLib()` (in
[`internal/sat/ort_onnx.go`](internal/sat/ort_onnx.go)) loads the library from
`<exe-dir>/lib/<name>` or `<exe-dir>/<name>` — so a registry-installed plugin
works with **no environment configuration**. `KAPI_SAT_ORT_LIB` remains an
explicit override (it wins when set) and is only *needed* for a from-source /
dev build, where no bundled library sits next to the binary.

The daulet/tokenizers static library is linked at **build time**, so it is not
part of the tarball and not needed at runtime.

### Models

Models are **not** bundled. They download from HuggingFace on first use and are
cached on disk (see [Models & cache](#models--cache)). The first `segment` for a
model fetches ~428 MB (ONNX) + ~9 MB (tokenizer).

### Supported platforms

`darwin/arm64`, `darwin/amd64`, `linux/amd64`, `linux/arm64`, `windows/amd64`.

### Release pipeline

The `build-sat-plugin` matrix job in
[`.github/workflows/release.yml`](../../.github/workflows/release.yml) builds
each platform on a native runner (cgo, `-tags onnx`): it downloads the pinned
onnxruntime release (the ABI matched to `yalue/onnxruntime_go`) and the pinned
`daulet/tokenizers` static library (built from Rust source on Windows, which has
no prebuilt archive), runs [`scripts/package-sat-plugin.sh`](../../scripts/package-sat-plugin.sh)
to build + bundle + tar, Developer-ID signs + notarizes the macOS binary, and
[cosign](https://github.com/sigstore/cosign)-signs every tarball (keyless;
`<file>.sigstore.json`). The `register-sat-plugin` job then publishes the
per-platform URLs, SHA-256s, and signatures into the registry so
`kapi plugins install sat` can verify the download. Run the same packaging
locally with `make package-sat-plugin SAT_TOKENIZERS_LIB=… SAT_ORT_DIR=…`.

## Models & cache

| Model | Source repo | ONNX file | Tokenizer | Default threshold |
|---|---|---|---|---|
| `sat-3l-sm` (default) | `segment-any-text/sat-3l-sm` | `model.onnx` | `facebookAI/xlm-roberta-base` | 0.25 |
| `sat-12l-sm` | `segment-any-text/sat-12l-sm` | `model.onnx` | `facebookAI/xlm-roberta-base` | 0.25 |

Files are fetched from `https://huggingface.co/<repo>/resolve/main/<file>` on
first use and cached under (in precedence order):

1. `$KAPI_SAT_CACHE`
2. `$XDG_CACHE_HOME/kapi/models/sat/<model>/`
3. `~/.cache/kapi/models/sat/<model>/`

Concurrent downloads are serialized by a per-file lock; writes are atomic
(temp file + rename) so a reader never sees a partial model.

## How it works (inference)

Mirrors `superlinear-ai/wtpsplit-lite` for the `*-sm` models:

1. Tokenize with the XLM-RoBERTa SentencePiece tokenizer (`addSpecialTokens=false`),
   keeping byte-offset spans per subword token.
2. Split the token sequence into overlapping blocks of `block_size=512`
   (content tokens), `stride=256`; the final block is pulled back to end at the
   sequence end so every token is covered.
3. For each block, prepend CLS (id 0) + append SEP (id 2), feed
   `input_ids` (int64) and `attention_mask` (**float16**, all ones) to the ONNX
   `SubwordXLMForTokenClassification` graph, read the **float16** `logits`
   (shape `[1, seq, 1]`), and strip the CLS/SEP positions.
4. Recombine overlapping per-token logits by averaging.
5. `sigmoid(logit) >= threshold` marks a boundary at that token; the cut point
   is the byte just past the token, advanced over following ASCII whitespace,
   converted to a **rune offset**. Offsets 0 and len are excluded; results are
   strictly ascending and de-duplicated.

> The SaT ONNX exports use IEEE-754 half-precision for the attention_mask input
> and logits output. The Go onnxruntime binding has no native float16 tensor,
> so those tensors travel as raw bytes via `NewCustomDataTensor(...,
> TensorElementDataTypeFloat16)` and are converted in `float16.go`. `input_ids`
> remains int64.

## Library choices

- **ONNX runtime:** [`github.com/yalue/onnxruntime_go`](https://github.com/yalue/onnxruntime_go)
  — actively maintained, supports `SetSharedLibraryPath` (so we point at a
  downloaded onnxruntime at runtime rather than vendoring it), a dynamic session
  API with named inputs/outputs, and a `CustomDataTensor` that lets us feed the
  float16 tensors the SaT graph requires.
- **Tokenizer:** [`github.com/daulet/tokenizers`](https://github.com/daulet/tokenizers)
  — cgo bindings over the HuggingFace Rust `tokenizers` crate. It loads
  `tokenizer.json` directly (`FromBytes`), produces the SentencePiece subword
  IDs **and byte-offset spans** the boundary→rune mapping needs, and ships
  prebuilt `libtokenizers.a` archives so Rust is not required to build the
  plugin. A pure-Go SentencePiece option was rejected because none reliably
  reproduce HF offset mappings for XLM-R.

## Notes for the host-side `sat` segment engine integrator

- **Spawn once, reuse.** Start `kapi-sat serve` once and keep it alive; correlate
  responses by `id`. Closing the plugin's stdin ends its loop cleanly. The
  `satproto.Client` is concurrency-safe and serializes round-trips.
- **Runtime env.** A registry-installed plugin needs **no** env: the install
  tarball ships the onnxruntime shared library at `lib/<name>` beside the
  binary, and the plugin auto-resolves it (see [Distribution](#distribution)).
  `KAPI_SAT_ORT_LIB` remains an **override** and is only *required* for a
  from-source / dev build (where no bundled lib sits next to the binary) — point
  it at the onnxruntime shared library path. Optionally set `KAPI_SAT_CACHE` to
  control where models are stored. The tokenizer lib is linked at build time,
  not needed at runtime.
- **First-request latency.** The first `segment` for a model downloads ~428 MB
  (ONNX) + ~9 MB (tokenizer) and loads the session. Consider an explicit warm-up
  `segment` (or surface download progress, which the plugin writes to stderr).
- **Capability probe without native deps.** A default-built `kapi-sat` answers
  `ping`/`info` and returns a descriptive error for `segment`, so the host can
  detect "installed but no ONNX backend" gracefully.
- **Boundaries are rune offsets.** Slice the original text with
  `[]rune(text)[prev:b]` per boundary; do not treat them as byte offsets.
- **Manifest.** `manifest.json` sets `capabilities.selfcheck: true` (advertising
  the standard `kapi-sat doctor` self-check that `kapi plugins doctor` runs) so
  the plugin registers under the unified plugin model, plus a top-level
  `metadata.segment` block describing the protocol, invocation
  (`["kapi-sat","serve"]`), the protocol package import path, and the supported
  models. The manifest schema has no dedicated "subprocess segmenter" capability
  slot, so the segment contract lives under `metadata` (the manifest root allows
  extra keys; `manifest.Parse` accepts it). If a first-class segment capability
  is later added to `core/plugin/manifest`, migrate this block into it.
- **Threshold.** `threshold` of `0` (or out of `(0,1)`) uses the model default
  (0.25 for `*-sm`). Lower values cut more aggressively.
