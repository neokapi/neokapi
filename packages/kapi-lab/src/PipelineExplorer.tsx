import React, { useCallback, useEffect, useMemo, useState } from "react";
import { Play } from "lucide-react";
import { Button, ToggleGroup, ToggleGroupItem, cn } from "@neokapi/ui-primitives";
import { useLabRuntime } from "./useLabRuntime";
import type { LabRuntimeAssets } from "./useLabRuntime";
import FileSelectorField from "./FileSelectorField";
import ActiveFileSwitcher from "./ActiveFileSwitcher";
import OutputView from "./OutputView";
import FlowTracePlayer from "./FlowTracePlayer";
import { useFileLibrary, resolveSelection } from "./fileLibrary";
import type { FileSelection } from "./fileLibrary";
import { SAMPLES } from "./samples";
import type { FlowTrace } from "@neokapi/ui-primitives/preview";

// A pipeline a learner can run in the browser. Tools run positionally
// (`kapi <tool> in -o out`); composed flows run via `kapi run <flow> -i in`.
interface Pipeline {
  id: string;
  label: string;
  build: (inPath: string, outPath: string) => string[];
}

const PIPELINES: Pipeline[] = [
  {
    id: "pseudo-translate",
    label: "Pseudo-translate",
    build: (i, o) => ["pseudo-translate", i, "-o", o],
  },
  {
    id: "translate-qa",
    label: "AI translate + QA (demo)",
    build: (i, o) => ["run", "translate-qa", "-i", i, "-o", o, "--target-lang", "fr"],
  },
  {
    id: "secure-translate",
    label: "Secure translate (demo)",
    build: (i, o) => ["run", "secure-translate", "-i", i, "-o", o, "--target-lang", "fr"],
  },
];

export interface PipelineExplorerProps {
  assets: LabRuntimeAssets | null;
  defaultSampleId?: string;
  defaultPipelineId?: string;
  sampleIds?: string[];
}

// PipelineExplorer runs a real flow over a file in WASM with tracing on, hands
// the live FlowTrace to <FlowTracePlayer>, and shows the file the run wrote in
// the shared OutputView — Blocks / Structure / Native, downloadable, with the
// changed blocks and lines highlighted so a learner sees exactly what changed.
export default function PipelineExplorer({
  assets,
  defaultSampleId,
  defaultPipelineId,
  sampleIds,
}: PipelineExplorerProps): React.ReactElement {
  const runtime = useLabRuntime(assets);
  const library = useFileLibrary({ sampleIds });

  const initial = SAMPLES.find((s) => s.id === defaultSampleId) ?? SAMPLES[0];
  const [selection, setSelection] = useState<FileSelection>({
    mode: "multi",
    paths: [initial.filename],
  });
  const [activePath, setActivePath] = useState<string | null>(null);
  const [pipelineId, setPipelineId] = useState(defaultPipelineId ?? PIPELINES[0].id);
  const [trace, setTrace] = useState<FlowTrace | null>(null);
  const [outPath, setOutPath] = useState<string | null>(null);
  const [version, setVersion] = useState(0);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  // The chooser builds the working set; the switcher picks which one to run.
  const selected = useMemo(() => resolveSelection(selection, library), [selection, library]);
  const file = useMemo(
    () => selected.find((f) => f.path === activePath) ?? selected[0],
    [selected, activePath],
  );

  const runPipeline = useCallback(async () => {
    if (!runtime.ready || !file) return;
    const pipeline = PIPELINES.find((p) => p.id === pipelineId) ?? PIPELINES[0];
    setBusy(true);
    setError(null);
    const inPath = runtime.writeFile(file.name, file.bytes);
    const outAbs = `/project/out-${file.name}`;
    const res = await runtime.trace(pipeline.build(inPath, outAbs));
    if (res.ok && res.trace) {
      setTrace(res.trace);
      const outBytes = runtime.readBytes(outAbs);
      if (outBytes && outBytes.length > 0) {
        library.setOutput(`out-${file.name}`, outBytes);
        setOutPath(outAbs);
        setVersion((v) => v + 1);
      }
    } else {
      setError(res.error ?? "the run produced no trace");
      setTrace(null);
    }
    setBusy(false);
  }, [
    runtime.ready,
    runtime.writeFile,
    runtime.trace,
    runtime.readBytes,
    library,
    file?.path,
    file?.changedAt,
    pipelineId,
  ]); // eslint-disable-line react-hooks/exhaustive-deps

  // Auto-run once ready, and whenever the selected file or pipeline changes.
  // Depend on stable primitives, not the runPipeline callback — that closes over
  // the file library, whose identity changes every render, which would re-fire
  // the effect on each render and loop.
  useEffect(() => {
    if (runtime.ready) void runPipeline();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [runtime.ready, file?.path, file?.changedAt, pipelineId]);

  return (
    <div className="kapi-reference flex flex-col gap-3 text-foreground">
      <div className="flex flex-wrap items-center gap-3">
        <FileSelectorField
          label="Inputs"
          library={library}
          selection={selection}
          onSelectionChange={setSelection}
          sampleIds={sampleIds}
        />
        <ToggleGroup
          type="single"
          variant="outline"
          value={pipelineId}
          onValueChange={(v) => v && setPipelineId(v)}
        >
          {PIPELINES.map((p) => (
            <ToggleGroupItem key={p.id} value={p.id} className="px-3 text-xs">
              {p.label}
            </ToggleGroupItem>
          ))}
        </ToggleGroup>
        <Button onClick={() => void runPipeline()} disabled={!runtime.ready || busy}>
          <Play /> Run
        </Button>
      </div>

      <ActiveFileSwitcher files={selected} activePath={file?.path} onChange={setActivePath} />

      <div
        className={cn("min-h-[1.4rem] text-sm text-muted-foreground", error && "text-destructive")}
      >
        {runtime.status === "booting" && "Booting kapi (first run downloads ~13 MB)…"}
        {runtime.status === "error" && `Failed to start: ${runtime.error}`}
        {runtime.ready && busy && "Running pipeline…"}
        {runtime.ready && !busy && error && `Error: ${error}`}
      </div>

      {trace && <FlowTracePlayer trace={trace} showDescription={false} />}
      {outPath && <OutputView runtime={runtime} path={outPath} version={version} />}
      {!trace && !error && (
        <div className="py-4 text-center text-sm text-muted-foreground">
          Pick a file and pipeline, then press Run.
        </div>
      )}
    </div>
  );
}
