import React, { useCallback, useEffect, useRef, useState } from "react";
import { Play } from "lucide-react";
import Editor, { loader } from "@monaco-editor/react";
import { useLabRuntime } from "./useLabRuntime";
import type { LabRuntimeAssets } from "./useLabRuntime";
import FileSource from "./FileSource";
import type { FileSourceValue } from "./FileSource";
import BlockResults from "./BlockResults";
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

// Pin a specific, stable Monaco from the CDN rather than @latest. "Latest"
// (0.55.x) regressed editor behavior in this embed — EditContext eating Space,
// and document words leaking into completions the suggest options don't gate.
// 0.52.2 honors those options. Configured once at module load (this is a
// client-only, lazily-imported chunk, so it runs before the editor boots).
loader.config({ paths: { vs: "https://cdn.jsdelivr.net/npm/monaco-editor@0.52.2/min/vs" } });

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
          language="typescript"
          theme={theme}
          value={code}
          onChange={(v) => setCode(v ?? "")}
          onMount={(editor, monaco) => {
            // Use the TypeScript service (the model language is "typescript").
            // In JS mode, member access offers every in-scope identifier (JS is
            // dynamic), so "part." leaked words like toUpperCase; TS resolves it
            // to only the real Part members.
            const tsd = monaco.languages.typescript.typescriptDefaults;
            // The script API .d.ts gives completion, hover and inline diagnostics
            // for part / emit / skip / log.
            tsd.addExtraLib(SCRIPT_API_DTS, "file:///script-api.d.ts");
            // Drop the DOM lib so browser globals (window, document, …) — which
            // the goja sandbox does not have — stop appearing in completions.
            tsd.setCompilerOptions({ ...tsd.getCompilerOptions(), lib: ["es2020"] });
            editor.updateOptions({ wordBasedSuggestions: "off", suggest: { showWords: false } });
            // Block OS text substitutions from injecting characters into code —
            // notably macOS "Add period with double-space", which turns a double
            // space into ". ". Autocorrect-style replacements are never wanted in
            // a code editor, and the textarea's autocorrect="off" doesn't stop
            // this one, so we cancel it at the input layer.
            const ta = editor.getDomNode()?.querySelector("textarea");
            ta?.addEventListener("beforeinput", (e) => {
              const ie = e as InputEvent;
              if (
                ie.inputType === "insertReplacementText" ||
                (ie.inputType === "insertText" && ie.data === ". ")
              ) {
                e.preventDefault();
              }
            });
          }}
          options={{
            minimap: { enabled: false },
            fontSize: 13,
            lineNumbers: "on",
            scrollBeyondLastLine: false,
            automaticLayout: true,
            tabSize: 2,
            padding: { top: 10, bottom: 10 },
            // Use the classic hidden-textarea input path rather than the newer
            // EditContext API: EditContext mishandles some keys (e.g. Space)
            // when the editor sits inside certain page layouts.
            editContext: false,
            // Render the suggest/hover widgets in a body-level layer so they are
            // not clipped by the editor's fixed height + rounded overflow box.
            fixedOverflowWidgets: true,
            // Only offer type-aware completions from the script API .d.ts — not
            // every word in the document (which surfaced bogus members like
            // "toUpperCase" on `part`). showWords:false is the version-proof
            // switch; wordBasedSuggestions covers older/newer Monaco too.
            wordBasedSuggestions: "off",
            suggest: { showWords: false },
          }}
        />
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
