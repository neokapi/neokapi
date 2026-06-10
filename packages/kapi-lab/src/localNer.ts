// On-device NER for the lab: loads a GLiNER ONNX model with onnxruntime-web
// (zero-shot NER — the model scores arbitrary label prompts, no fixed tag set)
// and registers the `kapiLocalNER` bridge the wasm engine's `ai-entity-extract`
// calls when configured with `engine: ner`. Everything runs in the browser:
// the text never leaves the page, which is exactly the property the redaction
// placement rule reasons about (no remote source egress).
//
// The model (~50 MB quantized) downloads from the Hugging Face CDN on first
// use and is cached by the browser; loading is explicit and lazy so the lab
// page itself stays light.

// The categories the bridge detects, matching the engine's entity vocabulary
// (redaction normalizes "organization" → org).
const LABELS = ["person", "organization", "location", "date", "product"];

// Quantized multilingual-capable small model from the official ONNX conversion.
const TOKENIZER_REPO = "onnx-community/gliner_small-v2";
const MODEL_URL =
  "https://huggingface.co/onnx-community/gliner_small-v2/resolve/main/onnx/model_quantized.onnx";
// onnxruntime-web loads its wasm runtime from the matching published version.
const ORT_WASM_CDN = "https://cdn.jsdelivr.net/npm/onnxruntime-web@1.19.2/dist/";

interface GlinerSpan {
  spanText: string;
  start?: number;
  end?: number;
  label?: string;
  score?: number;
}

interface GlinerLike {
  initialize(): Promise<unknown>;
  inference(args: {
    texts: string[];
    entities: string[];
    threshold?: number;
    flatNer?: boolean;
  }): Promise<GlinerSpan[][]>;
}

declare global {
  // The wasm engine's local-NER bridge probes this global per call.
  // eslint-disable-next-line no-var
  var kapiLocalNER: ((reqJSON: string) => Promise<string>) | undefined;
}

let loading: Promise<void> | null = null;

/** Whether the bridge is registered (the model is ready). */
export function localNerLoaded(): boolean {
  return typeof globalThis.kapiLocalNER === "function";
}

/** Map a GLiNER label onto the engine's bare entity categories. */
function mapLabel(label: string | undefined): string {
  const l = (label ?? "").toLowerCase();
  return LABELS.includes(l) ? l : "other";
}

/**
 * Load the GLiNER model (once) and register the `kapiLocalNER` bridge.
 * Subsequent calls await the same load. onStatus receives coarse progress
 * messages for the lab's status bar.
 */
export function ensureLocalNer(onStatus?: (msg: string) => void): Promise<void> {
  if (localNerLoaded()) return Promise.resolve();
  if (loading) return loading;
  loading = (async () => {
    onStatus?.("Downloading the on-device NER model (~50 MB, cached after the first load)…");
    const { Gliner } = (await import("gliner")) as unknown as {
      Gliner: new (opts: unknown) => GlinerLike;
    };
    const gliner = new Gliner({
      tokenizerPath: TOKENIZER_REPO,
      onnxSettings: {
        modelPath: MODEL_URL,
        executionProvider: "wasm",
        wasmPaths: ORT_WASM_CDN,
      },
    });
    onStatus?.("Initializing the on-device NER model…");
    await gliner.initialize();

    globalThis.kapiLocalNER = async (reqJSON: string): Promise<string> => {
      const req = JSON.parse(reqJSON) as { text: string; locale?: string };
      const results = await gliner.inference({
        texts: [req.text],
        entities: LABELS,
        threshold: 0.3,
        flatNer: true,
      });
      const spans = results[0] ?? [];
      return JSON.stringify({
        entities: spans.map((s) => {
          const start = s.start ?? 0;
          const end = s.end ?? start + s.spanText.length;
          return {
            text: s.spanText,
            type: mapLabel(s.label),
            confidence: s.score ?? 0,
            offset: start,
            length: Math.max(0, end - start),
          };
        }),
      });
    };
    onStatus?.("On-device NER model ready.");
  })().catch((err) => {
    loading = null; // allow a retry after a failed download
    throw err;
  });
  return loading;
}
