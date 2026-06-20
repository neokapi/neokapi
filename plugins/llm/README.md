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
**validated on-device** against the real q4 weights (macOS arm64) and produces
correct translations, matching the in-browser transformers.js path. Getting
there surfaced three model-contract details the engine now handles:

- **Per-layer KV-cache head_dim** — Gemma 4's hybrid attention gives
  sliding-window layers a wider KV head_dim (512) than full-attention layers
  (256), so the empty cache is built per-input from the model's declared dims
  (`ort.GetInputOutputInfo`), not one config value.
- **`num_logits_to_keep` is a rank-0 scalar** — it feeds `Neg→Unsqueeze→Slice`
  in the lm_head, so it must be an `ort.Scalar`, not a 1-D `[1]` tensor.
- **Chat turn markers by id** — the tokenizer does not parse the literal
  `<start_of_turn>`/`<end_of_turn>` strings as single tokens, so the prompt is
  assembled at the token-id level (`<start_of_turn>`=105, `<end_of_turn>`=106).

The **vision and audio** encoders are wired (sessions load, the embed-splice
works), but their preprocessing — Gemma's native-resolution patchified
`pixel_values` (`[N, 768]` 16×16×3 patches + `[N, 2]` position ids) and the
log-mel audio features (`[B, frames, 128]` + bool mask) — is **not yet validated
against reference outputs**, so multimodal input is gated off (a clear
"experimental" message). v0.1.0 ships **text-only**; multimodal is a tracked
follow-up.

## Runtime configuration

| Env var | Purpose |
|---|---|
| `KAPI_LLM_ORT_LIB` | Path to `libonnxruntime.{so,dylib,dll}` (else bundled `lib/` beside the binary, else loader search). |
| `KAPI_LLM_CACHE` | Override the model cache root (else `$XDG_CACHE_HOME/kapi/models/llm`). |

## License

Plugin code: Apache-2.0. The Gemma 4 model weights are distributed by Google
under the Apache-2.0 license; see the model card for terms.
