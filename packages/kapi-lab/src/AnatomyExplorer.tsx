import React, { useEffect, useMemo, useState } from "react";
import { cn } from "@neokapi/ui-primitives";
import { useLabRuntime } from "./useLabRuntime";
import type { LabRuntimeAssets } from "./useLabRuntime";
import FileSelectorField from "./FileSelectorField";
import ActiveFileSwitcher from "./ActiveFileSwitcher";
import ContentTreeView from "./ContentTreeView";
import { useFileLibrary, resolveSelection } from "./fileLibrary";
import type { FileSelection } from "./fileLibrary";
import { SAMPLES } from "./samples";
import type { ContentTree } from "./types";

export interface AnatomyExplorerProps {
  /** WASM asset URLs from the host; null defers booting (e.g. during SSR). */
  assets: LabRuntimeAssets | null;
  /** Sample selected on first render (default: first sample). */
  defaultSampleId?: string;
  /** Restrict the offered samples. */
  sampleIds?: string[];
}

// AnatomyExplorer decomposes a file (a bundled sample or the learner's own) into
// the neokapi content model — Layers → Groups → Blocks (with their run
// sequences, targets, overlays and annotations) → Data / Media — by running the
// real reader in WASM via labInspect. The file surface is the shared
// FileExplorer; the result is the shared ContentTreeView.
export default function AnatomyExplorer({
  assets,
  defaultSampleId,
  sampleIds,
}: AnatomyExplorerProps): React.ReactElement {
  const runtime = useLabRuntime(assets);
  const library = useFileLibrary({ sampleIds });

  const initial = SAMPLES.find((s) => s.id === defaultSampleId) ?? SAMPLES[0];
  const [selection, setSelection] = useState<FileSelection>({
    mode: "multi",
    paths: [initial.filename],
  });
  const [activePath, setActivePath] = useState<string | null>(null);
  const [tree, setTree] = useState<ContentTree | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  // The chooser builds the working set; the switcher picks which one to inspect.
  const selected = useMemo(() => resolveSelection(selection, library), [selection, library]);
  const file = useMemo(
    () => selected.find((f) => f.path === activePath) ?? selected[0],
    [selected, activePath],
  );

  // Re-inspect whenever the runtime becomes ready or the selected file changes.
  useEffect(() => {
    if (!runtime.ready || !file) return;
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
          setError(res.error ?? "could not inspect file");
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
        label="Files"
        library={library}
        selection={selection}
        onSelectionChange={setSelection}
        sampleIds={sampleIds}
      />

      <ActiveFileSwitcher files={selected} activePath={file?.path} onChange={setActivePath} />

      <div
        className={cn("min-h-[1.4rem] text-sm text-muted-foreground", error && "text-destructive")}
      >
        {runtime.status === "booting" && "Booting kapi (first run downloads ~13 MB)…"}
        {runtime.status === "error" && `Failed to start: ${runtime.error}`}
        {runtime.ready && busy && "Reading…"}
        {runtime.ready && !busy && error && `Error: ${error}`}
      </div>

      {tree && <ContentTreeView tree={tree} />}
    </div>
  );
}
