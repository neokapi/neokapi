// localLlmBridge installs the host hook the in-WASM "local" AI provider
// (kapi/cmd/kapi-wasm-cli/local_bridge.go) calls to run a model on-device in the
// browser. It is the browser counterpart of the native CLI/desktop story, where
// the "local" provider is a local Ollama runtime: here it is WebGPU.
//
// Engine selection — best per platform:
//   • WebGPU present → WebLLM (@mlc-ai/web-llm), MLC-compiled models that mirror
//     the native Ollama lineup by name (Llama 3.2 3B, Qwen3 1.7B). This is the
//     fast, desktop-like path.
//   • No WebGPU → transformers.js fallback (gemmaBridge, Llama 3.2 1B over WASM),
//     slower but keeps the demo working everywhere.
//
// # Host contract
//
//	globalThis.kapiLocalGenerate(payloadJSON: string)
//	  => Promise<{ text: string, input_tokens?: number, output_tokens?: number }>
//
// payloadJSON is {messages:[{role,text}], model, max_tokens, temperature}. The
// wasm provider blocks its goroutine on the returned promise (safe — kapiRun runs
// each command in a goroutine and returns a Promise, so the event loop keeps
// running).

import { CreateMLCEngine, type MLCEngine, type InitProgressReport } from "@mlc-ai/web-llm";
import { generateLLMText, type InstallGemmaOptions } from "./gemmaBridge";

/** A selectable in-browser local model. */
export interface LocalModelSpec {
  /** kapi-facing id, identical to the native `kapi ollama` name (passed as
   * --model) so the same model reference works on web and desktop. */
  id: string;
  /** Browser backend that has a build for this model. We pick the best engine
   * per model, not per platform: WebLLM (MLC) for models with an MLC build, and
   * transformers.js (ONNX) for models that only ship an ONNX build (e.g. Gemma 4,
   * which has no MLC build but runs well via transformers.js + WebGPU). */
  engine: "webllm" | "transformers";
  /** WebLLM prebuilt (MLC) model id — set when engine === "webllm". */
  webllm?: string;
  /** transformers.js model key (a gemmaBridge MODELS key) — the ONNX build for
   * engine "transformers", and the no-WebGPU fallback for "webllm" models. */
  transformers?: string;
  /** Human label for pickers. */
  label: string;
  /** Approx download size, for the UI. */
  size: string;
  /** Short note (e.g. "default"). */
  note?: string;
}

// Parity with the native `kapi ollama` recommended lineup — the SAME model names
// on web and desktop, even though the browser backend differs per model: Llama
// 3.2 and Qwen3 have MLC builds (WebLLM/WebGPU); Gemma 4 has only an ONNX build,
// so it runs via transformers.js (also WebGPU). aya-expanse:8b is desktop-only
// (no browser build, too large for the browser).
export const LOCAL_MODELS: LocalModelSpec[] = [
  {
    id: "llama3.2:3b",
    engine: "webllm",
    webllm: "Llama-3.2-3B-Instruct-q4f16_1-MLC",
    transformers: "llama-3.2-1b", // no-WebGPU fallback
    label: "Llama 3.2 3B",
    size: "~2.3 GB",
    note: "default · reliable inline tags",
  },
  {
    id: "gemma4:e2b",
    engine: "transformers",
    transformers: "gemma-4-e2b",
    label: "Gemma 4 E2B",
    size: "~3 GB",
    note: "best multilingual quality",
  },
  {
    id: "qwen3:1.7b",
    engine: "webllm",
    webllm: "Qwen3-1.7B-q4f16_1-MLC",
    transformers: "llama-3.2-1b", // no-WebGPU fallback
    label: "Qwen3 1.7B",
    size: "~1.4 GB",
    note: "fastest",
  },
];

export const DEFAULT_LOCAL_MODEL = "llama3.2:3b";

/** Progress for the local model's first-run download/initialization. */
export interface LocalProgress {
  /** "downloading" while fetching/compiling weights, "ready" once loaded. */
  status: "downloading" | "ready";
  /** 0–100 overall fraction, when known. */
  progress?: number;
  /** Human status line from the engine. */
  text?: string;
  /** File currently being fetched (transformers.js fallback). */
  file?: string;
  /** Which engine is serving: WebLLM (WebGPU) or the transformers.js fallback. */
  engine: "webllm" | "transformers";
}

export interface InstallLocalLLMOptions {
  /** Default model id when a payload names none. */
  model?: string;
  /** Progress callback for the first-run download/init. */
  onProgress?: (p: LocalProgress) => void;
}

/** Whether this platform can run the fast WebGPU (WebLLM) path. */
export function webgpuAvailable(): boolean {
  return typeof navigator !== "undefined" && "gpu" in navigator && Boolean(navigator.gpu);
}

function specFor(modelId: string | undefined): LocalModelSpec {
  return LOCAL_MODELS.find((m) => m.id === modelId) ?? LOCAL_MODELS[0];
}

// One WebLLM engine per MLC model id (weights are cached by the browser after the
// first load, so re-creating an engine for a cached model is cheap).
const engines = new Map<string, Promise<MLCEngine>>();

function getEngine(webllmId: string, onProgress?: (p: LocalProgress) => void): Promise<MLCEngine> {
  let p = engines.get(webllmId);
  if (!p) {
    p = CreateMLCEngine(webllmId, {
      initProgressCallback: (r: InitProgressReport) => {
        onProgress?.({
          status: r.progress >= 1 ? "ready" : "downloading",
          progress: Math.round(r.progress * 100),
          text: r.text,
          engine: "webllm",
        });
      },
    }).catch((e: unknown) => {
      engines.delete(webllmId); // allow a retry after a transient failure
      throw e;
    });
    engines.set(webllmId, p);
  }
  return p;
}

interface WirePayload {
  messages?: Array<{ role: string; text?: string }>;
  model?: string;
  max_tokens?: number;
  temperature?: number;
}

interface LocalResult {
  text: string;
  input_tokens?: number;
  output_tokens?: number;
}

function promptFromMessages(messages: WirePayload["messages"]): string {
  return (messages ?? [])
    .map((m) => m.text ?? "")
    .filter(Boolean)
    .join("\n\n");
}

async function generateWebLLM(
  spec: LocalModelSpec,
  payload: WirePayload,
  opts: InstallLocalLLMOptions,
): Promise<LocalResult> {
  const engine = await getEngine(spec.webllm as string, opts.onProgress);

  const msgs: Array<{ role: "system" | "user" | "assistant"; content: string }> = [];
  // Qwen3 is a reasoning model: keep it out of "thinking" mode so it returns the
  // answer directly instead of a <think> block (mirrors the native think:false).
  if (spec.id.startsWith("qwen3")) {
    msgs.push({ role: "system", content: "/no_think" });
  }
  for (const m of payload.messages ?? []) {
    const role = m.role === "assistant" || m.role === "system" ? m.role : "user";
    msgs.push({ role, content: m.text ?? "" });
  }

  const resp = await engine.chat.completions.create({
    messages: msgs,
    temperature: payload.temperature && payload.temperature > 0 ? payload.temperature : 0.2,
    max_tokens: payload.max_tokens && payload.max_tokens > 0 ? payload.max_tokens : undefined,
    stream: false,
  });
  const raw = resp.choices[0]?.message?.content ?? "";
  return {
    text: stripThink(raw),
    input_tokens: resp.usage?.prompt_tokens,
    output_tokens: resp.usage?.completion_tokens,
  };
}

// stripThink removes any <think>…</think> block from a reasoning model's output.
// `/no_think` makes Qwen3 emit an EMPTY think block rather than none, so the tags
// still arrive and must be stripped (the native Ollama path uses the think:false
// API flag, which suppresses them server-side).
function stripThink(s: string): string {
  return s.replace(/<think>[\s\S]*?<\/think>/gi, "").trim();
}

// generateTransformers runs a model through transformers.js (gemmaBridge): the
// ONNX build for an engine:"transformers" model (e.g. Gemma 4), or the small
// no-WebGPU fallback for a "webllm" model on a browser without WebGPU.
async function generateTransformers(
  spec: LocalModelSpec,
  payload: WirePayload,
  opts: InstallLocalLLMOptions,
): Promise<LocalResult> {
  const gemmaOpts: InstallGemmaOptions = {
    onProgress: (g) =>
      opts.onProgress?.({
        status: g.status,
        progress: g.progress,
        file: g.file,
        text: g.file,
        engine: "transformers",
      }),
  };
  const text = await generateLLMText(
    promptFromMessages(payload.messages),
    spec.transformers ?? "llama-3.2-1b",
    gemmaOpts,
  );
  return { text: stripThink(text) };
}

/**
 * installLocalLLMBridge installs globalThis.kapiLocalGenerate, routing each model
 * to the best browser backend that has a build for it: WebLLM (WebGPU) for MLC
 * models, transformers.js (ONNX) for the rest, and the transformers.js fallback
 * for WebLLM models on a browser without WebGPU. The model is resolved per call
 * (from the payload), so switching models in the UI just works. Idempotent; a
 * model only downloads when a generation actually runs.
 */
export function installLocalLLMBridge(opts: InstallLocalLLMOptions = {}): void {
  if (typeof window === "undefined") return;
  (globalThis as Record<string, unknown>).kapiLocalGenerate = async (
    payloadJSON: string,
  ): Promise<LocalResult> => {
    const payload = JSON.parse(payloadJSON) as WirePayload;
    const spec = specFor(payload.model ?? opts.model);
    const useWebLLM = spec.engine === "webllm" && webgpuAvailable();
    return useWebLLM ? generateWebLLM(spec, payload, opts) : generateTransformers(spec, payload, opts);
  };
}

/** Whether the host hook the wasm provider needs is installed. */
export function localLLMReady(): boolean {
  return typeof (globalThis as Record<string, unknown>).kapiLocalGenerate === "function";
}

/**
 * ensureLocalLLM pre-loads a model (with progress) so the first translate is
 * instant. Resolves once weights are downloaded and the engine is ready.
 */
export async function ensureLocalLLM(
  modelId: string,
  opts: InstallLocalLLMOptions = {},
): Promise<void> {
  installLocalLLMBridge({ ...opts, model: modelId });
  const spec = specFor(modelId);
  if (spec.engine === "webllm" && spec.webllm && webgpuAvailable()) {
    await getEngine(spec.webllm, opts.onProgress);
  }
  // transformers.js models (and the no-WebGPU fallback) load lazily on first generate.
}
