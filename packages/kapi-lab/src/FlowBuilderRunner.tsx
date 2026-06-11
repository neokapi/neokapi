import React, { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { FlowEditor, graphToSteps, stepsToGraph } from "@neokapi/flow-editor";
import type { FlowFocusRequest, FlowSpec, FlowTrace, ToolInfo } from "@neokapi/flow-editor";
import { tools as toolReference } from "@neokapi/reference-data";
import type { ReferenceEntry } from "@neokapi/reference-data";
import { useLabRuntime } from "./useLabRuntime";
import type { LabRuntimeAssets } from "./useLabRuntime";
import type { FileSourceValue } from "./FileSource";
import FileSelectorField from "./FileSelectorField";
import ActiveFileSwitcher from "./ActiveFileSwitcher";
import { useFileLibrary, resolveSelection } from "./fileLibrary";
import type { FileSelection } from "./fileLibrary";
import { fileType } from "@neokapi/ui-primitives/preview";
import { SourceContentPanel, SinkOutputPanel } from "./EndpointPanels";
import { SAMPLES } from "./samples";
import { LAB_SCENARIOS, type LabScenario, type LessonStep } from "./labScenarios";
import WalkthroughCard from "./WalkthroughCard";
import ScriptStepPanel from "./ScriptStepPanel";
import RecipeView from "./RecipeView";
import { TraceImportControl, specFromTrace } from "./traceImport";
import type { RecordedTraceInfo } from "./traceImport";
import { ensureLocalNer, localNerLoaded, type LocalNerProgress } from "./localNer";
import { PortalThemeProvider, ToggleGroup, ToggleGroupItem } from "@neokapi/ui-primitives";
import shared from "./styles.module.css";
import styles from "./FlowBuilderRunner.module.css";

// Only tools that run in the browser WASM build are offered in the palette:
// the offline tools (segmentation, pseudo-translate, word-count, term-check,
// search-replace, redact, unredact) plus the demo-provider-backed AI tools
// (ai-translate, qa-check). Listing anything else would let a learner build a
// flow that cannot run here.
const BROWSER_SAFE_TOOLS = [
  "search-replace",
  "script",
  "redact",
  "unredact",
  "segmentation",
  "pseudo-translate",
  "word-count",
  "term-check",
  "ai-entity-extract",
  "ai-translate",
  "qa-check",
];

// The reference dataset encodes IO ports as "type@side" tokens (a consumed port
// carries a trailing "?" when optional); the flow editor's ToolInfo wants typed
// IOPort objects. parsePorts bridges the two so the lab's nodes render the same
// typed IO chips as the desktop/web editors.
type IOPort = NonNullable<ToolInfo["consumes"]>[number];

function parsePorts(tokens: string[] | undefined): IOPort[] | undefined {
  if (!tokens?.length) return undefined;
  return tokens.map((tok) => {
    const optional = tok.endsWith("?");
    const body = optional ? tok.slice(0, -1) : tok;
    const at = body.lastIndexOf("@");
    const type = at >= 0 ? body.slice(0, at) : body;
    const side = (at >= 0 ? body.slice(at + 1) : "source") as IOPort["side"];
    return { type, side, ...(optional ? { optional: true } : {}) };
  });
}

// Build the palette's ToolInfo[] from the generated reference dataset so the
// names, descriptions, IO contracts and transformer flags stay truthful to the
// live engine. We keep only the browser-safe ids; categories pass through
// unchanged (already canonical). Exported for unit tests.
export function buildToolInfos(): ToolInfo[] {
  const byId = new Map<string, ToolInfo>();
  for (const entry of toolReference.entries) {
    if (!BROWSER_SAFE_TOOLS.includes(entry.id) || byId.has(entry.id)) continue;
    byId.set(entry.id, {
      name: entry.id,
      display_name: entry.displayName,
      description: entry.description ?? "",
      // The reference dataset's categories are already the canonical editor
      // vocabulary (translation / quality / analysis / text-processing / …), so
      // pass them through; the palette groups and colours key off the same set.
      category: entry.category || "pipeline",
      has_schema: !!entry.schema,
      consumes: parsePorts(entry.consumes),
      produces: parsePorts(entry.produces),
      side_effects: entry.sideEffects,
      tags: entry.tags,
      requires: entry.requires,
      cardinality: entry.cardinality as ToolInfo["cardinality"],
      isSourceTransform: entry.isSourceTransform,
      recoverable: entry.recoverable,
    });
  }
  // Preserve the curated order rather than the dataset's order.
  return BROWSER_SAFE_TOOLS.map((id) => byId.get(id)).filter((t): t is ToolInfo => !!t);
}

// Serialize a FlowSpec (plus optional project-level tool presets) into a
// minimal `.kapi` recipe with a single `lab` flow. The presets land under
// `defaults.tools` — the engine merges them under each step's own config (the
// step wins per key), exactly as in a real project. `config:` is emitted only
// when a step actually carries one. Exported for unit tests.
export function buildRecipe(
  spec: FlowSpec,
  presets?: Record<string, Record<string, unknown>>,
): string {
  const lines: string[] = ["version: v1", "name: Lab", "defaults:", "  source_language: en"];

  if (presets && Object.keys(presets).length > 0) {
    lines.push("  tools:");
    for (const [tool, config] of Object.entries(presets)) {
      lines.push(`    ${tool}:`);
      for (const [key, value] of Object.entries(config)) {
        lines.push(...emitEntry(key, value, 6));
      }
    }
  }

  lines.push("flows:", "  lab:", "    steps:");
  for (const step of spec.steps) {
    if (!step.tool) continue; // skip the empty wrapper of a parallel group
    lines.push(`      - tool: ${step.tool}`);
    const config = step.config;
    if (config && Object.keys(config).length > 0) {
      lines.push("        config:");
      for (const [key, value] of Object.entries(config)) {
        lines.push(...emitEntry(key, value, 10));
      }
    }
  }
  return lines.join("\n") + "\n";
}

// Emit one `key: value` config entry at the given indent. Multi-line strings
// (a script step's code) become a YAML literal block (`key: |`) so the recipe
// reads — and round-trips — as real lines rather than one escaped string.
function emitEntry(key: string, value: unknown, indent: number): string[] {
  const pad = " ".repeat(indent);
  if (typeof value === "string" && value.includes("\n")) {
    const body = value.split("\n");
    // The literal block's default (clip) chomping restores the single
    // trailing newline a code blob conventionally ends with.
    if (body.at(-1) === "") body.pop();
    return [`${pad}${key}: |`, ...body.map((l) => (l ? `${pad}  ${l}` : ""))];
  }
  return [`${pad}${key}: ${formatScalar(value)}`];
}

// Render a config value as a YAML scalar. Strings are quoted so values like
// locale codes survive unambiguously; lists/objects emit as JSON, which YAML
// accepts as flow notation.
function formatScalar(value: unknown): string {
  if (typeof value === "string") return JSON.stringify(value);
  if (typeof value === "boolean" || typeof value === "number") return String(value);
  return JSON.stringify(value);
}

/** A slim labelled progress bar for the engine / model downloads. */
function DownloadProgress({
  label,
  loaded,
  total,
}: {
  label: string;
  loaded?: number;
  total?: number | null;
}) {
  const mb = (n: number) => `${(n / 1024 / 1024).toFixed(1)} MB`;
  const pct = loaded !== undefined && total ? Math.min(100, (loaded / total) * 100) : null;
  return (
    <div className="flex flex-col gap-1 py-1">
      <div className="flex items-baseline justify-between gap-2 text-xs text-muted-foreground">
        <span>{label}</span>
        {loaded !== undefined && (
          <span className="font-mono text-[10px]">
            {mb(loaded)}
            {total ? ` / ${mb(total)}` : ""}
          </span>
        )}
      </div>
      <div className="h-1.5 w-full overflow-hidden rounded-full bg-secondary">
        {pct !== null ? (
          <div
            className="h-full rounded-full bg-primary transition-[width] duration-200"
            style={{ width: `${pct}%` }}
          />
        ) : (
          <div className="h-full w-1/3 animate-pulse rounded-full bg-primary" />
        )}
      </div>
    </div>
  );
}

export interface FlowBuilderRunnerProps {
  assets: LabRuntimeAssets | null;
  defaultSampleId?: string;
  sampleIds?: string[];
  /** Scenario preselected in the picker (default: the first). */
  defaultScenarioId?: string;
  /** Restrict the scenario picker (default: all). */
  scenarioIds?: string[];
  /**
   * Built-in recorded traces (`kapi run --trace` output) offered for replay —
   * native runs the wasm engine can't reproduce live (parallel workers, the
   * Java bridge's gRPC boundary). URLs must already be base-resolved.
   */
  recordedTraces?: RecordedTraceInfo[];
}

// FlowBuilderRunner is the lab's flow workspace: a learner picks a teaching
// scenario (or builds their own flow), and the SAME designed flow runs live —
// on Run it serializes the graph (and the project's tool presets) to a `.kapi`
// recipe, writes it into the WASM filesystem, runs `kapi run lab` with tracing
// on, and loads the trace back INTO the editor: the transport replays the
// events on the designed nodes, and clicking a node opens the run inspector
// showing what that step attached to each block. There is no separate run
// view.
export default function FlowBuilderRunner({
  assets,
  defaultSampleId,
  sampleIds,
  defaultScenarioId,
  scenarioIds,
  recordedTraces,
}: FlowBuilderRunnerProps): React.ReactElement {
  const runtime = useLabRuntime(assets);

  const toolInfos = useMemo(() => buildToolInfos(), []);

  const scenarios = useMemo(
    () => (scenarioIds ? LAB_SCENARIOS.filter((s) => scenarioIds.includes(s.id)) : LAB_SCENARIOS),
    [scenarioIds],
  );

  // Resolve a clicked tool's schema and docs for the editor's configuration
  // panel. Without these callbacks the panel sits on "Loading configuration…"
  // for any tool that declares a schema.
  const toolByName = useMemo(() => {
    const m = new Map<string, ReferenceEntry>();
    for (const e of toolReference.entries) if (!m.has(e.id)) m.set(e.id, e);
    return m;
  }, []);
  const handleGetSchema = useCallback(
    (toolName: string) => toolByName.get(toolName)?.schema ?? null,
    [toolByName],
  );
  const handleGetDoc = useCallback(
    (toolName: string) => toolByName.get(toolName)?.doc ?? null,
    [toolByName],
  );

  const initialScenario =
    scenarios.find((s) => s.id === defaultScenarioId) ?? scenarios[0] ?? LAB_SCENARIOS[0];
  const [scenario, setScenario] = useState<LabScenario>(initialScenario);
  const [presets, setPresets] = useState<Record<string, Record<string, unknown>> | undefined>(
    initialScenario.presets,
  );

  // The working set: the shared file library (samples + uploads) and a
  // selection over it (single, multi, or a glob like *.json). Every selected
  // file runs through the flow; the active file picks which run is reviewed.
  const library = useFileLibrary({ sampleIds });
  const selectionFor = useCallback(
    (sampleId?: string): FileSelection => {
      const s =
        SAMPLES.find((x) => x.id === (sampleId ?? defaultSampleId)) ??
        SAMPLES.find((x) => x.id === "support-reply") ??
        SAMPLES[0];
      return { mode: "multi", paths: [s.filename] };
    },
    [defaultSampleId],
  );
  const [selection, setSelection] = useState<FileSelection>(() =>
    selectionFor(initialScenario.sampleId),
  );
  const [activePath, setActivePath] = useState<string | null>(null);

  const selected = useMemo(() => resolveSelection(selection, library), [selection, library]);
  const activeFile = useMemo(
    () => selected.find((f) => f.path === activePath) ?? selected[0],
    [selected, activePath],
  );

  // The editor is controlled: `flow` is the source of truth and onChange feeds
  // the graph the learner edits (add / remove / reorder tool nodes) back in.
  const [flow, setFlow] = useState<FlowSpec>(() => {
    const graph = stepsToGraph({ steps: initialScenario.steps });
    return graphToSteps(graph.nodes);
  });

  // Per-file run results from the last Run (trace + written output, keyed by
  // library path). The ACTIVE file's run feeds the editor's run review and the
  // sink panel; the switcher flips between files.
  const [runs, setRuns] = useState<Record<string, { trace: FlowTrace; outPath: string }>>({});
  const [outVersion, setOutVersion] = useState(0);
  const [error, setError] = useState<string | null>(null);
  const [nerProgress, setNerProgress] = useState<LocalNerProgress | null>(null);
  const [busy, setBusy] = useState(false);

  const activeRun = activeFile ? runs[activeFile.path] : undefined;
  const trace = activeRun?.trace ?? null;
  const outPath = activeRun?.outPath ?? null;

  // Imported recorded trace (replay mode): the trace's tool nodes reconstruct
  // a read-only flow on the same canvas, and the transport plays the recorded
  // events back — showing what live wasm runs can't (parallel workers, the
  // Java bridge boundary). Dismissing the run returns to the editable flow.
  const [imported, setImported] = useState<{
    trace: FlowTrace;
    spec: FlowSpec;
    label: string;
  } | null>(null);
  const handleTraceImport = useCallback((t: FlowTrace, label: string) => {
    setImported({ trace: t, spec: specFromTrace(t), label });
    setError(null);
  }, []);

  // Guided walkthrough state: the active step of the scenario's lesson, plus
  // the focus request fed to the editor (one application per nonce). Stepping
  // applies the step's focus — selecting the node it talks about and opening
  // the matching panel — so the lesson points INTO the workspace.
  const [walkIndex, setWalkIndex] = useState(0);
  const [focusRequest, setFocusRequest] = useState<FlowFocusRequest | undefined>(undefined);
  const focusNonce = useRef(0);

  // The project lens: the live recipe the canvas serializes to, shown below it.
  const [recipeOpen, setRecipeOpen] = useState(false);

  const applyStepFocus = useCallback((step: LessonStep | undefined) => {
    if (!step) return;
    if (step.recipe !== undefined) setRecipeOpen(step.recipe);
    if (step.select === undefined) return;
    focusNonce.current += 1;
    setFocusRequest({ nonce: focusNonce.current, select: step.select, mode: step.mode });
  }, []);

  const goToStep = useCallback(
    (steps: LessonStep[] | undefined, index: number) => {
      setWalkIndex(index);
      applyStepFocus(steps?.[index]);
    },
    [applyStepFocus],
  );

  // Apply the initial scenario's first lesson focus once the engine is up
  // (panels show live engine output, so focusing before boot teaches nothing).
  const appliedInitialFocus = useRef(false);
  useEffect(() => {
    if (!runtime.ready || appliedInitialFocus.current) return;
    appliedInitialFocus.current = true;
    applyStepFocus(scenario.walkthrough?.[walkIndex]);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [runtime.ready]);

  const selectScenario = useCallback(
    (s: LabScenario) => {
      setScenario(s);
      setPresets(s.presets);
      setSelection(selectionFor(s.sampleId));
      setActivePath(null);
      const graph = stepsToGraph({ steps: s.steps });
      setFlow(graphToSteps(graph.nodes));
      setRuns({});
      setError(null);
      // Restart the lesson; scenarios without one clear any lingering focus.
      setWalkIndex(0);
      if (s.walkthrough) applyStepFocus(s.walkthrough[0]);
      else {
        focusNonce.current += 1;
        setFocusRequest({ nonce: focusNonce.current, select: null });
      }
    },
    [selectionFor, applyStepFocus],
  );

  // Editing the flow invalidates the loaded run review (the trace no longer
  // matches the designed nodes).
  const handleFlowChange = useCallback((next: FlowSpec) => {
    setFlow(next);
    setRuns({});
  }, []);

  // The script tool gets the full code-editor panel instead of the schema
  // form's textarea — the step IS the script (the lab's scripting lesson).
  const renderStepConfigPanel = useCallback(
    (ctx: {
      toolName: string;
      config: Record<string, unknown>;
      onConfigChange: (config: Record<string, unknown>) => void;
      onClose: () => void;
      onRemove?: () => void;
    }) =>
      ctx.toolName === "script" ? (
        <ScriptStepPanel
          config={ctx.config}
          onConfigChange={ctx.onConfigChange}
          onClose={ctx.onClose}
          onRemove={ctx.onRemove}
        />
      ) : null,
    [],
  );

  // Endpoint inspectors: the Source pill opens the content-model tree the
  // reader produces from the ACTIVE input (the anatomy lesson, in place); the
  // Sink pill opens that file's written output with its Native bytes diffed
  // against the input (the round-trip lesson, in place).
  const activeAsSource = useMemo<FileSourceValue | null>(() => {
    if (!activeFile) return null;
    return {
      filename: activeFile.name,
      label: activeFile.name,
      content: "",
      bytes: activeFile.bytes,
    };
  }, [activeFile]);
  const activeBaseline = useMemo(() => {
    if (!activeFile || fileType(activeFile.name).binary) return null;
    return new TextDecoder().decode(activeFile.bytes);
  }, [activeFile]);
  const renderEndpointPanel = useCallback(
    (role: "source" | "sink") =>
      role === "source" ? (
        activeAsSource ? (
          <SourceContentPanel runtime={runtime} file={activeAsSource} />
        ) : (
          <div className="py-4 text-center text-[11px] italic text-muted-foreground">
            Pick an input file first.
          </div>
        )
      ) : (
        <SinkOutputPanel
          runtime={runtime}
          outPath={outPath}
          version={outVersion}
          baseline={activeBaseline}
        />
      ),
    [runtime, activeAsSource, activeBaseline, outPath, outVersion],
  );

  const runFlow = useCallback(
    async (spec: FlowSpec) => {
      if (!runtime.ready) return;
      const steps = spec.steps.filter((s) => s.tool);
      if (steps.length === 0) {
        setError("add at least one tool to the flow before running");
        setRuns({});
        return;
      }
      if (selected.length === 0) {
        setError("pick at least one input file before running");
        setRuns({});
        return;
      }
      setBusy(true);
      setError(null);
      setNerProgress(null);

      // On-device NER: a step running ai-entity-extract with engine "ner"
      // needs the GLiNER model loaded in the page (the wasm engine bridges to
      // it). Load lazily on first use — everything stays in the browser.
      const needsLocalNer = steps.some(
        (s) =>
          (s.tool === "ai-entity-extract" &&
            (s.config as { engine?: string } | undefined)?.engine === "ner") ||
          s.parallel?.some(
            (b) =>
              b.tool === "ai-entity-extract" &&
              (b.config as { engine?: string } | undefined)?.engine === "ner",
          ),
      );
      if (needsLocalNer && !localNerLoaded()) {
        try {
          await ensureLocalNer(setNerProgress);
        } catch (err) {
          setError(`failed to load the on-device NER model: ${String(err)}`);
          setNerProgress(null);
          setBusy(false);
          return;
        }
      }
      setNerProgress(null);

      const recipe = buildRecipe({ steps }, presets);
      runtime.writeFile("flow.kapi", recipe);

      // Every selected file runs through the same designed flow (sequentially —
      // the wasm engine serializes runs anyway); the active file's run feeds
      // the review, and the switcher flips between results.
      const next: Record<string, { trace: FlowTrace; outPath: string }> = {};
      for (const f of selected) {
        const inPath = runtime.writeFile(f.name, f.bytes);
        const out = `/project/flow-out-${f.name}`;
        const res = await runtime.trace([
          "run",
          "lab",
          "-p",
          "/project/flow.kapi",
          "-i",
          inPath,
          "-o",
          out,
          "--target-lang",
          "fr",
        ]);
        if (res.ok && res.trace) {
          next[f.path] = { trace: res.trace as FlowTrace, outPath: out };
        } else {
          setError(`${f.name}: ${res.error ?? "the run produced no trace"}`);
          setRuns(next);
          setBusy(false);
          return;
        }
      }
      setRuns(next);
      setOutVersion((v) => v + 1);
      setBusy(false);
    },
    [runtime.ready, runtime.writeFile, runtime.trace, selected, presets],
  );

  // A walkthrough step whose action is Run auto-advances when the run lands,
  // so the next step can immediately point at what the run produced.
  useEffect(() => {
    if (!trace) return;
    const steps = scenario.walkthrough;
    if (steps?.[walkIndex]?.run && walkIndex < steps.length - 1) {
      goToStep(steps, walkIndex + 1);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [trace]);

  return (
    // `.kapi-reference` supplies the ui-primitives theme variables (--background,
    // --border, …) the flow-editor's Tailwind classes resolve against; the docs
    // site scopes those vars to that class so they don't leak into Infima docs.
    // PortalThemeProvider carries that same class onto popover content (the
    // source/sink dropdowns, tool-config selects) which Radix portals to
    // document.body — outside this wrapper — so their theme vars still resolve.
    <PortalThemeProvider className="kapi-reference">
      <div className={`${shared.explorer} kapi-reference`}>
        {/* Scenario picker: each is a complete teaching setup (flow + sample +
            project presets). */}
        {scenarios.length > 1 && (
          <div className="flex flex-wrap items-center gap-3">
            <span className="text-xs font-semibold text-muted-foreground">Scenario</span>
            {/* spacing>0 keeps each chip independently bordered/rounded so the
                row wraps cleanly on narrow screens (joined segments would lose
                their edge borders at the wrap points). */}
            <ToggleGroup
              type="single"
              variant="outline"
              spacing={1}
              className="flex-wrap"
              value={scenario.id}
              onValueChange={(v) => {
                const next = scenarios.find((s) => s.id === v);
                if (next) selectScenario(next);
              }}
            >
              {scenarios.map((s) => (
                <ToggleGroupItem key={s.id} value={s.id} className="px-3 text-xs">
                  {s.label}
                </ToggleGroupItem>
              ))}
            </ToggleGroup>
          </div>
        )}
        {/* The scenario's lesson: a walkthrough card that drives the workspace
            (each step focuses the node/panel it talks about). On sm+ it lives
            INSIDE the canvas as a callout (FlowEditor lessonPanel) so the
            editor keeps its vertical real estate; on phones it stacks here.
            Free-play scenarios show their static description instead. */}
        {scenario.walkthrough && !imported ? (
          <div className="sm:hidden">
            <WalkthroughCard
              steps={scenario.walkthrough}
              index={walkIndex}
              onIndexChange={(i) => goToStep(scenario.walkthrough, i)}
              onRun={() => void runFlow(flow)}
              runDisabled={!runtime.ready || busy}
            />
          </div>
        ) : (
          <p className="text-sm leading-relaxed text-muted-foreground">{scenario.description}</p>
        )}

        {/* The working set: one file, several, or a glob — every selected file
            runs through the flow; the switcher picks whose run is reviewed. */}
        <FileSelectorField
          label="Files"
          library={library}
          selection={selection}
          onSelectionChange={(sel) => {
            setSelection(sel);
            setRuns({});
          }}
          sampleIds={sampleIds}
          multiple
        />
        <ActiveFileSwitcher
          files={selected}
          activePath={activeFile?.path}
          onChange={setActivePath}
        />

        {/* Project presets (defaults.tools in the generated recipe): the
            project scope a step inherits; select a preset-backed node to see
            the inherited values in its config panel. */}
        {presets && Object.keys(presets).length > 0 && (
          <p className="text-xs leading-relaxed text-muted-foreground">
            This project pins presets for{" "}
            {Object.keys(presets)
              .map((tool) => `“${tool}”`)
              .join(", ")}{" "}
            under <code>defaults.tools</code> — every flow in the project inherits them, and a
            step's own config overrides per key.
          </p>
        )}

        {/* Workspace lenses: replay a recorded trace (native runs the wasm
            engine can't reproduce) and the project lens (the recipe the canvas
            serializes to). */}
        <div className="flex flex-wrap items-center justify-between gap-2">
          {imported ? (
            <span className="text-[11px] text-muted-foreground">
              Replaying <strong>{imported.label}</strong> (recorded native run, read-only) —{" "}
              <button
                type="button"
                className="font-medium underline underline-offset-2 hover:text-foreground"
                onClick={() => setImported(null)}
              >
                back to your flow
              </button>
            </span>
          ) : (
            <TraceImportControl
              traces={recordedTraces}
              onLoad={handleTraceImport}
              onError={setError}
            />
          )}
          <button
            type="button"
            className="text-[11px] font-medium text-muted-foreground underline-offset-2 hover:text-foreground hover:underline"
            onClick={() => setRecipeOpen((v) => !v)}
          >
            {recipeOpen ? "Hide recipe" : "View recipe"}
          </button>
        </div>

        {/* Sized container — FlowEditor lays out as `h-full`, so the host must
          give it explicit dimensions or it collapses to zero height. */}
        <div className={styles.editorFrame}>
          <FlowEditor
            flow={imported?.spec ?? flow}
            tools={toolInfos}
            onChange={handleFlowChange}
            onGetSchema={handleGetSchema}
            onGetDoc={handleGetDoc}
            onRun={(spec) => void runFlow(spec)}
            runDisabled={!runtime.ready || busy || !!imported}
            readOnly={!!imported}
            trace={imported?.trace ?? trace ?? undefined}
            onTraceDismiss={() => (imported ? setImported(null) : setRuns({}))}
            projectPresets={imported ? undefined : presets}
            renderEndpointPanel={imported ? undefined : renderEndpointPanel}
            focusRequest={imported ? undefined : focusRequest}
            renderStepConfigPanel={renderStepConfigPanel}
            lessonPanel={
              scenario.walkthrough && !imported ? (
                <WalkthroughCard
                  steps={scenario.walkthrough}
                  index={walkIndex}
                  onIndexChange={(i) => goToStep(scenario.walkthrough, i)}
                  onRun={() => void runFlow(flow)}
                  runDisabled={!runtime.ready || busy}
                />
              ) : undefined
            }
          />
        </div>

        {/* The project lens: the live recipe the canvas serializes to. */}
        {recipeOpen && (
          <RecipeView
            recipe={buildRecipe(
              { steps: (imported?.spec ?? flow).steps.filter((s) => s.tool) },
              imported ? undefined : presets,
            )}
          />
        )}

        {runtime.status === "booting" && (
          <DownloadProgress
            label="Downloading the kapi engine (one-time, cached)…"
            loaded={runtime.bootProgress?.loaded}
            total={runtime.bootProgress?.total}
          />
        )}
        {busy && nerProgress && (
          <DownloadProgress
            label={nerProgress.message}
            loaded={nerProgress.loaded}
            total={nerProgress.total}
          />
        )}
        <div className={`${shared.statusBar} ${error ? shared.statusError : ""}`}>
          {runtime.status === "error" && `Failed to start: ${runtime.error}`}
          {runtime.ready && busy && !nerProgress && "Running your flow…"}
          {runtime.ready && !busy && error && `Error: ${error}`}
          {runtime.ready &&
            !busy &&
            !error &&
            trace &&
            `Run complete${Object.keys(runs).length > 1 ? ` (${Object.keys(runs).length} files)` : ""} — scrub the transport, click a node to inspect what it did, or Inspect the Sink for what was written.`}
        </div>

        {!trace && !error && (
          <div className={shared.emptyHint}>
            Edit the flow above, then press Run in the flow's toolbar — the run plays back on the
            same nodes you designed.
          </div>
        )}
      </div>
    </PortalThemeProvider>
  );
}
