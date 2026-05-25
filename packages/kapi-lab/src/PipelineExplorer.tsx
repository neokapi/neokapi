import React, { useCallback, useEffect, useState } from "react";
import { Play } from "lucide-react";
import { useLabRuntime } from "./useLabRuntime";
import type { LabRuntimeAssets } from "./useLabRuntime";
import FileSource from "./FileSource";
import type { FileSourceValue } from "./FileSource";
import FlowTracePlayer from "./FlowTracePlayer";
import { SAMPLES } from "./samples";
import type { FlowTrace } from "./types";
import styles from "./styles.module.css";

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
    id: "ai-translate-qa",
    label: "AI translate + QA (demo)",
    build: (i, o) => ["run", "ai-translate-qa", "-i", i, "-o", o, "--target-lang", "fr"],
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

// PipelineExplorer runs a real flow over a file in WASM with tracing on, then
// hands the live FlowTrace to <FlowTracePlayer> — so a learner watches their own
// file stream through the reader, tools and writer, one step at a time.
export default function PipelineExplorer({
  assets,
  defaultSampleId,
  defaultPipelineId,
  sampleIds,
}: PipelineExplorerProps): React.ReactElement {
  const runtime = useLabRuntime(assets);

  const initial = SAMPLES.find((s) => s.id === defaultSampleId) ?? SAMPLES[0];
  const [file, setFile] = useState<FileSourceValue>({
    filename: initial.filename,
    content: initial.content,
    label: initial.label,
  });
  const [pipelineId, setPipelineId] = useState(defaultPipelineId ?? PIPELINES[0].id);
  const [trace, setTrace] = useState<FlowTrace | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const runPipeline = useCallback(async () => {
    if (!runtime.ready) return;
    const pipeline = PIPELINES.find((p) => p.id === pipelineId) ?? PIPELINES[0];
    setBusy(true);
    setError(null);
    const inPath = runtime.writeFile(file.filename, file.bytes ?? file.content);
    const outPath = `/project/out-${file.filename}`;
    const res = await runtime.trace(pipeline.build(inPath, outPath));
    if (res.ok && res.trace) {
      setTrace(res.trace);
    } else {
      setError(res.error ?? "the run produced no trace");
      setTrace(null);
    }
    setBusy(false);
  }, [runtime.ready, runtime.writeFile, runtime.trace, file, pipelineId]);

  // Auto-run once the runtime is ready, and whenever the file/pipeline changes.
  // runPipeline is a stable callback keyed on those inputs.
  useEffect(() => {
    if (runtime.ready) void runPipeline();
  }, [runtime.ready, runPipeline]);

  return (
    <div className={styles.explorer}>
      <FileSource value={file} onChange={setFile} sampleIds={sampleIds} />

      <div className={styles.pickerRow}>
        <label className={styles.pickerLabel}>Pipeline</label>
        <select
          className={styles.select}
          value={pipelineId}
          onChange={(e) => setPipelineId(e.target.value)}
        >
          {PIPELINES.map((p) => (
            <option key={p.id} value={p.id}>
              {p.label}
            </option>
          ))}
        </select>
        <button
          className={styles.runButton}
          onClick={() => void runPipeline()}
          disabled={!runtime.ready || busy}
        >
          <Play size={14} /> Run
        </button>
      </div>

      <div className={`${styles.statusBar} ${error ? styles.statusError : ""}`}>
        {runtime.status === "booting" && "Booting kapi (first run downloads ~13 MB)…"}
        {runtime.status === "error" && `Failed to start: ${runtime.error}`}
        {runtime.ready && busy && "Running pipeline…"}
        {runtime.ready && !busy && error && `Error: ${error}`}
      </div>

      {trace ? (
        <FlowTracePlayer trace={trace} showDescription={false} />
      ) : (
        !error && <div className={styles.emptyHint}>Pick a file and pipeline, then press Run.</div>
      )}
    </div>
  );
}
