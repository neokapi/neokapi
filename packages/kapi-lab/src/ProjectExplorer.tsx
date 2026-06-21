import React, { useCallback, useEffect, useMemo, useState } from "react";
import { useLabRuntime } from "./useLabRuntime";
import RunGate from "./RunGate";
import { useRunGate } from "./useRunGate";
import type { LabRuntimeAssets } from "./useLabRuntime";
import { WORKSPACE_SAMPLES, workspaceSampleById } from "./workspaceSamples";
import type { WorkspaceSample } from "./workspaceSamples";
import shared from "./styles.module.css";
import s from "./WorkspaceExplorer.module.css";

export interface ProjectExplorerProps {
  assets: LabRuntimeAssets | null;
  defaultSampleId?: string;
}

// The project's declared target locale. fr is a real shipping target — the
// `translate` flow leverages the project TM to fill it. (qps, the
// pseudo-translate test locale, is NOT a shipping target and must never appear
// in a recipe's target_languages.)
export const TARGETS = ["fr"] as const;
type Target = (typeof TARGETS)[number];

// Flows the recipe declares. Both leverage translation memory (tm-leverage) and
// run offline — no LLM, no network: the project TM (seeded via `kapi tm import`
// before extract) supplies the French. `translate` fills any TM match;
// `translate-exact` only fills 100% matches, so picking it visibly changes the
// merged output, demonstrating that a project is a multi-flow contract.
export interface FlowDef {
  id: string;
  label: string;
  /** Inline recipe YAML for this flow's steps (2-space indented under the id). */
  yaml: string;
}
export const FLOWS: FlowDef[] = [
  {
    id: "translate",
    label: "translate — TM leverage (exact + fuzzy)",
    yaml: `  translate:
    steps:
      - tool: tm-leverage`,
  },
  {
    id: "translate-exact",
    label: "translate-exact — TM leverage (100% only)",
    yaml: `  translate-exact:
    steps:
      - tool: tm-leverage
        config:
          fillTargetThreshold: 100`,
  },
];

const STEPS = ["extract", "run", "merge"] as const;
type StepName = (typeof STEPS)[number];

export function formatFor(filename: string): string {
  if (filename.endsWith(".json")) return "json";
  return "openxml"; // .docx / .xlsx / .pptx
}

export function targetGlob(filename: string): string {
  const dot = filename.lastIndexOf(".");
  const ext = dot >= 0 ? filename.slice(dot) : "";
  const base = dot >= 0 ? filename.slice(0, dot) : filename;
  return `out/{lang}/${base}${ext}`;
}

// The committed recipe: source/target languages, the content glob to localize,
// and every declared flow. This is the config-as-code a `kapi init` scaffolds
// and you edit once.
export function recipeFor(sample: WorkspaceSample): string {
  return `version: v1
name: demo
defaults:
  source_language: en
  target_languages: [${TARGETS.join(", ")}]
content:
  - path: ${sample.filename}
    format: ${formatFor(sample.filename)}
    target: "${targetGlob(sample.filename)}"
flows:
${FLOWS.map((f) => f.yaml).join("\n")}
`;
}

// ProjectExplorer teaches the .kapi *project* model — config-as-code in a
// committed recipe (content + multiple flows + a real fr target) plus a .kapi/
// state dir (the persistent project store + TM) — and runs the project
// lifecycle in WASM: import the project TM → extract → run a declared
// translate flow (tm-leverage, process-only, commits real fr targets to the
// store) → merge (materialize the localized file). The translation is genuine
// TM leverage — no LLM, no network — so the merged output is a real fr file,
// never a pseudo/qps test artifact. It is the multi-file, team/server
// counterpart to the single-file .klz workspace (AD-026 / AD-025 §5). In the
// browser the state dir is an in-memory store, so it is shown as regenerable
// state rather than files.
export default function ProjectExplorer({
  assets,
  defaultSampleId,
}: ProjectExplorerProps): React.ReactElement {
  const runtime = useLabRuntime(assets, { autoBoot: false });
  const gate = useRunGate(runtime);
  const [sampleId, setSampleId] = useState(() => workspaceSampleById(defaultSampleId ?? "json").id);
  const sample: WorkspaceSample = useMemo(() => workspaceSampleById(sampleId), [sampleId]);
  const [flowId, setFlowId] = useState<string>(FLOWS[0].id);
  const recipe = useMemo(() => recipeFor(sample), [sample]);

  const [done, setDone] = useState<Set<StepName>>(new Set());
  const [busy, setBusy] = useState<StepName | null>(null);
  const [err, setErr] = useState<string | null>(null);
  // Which target locales have a merged file on disk after `merge`.
  const [merged, setMerged] = useState<Target[]>([]);
  // The merged output for the currently inspected locale.
  const [viewLocale, setViewLocale] = useState<Target>(TARGETS[0]);
  const [output, setOutput] = useState<string | null>(null);

  // Per-sample project dir (distinct so explorers/samples don't collide).
  const dir = `proj-${sampleId}`;
  const absDir = `/project/${dir}`;
  const recipePath = `${absDir}/demo.kapi`;
  const srcPath = `${absDir}/${sample.filename}`;
  const tmxPath = `${absDir}/project.tmx`;
  const outPath = useCallback(
    (lang: Target) => `${absDir}/out/${lang}/${sample.filename}`,
    [absDir, sample.filename],
  );

  const reset = useCallback(() => {
    setDone(new Set());
    setErr(null);
    setMerged([]);
    setOutput(null);
    setViewLocale(TARGETS[0]);
  }, []);

  // Reset when the sample or flow changes — a new flow means a fresh lifecycle.
  useEffect(() => {
    reset();
  }, [sampleId, flowId, reset]);

  // Read the merged output for the inspected locale (text or a binary note).
  const readMerged = useCallback(
    (lang: Target) => {
      if (sample.binary) {
        const bytes = runtime.readBytes(outPath(lang));
        setOutput(
          bytes
            ? `✓ produced ${sample.filename} — ${bytes.length} bytes, valid OOXML zip: ${
                bytes[0] === 0x50 && bytes[1] === 0x4b
              }`
            : `(no ${lang} output — this flow left ${lang} untranslated)`,
        );
      } else {
        setOutput(
          runtime.readFile(outPath(lang)) ??
            `(no ${lang} output — this flow left ${lang} untranslated)`,
        );
      }
    },
    [runtime, sample, outPath],
  );

  // Re-read when the inspected locale changes after a merge.
  useEffect(() => {
    if (done.has("merge")) readMerged(viewLocale);
  }, [viewLocale, done, readMerged]);

  const runStep = useCallback(
    async (step: StepName) => {
      setBusy(step);
      setErr(null);
      try {
        // Seed the project (recipe + source + TMX) before extract so a
        // sample/flow switch starts clean, and import the TMX into the project
        // TM so the translate (tm-leverage) flow has matches to pull.
        if (step === "extract") {
          runtime.mkdir(dir);
          runtime.writeFile(`${dir}/demo.kapi`, recipe);
          runtime.writeFile(`${dir}/${sample.filename}`, sample.bytes());
          runtime.writeFile(`${dir}/project.tmx`, sample.tmx);
          const tmCode = await runtime.run(["tm", "import", tmxPath, "-s", "en", "-t", "fr"]);
          if (tmCode !== 0) {
            setErr(`\`kapi tm import ${tmxPath}\` exited ${tmCode}`);
            return;
          }
        }
        const argv: Record<StepName, string[]> = {
          // extract reads every content glob in the recipe into the project store.
          extract: ["extract", "-p", recipePath],
          // A run inside a project is process-only: it commits target overlays
          // to the store rather than writing files (AD-026). The translate flow
          // leverages the project TM (seeded above) to fill real fr targets.
          run: ["run", flowId, "-p", recipePath, "-i", srcPath],
          // merge replays the stored translations onto each source.
          merge: ["merge", "-p", recipePath],
        };
        const code = await runtime.run(argv[step]);
        if (code !== 0) {
          setErr(`\`kapi ${argv[step].join(" ")}\` exited ${code}`);
          return;
        }
        setDone((d) => new Set(d).add(step));
        if (step === "merge") {
          const present = TARGETS.filter((l) => runtime.readBytes(outPath(l)) != null);
          setMerged(present);
          const first = present[0] ?? TARGETS[0];
          setViewLocale(first);
          readMerged(first);
        }
      } finally {
        setBusy(null);
      }
    },
    [runtime, sample, dir, recipe, recipePath, srcPath, tmxPath, flowId, outPath, readMerged],
  );

  const stepEnabled = (i: number): boolean => {
    if (!runtime.ready || busy) return false;
    if (i === 0) return true;
    return done.has(STEPS[i - 1]);
  };

  const stepLabel = (step: StepName): string => {
    if (step === "run") return `run ${flowId}`;
    return step;
  };

  if (!gate.armed) {
    return (
      <RunGate
        gate={gate}
        title="Project"
        description="Open a .kapi project and inspect it with the real engine."
      />
    );
  }
  return (
    <div className={shared.explorer}>
      <div className={shared.statusBar}>
        {runtime.status === "booting" && "Booting kapi (first run downloads the WASM engine)…"}
        {runtime.status === "error" && `Failed to start: ${runtime.error}`}
        {runtime.ready && (
          <>
            <label>
              Content:{" "}
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
            </label>{" "}
            <label>
              Flow:{" "}
              <select value={flowId} onChange={(e) => setFlowId(e.target.value)} disabled={!!busy}>
                {FLOWS.map((f) => (
                  <option key={f.id} value={f.id}>
                    {f.label}
                  </option>
                ))}
              </select>
            </label>
          </>
        )}
      </div>

      <div className={s.steps}>
        {STEPS.map((step, i) => (
          <button
            key={step}
            className={`${s.step} ${done.has(step) ? s.stepDone : ""}`}
            disabled={!stepEnabled(i)}
            onClick={() => runStep(step)}
          >
            <span className={s.stepNum}>{i + 1}</span>
            {busy === step ? "…" : `kapi ${stepLabel(step)}`}
            {done.has(step) && " ✓"}
          </button>
        ))}
      </div>

      {err && <div className={`${shared.statusBar} ${shared.statusError}`}>{err}</div>}

      <div className={s.panel}>
        <div className={s.card}>
          <div className={s.cardTitle}>demo.kapi (the recipe — committed config)</div>
          <pre className={s.output}>{recipe}</pre>
        </div>

        <div className={s.card}>
          <div className={s.cardTitle}>Project state</div>
          <div className={s.kv}>
            <span className={s.kvKey}>recipe</span>
            <span>en → {TARGETS.join(", ")}</span>
            <span className={s.kvKey}>content</span>
            <span>{sample.filename}</span>
            <span className={s.kvKey}>flow</span>
            <span>{flowId}</span>
            <span className={s.kvKey}>.kapi/</span>
            <span>
              <span className={`${s.pill} ${done.has("run") ? s.pillDirty : s.pillClean}`}>
                {done.has("run")
                  ? done.has("merge")
                    ? "store committed · files written"
                    : "store committed · not yet merged"
                  : "empty store"}
              </span>
            </span>
          </div>

          <div className={s.cardTitle} style={{ marginTop: "0.8rem" }}>
            Per-locale output
          </div>
          {TARGETS.map((l) => {
            const present = merged.includes(l);
            return (
              <div key={l} className={s.overlayRow}>
                <span style={{ width: "3rem" }}>{l}</span>
                <span
                  className={`${s.pill} ${present ? s.pillClean : s.pillDirty}`}
                  style={{ minWidth: "5.5rem", textAlign: "center" }}
                >
                  {done.has("merge") ? (present ? "✓ written" : "untranslated") : "—"}
                </span>
              </div>
            );
          })}
        </div>
      </div>

      <div className={s.panel}>
        <div className={s.card} style={{ gridColumn: "1 / -1" }}>
          <div className={s.cardTitle}>
            Merged output{" "}
            {done.has("merge") && (
              <select
                value={viewLocale}
                onChange={(e) => setViewLocale(e.target.value as Target)}
                style={{ marginLeft: "0.4rem", fontWeight: 400 }}
              >
                {TARGETS.map((l) => (
                  <option key={l} value={l}>
                    {l}
                  </option>
                ))}
              </select>
            )}
          </div>
          {!done.has("merge") && (
            <div className={s.binaryNote}>
              Run all three steps to materialize the localized files from the project store.
            </div>
          )}
          {done.has("merge") &&
            output != null &&
            (sample.binary ? (
              <div className={s.binaryNote}>{output}</div>
            ) : (
              <pre className={s.output}>{output}</pre>
            ))}
        </div>
      </div>

      <p style={{ fontSize: "0.85rem", opacity: 0.8, marginTop: "0.6rem" }}>
        A <strong>project</strong> keeps config in a committed <code>.kapi</code> recipe and its
        working state in a <code>.kapi/</code> dir — built for teams and servers. A run is{" "}
        <strong>process-only</strong>: it commits to the project store, then <code>merge</code>{" "}
        writes the files. A <strong>.klz workspace</strong> folds the same content + work into one
        portable file — built for ad-hoc, single-file hand-off. Same engine underneath.
      </p>
    </div>
  );
}
