import React, { useCallback, useEffect, useMemo, useState } from "react";
import { useLabRuntime } from "./useLabRuntime";
import type { LabRuntimeAssets } from "./useLabRuntime";
import { WORKSPACE_SAMPLES, workspaceSampleById } from "./workspaceSamples";
import type { WorkspaceSample } from "./workspaceSamples";
import shared from "./styles.module.css";
import s from "./WorkspaceExplorer.module.css";

export interface ProjectExplorerProps {
  assets: LabRuntimeAssets | null;
  defaultSampleId?: string;
}

const TARGET = "qps";

function formatFor(filename: string): string {
  if (filename.endsWith(".json")) return "json";
  return "openxml"; // .docx / .xlsx / .pptx
}

function recipeFor(sample: WorkspaceSample): string {
  return `version: "v1"
name: demo
defaults:
  source_locale: en
  target_locales: [${TARGET}]
content:
  - path: ${sample.filename}
    format: ${formatFor(sample.filename)}
flows:
  pseudo:
    steps:
      - tool: pseudo-translate
`;
}

// ProjectExplorer teaches the .kapi *project* model — config-as-code in a
// committed recipe (content + flows + target languages) plus a .kapi/ state
// dir (the persistent runtime cache) — and runs a declared flow against it in
// WASM. It's the multi-file, team/server-oriented counterpart to the
// single-file .klz workspace (AD-025 §5). In the browser the state dir's cache
// is an in-memory store, so it's shown as regenerable state rather than files.
export default function ProjectExplorer({
  assets,
  defaultSampleId,
}: ProjectExplorerProps): React.ReactElement {
  const runtime = useLabRuntime(assets);
  const [sampleId, setSampleId] = useState(() => workspaceSampleById(defaultSampleId ?? "json").id);
  const sample: WorkspaceSample = useMemo(() => workspaceSampleById(sampleId), [sampleId]);
  const recipe = useMemo(() => recipeFor(sample), [sample]);

  const [ran, setRan] = useState(false);
  const [output, setOutput] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const dir = `/project/proj-${sampleId}`;
  const recipePath = `${dir}/demo.kapi`;
  const srcPath = `${dir}/${sample.filename}`;
  const outPath = `${dir}/out/${sample.filename}`;

  useEffect(() => {
    setRan(false);
    setOutput(null);
    setErr(null);
  }, [sampleId]);

  const runFlow = useCallback(async () => {
    setBusy(true);
    setErr(null);
    try {
      runtime.mkdir(`proj-${sampleId}`);
      runtime.writeFile(`proj-${sampleId}/demo.kapi`, recipe);
      runtime.writeFile(`proj-${sampleId}/${sample.filename}`, sample.bytes());
      const argv = [
        "run",
        "pseudo",
        "-p",
        recipePath,
        "-i",
        srcPath,
        "-o",
        outPath,
        "--target-lang",
        TARGET,
      ];
      const code = await runtime.run(argv);
      if (code !== 0) {
        setErr(`\`kapi ${argv.join(" ")}\` exited ${code}`);
        return;
      }
      setRan(true);
      if (sample.binary) {
        const bytes = runtime.readBytes(outPath);
        setOutput(
          bytes
            ? `✓ produced ${sample.filename} — ${bytes.length} bytes, valid OOXML zip: ${bytes[0] === 0x50 && bytes[1] === 0x4b}`
            : "(no output)",
        );
      } else {
        setOutput(runtime.readFile(outPath) ?? "(no output)");
      }
    } finally {
      setBusy(false);
    }
  }, [runtime, sample, sampleId, recipe, recipePath, srcPath, outPath]);

  return (
    <div className={shared.explorer}>
      <div className={shared.statusBar}>
        {runtime.status === "booting" && "Booting kapi (first run downloads the WASM engine)…"}
        {runtime.status === "error" && `Failed to start: ${runtime.error}`}
        {runtime.ready && (
          <label>
            Content:{" "}
            <select value={sampleId} onChange={(e) => setSampleId(e.target.value)} disabled={busy}>
              {WORKSPACE_SAMPLES.map((w) => (
                <option key={w.id} value={w.id}>
                  {w.label} — {w.kind}
                </option>
              ))}
            </select>
          </label>
        )}
      </div>

      <div className={s.steps}>
        <button
          className={`${s.step} ${ran ? s.stepDone : ""}`}
          disabled={!runtime.ready || busy}
          onClick={runFlow}
        >
          {busy ? "…" : "kapi run pseudo -p demo.kapi"}
          {ran && " ✓"}
        </button>
      </div>

      {err && <div className={`${shared.statusBar} ${shared.statusError}`}>{err}</div>}

      <div className={s.panel}>
        <div className={s.card}>
          <div className={s.cardTitle}>demo.kapi (the recipe — committed config)</div>
          <pre className={s.output}>{recipe}</pre>
        </div>
        <div className={s.card}>
          <div className={s.cardTitle}>Project layout</div>
          <div className={s.kv}>
            <span className={s.kvKey}>demo.kapi</span>
            <span>recipe · committed</span>
            <span className={s.kvKey}>{sample.filename}</span>
            <span>source content</span>
            <span className={s.kvKey}>.kapi/</span>
            <span>state · cache + TM + termbase · regenerable (in-memory in the browser)</span>
            <span className={s.kvKey}>out/{sample.filename}</span>
            <span>{ran ? "✓ written" : "— (run the flow)"}</span>
          </div>
          <div className={s.cardTitle} style={{ marginTop: "0.8rem" }}>
            Merged output
          </div>
          {output == null && (
            <div className={s.binaryNote}>Run the flow to localize the declared content.</div>
          )}
          {output != null &&
            (sample.binary ? (
              <div className={s.binaryNote}>{output}</div>
            ) : (
              <pre className={s.output}>{output}</pre>
            ))}
        </div>
      </div>

      <p style={{ fontSize: "0.85rem", opacity: 0.8, marginTop: "0.6rem" }}>
        A <strong>project</strong> keeps config in a committed <code>.kapi</code> recipe and its
        working state in a <code>.kapi/</code> dir — built for teams and servers. A{" "}
        <strong>.klz workspace</strong> folds the same content + work into one portable file — built
        for ad-hoc, single-file hand-off. Same engine underneath.
      </p>
    </div>
  );
}
