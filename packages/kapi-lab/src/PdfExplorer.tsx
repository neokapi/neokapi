import React, { useEffect, useMemo, useRef, useState } from "react";
import { DocumentViewer, type ContentTree } from "@neokapi/ui-primitives/preview";

import ActiveFileSwitcher from "./ActiveFileSwitcher";
import FileSelectorField from "./FileSelectorField";
import { resolveSelection, useFileLibrary, type FileSelection } from "./fileLibrary";
import { useLabRuntime, type LabRuntimeAssets } from "./useLabRuntime";
import RunGate from "./RunGate";
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
}: PdfExplorerProps): React.ReactElement {
  const runtime = useLabRuntime(assets, { autoBoot: false });
  const gate = useRunGate(runtime);
  const library = useFileLibrary({ sampleIds: [] }); // no text samples; we seed PDFs

  const [selection, setSelection] = useState<FileSelection>({ mode: "multi", paths: [] });
  const [activePath, setActivePath] = useState<string | null>(null);
  const [tree, setTree] = useState<ContentTree | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const seeded = useRef(false);

  // Normalize the bundled samples into one list (the `samples` array wins; the
  // single sampleUrl is the back-compat fallback).
  const sampleList = useMemo<PdfSampleSpec[]>(
    () => samples ?? (sampleUrl ? [{ url: sampleUrl, name: sampleName }] : []),
    [samples, sampleUrl, sampleName],
  );

  // Seed the bundled samples once: fetch each in order, add it to the in-memory
  // library, and select the first. Upload (via FileSelectorField) works the same way.
  useEffect(() => {
    if (seeded.current || sampleList.length === 0) return;
    seeded.current = true;
    let cancelled = false;
    void (async () => {
      const paths: string[] = [];
      for (const s of sampleList) {
        try {
          const resp = await fetch(s.url);
          if (!resp.ok) continue;
          const bytes = new Uint8Array(await resp.arrayBuffer());
          const name = s.name ?? s.url.split("/").pop() ?? "sample.pdf";
          paths.push(library.addFile(name, bytes, "sample"));
        } catch {
          /* samples are best-effort; the visitor can still upload their own */
        }
      }
      if (!cancelled && paths.length > 0) {
        setSelection({ mode: "multi", paths });
        setActivePath(paths[0]);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [sampleList, library]);

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

  if (!gate.armed) {
    return (
      <RunGate
        gate={gate}
        title="PDF extraction"
        description="Extract text and geometry from a PDF with PDFium in your browser."
      />
    );
  }
  return (
    <div className="kapi-reference flex flex-col gap-3 text-foreground">
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
        {runtime.status === "booting" && "Booting kapi (first run downloads ~13 MB)…"}
        {runtime.status === "error" && `Failed to start: ${runtime.error}`}
        {runtime.ready && !file && "Upload a PDF to see it parsed."}
        {runtime.ready && file && busy && "Parsing PDF (loading PDFium on first use)…"}
        {runtime.ready && file && !busy && error && `Error: ${error}`}
      </div>

      {tree && file && (
        <DocumentViewer tree={tree} filename={file.name} bytes={file.bytes} defaultTab="layout" />
      )}
    </div>
  );
}
