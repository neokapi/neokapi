import React, { useEffect, useMemo, useState } from "react";
import { DocumentViewer, type ContentTree } from "@neokapi/ui-primitives/preview";

import ActiveFileSwitcher from "./ActiveFileSwitcher";
import FileSelectorField from "./FileSelectorField";
import { resolveSelection, useFileLibrary, type FileSelection } from "./fileLibrary";
import { useLabRuntime, type LabRuntimeAssets } from "./useLabRuntime";
import GateOverlay from "./GateOverlay";
import { useRunGate } from "./useRunGate";

/** A bundled sample PDF the explorer fetches and seeds on first load. */
export interface PdfSampleSpec {
  /** URL to fetch the PDF bytes from (e.g. "/samples/report.pdf"). */
  url: string;
  /** File name shown in the switcher (defaults to the URL's basename). */
  name?: string;
}

export interface PdfExplorerProps {
  /** WASM asset URLs from the host; null defers booting (e.g. during SSR). */
  assets: LabRuntimeAssets | null;
  /** Optional sample PDF fetched on first load so the page shows something. */
  sampleUrl?: string;
  /** File name for the fetched sample (defaults to the URL's basename). */
  sampleName?: string;
  /**
   * Optional set of bundled sample PDFs. All are fetched and added to the file
   * switcher; the first is selected on load. Takes precedence over sampleUrl.
   */
  samples?: PdfSampleSpec[];
  /**
   * Start with the engine and the pdfium plugin when mounted, with no Run gate
   * of its own: for a host whose own Play already asked the reader.
   */
  autoStart?: boolean;
}

// PdfExplorer parses a PDF — a bundled sample or one the visitor uploads —
// entirely in the browser: the kapi WASM engine's PDF reader bridges to a
// PDFium WebAssembly module (no server, nothing mocked), extracting positioned
// text. The result renders in the shared DocumentViewer, whose Layout tab shows
// each text block in its place on the page (geometry), Structure shows the
// document outline, and Blocks lists the extracted content.
export default function PdfExplorer({
  assets,
  sampleUrl,
  sampleName,
  samples,
  autoStart = false,
}: PdfExplorerProps): React.ReactElement {
  const runtime = useLabRuntime(assets, { autoBoot: autoStart });
  // PDF parsing needs the pdfium plugin (PDFium-wasm); download it via the
  // manager on Run so the navbar status widget reflects it.
  const gate = useRunGate(runtime, { requires: ["pdfium"], autoArm: autoStart });
  const library = useFileLibrary({ sampleIds: [] }); // no text samples; we seed PDFs

  const [selection, setSelection] = useState<FileSelection>({
    mode: "multi",
    paths: [],
  });
  const [activePath, setActivePath] = useState<string | null>(null);
  const [tree, setTree] = useState<ContentTree | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [sampleAttempt, setSampleAttempt] = useState(0);
  const [sampleLoading, setSampleLoading] = useState(false);
  const [sampleError, setSampleError] = useState<string | null>(null);
  const { addFile } = library;

  // Normalize the bundled samples into one list (the `samples` array wins; the
  // single sampleUrl is the back-compat fallback).
  const sampleList = useMemo<PdfSampleSpec[]>(
    () => samples ?? (sampleUrl ? [{ url: sampleUrl, name: sampleName }] : []),
    [samples, sampleUrl, sampleName],
  );

  // Cancel abandoned loads (including StrictMode's effect replay) before they
  // write to the library. Each retry replaces the same sample paths.
  useEffect(() => {
    if (sampleList.length === 0) return;
    const controller = new AbortController();
    setSampleLoading(true);
    setSampleError(null);
    void Promise.allSettled(
      sampleList.map(async (sample) => {
        const response = await fetch(sample.url, { signal: controller.signal });
        if (!response.ok) throw new Error(`HTTP ${response.status}`);
        const bytes = new Uint8Array(await response.arrayBuffer());
        const name = sample.name ?? sample.url.split("/").pop() ?? "sample.pdf";
        return { name, bytes };
      }),
    ).then((results) => {
      if (controller.signal.aborted) return;
      const paths: string[] = [];
      const failed: string[] = [];
      results.forEach((result, index) => {
        if (result.status === "fulfilled") {
          paths.push(addFile(result.value.name, result.value.bytes, "sample"));
        } else {
          failed.push(sampleList[index].name ?? "sample PDF");
        }
      });
      if (paths.length > 0) {
        setSelection({ mode: "multi", paths });
        setActivePath(paths[0]);
      }
      if (failed.length > 0)
        setSampleError(
          `Could not load ${failed.join(", ")}. Check your connection, retry, or upload a PDF.`,
        );
      setSampleLoading(false);
    });
    return () => controller.abort();
  }, [sampleList, addFile, sampleAttempt]);

  const selected = useMemo(() => resolveSelection(selection, library), [selection, library]);
  const file = useMemo(
    () => selected.find((f) => f.path === activePath) ?? selected[0],
    [selected, activePath],
  );

  // Re-inspect whenever the runtime becomes ready or the selected file changes.
  useEffect(() => {
    if (!runtime.ready || !file) {
      if (!file) setTree(null);
      return;
    }
    let cancelled = false;
    setBusy(true);
    setError(null);
    void runtime
      .inspect(file.name, file.bytes)
      .then((res) => {
        if (cancelled) return;
        if (res.ok && res.tree) {
          setTree(res.tree);
        } else {
          setError(res.error ?? "could not parse this PDF");
          setTree(null);
        }
      })
      .finally(() => !cancelled && setBusy(false));
    return () => {
      cancelled = true;
    };
  }, [runtime.ready, runtime.inspect, file?.path, file?.changedAt]); // eslint-disable-line react-hooks/exhaustive-deps

  return (
    <div className="kapi-reference flex flex-col gap-3 text-foreground">
      {sampleLoading && <p role="status">Loading sample PDFs…</p>}
      {sampleError && (
        <div role="alert" className="rounded-lg border p-3 text-sm">
          <p>{sampleError}</p>
          <button
            type="button"
            className="rounded border px-3 py-1"
            onClick={() => setSampleAttempt((value) => value + 1)}
          >
            Retry sample download
          </button>
        </div>
      )}
      <div className="relative flex flex-col gap-3">
        <FileSelectorField
          label="PDF"
          library={library}
          selection={selection}
          onSelectionChange={setSelection}
          sampleIds={[]}
        />

        <ActiveFileSwitcher files={selected} activePath={file?.path} onChange={setActivePath} />

        <div
          className={
            error
              ? "min-h-[1.4rem] text-sm text-destructive"
              : "min-h-[1.4rem] text-sm text-muted-foreground"
          }
        >
          {runtime.status === "booting" && "Loading the browser engine…"}
          {runtime.status === "error" && `Failed to start: ${runtime.error}`}
          {runtime.ready && !file && "Upload a PDF to see it parsed."}
          {runtime.ready && file && busy && "Parsing PDF (loading PDFium on first use)…"}
          {runtime.ready && file && !busy && error && `Error: ${error}`}
        </div>

        <div className="min-h-[420px]">
          {tree && file && (
            <DocumentViewer
              tree={tree}
              filename={file.name}
              bytes={file.bytes}
              defaultTab="layout"
            />
          )}
        </div>

        <GateOverlay
          gate={gate}
          title="PDF extraction"
          description="Extract text and geometry from a PDF with PDFium in your browser."
        />
      </div>
    </div>
  );
}
