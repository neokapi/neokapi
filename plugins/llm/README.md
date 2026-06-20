# kapi-llm — local Gemma 4 LLM plugin

`kapi-llm` runs Google's **Gemma 4 (E2B)** on-device, in-process, as a free and
private alternative to paid cloud models. It powers the kapi AI tools
(translate, chat, QA, brand-voice) without sending content to a remote endpoint.

The heavy native stack — onnxruntime + the SentencePiece tokenizer — lives here,
in the plugin, never in the portable `kapi` binary. The host (`kapi`) drives the
plugin out-of-process over a tiny line-delimited JSON protocol on stdin/stdout
(see [`llmproto`](llmproto/llmproto.go)), exactly like `kapi-sat` and
`kapi-vision`.

## Model

[`onnx-community/gemma-4-E2B-it-ONNX`](https://huggingface.co/onnx-community/gemma-4-E2B-it-ONNX)
— the instruction-tuned, multimodal (text + image + audio) on-device variant,
Apache-2.0, 140+ languages, 128K context. The plugin uses the **q4** variant
(4-bit weights, float32 tensor I/O), which keeps the whole pipeline in float32 —
the Go onnxruntime binding has no native float16 tensor type, the same reason
`kapi-sat` uses its float32 graph.

Gemma 4 ships as a transformers.js-style split export. Generation threads four
ONNX graphs:

```
input_ids ─▶ embed_tokens ─▶ inputs_embeds + per_layer_inputs
                                  │  (media rows replaced)
  image ─▶ vision_encoder ─▶ image_features ─┐
  audio ─▶ audio_encoder  ─▶ audio_features ─┴▶ decoder_model_merged ─▶ logits
                                                  ▲   (+ KV-cache loop)
```

The decoder graph contains the entire transformer (RoPE, attention, the
alternating full/sliding-window mask, norms). The Go code never implements
attention — it marshals tensors and threads the KV cache from each step's
`present.*` outputs into the next step's `past_key_values.*` inputs. Session I/O
names and dtypes are **introspected at load** (`ort.GetInputOutputInfo`), not
hardcoded, so the engine adapts to the export.

Models are downloaded on demand into an XDG cache on first use (verified, locked,
atomic — see [`internal/model`](internal/model/model.go)).

## Build

```bash
# Pure-Go (no backend): builds the protocol, model downloader, and a stub engine.
# Generate returns "built without ONNX support"; ping/info still work.
make build-llm-plugin

# With the ONNX backend (cgo; needs onnxruntime + tokenizer native libs):
make build-llm-plugin-onnx
```

The pure-Go module (protocol, downloader, chat templating, sampling) builds and
tests with **no native dependency**: `GOWORK=off go test ./...`.

## Validation status

The **text** path (tokenize → embed → KV-cache decode → sample → detokenize) is
structurally complete and compiles under `-tags onnx`. The **multimodal** path
(vision/audio encoder runs + embed-row splice) is fully wired. Exact numerics —
tensor dtypes, the `num_logits_to_keep` rank, image size / normalization, and
audio mel parameters — are validated **on-device** against the real q4 weights,
mirroring how the `kapi-vision` ONNX engine landed. Until validated on a given
platform, treat generation quality as provisional.

## Runtime configuration

| Env var | Purpose |
|---|---|
| `KAPI_LLM_ORT_LIB` | Path to `libonnxruntime.{so,dylib,dll}` (else bundled `lib/` beside the binary, else loader search). |
| `KAPI_LLM_CACHE` | Override the model cache root (else `$XDG_CACHE_HOME/kapi/models/llm`). |

## License

Plugin code: Apache-2.0. The Gemma 4 model weights are distributed by Google
under the Apache-2.0 license; see the model card for terms.
