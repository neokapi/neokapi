// satBridge — SaT (Segment any Text / wtpsplit) sentence segmentation in the
// browser, the counterpart to the native kapi-sat plugin. The native plugin is
// cgo + onnxruntime; here the SAME ONNX model (segment-any-text/sat-3l-sm, a
// SubwordXLM token-classification head over XLM-RoBERTa) runs on onnxruntime-web,
// with the XLM-R SentencePiece tokenizer from @huggingface/transformers. Nothing
// is mocked — it is the real model.
//
// The deterministic pipeline mirrors plugins/sat/internal/sat/algo.go verbatim:
// tokenize → plan overlapping blocks (512/256) → run each block → average the
// overlapping per-token boundary logits → sigmoid+threshold → interior sentence
// boundaries. The one browser-specific wrinkle is offsets: transformers.js does
// not expose an offset mapping, so we reconstruct token→codepoint spans from the
// SentencePiece pieces (the `▁` metaspace marks word starts) by greedily aligning
// them to the source. Boundaries are returned as interior code-point offsets —
// exactly the `satproto` Response.Boundaries contract (rune offsets).
//
// Loading is lazy and opt-in: the ~428 MB fp16 model downloads on first use,
// cached in the Cache API so a return visit is instant. Gate it behind an
// explicit action (the plugin manager's Download); progress flows via onProgress.

import * as ort from "onnxruntime-web";

// Match the shared onnxruntime-web config used by visionBridge: single-threaded
// (no COOP/COEP needed), wasm from a version-matched CDN, quiet logs.
ort.env.wasm.numThreads = 1;
ort.env.wasm.wasmPaths = `https://cdn.jsdelivr.net/npm/onnxruntime-web@${ort.env.versions.web}/dist/`;
ort.env.logLevel = "error";
const SESSION_OPTS: ort.InferenceSession.SessionOptions = { logSeverityLevel: 3 };

// Default inference parameters, mirroring algo.go / wtpsplit-lite for *-sm.
const BLOCK_SIZE = 512;
const STRIDE = 256;
const DEFAULT_THRESHOLD = 0.25;

// The default browser model. fp16, ~428 MB; cached after first download.
const MODEL_ID = "segment-any-text/sat-3l-sm";
const MODEL_URL = `https://huggingface.co/${MODEL_ID}/resolve/main/model.onnx`;
// XLM-RoBERTa SentencePiece tokenizer (transformers.js-compatible mirror).
const TOKENIZER_ID = "Xenova/xlm-roberta-base";

// XLM-R special tokens (from the SaT config): CLS=0, SEP=2.
const CLS_ID = 0n;
const SEP_ID = 2n;
const ONE_F16 = 0x3c00; // 1.0 in IEEE-754 half precision

// ---------------------------------------------------------------------------
// Lazy load
// ---------------------------------------------------------------------------

interface Tokenizer {
  encode(text: string, pair: unknown, opts: { add_special_tokens: boolean }): number[];
  model: { vocab: Array<[string, number]> | Record<number, string> };
}

interface Loaded {
  session: ort.InferenceSession;
  tokenizer: Tokenizer;
}

let loadPromise: Promise<Loaded> | null = null;

/** Whether the SaT model has started loading. */
export function satLoaded(): boolean {
  return loadPromise !== null;
}

// cachedFetch fetches a (large) asset through the Cache API so it downloads once,
// reporting byte progress to onProgress. Falls back to a plain fetch if the
// Cache API is unavailable.
async function cachedFetch(url: string, onProgress?: (frac: number) => void): Promise<ArrayBuffer> {
  const cacheName = "kapi-sat-v1";
  let cache: Cache | null = null;
  try {
    cache = await caches.open(cacheName);
    const hit = await cache.match(url);
    if (hit) {
      onProgress?.(1);
      return await hit.arrayBuffer();
    }
  } catch {
    cache = null;
  }
  const resp = await fetch(url);
  if (!resp.ok || !resp.body) throw new Error(`sat: fetch ${url}: ${resp.status}`);
  const total = Number(resp.headers.get("content-length")) || 0;
  const reader = resp.body.getReader();
  const chunks: Uint8Array[] = [];
  let loaded = 0;
  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;
    chunks.push(value);
    loaded += value.length;
    if (total) onProgress?.(loaded / total);
  }
  const buf = new Uint8Array(loaded);
  let off = 0;
  for (const c of chunks) {
    buf.set(c, off);
    off += c.length;
  }
  if (cache) {
    try {
      await cache.put(url, new Response(buf, { headers: { "content-length": String(loaded) } }));
    } catch {
      /* over quota — fine, just re-download next time */
    }
  }
  onProgress?.(1);
  return buf.buffer;
}

function load(onProgress?: (frac: number) => void): Promise<Loaded> {
  if (loadPromise) return loadPromise;
  loadPromise = (async () => {
    const { AutoTokenizer } = await import("@huggingface/transformers");
    // The model is the dominant download; weight progress toward it.
    const [buf, tokenizer] = await Promise.all([
      cachedFetch(MODEL_URL, onProgress),
      AutoTokenizer.from_pretrained(TOKENIZER_ID) as unknown as Promise<Tokenizer>,
    ]);
    const session = await ort.InferenceSession.create(buf, SESSION_OPTS);
    return { session, tokenizer };
  })().catch((err) => {
    loadPromise = null; // allow retry after a transient failure
    throw err;
  });
  return loadPromise;
}

/**
 * ensureSat proactively downloads + loads the SaT model (and tokenizer) so the
 * plugin manager's Download action can warm it before the first segment call.
 */
export async function ensureSat(onProgress?: (frac: number) => void): Promise<void> {
  await load(onProgress);
}

// ---------------------------------------------------------------------------
// Tokenization with reconstructed code-point offsets
// ---------------------------------------------------------------------------

interface TokenSpan {
  start: number; // code-point index, inclusive
  end: number; // code-point index, exclusive
}

function idToPiece(tk: Tokenizer, id: number): string {
  const v = tk.model.vocab as unknown;
  if (Array.isArray(v)) {
    const entry = v[id];
    return Array.isArray(entry) ? entry[0] : String(entry ?? "");
  }
  return (v as Record<number, string>)[id] ?? "";
}

// tokenizeWithOffsets returns the content token ids (no special tokens) and a
// parallel array of code-point spans into `text`. Offsets are reconstructed from
// the SentencePiece pieces: a leading `▁` (U+2581) marks a word start (a space in
// the source); the remaining characters are matched greedily against the source.
// The match is tolerant (it skips a source char on a normalization mismatch) so
// offsets stay monotonic even when XLM-R's normalizer diverges from the raw text.
function tokenizeWithOffsets(tk: Tokenizer, text: string): { ids: number[]; spans: TokenSpan[] } {
  const ids = tk.encode(text, null, { add_special_tokens: false });
  const cps = Array.from(text);
  const isSpace = (s: string) => /\s/.test(s);
  const spans: TokenSpan[] = [];
  let pos = 0;
  for (const id of ids) {
    let piece = idToPiece(tk, id);
    const wordStart = piece.startsWith("▁");
    if (wordStart) piece = piece.slice(1);
    // A word-initial piece follows whitespace in the source; consume it.
    if (wordStart) {
      while (pos < cps.length && isSpace(cps[pos])) pos++;
    }
    const coreCps = Array.from(piece);
    const start = pos;
    for (const ch of coreCps) {
      // Advance to the matching source char within a small window; on a
      // normalization mismatch, consume one source char so we never stall.
      let scan = pos;
      let matched = false;
      const limit = Math.min(cps.length, pos + 4);
      while (scan < limit) {
        if (cps[scan] === ch || cps[scan].toLowerCase() === ch.toLowerCase()) {
          pos = scan + 1;
          matched = true;
          break;
        }
        scan++;
      }
      if (!matched && pos < cps.length) pos++;
    }
    spans.push({ start, end: Math.max(start, pos) });
  }
  return { ids, spans };
}

// ---------------------------------------------------------------------------
// Block planning + recombination (ports of algo.go)
// ---------------------------------------------------------------------------

interface Block {
  start: number;
  end: number;
}

function planBlocks(n: number, blockSize: number, stride: number): Block[] {
  if (n <= 0) return [];
  if (n <= blockSize) return [{ start: 0, end: n }];
  const blocks: Block[] = [];
  for (let j = 0; j < n; j += stride) {
    let start = j;
    let end = j + blockSize;
    if (end >= n) {
      end = n;
      start = Math.max(end - blockSize, 0);
      blocks.push({ start, end });
      break;
    }
    blocks.push({ start, end });
  }
  return blocks;
}

function combineLogits(n: number, blocks: Block[], blockLogits: number[][]): number[] {
  const sum = Array.from<number>({ length: n }).fill(0);
  const count = Array.from<number>({ length: n }).fill(0);
  for (let bi = 0; bi < blocks.length; bi++) {
    const b = blocks[bi];
    const logits = blockLogits[bi];
    const span = b.end - b.start;
    for (let k = 0; k < span && k < logits.length; k++) {
      const idx = b.start + k;
      sum[idx] += logits[k];
      count[idx]++;
    }
  }
  const out = Array.from<number>({ length: n });
  for (let i = 0; i < n; i++) out[i] = count[i] > 0 ? sum[i] / count[i] : -Infinity;
  return out;
}

function sigmoid(x: number): number {
  return 1 / (1 + Math.exp(-x));
}

function halfToFloat(h: number): number {
  const s = (h & 0x8000) >> 15;
  const e = (h & 0x7c00) >> 10;
  const f = h & 0x03ff;
  const sign = s ? -1 : 1;
  if (e === 0) return sign * Math.pow(2, -14) * (f / 1024);
  if (e === 0x1f) return f ? NaN : sign * Infinity;
  return sign * Math.pow(2, e - 15) * (1 + f / 1024);
}

// ---------------------------------------------------------------------------
// Inference
// ---------------------------------------------------------------------------

// runBlock runs the model over one window of content token ids and returns the
// per-content-token boundary logits (CLS/SEP stripped). Inputs: input_ids (int64)
// and attention_mask (float16, all ones — one block per inference, no padding);
// output: logits [1, seq, 1] (float16).
async function runBlock(session: ort.InferenceSession, content: number[]): Promise<number[]> {
  const seqLen = content.length + 2; // + CLS, SEP
  const idData = new BigInt64Array(seqLen);
  idData[0] = CLS_ID;
  for (let i = 0; i < content.length; i++) idData[i + 1] = BigInt(content[i]);
  idData[seqLen - 1] = SEP_ID;

  const maskData = new Uint16Array(seqLen).fill(ONE_F16);

  const inputIds = new ort.Tensor("int64", idData, [1, seqLen]);
  const attentionMask = new ort.Tensor("float16", maskData, [1, seqLen]);

  const out = await session.run({ input_ids: inputIds, attention_mask: attentionMask });
  const logits = out.logits;
  // float16 output arrives as a Uint16Array; decode each half to float32. The
  // head emits one label per token, so the data is [seq] boundary logits.
  const raw = logits.data as unknown as Uint16Array;
  const result: number[] = [];
  // Strip CLS (index 0) and SEP (index seqLen-1); keep the content tokens.
  for (let i = 1; i <= content.length; i++) result.push(halfToFloat(raw[i]));
  return result;
}

// ---------------------------------------------------------------------------
// Public: segment
// ---------------------------------------------------------------------------

export interface SatSegmentResult {
  /** Interior sentence-boundary offsets as code-point (rune) indices, ascending. */
  boundaries: number[];
  /** The sentences, split at the boundaries. */
  sentences: string[];
}

/**
 * segmentSat segments `text` into sentences with the SaT model, returning both
 * the interior boundary offsets (the satproto contract) and the split sentences.
 * Loads the model on first call (gate it behind an explicit action). Threshold
 * defaults to the *-sm model's 0.25.
 */
export async function segmentSat(
  text: string,
  threshold: number = DEFAULT_THRESHOLD,
): Promise<SatSegmentResult> {
  const { session, tokenizer } = await load();
  const cps = Array.from(text);
  if (cps.length === 0) return { boundaries: [], sentences: [] };

  const { ids, spans } = tokenizeWithOffsets(tokenizer, text);
  const n = ids.length;
  if (n === 0) return { boundaries: [], sentences: [text] };

  const blocks = planBlocks(n, BLOCK_SIZE, STRIDE);
  const blockLogits: number[][] = [];
  for (const b of blocks) blockLogits.push(await runBlock(session, ids.slice(b.start, b.end)));
  const combined = combineLogits(n, blocks, blockLogits);

  const isSpace = (s: string) => /\s/.test(s);
  const boundaries: number[] = [];
  let last = -1;
  for (let i = 0; i < spans.length && i < combined.length; i++) {
    if (sigmoid(combined[i]) < threshold) continue;
    // Boundary at the end of this token; skip following whitespace so the next
    // sentence starts on a non-space character (matches indices_to_sentences).
    let cut = spans[i].end;
    while (cut < cps.length && isSpace(cps[cut])) cut++;
    if (cut <= 0 || cut >= cps.length) continue; // interior only
    if (cut === last) continue;
    boundaries.push(cut);
    last = cut;
  }

  const sentences: string[] = [];
  let prev = 0;
  for (const b of boundaries) {
    sentences.push(cps.slice(prev, b).join(""));
    prev = b;
  }
  sentences.push(cps.slice(prev).join(""));
  return { boundaries, sentences: sentences.map((s) => s.trim()).filter((s) => s.length > 0) };
}
