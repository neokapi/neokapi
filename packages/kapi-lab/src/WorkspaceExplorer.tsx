import React, { useCallback, useEffect, useMemo, useState } from "react";
import { useLabRuntime } from "./useLabRuntime";
import GateOverlay from "./GateOverlay";
import { useRunGate } from "./useRunGate";
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

const STEPS = ["extract", "transform", "pack", "reopen", "merge"] as const;
type StepName = (typeof STEPS)[number];
const TARGET = "qps"; // pseudo-translate's default locale — deterministic, no network

const STEP_LABEL: Record<StepName, string> = {
  extract: "kapi extract",
  transform: "kapi pseudo-translate",
  pack: "kapi pack",
  reopen: "reopen elsewhere",
  merge: "kapi merge",
};

// WorkspaceExplorer drives the real .klz workspace lifecycle in WASM and makes
// the page's headline — pause & resume — visible. You extract → pseudo-translate
// → pack a workspace, then *reopen the packed file on a fresh path* (a stand-in
// for another machine, where the working cache starts empty) and watch kapi
// rebuild the whole workspace from the file alone before merging the output. The
// single-file, serverless twin of a .kapi project (AD-025 §5).
export default function WorkspaceExplorer({
  assets,
  defaultSampleId,
}: WorkspaceExplorerProps): React.ReactElement {
  const runtime = useLabRuntime(assets, { autoBoot: false });
  const gate = useRunGate(runtime);
  const [sampleId, setSampleId] = useState(() => workspaceSampleById(defaultSampleId ?? "json").id);
  const sample: WorkspaceSample = useMemo(() => workspaceSampleById(sampleId), [sampleId]);

  const [done, setDone] = useState<Set<StepName>>(new Set());
  const [info, setInfo] = useState<WorkspaceInfo | null>(null);
  const [resumed, setResumed] = useState(false);
  const [output, setOutput] = useState<string | null>(null);
  const [busy, setBusy] = useState<StepName | null>(null);
  const [err, setErr] = useState<string | null>(null);

  // Two workspace paths: the one you work on, and the fresh path you "receive"
  // the packed parcel on. The working cache is keyed by path, so the received
  // path has none and the first command there must rebuild from the file — that
  // reconstruction is the resume.
  const workKlz = `/project/ws-${sampleId}.klz`;
  const recvName = `ws-${sampleId}-received.klz`;
  const recvKlz = `/project/${recvName}`;
  const srcPath = `/project/${sample.filename}`;
  const outPath = `/project/out/${sample.filename}`;
  const activeKlz = resumed ? recvKlz : workKlz;

  const reset = useCallback(() => {
    setDone(new Set());
    setInfo(null);
    setResumed(false);
    setOutput(null);
    setErr(null);
  }, []);

  // Reset and re-seed when the sample changes.
  useEffect(() => {
    reset();
  }, [sampleId, reset]);

  const refreshInfo = useCallback(
    async (klz: string) => {
      const { code, output: out } = await runtime.runCapture(["info", klz, "--json"]);
      if (code === 0) {
        try {
          setInfo(JSON.parse(out) as WorkspaceInfo);
        } catch {
          /* ignore parse error */
        }
      }
    },
    [runtime],
  );

  const runStep = useCallback(
    async (step: StepName) => {
      setBusy(step);
      setErr(null);
      try {
        // Reopen is a filesystem hand-off, not a kapi command: copy the packed
        // work.klz onto a fresh path with no cache, then let the first `info`
        // there rebuild the workspace from the file.
        if (step === "reopen") {
          const bytes = runtime.readBytes(workKlz);
          if (!bytes) {
            setErr("could not read the packed workspace");
            return;
          }
          runtime.writeFile(recvName, bytes);
          setResumed(true);
          setDone((d) => new Set(d).add(step));
          await refreshInfo(recvKlz);
          return;
        }

        // (Re)seed the source before extract so a sample switch is clean.
        if (step === "extract") {
          runtime.writeFile(sample.filename, sample.bytes());
        }
        let argv: string[];
        if (step === "extract") {
          argv = ["extract", srcPath, "-o", workKlz, "--target-lang", TARGET, "--out", "out/{name}.{ext}"];
        } else if (step === "transform") {
          argv = ["pseudo-translate", workKlz];
        } else if (step === "pack") {
          argv = ["pack", workKlz];
        } else {
          // merge runs against whichever workspace is active — the resumed one.
          argv = ["merge", activeKlz];
        }
        const code = await runtime.run(argv);
        if (code !== 0) {
          setErr(`\`kapi ${argv.join(" ")}\` exited ${code}`);
          return;
        }
        setDone((d) => new Set(d).add(step));
        await refreshInfo(step === "merge" ? activeKlz : workKlz);
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
    [runtime, sample, srcPath, workKlz, recvName, recvKlz, activeKlz, outPath, refreshInfo],
  );

  const stepEnabled = (step: StepName, i: number): boolean => {
    if (!runtime.ready || busy) return false;
    if (i === 0) return true;
    return done.has(STEPS[i - 1]);
  };

  return (
    <div className={`kapi-reference relative ${shared.explorer}`}>
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
            {busy === step ? "…" : STEP_LABEL[step]}
            {done.has(step) && " ✓"}
          </button>
        ))}
      </div>

      {err && <div className={`${shared.statusBar} ${shared.statusError}`}>{err}</div>}

      <div className={s.panel} style={{ minHeight: 420 }}>
        <div className={s.card}>
          <div className={s.cardTitle}>
            Workspace{info ? ` · ${activeKlz.replace("/project/", "")}` : ""}
          </div>
          {!info && <div className={s.binaryNote}>Run “extract” to create the workspace.</div>}
          {resumed && (
            <div className={s.binaryNote}>
              ↻ reopened on a fresh path — the working cache started empty here, so kapi rebuilt the
              whole workspace from the packed <code>.klz</code> alone. The work below is the work you
              left, picked up unchanged.
            </div>
          )}
          {info && <InfoView info={info} />}
        </div>
        <div className={s.card}>
          <div className={s.cardTitle}>Merged output</div>
          {output == null && (
            <div className={s.binaryNote}>
              Run every step to emit the localized file from the resumed workspace.
            </div>
          )}
          {output != null &&
            (sample.binary ? (
              <div className={s.binaryNote}>{output}</div>
            ) : (
              <pre className={s.output}>{output}</pre>
            ))}
        </div>
      </div>

      <GateOverlay
        gate={gate}
        title="Workspace"
        description="Pause a .klz workspace and resume it on a fresh path."
      />
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
