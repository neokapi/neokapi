import React, { useCallback, useEffect, useMemo, useState } from "react";
import { useLabRuntime } from "./useLabRuntime";
import type { LabRuntimeAssets } from "./useLabRuntime";
import { WORKSPACE_SAMPLES, workspaceSampleById } from "./workspaceSamples";
import type { WorkspaceSample } from "./workspaceSamples";
import shared from "./styles.module.css";
import s from "./WorkspaceExplorer.module.css";

export interface WorkspaceExplorerProps {
  assets: LabRuntimeAssets | null;
  defaultSampleId?: string;
}

// Structured `kapi info --json` output.
interface WorkspaceInfo {
  workspace: string;
  sourceLang?: string;
  targetLangs?: string[];
  out?: string;
  documents: string[];
  overlays: Record<string, number>;
  dirty: boolean;
}

const STEPS = ["extract", "transform", "pack", "merge"] as const;
type StepName = (typeof STEPS)[number];
const TARGET = "qps"; // pseudo-translate's default locale — deterministic, no network

// WorkspaceExplorer drives the real .klz workspace lifecycle in WASM —
// extract → transform → pack → merge — on a text or Office sample, showing the
// workspace's recipe, per-locale overlay coverage, and dirty/clean state after
// each step (via `kapi info --json`), plus the merged output. It is the
// single-file, serverless twin of a .kapi project (AD-025 §5).
export default function WorkspaceExplorer({
  assets,
  defaultSampleId,
}: WorkspaceExplorerProps): React.ReactElement {
  const runtime = useLabRuntime(assets);
  const [sampleId, setSampleId] = useState(() => workspaceSampleById(defaultSampleId ?? "json").id);
  const sample: WorkspaceSample = useMemo(() => workspaceSampleById(sampleId), [sampleId]);

  const [done, setDone] = useState<Set<StepName>>(new Set());
  const [info, setInfo] = useState<WorkspaceInfo | null>(null);
  const [output, setOutput] = useState<string | null>(null);
  const [busy, setBusy] = useState<StepName | null>(null);
  const [err, setErr] = useState<string | null>(null);

  // Per-sample workspace + output paths (distinct so explorers/samples don't collide).
  const klz = `/project/ws-${sampleId}.klz`;
  const srcPath = `/project/${sample.filename}`;
  const outPath = `/project/out/${sample.filename}`;

  const reset = useCallback(() => {
    setDone(new Set());
    setInfo(null);
    setOutput(null);
    setErr(null);
  }, []);

  // Reset and re-seed when the sample changes.
  useEffect(() => {
    reset();
  }, [sampleId, reset]);

  const refreshInfo = useCallback(async () => {
    const { code, output: out } = await runtime.runCapture(["info", klz, "--json"]);
    if (code === 0) {
      try {
        setInfo(JSON.parse(out) as WorkspaceInfo);
      } catch {
        /* ignore parse error */
      }
    }
  }, [runtime, klz]);

  const runStep = useCallback(
    async (step: StepName) => {
      setBusy(step);
      setErr(null);
      try {
        // (Re)seed the source before extract so a sample switch is clean.
        if (step === "extract") {
          runtime.writeFile(sample.filename, sample.bytes());
        }
        const argv: Record<StepName, string[]> = {
          extract: [
            "extract",
            srcPath,
            "-o",
            klz,
            "--target-lang",
            TARGET,
            "--out",
            "out/{name}.{ext}",
          ],
          transform: ["pseudo-translate", klz],
          pack: ["pack", klz],
          merge: ["merge", klz],
        };
        const code = await runtime.run(argv[step]);
        if (code !== 0) {
          setErr(`\`kapi ${argv[step].join(" ")}\` exited ${code}`);
          return;
        }
        setDone((d) => new Set(d).add(step));
        await refreshInfo();
        if (step === "merge") {
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
        }
      } finally {
        setBusy(null);
      }
    },
    [runtime, sample, srcPath, klz, outPath, refreshInfo],
  );

  const stepEnabled = (step: StepName, i: number): boolean => {
    if (!runtime.ready || busy) return false;
    if (i === 0) return true;
    return done.has(STEPS[i - 1]);
  };

  return (
    <div className={shared.explorer}>
      <div className={shared.statusBar}>
        {runtime.status === "booting" && "Booting kapi (first run downloads the WASM engine)…"}
        {runtime.status === "error" && `Failed to start: ${runtime.error}`}
        {runtime.ready && (
          <label>
            Sample:{" "}
            <select
              value={sampleId}
              onChange={(e) => setSampleId(e.target.value)}
              disabled={!!busy}
            >
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
        {STEPS.map((step, i) => (
          <button
            key={step}
            className={`${s.step} ${done.has(step) ? s.stepDone : ""}`}
            disabled={!stepEnabled(step, i)}
            onClick={() => runStep(step)}
          >
            <span className={s.stepNum}>{i + 1}</span>
            {busy === step ? "…" : `kapi ${step === "transform" ? "pseudo-translate" : step}`}
            {done.has(step) && " ✓"}
          </button>
        ))}
      </div>

      {err && <div className={`${shared.statusBar} ${shared.statusError}`}>{err}</div>}

      <div className={s.panel}>
        <div className={s.card}>
          <div className={s.cardTitle}>Workspace</div>
          {!info && <div className={s.binaryNote}>Run “extract” to create the workspace.</div>}
          {info && <InfoView info={info} />}
        </div>
        <div className={s.card}>
          <div className={s.cardTitle}>Merged output</div>
          {output == null && (
            <div className={s.binaryNote}>Run all four steps to emit the localized file.</div>
          )}
          {output != null &&
            (sample.binary ? (
              <div className={s.binaryNote}>{output}</div>
            ) : (
              <pre className={s.output}>{output}</pre>
            ))}
        </div>
      </div>
    </div>
  );
}

function InfoView({ info }: { info: WorkspaceInfo }): React.ReactElement {
  const locales = info.targetLangs ?? [];
  const maxCount = Math.max(1, ...Object.values(info.overlays ?? {}));
  return (
    <>
      <div className={s.kv}>
        <span className={s.kvKey}>documents</span>
        <span>{info.documents.join(", ")}</span>
        <span className={s.kvKey}>recipe</span>
        <span>
          {info.sourceLang} → {locales.join(", ") || "—"}
        </span>
        <span className={s.kvKey}>output</span>
        <span>{info.out || "(default)"}</span>
        <span className={s.kvKey}>state</span>
        <span>
          <span className={`${s.pill} ${info.dirty ? s.pillDirty : s.pillClean}`}>
            {info.dirty ? "dirty" : "clean (packed)"}
          </span>
        </span>
      </div>
      {locales.length > 0 && (
        <div style={{ marginTop: "0.6rem" }}>
          <div className={s.cardTitle}>Overlays (translated blocks)</div>
          {locales.map((l) => {
            const n = info.overlays?.[l] ?? 0;
            return (
              <div key={l} className={s.overlayRow}>
                <span style={{ width: "3rem" }}>{l}</span>
                <span className={s.overlayBar} style={{ width: `${(n / maxCount) * 60 + 2}px` }} />
                <span>{n}</span>
              </div>
            );
          })}
        </div>
      )}
    </>
  );
}
