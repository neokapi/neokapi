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

  const runOCR = useCallback(
    async (r: Raster) => {
      setBusy("ocr");
      setErr(null);
      try {
        setOcrRes(await ocr(r, modelBase));
      } catch (e) {
        setErr(`OCR failed: ${(e as Error).message}`);
      } finally {
        setBusy("");
      }
    },
    [modelBase],
  );

  const loadImage = useCallback(
    async (url: string) => {
      setOcrRes(null);
      setLayoutRes(null);
      setErr(null);
      try {
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
      } catch (e) {
        setErr(`Could not load image: ${(e as Error).message}`);
      }
    },
    [runOCR],
  );

  useEffect(() => {
    if (src) void loadImage(src);
  }, [src, loadImage]);

  const onUpload = (e: React.ChangeEvent<HTMLInputElement>): void => {
    const file = e.target.files?.[0];
    if (file) setSrc(URL.createObjectURL(file));
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
          Upload image…
          <input type="file" accept="image/png,image/jpeg" onChange={onUpload} style={{ display: "none" }} />
        </label>
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
            marginLeft: "auto",
          }}
        >
          Detect layout (~132 MB)
        </button>
      </div>

      {busy === "ocr" && <p style={{ fontStyle: "italic" }}>Running OCR (loading ~21 MB models on first use)…</p>}
      {busy === "layout" && (
        <p style={{ fontStyle: "italic" }}>
          {progress < 1
            ? `Downloading layout model… ${Math.round(progress * 100)}%`
            : "Running layout detection…"}
        </p>
      )}
      {err && <p style={{ color: "var(--ifm-color-danger)" }}>{err}</p>}

      <div style={{ display: "grid", gridTemplateColumns: "minmax(0, 2fr) minmax(0, 1fr)", gap: "1rem" }}>
        {/* Image with overlays */}
        <div style={{ position: "relative", lineHeight: 0 }}>
          {src && (
            <img
              ref={imgRef}
              src={src}
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
          {/* OCR text boxes */}
          {ocrRes?.lines.map((l, i) => (
            <div
              key={`l${i}`}
              title={l.text}
              style={{
                position: "absolute",
                left: l.x * scale,
                top: l.y * scale,
                width: l.w * scale,
                height: l.h * scale,
                border: "1px solid rgba(37,99,235,0.9)",
                background: "rgba(37,99,235,0.08)",
                pointerEvents: "none",
              }}
            />
          ))}
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
