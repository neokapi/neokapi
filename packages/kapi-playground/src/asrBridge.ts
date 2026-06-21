// asrBridge — speech recognition in the browser via @huggingface/transformers
// (onnxruntime-web under the hood), the same Whisper family the native kapi-asr
// plugin runs through whisper.cpp. Only the runtime differs: WebAssembly here,
// native there. The model downloads from the HF hub on first use and is cached
// by the browser; nothing is mocked.
//
// The output is the format-agnostic shape the content model uses — timing-anchored
// segments (start/end in ms) — so the lab can feed them straight into the
// SubtitleTimeline / VideoPlayer components without a translation step.

export interface ASRSegment {
  text: string;
  startMs: number;
  endMs: number;
}

export interface ASRResult {
  segments: ASRSegment[];
}

export interface ASROptions {
  /** transformers.js model id; defaults to a small English Whisper. */
  model?: string;
  /** Source language hint (e.g. "en"); omit for auto-detect. */
  language?: string;
  /** Download/inference progress in [0,1]. */
  onProgress?: (frac: number) => void;
}

const DEFAULT_MODEL = "Xenova/whisper-tiny.en";

// transformers.js types are intentionally loose at the call boundary; we narrow
// to the only shapes we read (the ASR pipeline fn + its timestamped chunks).
type Transcriber = (
  audio: Float32Array,
  opts: Record<string, unknown>,
) => Promise<{ text?: string; chunks?: { text?: string; timestamp?: [number, number | null] }[] }>;

let pipe: Promise<Transcriber> | null = null;

/** Whether the ASR model has started loading (so the UI can show a one-time hint). */
export function asrLoaded(): boolean {
  return pipe !== null;
}

/**
 * ensureASRModel proactively downloads + loads the Whisper model so the plugin
 * manager's "Download" action can warm ASR before the first transcription.
 * Idempotent; progress (0..1) flows through onProgress.
 */
export async function ensureASRModel(
  onProgress?: (frac: number) => void,
  model: string = DEFAULT_MODEL,
): Promise<void> {
  await ensureASR(model, onProgress);
}

async function ensureASR(model: string, onProgress?: (frac: number) => void): Promise<Transcriber> {
  if (!pipe) {
    pipe = (async () => {
      const { pipeline } = await import("@huggingface/transformers");
      // Pin a per-component fp32 dtype. With the default, transformers.js loads a
      // 4-bit (MatMulNBits) Whisper decoder that onnxruntime-web 1.26 can't create
      // a session for ("TransposeDQWeightsForMatMulNBits Missing required scale …").
      // fp32 has no quantize/dequantize ops at all, so it always loads. Run on the
      // wasm (CPU) EP, matching visionBridge; Whisper-tiny is small, so it's fast.
      const p = await pipeline("automatic-speech-recognition", model, {
        dtype: { encoder_model: "fp32", decoder_model_merged: "fp32" },
        device: "wasm",
        progress_callback: (e: { status?: string; progress?: number }) => {
          if (onProgress && e.status === "progress") onProgress((e.progress ?? 0) / 100);
        },
      });
      return p as unknown as Transcriber;
    })().catch((err) => {
      pipe = null; // reset so a Retry re-attempts instead of returning the rejection
      throw err;
    });
  }
  return pipe;
}

/**
 * decodeAudio decodes an audio (or video) file's audio track to a mono, 16 kHz
 * Float32Array — the input Whisper expects. It uses the Web Audio API to decode
 * the container, then an OfflineAudioContext to downmix + resample. Works for
 * the audio track of common containers the browser can decode (wav/mp3/m4a/ogg,
 * and the audio of mp4/webm in browsers that decode it).
 */
export async function decodeAudio(data: ArrayBuffer): Promise<Float32Array> {
  const Ctx: typeof AudioContext =
    window.AudioContext ??
    (window as unknown as { webkitAudioContext: typeof AudioContext }).webkitAudioContext;
  const ctx = new Ctx();
  try {
    const decoded = await ctx.decodeAudioData(data.slice(0));
    const targetRate = 16000;
    const frames = Math.max(1, Math.ceil(decoded.duration * targetRate));
    const offline = new OfflineAudioContext(1, frames, targetRate);
    const src = offline.createBufferSource();
    src.buffer = decoded;
    src.connect(offline.destination);
    src.start();
    const rendered = await offline.startRendering();
    return rendered.getChannelData(0).slice();
  } finally {
    await ctx.close();
  }
}

/**
 * transcribe runs Whisper over a decoded mono 16 kHz audio buffer and returns
 * timing-anchored segments. Pass `decodeAudio(fileBytes)` to get the buffer.
 */
export async function transcribe(audio: Float32Array, opts: ASROptions = {}): Promise<ASRResult> {
  const transcriber = await ensureASR(opts.model ?? DEFAULT_MODEL, opts.onProgress);
  const out = await transcriber(audio, {
    return_timestamps: true,
    chunk_length_s: 30,
    stride_length_s: 5,
    ...(opts.language ? { language: opts.language } : {}),
  });
  const chunks = out.chunks ?? [];
  const segments: ASRSegment[] = chunks
    .map((c) => {
      const start = c.timestamp?.[0] ?? 0;
      const end = c.timestamp?.[1] ?? start;
      return {
        text: (c.text ?? "").trim(),
        startMs: Math.round(start * 1000),
        endMs: Math.round((end ?? start) * 1000),
      };
    })
    .filter((s) => s.text.length > 0);
  return { segments };
}
