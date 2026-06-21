// Local-LLM bridge — the browser counterpart to the native kapi-llm plugin.
//
// The native plugin is cgo + onnxruntime; the kapi wasm engine can't run that.
// But the same ONNX models run in the browser via transformers.js, so the LLM is
// real, nothing mocked. This module installs the host function the wasm "gemma"
// provider calls (see kapi/cmd/kapi-wasm-cli/gemma_bridge.go):
//
//   globalThis.kapiGemmaGenerate(payloadJSON) =>
//     Promise<{ text, input_tokens, output_tokens }>
//
// Two models are supported, auto-selected by modality:
//   • a small TEXT model (the default) for translate / segmentation / chat — a
//     light download so "use the local LLM" is reasonable, and
//   • the multimodal Gemma 4 E2B for IMAGE tasks (OCR), which is several GB and
//     only pulled when a request actually carries an image.
// Loading is lazy and opt-in; transformers.js caches each model in the browser
// Cache API so a return visit is instant. Progress flows through onProgress
// (aggregated across shards so the percentage is smooth).

import {
  AutoProcessor,
  AutoTokenizer,
  AutoModelForCausalLM,
  Gemma4ForConditionalGeneration,
  load_image,
} from "@huggingface/transformers";

// ---------------------------------------------------------------------------
// Model registry
// ---------------------------------------------------------------------------

/** Key of the small text-only model used for text tasks (the default). */
export const DEFAULT_TEXT_MODEL = "llama-3.2-1b";
/** Key of the multimodal model used when a request carries an image. */
export const MULTIMODAL_MODEL = "gemma-4-e2b";

type DType = "q4" | "q4f16" | "fp16" | "q8" | Record<string, "q4" | "q4f16" | "q8" | "fp16">;

/** Input modalities a model can consume. */
export type Modality = "text" | "image" | "audio" | "video";

interface ModelSpec {
  key: string;
  /** HuggingFace repo id. */
  hfId: string;
  /** Human label for the widget / picker. */
  label: string;
  /**
   * Input modalities this model supports. Selection (pickModelForModalities)
   * matches a task's required modalities against this list, so adding a model
   * with new capabilities is just another registry entry. (Text is assumed
   * supported by every chat model and is listed explicitly for clarity.)
   */
  modalities: Modality[];
  /** Load strategy: a plain causal LM (tokenizer) or a multimodal processor. */
  loader: "causal" | "multimodal";
  /** Approximate download size, for hints + smallest-capable selection. */
  sizeBytes: number;
  dtype: DType;
  device: "webgpu" | "wasm";
}

const MODELS: Record<string, ModelSpec> = {
  [DEFAULT_TEXT_MODEL]: {
    key: DEFAULT_TEXT_MODEL,
    hfId: "onnx-community/Llama-3.2-1B-Instruct",
    label: "Llama 3.2 1B (text)",
    modalities: ["text"],
    loader: "causal",
    // ~0.9 GB at q4f16. Both Qwen2.5 0.5B and 1.5B degenerated into repeated
    // tokens under q4f16/WebGPU in this onnxruntime-web dev build even with a
    // repetition penalty; Llama 3.2 1B follows the instruction reliably. Still a
    // fraction of the multimodal Gemma's ~6.8 GB.
    sizeBytes: 900_000_000,
    dtype: "q4f16",
    device: "webgpu",
  },
  [MULTIMODAL_MODEL]: {
    key: MULTIMODAL_MODEL,
    hfId: "onnx-community/gemma-4-E2B-it-ONNX",
    label: "Gemma 4 E2B (image + audio + text)",
    // Gemma 4 E2B accepts image + audio (mirrors the native provider's
    // InputModalities in kapi/cmd/kapi-wasm-cli/gemma_bridge.go); a video task is
    // satisfied by image (frames) + audio.
    modalities: ["text", "image", "audio"],
    loader: "multimodal",
    // q4f16 quantizes the decoder; the other components have no q4f16 variant and
    // load at fp16, so the full multimodal download is ~6.8 GB.
    sizeBytes: 6_800_000_000,
    dtype: "q4f16",
    device: "webgpu",
  },
};

// requiredModalities derives the input modalities a request needs from its media
// (a video part needs both image frames and audio); text when there is no media.
function requiredModalities(messages: WireMessage[]): Modality[] {
  const req = new Set<Modality>();
  for (const m of messages) {
    for (const md of m.media ?? []) {
      if (!md.data_url) continue;
      if (md.kind === "image") req.add("image");
      else if (md.kind === "audio") req.add("audio");
      else if (md.kind === "video") {
        req.add("image");
        req.add("audio");
      }
    }
  }
  if (req.size === 0) req.add("text");
  return [...req];
}

/**
 * pickModelForModalities returns the SMALLEST registered model whose modalities
 * cover every required one — so a text task gets the light model and an image /
 * audio / video task gets the multimodal one. Throws if nothing covers them.
 */
export function pickModelForModalities(required: Modality[]): ModelSpec {
  const need: Modality[] = required.length ? required : ["text"];
  const candidates = Object.values(MODELS)
    .filter((s) => need.every((r) => s.modalities.includes(r)))
    .sort((a, b) => a.sizeBytes - b.sizeBytes);
  if (candidates.length === 0) {
    throw new Error(`no local LLM supports modalities: ${need.join(", ")}`);
  }
  return candidates[0];
}

export interface GemmaProgress {
  /** "downloading" while fetching shards, "ready" once the model is loaded. */
  status: "downloading" | "ready";
  /** File currently being fetched (during downloading). */
  file?: string;
  /** 0–100 fraction for the current file, when known. */
  progress?: number;
  /** Aggregate bytes downloaded across all shards seen so far. */
  loaded?: number;
  /** Aggregate byte total across all shards seen so far (grows as new shards are
   *  discovered). Derive a smooth overall percentage from loaded/total rather
   *  than the per-file `progress`, which restarts at 0 for each shard. */
  total?: number;
}

export interface InstallGemmaOptions {
  /** Override the model's default quantization (each model picks a sensible one). */
  dtype?: DType;
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
// published types are imperfect for our exact runtime shapes, so we pin them.
interface EncodedInputs {
  input_ids: { dims: number[] };
  [key: string]: unknown;
}
interface GeneratedSequence {
  dims: number[];
  slice(...args: unknown[]): unknown;
}
interface GemmaProcessor {
  apply_chat_template(messages: unknown, opts: unknown): Promise<EncodedInputs>;
  batch_decode(tokens: unknown, opts: unknown): string[];
}
interface CausalTokenizer {
  apply_chat_template(messages: unknown, opts: unknown): EncodedInputs | Promise<EncodedInputs>;
  batch_decode(tokens: unknown, opts: unknown): string[];
}
interface GenModel {
  generate(opts: Record<string, unknown>): Promise<GeneratedSequence>;
}

type LoadedModel =
  | { kind: "causal"; tokenizer: CausalTokenizer; model: GenModel }
  | { kind: "multimodal"; processor: GemmaProcessor; model: GenModel };

// ---------------------------------------------------------------------------
// Per-model load cache
// ---------------------------------------------------------------------------

const loadCache = new Map<string, Promise<LoadedModel>>();

// Build a transformers.js progress_callback that aggregates per-shard bytes into
// a smooth overall loaded/total (the library reports per FILE, restarting at 0
// for each shard, which makes a naive bar jump backward).
function makeProgressCallback(onProgress?: (p: GemmaProgress) => void) {
  if (!onProgress) return undefined;
  const fileBytes = new Map<string, { loaded: number; total: number }>();
  return (p: {
    status?: string;
    file?: string;
    name?: string;
    loaded?: number;
    total?: number;
    progress?: number;
  }) => {
    const key = p.file ?? p.name;
    if (key) {
      if (p.status === "done") {
        const prev = fileBytes.get(key);
        if (prev) fileBytes.set(key, { loaded: prev.total, total: prev.total });
      } else if (typeof p.total === "number" && p.total > 0) {
        fileBytes.set(key, { loaded: p.loaded ?? 0, total: p.total });
      }
    }
    let loaded = 0;
    let total = 0;
    for (const b of fileBytes.values()) {
      loaded += b.loaded;
      total += b.total;
    }
    onProgress({
      status: p.status === "ready" || p.status === "done" ? "ready" : "downloading",
      file: p.file,
      progress: p.progress,
      loaded: total > 0 ? loaded : undefined,
      total: total > 0 ? total : undefined,
    });
  };
}

// loadLLM loads (once, cached) the model for `modelKey`. Multimodal models use
// AutoProcessor + Gemma4ForConditionalGeneration; text models use AutoTokenizer +
// AutoModelForCausalLM.
function loadLLM(modelKey: string, opts: InstallGemmaOptions): Promise<LoadedModel> {
  const cached = loadCache.get(modelKey);
  if (cached) return cached;
  const spec = MODELS[modelKey];
  if (!spec) return Promise.reject(new Error(`unknown LLM model: ${modelKey}`));
  const dtype = opts.dtype ?? spec.dtype;
  const device = opts.device ?? spec.device;
  const progress_callback = makeProgressCallback(opts.onProgress);

  const p = (async (): Promise<LoadedModel> => {
    if (spec.loader === "multimodal") {
      const processor = (await AutoProcessor.from_pretrained(spec.hfId, {
        progress_callback,
      })) as unknown as GemmaProcessor;
      const model = (await Gemma4ForConditionalGeneration.from_pretrained(spec.hfId, {
        dtype,
        device,
        progress_callback,
      })) as unknown as GenModel;
      opts.onProgress?.({ status: "ready" });
      return { kind: "multimodal", processor, model };
    }
    const tokenizer = (await AutoTokenizer.from_pretrained(spec.hfId, {
      progress_callback,
    })) as unknown as CausalTokenizer;
    const model = (await AutoModelForCausalLM.from_pretrained(spec.hfId, {
      dtype,
      device,
      progress_callback,
    })) as unknown as GenModel;
    opts.onProgress?.({ status: "ready" });
    return { kind: "causal", tokenizer, model };
  })().catch((err) => {
    loadCache.delete(modelKey); // allow a retry after a transient failure
    throw err;
  });
  loadCache.set(modelKey, p);
  return p;
}

// ---------------------------------------------------------------------------
// Generation
// ---------------------------------------------------------------------------

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

// toMultimodalContent / toTemplateMessages mirror the two-step multimodal flow:
// images are passed SEPARATELY to processor(text, images); apply_chat_template
// emits the image soft-token placeholders, then the processor aligns the images.
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

function decodeContinuation(
  decode: (tokens: unknown, opts: unknown) => string[],
  inputs: EncodedInputs,
  generated: GeneratedSequence,
): GemmaResult {
  const promptLen = inputs.input_ids.dims.at(-1) ?? 0;
  const totalLen = generated.dims.at(-1) ?? promptLen;
  const newTokens = generated.slice(null, [promptLen, null]);
  const text = decode(newTokens, { skip_special_tokens: true })[0];
  return {
    text: (text ?? "").trim(),
    input_tokens: promptLen,
    output_tokens: Math.max(0, totalLen - promptLen),
  };
}

function genArgs(inputs: EncodedInputs, gen: GenOpts): Record<string, unknown> {
  const doSample = (gen.temperature ?? 0) > 0;
  return {
    ...inputs,
    max_new_tokens: gen.maxTokens && gen.maxTokens > 0 ? gen.maxTokens : 256,
    do_sample: doSample,
    // Small quantized models loop under pure greedy decoding (repeating a phrase
    // or a single token). A mild repetition penalty curbs that without harming
    // tasks that legitimately repeat source words (segmentation/translation).
    repetition_penalty: 1.3,
    ...(doSample ? { temperature: gen.temperature, top_p: gen.topP } : {}),
  };
}

// generate runs one chat completion on the (cached) model and returns the decoded
// continuation. Text models take the one-step tokenizer path; multimodal models
// take the two-step processor flow when an image is present.
async function generate(
  loaded: LoadedModel,
  wire: WireMessage[],
  gen: GenOpts,
): Promise<GemmaResult> {
  if (loaded.kind === "causal") {
    const { tokenizer, model } = loaded;
    const messages = wire.map((m) => ({ role: m.role, content: m.text ?? "" }));
    const inputs = (await tokenizer.apply_chat_template(messages, {
      add_generation_prompt: true,
      tokenize: true,
      return_dict: true,
    })) as EncodedInputs;
    const generated = await model.generate(genArgs(inputs, gen));
    return decodeContinuation((t, o) => tokenizer.batch_decode(t, o), inputs, generated);
  }

  const { processor, model } = loaded;
  const imageURLs: string[] = [];
  for (const m of wire) {
    for (const md of m.media ?? []) {
      if (md.kind === "image" && md.data_url) imageURLs.push(md.data_url);
    }
  }
  const proc = processor as unknown as {
    (text: string, images?: unknown): Promise<EncodedInputs>;
    apply_chat_template(messages: unknown, opts: unknown): string;
  };
  const text = proc.apply_chat_template(toTemplateMessages(wire), { add_generation_prompt: true });
  const images = await Promise.all(imageURLs.map((u) => load_image(u)));
  const inputs = await proc(text, images.length === 1 ? images[0] : images);
  const generated = await model.generate(genArgs(inputs, gen));
  return decodeContinuation((t, o) => processor.batch_decode(t, o), inputs, generated);
}

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

/**
 * installGemmaBridge wires globalThis.kapiGemmaGenerate so the in-wasm "gemma"
 * provider (kapi --provider gemma) runs a real model on this page. The model is
 * auto-selected per request: image present → multimodal Gemma; text only → the
 * small text model. Each loads on first use, cached thereafter.
 */
export function installGemmaBridge(opts: InstallGemmaOptions = {}): void {
  (globalThis as Record<string, unknown>).kapiGemmaGenerate = async (
    payloadJSON: string,
  ): Promise<GemmaResult> => {
    const payload: WirePayload = JSON.parse(payloadJSON);
    const messages = payload.messages ?? [];
    const spec = pickModelForModalities(requiredModalities(messages));
    const loaded = await loadLLM(spec.key, opts);
    return generate(loaded, messages, {
      maxTokens: payload.max_tokens,
      temperature: payload.temperature,
      topP: payload.top_p,
    });
  };
}

/** uninstallGemmaBridge removes the host hook (e.g. on component unmount). */
export function uninstallGemmaBridge(): void {
  delete (globalThis as Record<string, unknown>).kapiGemmaGenerate;
}

/** isGemmaModelLoaded reports whether any LLM model has started loading. */
export function isGemmaModelLoaded(): boolean {
  return loadCache.size > 0;
}

/**
 * ensureLLM proactively downloads + loads a specific model AND installs the wasm
 * host hook. Idempotent per model.
 */
export async function ensureLLM(modelKey: string, opts: InstallGemmaOptions = {}): Promise<void> {
  await loadLLM(modelKey, opts);
  installGemmaBridge(opts);
}

/**
 * ensureLLMForModalities pre-loads the smallest model that covers the given input
 * modalities (with progress), so a lab/flow can warm the RIGHT model before it
 * runs — text-only stays light; an image/audio/video task pulls the multimodal
 * one. The same selection drives kapiGemmaGenerate.
 */
export async function ensureLLMForModalities(
  modalities: Modality[],
  opts: InstallGemmaOptions = {},
): Promise<void> {
  const spec = pickModelForModalities(modalities);
  await loadLLM(spec.key, opts);
  installGemmaBridge(opts);
}

/**
 * ensureGemma warms the DEFAULT TEXT model (the small download) and installs the
 * host hook, so the plugin manager's "Download" action / widget loads the light
 * model. Heavier models load on demand only when an image/audio task runs.
 */
export async function ensureGemma(opts: InstallGemmaOptions = {}): Promise<void> {
  await ensureLLMForModalities(["text"], opts);
}

/**
 * runGemmaImageOCR runs an image-capable model directly on an image (from React,
 * not via the wasm global) and returns its transcription — used by the Vision
 * lab to compare generative OCR. Pulls the multimodal model on first use.
 */
export async function runGemmaImageOCR(
  imageDataURL: string,
  prompt: string,
  opts: InstallGemmaOptions = {},
): Promise<GemmaResult> {
  const loaded = await loadLLM(pickModelForModalities(["image"]).key, opts);
  const wire: WireMessage[] = [
    { role: "user", text: prompt, media: [{ kind: "image", data_url: imageDataURL }] },
  ];
  // OCR wants faithful transcription, not creativity → greedy, generous budget.
  return generate(loaded, wire, { maxTokens: 1024, temperature: 0 });
}

/**
 * generateGemmaText runs a single text prompt through the small TEXT model and
 * returns the decoded continuation — a direct React-side call (not via the wasm
 * host hook). Used e.g. by LLM-assisted sentence segmentation. `temperature`
 * defaults to 0 (greedy) for deterministic tasks.
 */
export async function generateGemmaText(
  prompt: string,
  opts: InstallGemmaOptions & GenOpts = {},
): Promise<string> {
  return generateLLMText(prompt, DEFAULT_TEXT_MODEL, opts);
}

/**
 * generateLLMText runs a single text prompt through a SPECIFIC model (by key),
 * loading it (once, cached) on first use. Lets a caller compare models — e.g.
 * the small Llama vs. the larger multimodal Gemma — on the same prompt.
 */
export async function generateLLMText(
  prompt: string,
  modelKey: string,
  opts: InstallGemmaOptions & GenOpts = {},
): Promise<string> {
  const loaded = await loadLLM(modelKey, opts);
  const res = await generate(loaded, [{ role: "user", text: prompt }], {
    maxTokens: opts.maxTokens ?? 512,
    temperature: opts.temperature ?? 0,
    topP: opts.topP,
  });
  return res.text;
}

/** Public, UI-friendly view of a registered LLM model. */
export interface LLMModelInfo {
  key: string;
  label: string;
  sizeBytes: number;
  modalities: Modality[];
}

/** listLLMModels returns the registered models (for a picker / engine list). */
export function listLLMModels(): LLMModelInfo[] {
  return Object.values(MODELS).map((s) => ({
    key: s.key,
    label: s.label,
    sizeBytes: s.sizeBytes,
    modalities: [...s.modalities],
  }));
}
