import React, { useCallback, useEffect, useRef, useState } from "react";
import { Play } from "lucide-react";
import Editor from "@monaco-editor/react";
import { useLabRuntime } from "./useLabRuntime";
import type { LabRuntimeAssets } from "./useLabRuntime";
import FileSource from "./FileSource";
import type { FileSourceValue } from "./FileSource";
import { SAMPLES } from "./samples";
import { DEFAULT_SCRIPT, SCRIPT_API_DTS, SCRIPT_EXAMPLES } from "./scriptApi";
import type { FlowTrace } from "./types";
import shared from "./styles.module.css";
import styles from "./ScriptLab.module.css";

export interface ScriptLabProps {
  assets: LabRuntimeAssets | null;
  defaultSampleId?: string;
  sampleIds?: string[];
}

interface BlockDiff {
  id: string;
  before: string;
  after: string | null; // null = the script dropped this block
}

// Build a .kapi recipe with a single `script` step carrying the user's code as
// a YAML literal block (each line indented under `code: |`), so arbitrary
// multi-line JS survives the round-trip into the engine intact.
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
          code: |
${body}
`;
}

const ANSI = /\[[0-9;]*m/g;

function currentTheme(): "vs-dark" | "light" {
  if (typeof document === "undefined") return "light";
  return document.documentElement.getAttribute("data-theme") === "dark" ? "vs-dark" : "light";
}

// ScriptLab lets a learner write a JavaScript transform against the content
// model, with full IntelliSense (Monaco + the script API .d.ts), pick from a
// library of examples, run it on a file in WASM (goja), and read the per-Block
// before/after plus any log() output — all in the browser.
export default function ScriptLab({
  assets,
  defaultSampleId,
  sampleIds,
}: ScriptLabProps): React.ReactElement {
  const runtime = useLabRuntime(assets);

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
  const [theme, setTheme] = useState<"vs-dark" | "light">("light");

  // Follow the site's light/dark mode.
  useEffect(() => {
    setTheme(currentTheme());
    const obs = new MutationObserver(() => setTheme(currentTheme()));
    obs.observe(document.documentElement, { attributes: true, attributeFilter: ["data-theme"] });
    return () => obs.disconnect();
  }, []);

  // Keep the latest code in a ref so the Run handler stays stable.
  const codeRef = useRef(code);
  codeRef.current = code;

  const run = useCallback(async () => {
    if (!runtime.ready) return;
    setBusy(true);
    setError(null);
    const inPath = runtime.writeFile(file.filename, file.content);
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

  const diffs: BlockDiff[] = trace
    ? Object.values(trace.parts)
        .filter((ss) => ss.initial.type === "Block")
        .map((ss) => {
          const after = ss.afterNode?.["tool-0"];
          return {
            id: ss.initial.id,
            before: ss.initial.sourceText ?? "",
            after: after ? (after.sourceText ?? "") : null,
          };
        })
    : [];

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
        <Editor
          height="300px"
          language="javascript"
          theme={theme}
          value={code}
          onChange={(v) => setCode(v ?? "")}
          onMount={(_editor, monaco) => {
            // Wire IntelliSense: the script API .d.ts gives completion, hover
            // and inline diagnostics for part / emit / skip / log.
            monaco.languages.typescript.javascriptDefaults.addExtraLib(
              SCRIPT_API_DTS,
              "file:///script-api.d.ts",
            );
          }}
          options={{
            minimap: { enabled: false },
            fontSize: 13,
            lineNumbers: "on",
            scrollBeyondLastLine: false,
            automaticLayout: true,
            tabSize: 2,
            padding: { top: 10, bottom: 10 },
          }}
        />
      </div>

      <div className={`${shared.statusBar} ${error ? shared.statusError : ""}`}>
        {runtime.status === "booting" && "Booting kapi (first run downloads ~13 MB)…"}
        {runtime.status === "error" && `Failed to start: ${runtime.error}`}
        {runtime.ready && busy && "Running script…"}
        {runtime.ready && !busy && error && `Error: ${error}`}
        {runtime.ready && !busy && !error && trace && `${diffs.length} block(s) processed`}
      </div>

      {trace && diffs.length > 0 && (
        <table className={styles.diff}>
          <thead>
            <tr>
              <th>Block</th>
              <th>Source (before)</th>
              <th>Source (after)</th>
            </tr>
          </thead>
          <tbody>
            {diffs.map((d) => {
              const changed = d.after !== null && d.after !== d.before;
              return (
                <tr
                  key={d.id}
                  className={d.after === null ? styles.dropped : changed ? styles.changed : ""}
                >
                  <td className={styles.blockId}>{d.id}</td>
                  <td>{d.before}</td>
                  <td>{d.after === null ? <em>dropped (skip)</em> : d.after}</td>
                </tr>
              );
            })}
          </tbody>
        </table>
      )}

      {logOutput && (
        <div className={styles.logPanel}>
          <div className={styles.logLabel}>log output</div>
          <pre className={styles.logPre}>{logOutput}</pre>
        </div>
      )}
    </div>
  );
}
