import React, { useCallback, useEffect, useState } from "react";
import { useLabRuntime } from "./useLabRuntime";
import type { LabRuntimeAssets } from "./useLabRuntime";
import FileSource from "./FileSource";
import type { FileSourceValue } from "./FileSource";
import { SAMPLES } from "./samples";
import shared from "./styles.module.css";
import styles from "./ConversionExplorer.module.css";

// ConversionExplorer re-expresses one document in a different format. It reads
// the input with its native reader, then writes the content model out through a
// *generative* writer (one that reconstructs a whole document from the model,
// no original-file skeleton needed) — the real kapi `convert` (kconv) command
// running in WASM. The same engine powers a live visual preview: the document
// is also projected to HTML and shown in a sandboxed iframe, so you see both the
// rendered page and the chosen format's source side by side.
//
// Only generative targets are offered. Skeleton-driven formats (docx/odt/idml/
// epub/…) inject translations back into the *original* file and cannot be
// generated from a foreign model, so they are deliberately absent.

export interface ConversionTarget {
  id: string;
  label: string;
  /** Output extension used for the in-FS path (the --to flag selects the format). */
  ext: string;
}

export const GENERATIVE_TARGETS: ConversionTarget[] = [
  { id: "doclang", label: "DocLang", ext: "dclg.xml" },
  { id: "markdown", label: "Markdown", ext: "md" },
  { id: "html", label: "HTML", ext: "html" },
  { id: "xliff", label: "XLIFF", ext: "xliff" },
  { id: "po", label: "Gettext PO", ext: "po" },
  { id: "json", label: "JSON", ext: "json" },
  { id: "yaml", label: "YAML", ext: "yaml" },
  { id: "plaintext", label: "Plain text", ext: "txt" },
];

const DEFAULT_SAMPLE_IDS = [
  "article-md",
  "page-html",
  "report-doclang",
  "messages-json",
  "app-xliff",
];

type ViewTab = "source" | "rendered";

export interface ConversionExplorerProps {
  /** WASM asset URLs from the host; null defers booting (e.g. during SSR). */
  assets: LabRuntimeAssets | null;
  /** Sample selected on first render. */
  defaultSampleId?: string;
  /** Restrict the offered samples. */
  sampleIds?: string[];
  /** Output format selected on first render (default: doclang). */
  defaultTarget?: string;
}

export default function ConversionExplorer({
  assets,
  defaultSampleId,
  sampleIds,
  defaultTarget,
}: ConversionExplorerProps): React.ReactElement {
  const runtime = useLabRuntime(assets);
  const offered = sampleIds ?? DEFAULT_SAMPLE_IDS;

  const initial =
    SAMPLES.find((s) => s.id === defaultSampleId) ??
    SAMPLES.find((s) => s.id === offered[0]) ??
    SAMPLES[0];
  const [file, setFile] = useState<FileSourceValue>({
    filename: initial.filename,
    label: initial.label,
    content: initial.content,
  });
  const [target, setTarget] = useState<string>(defaultTarget ?? "doclang");
  const [view, setView] = useState<ViewTab>("source");
  const [output, setOutput] = useState<string | null>(null);
  const [previewHtml, setPreviewHtml] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  // convertTo runs `kapi convert <in> --to <fmt> -o <out>` in WASM and returns
  // the written output (or throws with the captured error).
  const convertTo = useCallback(
    async (inPath: string, fmt: string, ext: string): Promise<string> => {
      const outPath = `/project/converted.${ext}`;
      const { code, output: log } = await runtime.runCapture([
        "convert",
        inPath,
        "--to",
        fmt,
        "-o",
        outPath,
      ]);
      const written = runtime.readFile(outPath);
      if (code !== 0 || written === null) {
        throw new Error((log || "").trim() || `conversion to ${fmt} produced no output`);
      }
      return written;
    },
    [runtime.runCapture, runtime.readFile],
  );

  const runConversion = useCallback(async () => {
    if (!runtime.ready) return;
    setBusy(true);
    setError(null);
    try {
      const inPath = runtime.writeFile(file.filename, file.bytes ?? file.content);
      const def = GENERATIVE_TARGETS.find((t) => t.id === target) ?? GENERATIVE_TARGETS[0];
      const out = await convertTo(inPath, def.id, def.ext);
      setOutput(out);
      // Visual preview: reuse the HTML projection (or the output itself when the
      // chosen target already is HTML), sandboxed.
      setPreviewHtml(def.id === "html" ? out : await convertTo(inPath, "html", "html"));
    } catch (e) {
      setOutput(null);
      setPreviewHtml(null);
      setError(e instanceof Error ? e.message : String(e));
    }
    setBusy(false);
  }, [runtime.ready, runtime.writeFile, convertTo, file, target]);

  useEffect(() => {
    if (runtime.ready) void runConversion();
  }, [runtime.ready, runConversion]);

  return (
    <div className={shared.explorer}>
      <div className={styles.controls}>
        <FileSource value={file} onChange={setFile} sampleIds={offered} label="Input" />
        <label className={styles.targetField}>
          <span className={styles.targetLabel}>Convert to</span>
          <select
            className={styles.targetSelect}
            value={target}
            onChange={(e) => setTarget(e.target.value)}
          >
            {GENERATIVE_TARGETS.map((t) => (
              <option key={t.id} value={t.id}>
                {t.label}
              </option>
            ))}
          </select>
        </label>
      </div>

      <div className={`${shared.statusBar} ${error ? shared.statusError : ""}`}>
        {runtime.status === "booting" && "Booting kapi (first run downloads the WASM engine)…"}
        {runtime.status === "error" && `Failed to start: ${runtime.error}`}
        {runtime.ready && busy && "Converting…"}
        {runtime.ready && !busy && error && `Error: ${error}`}
        {runtime.ready && !busy && !error && output !== null && (
          <span className={shared.stats}>
            <span className={shared.statBadge}>
              {file.filename} → {GENERATIVE_TARGETS.find((t) => t.id === target)?.label}
            </span>
          </span>
        )}
      </div>

      {output !== null && (
        <>
          <div className={styles.tabs} role="tablist" aria-label="Output view">
            <button
              role="tab"
              aria-selected={view === "rendered"}
              className={`${styles.tab} ${view === "rendered" ? styles.tabActive : ""}`}
              onClick={() => setView("rendered")}
            >
              Rendered
            </button>
            <button
              role="tab"
              aria-selected={view === "source"}
              className={`${styles.tab} ${view === "source" ? styles.tabActive : ""}`}
              onClick={() => setView("source")}
            >
              Source
            </button>
          </div>

          {view === "source" && <pre className={styles.code}>{output}</pre>}
          {view === "rendered" &&
            (previewHtml !== null ? (
              <iframe
                className={styles.preview}
                title="Rendered preview"
                sandbox=""
                srcDoc={previewHtml}
              />
            ) : (
              <p className={styles.note}>No visual preview for this document.</p>
            ))}

          <p className={styles.note}>
            The reader parses the input into the content model (roles, runs, tables, geometry); a
            generative writer re-serializes it as{" "}
            {GENERATIVE_TARGETS.find((t) => t.id === target)?.label}. Skeleton-driven formats (docx,
            odt, idml, epub) inject into an original file and so cannot be conversion targets.
          </p>
        </>
      )}
    </div>
  );
}
