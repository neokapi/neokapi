import React, { useEffect, useMemo, useState } from "react";
import { cn } from "@neokapi/ui-primitives";
import { DocumentViewer } from "@neokapi/ui-primitives/preview";
import type { ContentTree } from "@neokapi/ui-primitives/preview";
import {
  useLabRuntime,
  useRunGate,
  RunGate,
  FileSelectorField,
  ActiveFileSwitcher,
  useFileLibrary,
  resolveSelection,
  SAMPLES,
} from "@neokapi/kapi-lab";
import type { LabRuntimeAssets, FileSelection } from "@neokapi/kapi-lab";
import { loadICU4X, icu4xReady } from "../../lib/icu4x";

export interface SegmentationPreviewInnerProps {
  assets: LabRuntimeAssets | null;
  defaultSampleId?: string;
  sampleIds?: string[];
}

// The segmentation engines, matching the native architecture: the pure-Go rule
// engine, the raw Unicode baseline, the hybrid (baseline + SRX exceptions), and
// the SaT ML model. uax29 + hybrid run ICU4X (companion wasm); srx is pure-Go;
// SaT is an out-of-process native plugin (kapi-sat) that can't run in the
// browser, so it's shown but disabled here.
const ENGINES: {
  id: string;
  label: string;
  needsICU: boolean;
  nativeOnly?: boolean;
  title?: string;
}[] = [
  { id: "srx", label: "SRX rules (pure-Go)", needsICU: false },
  { id: "uax29", label: "UAX-29 (ICU4X)", needsICU: true },
  {
    id: "hybrid",
    label: "Hybrid",
    needsICU: true,
    title: "ICU4X UAX-29 base + SRX exceptions — how neokapi segments natively (Okapi-compatible)",
  },
  {
    id: "sat",
    label: "SaT (ML plugin)",
    needsICU: false,
    nativeOnly: true,
    title: "wtpsplit ML model via the kapi-sat plugin — native only, not available in the browser",
  },
];

// SegmentationPreviewInner segments a real document in the browser and shows the
// sentence boundaries in the shared DocumentViewer (Blocks view), with a live
// SRX ↔ UAX-29 engine switch. SRX runs pure-Go in the wasm engine; UAX-29 is
// served by the ICU4X companion-wasm bridge (loadICU4X installs the global the
// Go "uax29" engine calls). It runs labInspectAnnotated with segment:true, so
// segmentation rides the same annotated-inspect path as term/brand/QA overlays.
export default function SegmentationPreviewInner({
  assets,
  defaultSampleId,
  sampleIds,
}: SegmentationPreviewInnerProps): React.ReactElement {
  const runtime = useLabRuntime(assets, { autoBoot: false });
  const gate = useRunGate(runtime);
  const library = useFileLibrary({ sampleIds });

  const initial = SAMPLES.find((s) => s.id === defaultSampleId) ?? SAMPLES[0];
  const [selection, setSelection] = useState<FileSelection>({
    mode: "multi",
    paths: [initial.filename],
  });
  const [activePath, setActivePath] = useState<string | null>(null);
  const [engine, setEngine] = useState("srx");
  const [icuReady, setIcuReady] = useState(false);
  const [tree, setTree] = useState<ContentTree | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const selected = useMemo(() => resolveSelection(selection, library), [selection, library]);
  const file = useMemo(
    () => selected.find((f) => f.path === activePath) ?? selected[0],
    [selected, activePath],
  );

  // Load ICU4X in the background so the UAX-29 option becomes available without
  // blocking the SRX path (which works immediately, pure-Go in the wasm engine).
  useEffect(() => {
    let cancelled = false;
    void loadICU4X().then(() => !cancelled && setIcuReady(icu4xReady()));
    return () => {
      cancelled = true;
    };
  }, []);

  // Re-segment whenever the runtime is ready, the file changes, or the engine
  // changes. For UAX-29 we await the ICU4X bridge first (the Go engine calls it
  // synchronously over syscall/js, so it must be installed before the run).
  useEffect(() => {
    if (!runtime.ready || !file) return;
    let cancelled = false;
    setBusy(true);
    setError(null);
    void (async () => {
      // uax29 and okapi run ICU4X; ensure the bridge is installed before the
      // Go engine calls it (srx is pure-Go and needs nothing).
      if (ENGINES.find((e) => e.id === engine)?.needsICU) await loadICU4X();
      const res = await runtime.inspectAnnotated(file.name, file.bytes, {
        term: false,
        brand: false,
        qa: false,
        segment: true,
        segmentEngine: engine,
      });
      if (cancelled) return;
      if (res.ok && res.tree) {
        setTree(res.tree);
      } else {
        setTree(null);
        setError(res.error ?? "could not segment file");
      }
    })().finally(() => !cancelled && setBusy(false));
    return () => {
      cancelled = true;
    };
  }, [runtime.ready, runtime.inspectAnnotated, file?.path, file?.changedAt, engine]); // eslint-disable-line react-hooks/exhaustive-deps

  if (!gate.armed) {
    return (
      <RunGate
        gate={gate}
        title="Segmentation"
        description="Segment text with the real engine (SRX / ICU4X) in your browser."
      />
    );
  }

  return (
    <div className="kapi-reference flex flex-col gap-3 text-foreground">
      <FileSelectorField
        label="Files"
        library={library}
        selection={selection}
        onSelectionChange={setSelection}
        sampleIds={sampleIds}
      />
      <ActiveFileSwitcher files={selected} activePath={file?.path} onChange={setActivePath} />

      <div className="flex flex-wrap items-center gap-2">
        <span className="text-sm text-muted-foreground">Engine</span>
        {ENGINES.map((e) => {
          const loading = e.needsICU && !icuReady && !e.nativeOnly;
          const disabled = e.nativeOnly || loading;
          return (
            <button
              key={e.id}
              type="button"
              disabled={disabled}
              title={e.title}
              onClick={() => setEngine(e.id)}
              className={cn(
                "rounded border px-2 py-1 text-sm",
                engine === e.id
                  ? "border-primary bg-primary/10 text-foreground"
                  : "border-border text-muted-foreground",
                disabled && "cursor-not-allowed opacity-50",
              )}
            >
              {e.label}
              {loading && " — loading…"}
              {e.nativeOnly && " — native only"}
            </button>
          );
        })}
      </div>

      <div
        className={cn("min-h-[1.4rem] text-sm text-muted-foreground", error && "text-destructive")}
      >
        {runtime.status === "booting" && "Booting kapi (first run downloads ~13 MB)…"}
        {runtime.status === "error" && `Failed to start: ${runtime.error}`}
        {runtime.ready && busy && "Segmenting…"}
        {runtime.ready && !busy && error && `Error: ${error}`}
      </div>

      {tree && file && (
        <DocumentViewer tree={tree} filename={file.name} bytes={file.bytes} defaultTab="blocks" />
      )}
    </div>
  );
}
