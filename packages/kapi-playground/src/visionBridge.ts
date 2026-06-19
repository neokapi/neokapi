// Vision bridge — the browser counterpart to the native kapi-vision plugin.
//
// The native plugin is cgo + onnxruntime; the kapi wasm engine can't run that.
// But the *same* PP-OCRv5 (detection + recognition) and PP-DocLayoutV3 (layout)
// models are plain ONNX, so they run in the browser via onnxruntime-web — the ML
// is the real model, nothing mocked. The deterministic pre/post-processing
// (DBNet binarize → connected components → unclip, CRNN+CTC decode, RT-DETR
// decode, reading order) is a faithful TS port of the validated Go pipeline in
// plugins/vision/internal/ocr (algo.go, engine_onnx.go, layout_onnx.go) — kept
// in lockstep with it.
//
// Loading is lazy and tiered: the OCR models (~21 MB) load on first OCR; the
// layout model (~132 MB) loads only when the caller opts in (ensureLayout), so a
// visitor pays for layout only if they ask for it.

import * as ort from "onnxruntime-web";

// onnxruntime-web fetches its own wasm from a version-matched CDN (single
// threaded, so no COOP/COEP cross-origin-isolation headers are needed).
ort.env.wasm.numThreads = 1;
ort.env.wasm.wasmPaths = `https://cdn.jsdelivr.net/npm/onnxruntime-web@${ort.env.versions.web}/dist/`;
// The PaddleOCR→ONNX export leaves unused initializers; ort's graph optimizer
// logs a "Removing initializer …" warning for each (dozens) at session create.
// They are harmless — suppress below the error level so the console stays clean.
ort.env.logLevel = "error";
const SESSION_OPTS: ort.InferenceSession.SessionOptions = { logSeverityLevel: 3 };

// Fallback model host: the pinned neokapi release. NOTE: GitHub release download
// URLs are CORS-blocked for browser fetch(), so in the browser callers MUST pass
// a same-origin (or otherwise CORS-enabled) base — the docs Vision Lab stages the
// models under /models/vision and passes that. This default exists only for
// non-browser / CORS-enabled contexts.
const MODEL_BASE =
  "https://github.com/neokapi/neokapi/releases/download/vision-models-v1";

export interface OCRLine {
  text: string;
  x: number;
  y: number;
  w: number;
  h: number;
  confidence: number;
}
export interface OCRResult {
  width: number;
  height: number;
  lines: OCRLine[];
}
export interface Region {
  role: string;
  x: number;
  y: number;
  w: number;
  h: number;
  confidence: number;
}
export interface LayoutResult {
  width: number;
  height: number;
  regions: Region[];
}

// --- detection / recognition constants (mirror engine_onnx.go) ---
const DET_MAX_SIDE = 960;
const DET_THRESHOLD = 0.3;
const REC_HEIGHT = 48;
// --- layout constants (mirror layout_onnx.go) ---
const LAYOUT_SIZE = 800;
const LAYOUT_SCORE_THRESHOLD = 0.5;

// PP-DocLayoutV3 class labels (layoutmap.go) → neokapi content roles.
const LAYOUT_LABELS = [
  "abstract", "algorithm", "aside_text", "chart", "content",
  "display_formula", "doc_title", "figure_title", "footer", "footer_image",
  "footnote", "formula_number", "header", "header_image", "image",
  "inline_formula", "number", "paragraph_title", "reference", "reference_content",
  "seal", "table", "text", "vertical_text", "vision_footnote",
];
const LAYOUT_ROLE_BY_LABEL: Record<string, string> = {
  doc_title: "title", paragraph_title: "heading", abstract: "paragraph",
  content: "paragraph", text: "paragraph", vertical_text: "paragraph",
  aside_text: "paragraph", reference: "paragraph", reference_content: "paragraph",
  number: "paragraph", table: "table", figure_title: "caption",
  chart: "picture", image: "picture", header_image: "picture",
  footer_image: "picture", seal: "picture", algorithm: "code",
  display_formula: "formula", inline_formula: "formula", formula_number: "formula",
  footnote: "footnote", vision_footnote: "footnote",
  header: "page-header", footer: "page-footer",
};
function layoutRole(classID: number): string {
  if (classID < 0 || classID >= LAYOUT_LABELS.length) return "paragraph";
  return LAYOUT_ROLE_BY_LABEL[LAYOUT_LABELS[classID]] ?? "paragraph";
}

// RGBA is a flat raster (canvas ImageData shape) used for all pixel ops.
interface RGBA {
  data: Uint8ClampedArray | Uint8Array;
  width: number;
  height: number;
}

let ocrModels: Promise<{
  det: ort.InferenceSession;
  rec: ort.InferenceSession;
  dict: string[];
}> | null = null;
let layoutModel: Promise<ort.InferenceSession> | null = null;

async function fetchBuf(
  url: string,
  onProgress?: (frac: number) => void,
): Promise<ArrayBuffer> {
  const resp = await fetch(url);
  if (!resp.ok) throw new Error(`vision: fetch ${url}: ${resp.status}`);
  const total = Number(resp.headers.get("content-length") ?? 0);
  if (!onProgress || !total || !resp.body) return resp.arrayBuffer();
  const reader = resp.body.getReader();
  const chunks: Uint8Array[] = [];
  let got = 0;
  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;
    chunks.push(value);
    got += value.length;
    onProgress(got / total);
  }
  const out = new Uint8Array(got);
  let off = 0;
  for (const c of chunks) {
    out.set(c, off);
    off += c.length;
  }
  return out.buffer;
}

/** ensureOCR loads (once) the detection + recognition models and the dictionary. */
export function ensureOCR(base = MODEL_BASE): Promise<{
  det: ort.InferenceSession;
  rec: ort.InferenceSession;
  dict: string[];
}> {
  if (!ocrModels) {
    ocrModels = (async () => {
      const [detBuf, recBuf, dictTxt] = await Promise.all([
        fetchBuf(`${base}/ppocrv5_det.onnx`),
        fetchBuf(`${base}/ppocrv5_rec.onnx`),
        fetch(`${base}/ppocrv5_dict.txt`).then((r) => r.text()),
      ]);
      const det = await ort.InferenceSession.create(detBuf, SESSION_OPTS);
      const rec = await ort.InferenceSession.create(recBuf, SESSION_OPTS);
      // Dict indexed by (CTC class - 1); a trailing space is appended per
      // PP-OCR convention (loadDict in algo.go).
      const dict = dictTxt.replace(/\r\n/g, "\n").split("\n");
      while (dict.length && dict[dict.length - 1] === "") dict.pop();
      dict.push(" ");
      return { det, rec, dict };
    })();
  }
  return ocrModels;
}

// A model file may be served whole, or split into sub-100 MB parts (so the
// ~132 MB layout model fits under GitHub Pages' git-push per-file limit). When a
// "<name>.json" parts manifest exists, fetch and concatenate the parts; otherwise
// fetch the single file. The browser reassembles the bytes before handing them to
// onnxruntime-web — identical to loading the whole file, just CORS/Pages-safe.
interface PartsManifest {
  parts: string[];
  bytes: number;
}
async function fetchModel(
  base: string,
  name: string,
  onProgress?: (frac: number) => void,
): Promise<ArrayBuffer> {
  let manifest: PartsManifest | null = null;
  try {
    const r = await fetch(`${base}/${name}.json`);
    if (r.ok) manifest = (await r.json()) as PartsManifest;
  } catch {
    manifest = null; // no manifest → whole file
  }
  if (!manifest?.parts?.length) {
    return fetchBuf(`${base}/${name}`, onProgress);
  }
  const out = new Uint8Array(manifest.bytes);
  let off = 0;
  for (const part of manifest.parts) {
    const resp = await fetch(`${base}/${part}`);
    if (!resp.ok) throw new Error(`vision: fetch ${part}: ${resp.status}`);
    if (resp.body && onProgress) {
      const reader = resp.body.getReader();
      for (;;) {
        const { done, value } = await reader.read();
        if (done) break;
        out.set(value, off);
        off += value.length;
        onProgress(off / manifest.bytes);
      }
    } else {
      const buf = new Uint8Array(await resp.arrayBuffer());
      out.set(buf, off);
      off += buf.length;
      onProgress?.(off / manifest.bytes);
    }
  }
  return out.buffer;
}

/** ensureLayout loads (once) the large PP-DocLayoutV3 model — opt-in. */
export function ensureLayout(
  onProgress?: (frac: number) => void,
  base = MODEL_BASE,
): Promise<ort.InferenceSession> {
  if (!layoutModel) {
    layoutModel = fetchModel(base, "ppdoclayoutv3.onnx", onProgress).then((buf) =>
      ort.InferenceSession.create(buf, SESSION_OPTS),
    );
  }
  return layoutModel;
}

/** layoutAvailable reports whether the layout model has been loaded. */
export function layoutAvailable(): boolean {
  return layoutModel !== null;
}

// --- pure pixel helpers (faithful ports of engine_onnx.go) ---

function clamp(v: number, lo: number, hi: number): number {
  return v < lo ? lo : v > hi ? hi : v;
}

// detInputSize rounds to fit DET_MAX_SIDE and to multiples of 32.
function detInputSize(w: number, h: number): [number, number] {
  let scale = 1;
  const m = Math.max(w, h);
  if (m > DET_MAX_SIDE) scale = DET_MAX_SIDE / m;
  const rw = Math.round((w * scale) / 32) * 32;
  const rh = Math.round((h * scale) / 32) * 32;
  return [Math.max(rw, 32), Math.max(rh, 32)];
}

// bilinear resize of an RGBA raster to w×h.
function resize(src: RGBA, w: number, h: number): RGBA {
  const out = new Uint8ClampedArray(w * h * 4);
  const { width: sw, height: sh, data } = src;
  if (sw === 0 || sh === 0) return { data: out, width: w, height: h };
  for (let y = 0; y < h; y++) {
    const fy = ((y + 0.5) * sh) / h - 0.5;
    const y0 = clamp(Math.floor(fy), 0, sh - 1);
    const y1 = clamp(y0 + 1, 0, sh - 1);
    const wy = fy - Math.floor(fy);
    for (let x = 0; x < w; x++) {
      const fx = ((x + 0.5) * sw) / w - 0.5;
      const x0 = clamp(Math.floor(fx), 0, sw - 1);
      const x1 = clamp(x0 + 1, 0, sw - 1);
      const wx = fx - Math.floor(fx);
      const o = (y * w + x) * 4;
      for (let c = 0; c < 4; c++) {
        const p00 = data[(y0 * sw + x0) * 4 + c];
        const p10 = data[(y0 * sw + x1) * 4 + c];
        const p01 = data[(y1 * sw + x0) * 4 + c];
        const p11 = data[(y1 * sw + x1) * 4 + c];
        const top = p00 * (1 - wx) + p10 * wx;
        const bot = p01 * (1 - wx) + p11 * wx;
        out[o + c] = Math.round(top * (1 - wy) + bot * wy);
      }
    }
  }
  return { data: out, width: w, height: h };
}

function crop(src: RGBA, x0: number, y0: number, x1: number, y1: number): RGBA | null {
  x0 = clamp(x0, 0, src.width - 1);
  y0 = clamp(y0, 0, src.height - 1);
  x1 = clamp(x1, 0, src.width - 1);
  y1 = clamp(y1, 0, src.height - 1);
  if (x1 <= x0 || y1 <= y0) return null;
  const w = x1 - x0 + 1;
  const h = y1 - y0 + 1;
  const out = new Uint8ClampedArray(w * h * 4);
  for (let y = 0; y < h; y++) {
    for (let x = 0; x < w; x++) {
      const s = ((y0 + y) * src.width + (x0 + x)) * 4;
      const o = (y * w + x) * 4;
      out[o] = src.data[s];
      out[o + 1] = src.data[s + 1];
      out[o + 2] = src.data[s + 2];
      out[o + 3] = src.data[s + 3];
    }
  }
  return { data: out, width: w, height: h };
}

// normalizeCHW: (v/255 - mean)/std, RGB order, CHW layout.
function normalizeCHW(img: RGBA, mean: number[], std: number[]): Float32Array {
  const { width: w, height: h, data } = img;
  const out = new Float32Array(3 * h * w);
  for (let y = 0; y < h; y++) {
    for (let x = 0; x < w; x++) {
      const s = (y * w + x) * 4;
      const rgb = [data[s], data[s + 1], data[s + 2]];
      for (let c = 0; c < 3; c++) out[c * h * w + y * w + x] = (rgb[c] / 255 - mean[c]) / std[c];
    }
  }
  return out;
}

// normalizeRec: (v/255 - 0.5)/0.5 in BGR order (PP-OCR rec training order).
function normalizeRec(img: RGBA): Float32Array {
  const { width: w, height: h, data } = img;
  const out = new Float32Array(3 * h * w);
  for (let y = 0; y < h; y++) {
    for (let x = 0; x < w; x++) {
      const s = (y * w + x) * 4;
      const bgr = [data[s + 2], data[s + 1], data[s]];
      for (let c = 0; c < 3; c++) out[c * h * w + y * w + x] = (bgr[c] / 255 - 0.5) / 0.5;
    }
  }
  return out;
}

// normalizeLayout: v/255, RGB, CHW (PP-DocLayoutV3 NormalizeImage mean 0/std 1).
function normalizeLayout(img: RGBA): Float32Array {
  const { width: w, height: h, data } = img;
  const out = new Float32Array(3 * h * w);
  for (let y = 0; y < h; y++) {
    for (let x = 0; x < w; x++) {
      const s = (y * w + x) * 4;
      out[0 * h * w + y * w + x] = data[s] / 255;
      out[1 * h * w + y * w + x] = data[s + 1] / 255;
      out[2 * h * w + y * w + x] = data[s + 2] / 255;
    }
  }
  return out;
}

// unclip expands a shrunk DBNet box (engine_onnx.go: ±0.6×h vert, ±0.35×h horiz).
function unclip(
  x0: number, y0: number, x1: number, y1: number, w: number, h: number,
): [number, number, number, number] {
  const bh = y1 - y0 + 1;
  const padY = Math.floor(0.6 * bh);
  const padX = Math.floor(0.35 * bh);
  return [clamp(x0 - padX, 0, w - 1), clamp(y0 - padY, 0, h - 1),
    clamp(x1 + padX, 0, w - 1), clamp(y1 + padY, 0, h - 1)];
}

interface Box { x0: number; y0: number; x1: number; y1: number }

// connectedBoxes: 4-connected components of a w×h mask, bounded, ≥minArea,
// returned in reading order (algo.go).
function connectedBoxes(mask: Uint8Array, w: number, h: number, minArea: number): Box[] {
  const seen = new Uint8Array(w * h);
  const boxes: Box[] = [];
  const stack: number[] = [];
  for (let start = 0; start < w * h; start++) {
    if (!mask[start] || seen[start]) continue;
    let x0 = start % w, y0 = (start / w) | 0, x1 = x0, y1 = y0;
    stack.length = 0;
    stack.push(start);
    seen[start] = 1;
    while (stack.length) {
      const idx = stack.pop()!;
      const x = idx % w, y = (idx / w) | 0;
      if (x < x0) x0 = x;
      if (x > x1) x1 = x;
      if (y < y0) y0 = y;
      if (y > y1) y1 = y;
      if (x > 0 && mask[idx - 1] && !seen[idx - 1]) { seen[idx - 1] = 1; stack.push(idx - 1); }
      if (x < w - 1 && mask[idx + 1] && !seen[idx + 1]) { seen[idx + 1] = 1; stack.push(idx + 1); }
      if (y > 0 && mask[idx - w] && !seen[idx - w]) { seen[idx - w] = 1; stack.push(idx - w); }
      if (y < h - 1 && mask[idx + w] && !seen[idx + w]) { seen[idx + w] = 1; stack.push(idx + w); }
    }
    if ((x1 - x0 + 1) * (y1 - y0 + 1) >= minArea) boxes.push({ x0, y0, x1, y1 });
  }
  sortReadingOrder(boxes);
  return boxes;
}

// sortReadingOrder: top-to-bottom then left-to-right, same line within ½ median height.
function sortReadingOrder(boxes: Box[]): void {
  if (boxes.length < 2) return;
  const heights = boxes.map((b) => b.y1 - b.y0 + 1).sort((a, b) => a - b);
  const med = heights[heights.length >> 1];
  const tol = Math.max(med >> 1, 1);
  boxes.sort((a, b) => {
    const ca = (a.y0 + a.y1) >> 1, cb = (b.y0 + b.y1) >> 1;
    if (Math.abs(ca - cb) > tol) return ca - cb;
    return a.x0 - b.x0;
  });
}

function argmax(v: Float32Array, off: number, n: number): [number, number] {
  let best = 0, bestV = -Infinity;
  for (let i = 0; i < n; i++) {
    const x = v[off + i];
    if (x > bestV) { best = i; bestV = x; }
  }
  return [best, bestV];
}

// ctcGreedyDecode: argmax per timestep, drop blanks (class 0) + repeats (algo.go).
function ctcGreedyDecode(
  logits: Float32Array, steps: number, classes: number, dict: string[],
): { text: string; conf: number } {
  let text = "";
  let confSum = 0, kept = 0, prev = -1;
  for (let t = 0; t < steps; t++) {
    const [cls, p] = argmax(logits, t * classes, classes);
    if (cls !== 0 && cls !== prev) {
      const idx = cls - 1;
      if (idx >= 0 && idx < dict.length) {
        text += dict[idx];
        confSum += p;
        kept++;
      }
    }
    prev = cls;
  }
  return { text, conf: kept > 0 ? confSum / kept : 0 };
}

function firstTensor(out: ort.InferenceSession.OnnxValueMapType, names: readonly string[]): ort.Tensor {
  return out[names[0]] as ort.Tensor;
}

/** ocr runs PP-OCRv5 detection + recognition over the raster, returning text lines. */
export async function ocr(img: RGBA, base = MODEL_BASE): Promise<OCRResult> {
  const { det, rec, dict } = await ensureOCR(base);
  const ow = img.width, oh = img.height;

  // Detection.
  const [dw, dh] = detInputSize(ow, oh);
  const detIn = normalizeCHW(resize(img, dw, dh), [0.485, 0.456, 0.406], [0.229, 0.224, 0.225]);
  const detOut = await det.run({
    [det.inputNames[0]]: new ort.Tensor("float32", detIn, [1, 3, dh, dw]),
  });
  const prob = firstTensor(detOut, det.outputNames);
  const ps = prob.dims;
  const pw = ps.length === 4 ? Number(ps[3]) : dw;
  const ph = ps.length === 4 ? Number(ps[2]) : dh;
  const probData = prob.data as Float32Array;
  const mask = new Uint8Array(pw * ph);
  for (let i = 0; i < mask.length; i++) mask[i] = probData[i] >= DET_THRESHOLD ? 1 : 0;
  let minArea = Math.floor((pw * ph) / 5000);
  if (minArea < 4) minArea = 4;
  const boxes = connectedBoxes(mask, pw, ph, minArea);

  // Recognition, per box.
  const sx = ow / pw, sy = oh / ph;
  const lines: OCRLine[] = [];
  for (const b of boxes) {
    let ox0 = Math.floor(b.x0 * sx), oy0 = Math.floor(b.y0 * sy);
    let ox1 = Math.floor(b.x1 * sx), oy1 = Math.floor(b.y1 * sy);
    [ox0, oy0, ox1, oy1] = unclip(ox0, oy0, ox1, oy1, ow, oh);
    const c = crop(img, ox0, oy0, ox1, oy1);
    if (!c) continue;
    const rw = Math.max(Math.round((c.width * REC_HEIGHT) / c.height), 1);
    const recIn = normalizeRec(resize(c, rw, REC_HEIGHT));
    const recOut = await rec.run({
      [rec.inputNames[0]]: new ort.Tensor("float32", recIn, [1, 3, REC_HEIGHT, rw]),
    });
    const logitsT = firstTensor(recOut, rec.outputNames);
    const sh = logitsT.dims;
    if (sh.length !== 3) continue;
    const steps = Number(sh[1]), classes = Number(sh[2]);
    const { text, conf } = ctcGreedyDecode(logitsT.data as Float32Array, steps, classes, dict);
    if (!text) continue;
    lines.push({ text, x: ox0, y: oy0, w: ox1 - ox0 + 1, h: oy1 - oy0 + 1, confidence: conf });
  }
  return { width: ow, height: oh, lines };
}

/** layout runs PP-DocLayoutV3 over the raster, returning regions in original coords. */
export async function layout(img: RGBA, base = MODEL_BASE): Promise<LayoutResult> {
  const sess = await ensureLayout(undefined, base);
  const ow = img.width, oh = img.height;
  const imgIn = normalizeLayout(resize(img, LAYOUT_SIZE, LAYOUT_SIZE));
  const byName: Record<string, ort.Tensor> = {
    image: new ort.Tensor("float32", imgIn, [1, 3, LAYOUT_SIZE, LAYOUT_SIZE]),
    scale_factor: new ort.Tensor("float32", Float32Array.from([LAYOUT_SIZE / oh, LAYOUT_SIZE / ow]), [1, 2]),
    im_shape: new ort.Tensor("float32", Float32Array.from([oh, ow]), [1, 2]),
  };
  const feeds: Record<string, ort.Tensor> = {};
  for (const n of sess.inputNames) {
    if (!byName[n]) throw new Error(`vision: layout model wants unknown input ${n}`);
    feeds[n] = byName[n];
  }
  const out = await sess.run(feeds);
  // The detection output is the 2D float tensor [N, >=6].
  let det: ort.Tensor | null = null;
  for (const n of sess.outputNames) {
    const v = out[n] as ort.Tensor;
    if (v && v.type === "float32" && v.dims.length === 2 && Number(v.dims[1]) >= 6) { det = v; break; }
  }
  if (!det) throw new Error("vision: layout produced no float detection output");
  const rows = Number(det.dims[0]), cols = Number(det.dims[1]);
  const data = det.data as Float32Array;
  const regions: Region[] = [];
  for (let r = 0; r < rows; r++) {
    const o = r * cols;
    const score = data[o + 1];
    if (score < LAYOUT_SCORE_THRESHOLD) continue;
    const x1 = data[o + 2], y1 = data[o + 3], x2 = data[o + 4], y2 = data[o + 5];
    if (x2 <= x1 || y2 <= y1) continue;
    regions.push({ role: layoutRole(Math.round(data[o])), x: x1, y: y1, w: x2 - x1, h: y2 - y1, confidence: score });
  }
  return { width: ow, height: oh, regions };
}
