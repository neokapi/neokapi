import React, { useCallback, useEffect, useRef, useState } from "react";
import {
  ocr,
  layout,
  type OCRResult,
  type LayoutResult,
} from "@neokapi/kapi-playground/visionBridge";

export interface VisionSampleSpec {
  url: string;
  name: string;
}
export interface VisionExplorerProps {
  samples?: VisionSampleSpec[];
  /**
   * Base URL the ONNX models are served from (det/rec/dict + layout). Must be
   * same-origin or a CORS-enabled host — GitHub release download URLs are
   * CORS-blocked for browser fetch, so the docs site stages the models locally.
   */
  modelBase?: string;
}

interface Raster {
  data: Uint8ClampedArray;
  width: number;
  height: number;
}

// Role → overlay color for layout regions.
const ROLE_COLOR: Record<string, string> = {
  title: "#7c3aed",
  heading: "#2563eb",
  paragraph: "#16a34a",
  table: "#dc2626",
  picture: "#d97706",
  caption: "#0891b2",
  "page-header": "#9333ea",
  "page-footer": "#9333ea",
  footnote: "#65a30d",
  formula: "#db2777",
  code: "#475569",
};
const roleColor = (r: string): string => ROLE_COLOR[r] ?? "#64748b";

// VisionExplorer: upload an image (or pick a sample) and run the real PP-OCRv5
// (text) and PP-DocLayoutV3 (layout) ONNX models in the browser via
// onnxruntime-web — the same models the native kapi-vision plugin runs. OCR
// loads ~21 MB on first use; layout is opt-in (~132 MB).
export default function VisionExplorer({ samples = [], modelBase }: VisionExplorerProps): React.ReactElement {
  const [src, setSrc] = useState<string | null>(samples[0]?.url ?? null);
  const [raster, setRaster] = useState<Raster | null>(null);
  const [ocrRes, setOcrRes] = useState<OCRResult | null>(null);
  const [layoutRes, setLayoutRes] = useState<LayoutResult | null>(null);
  const [busy, setBusy] = useState<"" | "ocr" | "layout">("");
  const [progress, setProgress] = useState(0);
  const [err, setErr] = useState<string | null>(null);
  const imgRef = useRef<HTMLImageElement>(null);
  const [shownW, setShownW] = useState(0);
  // imgSrc is the actual image shown/processed; it differs from src when src is a
  // .docx (then it's the image extracted from the document). extractedNote names
  // that image.
  const [imgSrc, setImgSrc] = useState<string | null>(null);
  const [extractedNote, setExtractedNote] = useState<string | null>(null);
  // Handwriting cascade: PP-OCR reads every line fast; lines below this
  // confidence are re-read by TrOCR (loaded on first escalation).
  const [handwriting, setHandwriting] = useState(false);
  const [hwThreshold, setHwThreshold] = useState(0.85);

  const runOCR = useCallback(
    async (r: Raster) => {
      setBusy("ocr");
      setErr(null);
      try {
        setOcrRes(await ocr(r, modelBase, { handwriting, hwThreshold }));
      } catch (e) {
        setErr(`OCR failed: ${(e as Error).message}`);
      } finally {
        setBusy("");
      }
    },
    [modelBase, handwriting, hwThreshold],
  );

  // Re-run OCR with explicit cascade settings (avoids stale-closure on toggle).
  const rerunOCR = (hw: boolean, thr: number): void => {
    if (!raster) return;
    setBusy("ocr");
    setErr(null);
    ocr(raster, modelBase, { handwriting: hw, hwThreshold: thr })
      .then(setOcrRes)
      .catch((e: unknown) => setErr(`OCR failed: ${(e as Error).message}`))
      .finally(() => setBusy(""));
  };

  // Decode an image URL to a raster and run OCR.
  const processImageUrl = useCallback(
    async (url: string) => {
      setImgSrc(url);
      const image = new Image();
      image.crossOrigin = "anonymous";
      image.src = url;
      await image.decode();
      const canvas = document.createElement("canvas");
      canvas.width = image.naturalWidth;
      canvas.height = image.naturalHeight;
      const ctx = canvas.getContext("2d");
      if (!ctx) throw new Error("no 2D canvas context");
      ctx.drawImage(image, 0, 0);
      const id = ctx.getImageData(0, 0, canvas.width, canvas.height);
      const r: Raster = { data: id.data, width: canvas.width, height: canvas.height };
      setRaster(r);
      await runOCR(r);
    },
    [runOCR],
  );

  // Pull the first embedded image out of a .docx/OOXML package and process it —
  // the same image the engine's openxml reader surfaces as a Media part.
  const processDocxBytes = useCallback(
    async (bytes: Uint8Array) => {
      const { extractEmbeddedImages } = await import("@neokapi/kapi-playground/ooxml");
      const imgs = await extractEmbeddedImages(bytes);
      if (!imgs.length) throw new Error("no embedded image found in the document");
      const first = imgs[0];
      setExtractedNote(`Extracted ${first.name} from the document`);
      // copy into a fresh ArrayBuffer so the Blob owns a plain ArrayBuffer
      await processImageUrl(URL.createObjectURL(new Blob([first.bytes.slice()], { type: first.mime })));
    },
    [processImageUrl],
  );

  const reset = (): void => {
    setOcrRes(null);
    setLayoutRes(null);
    setErr(null);
    setExtractedNote(null);
  };

  const isDocx = (u: string): boolean => /\.docx(\?|$)/i.test(u);

  const loadSource = useCallback(
    async (url: string) => {
      reset();
      try {
        if (isDocx(url)) {
          const bytes = new Uint8Array(await (await fetch(url)).arrayBuffer());
          await processDocxBytes(bytes);
        } else {
          await processImageUrl(url);
        }
      } catch (e) {
        setErr(`Could not load source: ${(e as Error).message}`);
      }
    },
    [processDocxBytes, processImageUrl],
  );

  useEffect(() => {
    if (src) void loadSource(src);
  }, [src, loadSource]);

  const onUpload = (e: React.ChangeEvent<HTMLInputElement>): void => {
    const file = e.target.files?.[0];
    if (!file) return;
    const docx = file.name.toLowerCase().endsWith(".docx") || file.type.includes("wordprocessing");
    if (docx) {
      reset();
      void file
        .arrayBuffer()
        .then((b) => processDocxBytes(new Uint8Array(b)))
        .catch((err: unknown) => setErr(`Could not read document: ${(err as Error).message}`));
    } else {
      setSrc(URL.createObjectURL(file));
    }
  };

  // Toggle the handwriting cascade and re-run OCR with the new setting.
  const onToggleHandwriting = (v: boolean): void => {
    setHandwriting(v);
    rerunOCR(v, hwThreshold);
  };

  const runLayout = useCallback(async () => {
    if (!raster) return;
    setBusy("layout");
    setErr(null);
    setProgress(0);
    try {
      // Trigger the (progress-reported) large-model download first, then run.
      const { ensureLayout } = await import("@neokapi/kapi-playground/visionBridge");
      await ensureLayout((f) => setProgress(f), modelBase);
      setLayoutRes(await layout(raster, modelBase));
    } catch (e) {
      setErr(`Layout failed: ${(e as Error).message}`);
    } finally {
      setBusy("");
    }
  }, [raster, modelBase]);

  const scale = raster && shownW ? shownW / raster.width : 1;

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: "1rem" }}>
      <div style={{ display: "flex", gap: "0.5rem", flexWrap: "wrap", alignItems: "center" }}>
        {samples.map((s) => (
          <button
            key={s.url}
            onClick={() => setSrc(s.url)}
            disabled={busy !== ""}
            style={{
              padding: "0.35rem 0.7rem",
              borderRadius: 6,
              border: "1px solid var(--ifm-color-emphasis-300)",
              background: src === s.url ? "var(--ifm-color-primary)" : "transparent",
              color: src === s.url ? "#fff" : "inherit",
              cursor: "pointer",
            }}
          >
            {s.name}
          </button>
        ))}
        <label
          style={{
            padding: "0.35rem 0.7rem",
            borderRadius: 6,
            border: "1px dashed var(--ifm-color-emphasis-400)",
            cursor: "pointer",
          }}
        >
          Upload image or .docx…
          <input
            type="file"
            accept="image/png,image/jpeg,.docx,application/vnd.openxmlformats-officedocument.wordprocessingml.document"
            onChange={onUpload}
            style={{ display: "none" }}
          />
        </label>
        <label
          title="Re-read low-confidence lines with the TrOCR handwriting model (loads on first use). PP-OCR handles clean text fast; TrOCR rescues the hard lines."
          style={{
            display: "inline-flex",
            alignItems: "center",
            gap: "0.35rem",
            marginLeft: "auto",
            cursor: raster ? "pointer" : "not-allowed",
            fontSize: "0.85rem",
          }}
        >
          <input
            type="checkbox"
            checked={handwriting}
            disabled={!raster || busy !== ""}
            onChange={(e) => onToggleHandwriting(e.target.checked)}
          />
          ✍ Handwriting fallback
        </label>
        {handwriting && (
          <label
            title="Lines whose PP-OCR confidence is below this are re-read by TrOCR. Higher = escalate more lines."
            style={{ display: "inline-flex", alignItems: "center", gap: "0.35rem", fontSize: "0.8rem" }}
          >
            below
            <input
              type="range"
              min={0.5}
              max={0.99}
              step={0.01}
              value={hwThreshold}
              disabled={busy !== ""}
              onChange={(e) => setHwThreshold(Number(e.target.value))}
              onPointerUp={(e) => rerunOCR(true, Number((e.target as HTMLInputElement).value))}
            />
            {Math.round(hwThreshold * 100)}%
          </label>
        )}
        <button
          onClick={() => void runLayout()}
          disabled={!raster || busy !== ""}
          title="Downloads the PP-DocLayoutV3 model (~132 MB) on first use"
          style={{
            padding: "0.35rem 0.7rem",
            borderRadius: 6,
            border: "1px solid var(--ifm-color-emphasis-300)",
            background: layoutRes ? "var(--ifm-color-primary)" : "transparent",
            color: layoutRes ? "#fff" : "inherit",
            cursor: raster ? "pointer" : "not-allowed",
          }}
        >
          Detect layout (~132 MB)
        </button>
      </div>

      {busy === "ocr" && (
        <p style={{ fontStyle: "italic" }}>
          {handwriting
            ? "Running OCR + handwriting fallback (TrOCR loads on first use)…"
            : "Running OCR (loading ~21 MB models on first use)…"}
        </p>
      )}
      {busy === "layout" && (
        <p style={{ fontStyle: "italic" }}>
          {progress < 1
            ? `Downloading layout model… ${Math.round(progress * 100)}%`
            : "Running layout detection…"}
        </p>
      )}
      {extractedNote && (
        <p style={{ fontStyle: "italic", color: "var(--ifm-color-emphasis-600)" }}>{extractedNote}</p>
      )}
      {err && <p style={{ color: "var(--ifm-color-danger)" }}>{err}</p>}

      <div style={{ display: "grid", gridTemplateColumns: "minmax(0, 2fr) minmax(0, 1fr)", gap: "1rem" }}>
        {/* Image with overlays */}
        <div style={{ position: "relative", lineHeight: 0 }}>
          {imgSrc && (
            <img
              ref={imgRef}
              src={imgSrc}
              alt="vision input"
              onLoad={(e) => setShownW(e.currentTarget.clientWidth)}
              style={{ maxWidth: "100%", height: "auto", border: "1px solid var(--ifm-color-emphasis-200)" }}
            />
          )}
          {/* Layout regions (drawn under text boxes) */}
          {layoutRes?.regions.map((r, i) => (
            <div
              key={`r${i}`}
              title={r.role}
              style={{
                position: "absolute",
                left: r.x * scale,
                top: r.y * scale,
                width: r.w * scale,
                height: r.h * scale,
                border: `2px solid ${roleColor(r.role)}`,
                background: `${roleColor(r.role)}14`,
                pointerEvents: "none",
              }}
            >
              <span
                style={{
                  position: "absolute",
                  top: -16,
                  left: 0,
                  fontSize: 10,
                  background: roleColor(r.role),
                  color: "#fff",
                  padding: "0 4px",
                  borderRadius: 3,
                  lineHeight: "14px",
                  whiteSpace: "nowrap",
                }}
              >
                {r.role}
              </span>
            </div>
          ))}
          {/* OCR text boxes — purple where TrOCR re-read the line, blue for PP-OCR. */}
          {ocrRes?.lines.map((l, i) => {
            const hw = l.engine === "trocr";
            return (
              <div
                key={`l${i}`}
                title={`${l.text}${hw ? " [TrOCR]" : ""}`}
                style={{
                  position: "absolute",
                  left: l.x * scale,
                  top: l.y * scale,
                  width: l.w * scale,
                  height: l.h * scale,
                  border: `1px solid ${hw ? "rgba(124,58,237,0.95)" : "rgba(37,99,235,0.9)"}`,
                  background: hw ? "rgba(124,58,237,0.12)" : "rgba(37,99,235,0.08)",
                  pointerEvents: "none",
                }}
              />
            );
          })}
        </div>

        {/* Results panel */}
        <div style={{ fontSize: "0.85rem", maxHeight: 520, overflow: "auto" }}>
          {ocrRes && (
            <>
              <h4 style={{ marginTop: 0 }}>Text ({ocrRes.lines.length})</h4>
              <ol style={{ paddingLeft: "1.2rem", margin: 0 }}>
                {ocrRes.lines.map((l, i) => (
                  <li key={i} style={{ marginBottom: 2 }}>
                    {l.text}{" "}
                    <span style={{ color: "var(--ifm-color-emphasis-500)" }}>
                      ({Math.round(l.confidence * 100)}%)
                    </span>
                    {l.engine === "trocr" && (
                      <span style={{ color: "#7c3aed", fontWeight: 600 }}> ✍ TrOCR</span>
                    )}
                  </li>
                ))}
              </ol>
            </>
          )}
          {layoutRes && (
            <>
              <h4>Layout regions ({layoutRes.regions.length})</h4>
              <ul style={{ paddingLeft: "1.2rem", margin: 0 }}>
                {layoutRes.regions.map((r, i) => (
                  <li key={i} style={{ color: roleColor(r.role) }}>
                    {r.role}{" "}
                    <span style={{ color: "var(--ifm-color-emphasis-500)" }}>
                      ({Math.round(r.confidence * 100)}%)
                    </span>
                  </li>
                ))}
              </ul>
            </>
          )}
        </div>
      </div>
    </div>
  );
}
