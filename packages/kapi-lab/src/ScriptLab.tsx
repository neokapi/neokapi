import React, { useCallback, useEffect, useRef, useState } from "react";
import { Play } from "lucide-react";
import { useLabRuntime } from "./useLabRuntime";
import RunGate from "./RunGate";
import { useRunGate } from "./useRunGate";
import type { LabRuntimeAssets } from "./useLabRuntime";
import FileSource from "./FileSource";
import type { FileSourceValue } from "./FileSource";
import BlockResults from "./BlockResults";
import ScriptCodeEditor from "./ScriptCodeEditor";
import { SAMPLES } from "./samples";
import { DEFAULT_SCRIPT, SCRIPT_EXAMPLES } from "./scriptApi";
import type { FlowTrace } from "@neokapi/ui-primitives/preview";
import shared from "./styles.module.css";
import styles from "./ScriptLab.module.css";

export interface ScriptLabProps {
  assets: LabRuntimeAssets | null;
  defaultSampleId?: string;
  sampleIds?: string[];
}

// Build a .kapi recipe with a single `script` step carrying the user's code as
// a YAML literal block (each line indented under `code: |`), so arbitrary
// multi-line JS survives the round-trip into the engine intact. The sandbox
// opts the script into source mutation (AD-006: source is read-only for
// scripts by default) so the examples that rewrite text actually do.
function buildRecipe(code: string): string {
  const body = code
    .split("\n")
    .map((l) => "            " + l)
    .join("\n");
  return `version: v1
name: Lab
defaults:
  source_language: en
flows:
  lab:
    steps:
      - tool: script
        config:
          allowSourceMutation: true
          code: |
${body}
`;
}

const ANSI = /\[[0-9;]*m/g;

// ScriptLab lets a learner write a JavaScript transform against the content
// model, with full IntelliSense (Monaco + the script API .d.ts), pick from a
// library of examples, run it on a file in WASM (goja), and read the per-Block
// before/after plus any log() output — all in the browser.
export default function ScriptLab({
  assets,
  defaultSampleId,
  sampleIds,
}: ScriptLabProps): React.ReactElement {
  const runtime = useLabRuntime(assets, { autoBoot: false });
  const gate = useRunGate(runtime);

  const initial = SAMPLES.find((s) => s.id === defaultSampleId) ?? SAMPLES[0];
  const [file, setFile] = useState<FileSourceValue>({
    filename: initial.filename,
    content: initial.content,
    label: initial.label,
  });
  const [code, setCode] = useState(DEFAULT_SCRIPT);
  const [trace, setTrace] = useState<FlowTrace | null>(null);
  const [logOutput, setLogOutput] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  // Keep the latest code in a ref so the Run handler stays stable.
  const codeRef = useRef(code);
  codeRef.current = code;

  const run = useCallback(async () => {
    if (!runtime.ready) return;
    setBusy(true);
    setError(null);
    const inPath = runtime.writeFile(file.filename, file.bytes ?? file.content);
    runtime.writeFile("script.kapi", buildRecipe(codeRef.current));
    const res = await runtime.trace([
      "run",
      "lab",
      "-p",
      "/project/script.kapi",
      "-i",
      inPath,
      "-o",
      `/project/script-out-${file.filename}`,
      "--target-lang",
      "fr",
    ]);
    setLogOutput((res.output ?? "").replace(ANSI, "").trim());
    if (res.ok && res.trace) {
      setTrace(res.trace);
    } else {
      setError(res.error ?? "the script run produced no trace");
      setTrace(null);
    }
    setBusy(false);
  }, [runtime.ready, runtime.writeFile, runtime.trace, file]);

  // Run once when the runtime is ready and whenever the file changes (not on
  // every keystroke — code is re-run explicitly via the Run button).
  useEffect(() => {
    if (runtime.ready) void run();
  }, [runtime.ready, file]); // eslint-disable-line react-hooks/exhaustive-deps

  const blockCount = trace
    ? Object.values(trace.parts).filter((ss) => ss.initial.type === "Block").length
    : 0;

  if (!gate.armed) {
    return (
      <RunGate
        gate={gate}
        title="Script lab"
        description="Author and run a kapi flow script in the browser."
      />
    );
  }
  return (
    <div className={shared.explorer}>
      <FileSource value={file} onChange={setFile} sampleIds={sampleIds} />

      <div className={shared.pickerRow}>
        <label className={shared.pickerLabel}>Example</label>
        <select
          className={shared.select}
          defaultValue=""
          onChange={(e) => {
            const ex = SCRIPT_EXAMPLES.find((x) => x.id === e.target.value);
            if (ex) setCode(ex.code);
            e.target.value = "";
          }}
        >
          <option value="" disabled>
            Load an example…
          </option>
          {SCRIPT_EXAMPLES.map((ex) => (
            <option key={ex.id} value={ex.id} title={ex.blurb}>
              {ex.label}
            </option>
          ))}
        </select>
        <button
          className={shared.runButton}
          onClick={() => void run()}
          disabled={!runtime.ready || busy}
        >
          <Play size={14} /> Run
        </button>
      </div>

      <div className={styles.editor}>
        <ScriptCodeEditor code={code} onChange={setCode} height="300px" />
      </div>

      <div className={`${shared.statusBar} ${error ? shared.statusError : ""}`}>
        {runtime.status === "booting" && "Booting kapi (first run downloads ~13 MB)…"}
        {runtime.status === "error" && `Failed to start: ${runtime.error}`}
        {runtime.ready && busy && "Running script…"}
        {runtime.ready && !busy && error && `Error: ${error}`}
        {runtime.ready && !busy && !error && trace && `${blockCount} block(s) processed`}
      </div>

      {trace && <BlockResults trace={trace} targetLocale="fr" />}

      {logOutput && (
        <div className={styles.logPanel}>
          <div className={styles.logLabel}>log output</div>
          <pre className={styles.logPre}>{logOutput}</pre>
        </div>
      )}
    </div>
  );
}
