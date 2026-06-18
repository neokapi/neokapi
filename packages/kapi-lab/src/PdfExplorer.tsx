import React, { useEffect, useMemo, useRef, useState } from "react";
import { DocumentViewer, type ContentTree } from "@neokapi/ui-primitives/preview";

import ActiveFileSwitcher from "./ActiveFileSwitcher";
import FileSelectorField from "./FileSelectorField";
import { resolveSelection, useFileLibrary, type FileSelection } from "./fileLibrary";
import { useLabRuntime, type LabRuntimeAssets } from "./useLabRuntime";

export interface PdfExplorerProps {
  /** WASM asset URLs from the host; null defers booting (e.g. during SSR). */
  assets: LabRuntimeAssets | null;
  /** Optional sample PDF fetched on first load so the page shows something. */
  sampleUrl?: string;
  /** File name for the fetched sample (defaults to the URL's basename). */
  sampleName?: string;
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
}: PdfExplorerProps): React.ReactElement {
  const runtime = useLabRuntime(assets);
  const library = useFileLibrary({ sampleIds: [] }); // no text samples; we seed a PDF

  const [selection, setSelection] = useState<FileSelection>({ mode: "multi", paths: [] });
  const [activePath, setActivePath] = useState<string | null>(null);
  const [tree, setTree] = useState<ContentTree | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const seeded = useRef(false);

  // Seed the bundled sample once: fetch its bytes, add it to the in-memory
  // library, and select it. Upload (via FileSelectorField) works the same way.
  useEffect(() => {
    if (seeded.current || !sampleUrl) return;
    seeded.current = true;
    let cancelled = false;
    void (async () => {
      try {
        const resp = await fetch(sampleUrl);
        if (!resp.ok) return;
        const bytes = new Uint8Array(await resp.arrayBuffer());
        const name = sampleName ?? sampleUrl.split("/").pop() ?? "sample.pdf";
        const path = library.addFile(name, bytes, "sample");
        if (!cancelled) {
          setSelection({ mode: "multi", paths: [path] });
          setActivePath(path);
        }
      } catch {
        /* sample is best-effort; the visitor can still upload their own */
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [sampleUrl, sampleName, library]);

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
