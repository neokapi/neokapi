// Gemma bridge — the browser counterpart to the native kapi-llm plugin.
//
// The native plugin is cgo + onnxruntime; the kapi wasm engine can't run that.
// But the *same* Gemma 4 (E2B) model is plain ONNX, so it runs in the browser
// via transformers.js on WebGPU — the LLM is the real model, nothing mocked.
//
// This module installs the host function the wasm Gemma provider calls (see
// kapi/cmd/kapi-wasm-cli/gemma_bridge.go):
//
//   globalThis.kapiGemmaGenerate(payloadJSON) =>
//     Promise<{ text, input_tokens, output_tokens }>
//
// Loading is lazy and opt-in: the model (~1.5–3 GB at q4f16) downloads on the
// first generate call, cached by transformers.js in the browser Cache API so a
// return visit is instant. Because the download is large, the lab gates it behind
// an explicit "use local Gemma" action and surfaces progress via onProgress.

import {
  AutoProcessor,
  Gemma4ForConditionalGeneration,
  load_image,
} from "@huggingface/transformers";

const MODEL_ID = "onnx-community/gemma-4-E2B-it-ONNX";

export interface GemmaProgress {
  /** "downloading" while fetching shards, "ready" once the model is loaded. */
  status: "downloading" | "ready";
  /** File currently being fetched (during downloading). */
  file?: string;
  /** 0–100 fraction for the current file, when known. */
  progress?: number;
}

export interface InstallGemmaOptions {
  /** Quantization variant. q4f16 is the WebGPU sweet spot. */
  dtype?: "q4" | "q4f16" | "fp16";
  /** Inference device. WebGPU is required for usable speed. */
  device?: "webgpu" | "wasm";
  /** Progress callback for the (one-time) model download + load. */
  onProgress?: (p: GemmaProgress) => void;
}

interface WireMedia {
  kind: "image" | "audio" | "video";
  mime?: string;
  data_url?: string;
}
interface WireMessage {
  role: string;
  text?: string;
  media?: WireMedia[];
}
interface WirePayload {
  messages: WireMessage[];
  model?: string;
  max_tokens?: number;
  temperature?: number;
  top_p?: number;
  schema?: unknown;
}

// Minimal structural types for the transformers.js surface we use. The library's
// published types model apply_chat_template as non-Promise and inputs as a tensor
// container; we pin the exact runtime shapes we rely on so the bridge typechecks
// without depending on the library's (imperfect) generic typings.
interface EncodedInputs {
  input_ids: { dims: number[] };
  [key: string]: unknown;
}
interface GemmaProcessor {
  apply_chat_template(messages: unknown, opts: unknown): Promise<EncodedInputs>;
  batch_decode(tokens: unknown, opts: unknown): string[];
}
interface GeneratedSequence {
  dims: number[];
  slice(...args: unknown[]): unknown;
}
interface GemmaModel {
  generate(opts: Record<string, unknown>): Promise<GeneratedSequence>;
}

type LoadedModel = {
  processor: GemmaProcessor;
  model: GemmaModel;
};

let loadPromise: Promise<LoadedModel> | null = null;

function loadModel(opts: InstallGemmaOptions): Promise<LoadedModel> {
  if (loadPromise) return loadPromise;
  const dtype = opts.dtype ?? "q4f16";
  const device = opts.device ?? "webgpu";
  const progress_callback = opts.onProgress
    ? (p: { status?: string; file?: string; progress?: number }) => {
        opts.onProgress!({
          status: p.status === "ready" || p.status === "done" ? "ready" : "downloading",
          file: p.file,
          progress: p.progress,
        });
      }
    : undefined;

  loadPromise = (async () => {
    const processor = (await AutoProcessor.from_pretrained(MODEL_ID, {
      progress_callback,
    })) as unknown as GemmaProcessor;
    const model = (await Gemma4ForConditionalGeneration.from_pretrained(MODEL_ID, {
      dtype,
      device,
      progress_callback,
    })) as unknown as GemmaModel;
    opts.onProgress?.({ status: "ready" });
    return { processor, model };
  })().catch((err) => {
    // Reset so a later call can retry after a transient failure.
    loadPromise = null;
    throw err;
  });
  return loadPromise;
}

// toChatMessages converts the wire messages into the transformers.js chat
// format: a content array mixing text and media parts (data URLs).
function toChatMessages(messages: WireMessage[]) {
  return messages.map((m) => {
    const content: Array<Record<string, unknown>> = [];
    for (const md of m.media ?? []) {
      if (!md.data_url) continue;
      if (md.kind === "image") content.push({ type: "image", image: md.data_url });
      else if (md.kind === "audio") content.push({ type: "audio", audio: md.data_url });
    }
    if (m.text) content.push({ type: "text", text: m.text });
    // A pure-text turn can stay a plain string; mixed turns use the array form.
    return content.length === 1 && "text" in content[0]
      ? { role: m.role, content: m.text ?? "" }
      : { role: m.role, content };
  });
}

// toTemplateMessages builds chat messages for the multimodal two-step flow where
// images are passed SEPARATELY to processor(text, images): each image becomes a
// bare {type:"image"} placeholder (no data) so apply_chat_template emits the
// image soft-token sequence, which the processor then aligns with the loaded
// images. Text becomes a {type:"text"} part.
function toTemplateMessages(messages: WireMessage[]) {
  return messages.map((m) => {
    const content: Array<Record<string, unknown>> = [];
    for (const md of m.media ?? []) {
      if (md.kind === "image") content.push({ type: "image" });
    }
    if (m.text) content.push({ type: "text", text: m.text });
    return { role: m.role, content };
  });
}

/**
 * installGemmaBridge wires globalThis.kapiGemmaGenerate so the in-wasm Gemma
 * provider (kapi --provider gemma) runs the real model on this page. Call once
 * before issuing a `kapi ai-translate --provider gemma` (or any gemma-backed
 * flow) from the wasm CLI.
 */
export interface GemmaResult {
  text: string;
  input_tokens: number;
  output_tokens: number;
}

interface GenOpts {
  maxTokens?: number;
  temperature?: number;
  topP?: number;
}

// generate runs one chat completion on the (cached) model and returns the
// decoded continuation. Shared by the wasm bridge and the direct-call helpers.
async function generate(
  loaded: LoadedModel,
  wire: WireMessage[],
  gen: GenOpts,
): Promise<GemmaResult> {
  const { processor, model } = loaded;

  // Collect image data URLs across the conversation. Multimodal needs the
  // two-step processor flow (apply_chat_template → text, then processor(text,
  // images)); embedding a data URL in the chat content with tokenize:true does
  // NOT load the image. Text-only keeps the proven one-step path.
  const imageURLs: string[] = [];
  for (const m of wire) {
    for (const md of m.media ?? []) {
      if (md.kind === "image" && md.data_url) imageURLs.push(md.data_url);
    }
  }

  let inputs: EncodedInputs;
  if (imageURLs.length > 0) {
    const proc = processor as unknown as {
      (text: string, images?: unknown): Promise<EncodedInputs>;
      apply_chat_template(messages: unknown, opts: unknown): string;
    };
    const text = proc.apply_chat_template(toTemplateMessages(wire), {
      add_generation_prompt: true,
    });
    const images = await Promise.all(imageURLs.map((u) => load_image(u)));
    inputs = await proc(text, images.length === 1 ? images[0] : images);
  } else {
    inputs = await processor.apply_chat_template(toChatMessages(wire), {
      add_generation_prompt: true,
      tokenize: true,
      return_dict: true,
    });
  }

  const doSample = (gen.temperature ?? 0) > 0;
  const generated = await model.generate({
    ...inputs,
    max_new_tokens: gen.maxTokens && gen.maxTokens > 0 ? gen.maxTokens : 256,
    do_sample: doSample,
    ...(doSample ? { temperature: gen.temperature, top_p: gen.topP } : {}),
  });

  // Slice off the prompt tokens, decode only the newly generated continuation.
  const promptLen = inputs.input_ids.dims.at(-1) ?? 0;
  const totalLen = generated.dims.at(-1) ?? promptLen;
  const newTokens = generated.slice(null, [promptLen, null]);
  const text = processor.batch_decode(newTokens, { skip_special_tokens: true })[0];

  return {
    text: (text ?? "").trim(),
    input_tokens: promptLen,
    output_tokens: Math.max(0, totalLen - promptLen),
  };
}

export function installGemmaBridge(opts: InstallGemmaOptions = {}): void {
  (globalThis as Record<string, unknown>).kapiGemmaGenerate = async (
    payloadJSON: string,
  ): Promise<GemmaResult> => {
    const payload: WirePayload = JSON.parse(payloadJSON);
    const loaded = await loadModel(opts);
    return generate(loaded, payload.messages ?? [], {
      maxTokens: payload.max_tokens,
      temperature: payload.temperature,
      topP: payload.top_p,
    });
  };
}

/**
 * runGemmaImageOCR runs Gemma 4 directly on an image (from React, not via the
 * wasm global) and returns its transcription. Used by the Vision Lab to compare
 * Gemma's generative OCR against the PP-OCRv5 ML pipeline on the same image. The
 * model is the same cached instance loadModel() shares with the bridge.
 */
export async function runGemmaImageOCR(
  imageDataURL: string,
  prompt: string,
  opts: InstallGemmaOptions = {},
): Promise<GemmaResult> {
  const loaded = await loadModel(opts);
  const wire: WireMessage[] = [
    { role: "user", text: prompt, media: [{ kind: "image", data_url: imageDataURL }] },
  ];
  // OCR wants faithful transcription, not creativity → greedy, generous budget.
  return generate(loaded, wire, { maxTokens: 1024, temperature: 0 });
}

/** uninstallGemmaBridge removes the host hook (e.g. on component unmount). */
export function uninstallGemmaBridge(): void {
  delete (globalThis as Record<string, unknown>).kapiGemmaGenerate;
}

/** isGemmaModelLoaded reports whether the model has finished its one-time load. */
export function isGemmaModelLoaded(): boolean {
  return loadPromise !== null;
}
