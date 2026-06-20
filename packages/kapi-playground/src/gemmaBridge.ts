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

type LoadedModel = {
  processor: Awaited<ReturnType<typeof AutoProcessor.from_pretrained>>;
  model: Awaited<ReturnType<typeof Gemma4ForConditionalGeneration.from_pretrained>>;
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
    const processor = await AutoProcessor.from_pretrained(MODEL_ID, { progress_callback });
    const model = await Gemma4ForConditionalGeneration.from_pretrained(MODEL_ID, {
      dtype,
      device,
      progress_callback,
    });
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

/**
 * installGemmaBridge wires globalThis.kapiGemmaGenerate so the in-wasm Gemma
 * provider (kapi --provider gemma) runs the real model on this page. Call once
 * before issuing a `kapi ai-translate --provider gemma` (or any gemma-backed
 * flow) from the wasm CLI.
 */
export function installGemmaBridge(opts: InstallGemmaOptions = {}): void {
  (globalThis as Record<string, unknown>).kapiGemmaGenerate = async (
    payloadJSON: string,
  ): Promise<{ text: string; input_tokens: number; output_tokens: number }> => {
    const payload: WirePayload = JSON.parse(payloadJSON);
    const { processor, model } = await loadModel(opts);

    const chat = toChatMessages(payload.messages ?? []);
    const inputs = await processor.apply_chat_template(chat, {
      add_generation_prompt: true,
      tokenize: true,
      return_dict: true,
    });

    const doSample = (payload.temperature ?? 0) > 0;
    const generated = await model.generate({
      ...inputs,
      max_new_tokens: payload.max_tokens && payload.max_tokens > 0 ? payload.max_tokens : 256,
      do_sample: doSample,
      ...(doSample ? { temperature: payload.temperature, top_p: payload.top_p } : {}),
    });

    // Slice off the prompt tokens, decode only the newly generated continuation.
    const promptLen = (inputs as { input_ids: { dims: number[] } }).input_ids.dims.at(-1) ?? 0;
    const seq = generated as { dims: number[]; slice: (...a: unknown[]) => unknown };
    const totalLen = seq.dims.at(-1) ?? promptLen;
    const newTokens = seq.slice(null, [promptLen, null]);
    const text = (processor as { batch_decode: (t: unknown, o: unknown) => string[] }).batch_decode(
      newTokens,
      { skip_special_tokens: true },
    )[0];

    return {
      text: (text ?? "").trim(),
      input_tokens: promptLen,
      output_tokens: Math.max(0, totalLen - promptLen),
    };
  };
}

/** uninstallGemmaBridge removes the host hook (e.g. on component unmount). */
export function uninstallGemmaBridge(): void {
  delete (globalThis as Record<string, unknown>).kapiGemmaGenerate;
}

/** isGemmaModelLoaded reports whether the model has finished its one-time load. */
export function isGemmaModelLoaded(): boolean {
  return loadPromise !== null;
}
