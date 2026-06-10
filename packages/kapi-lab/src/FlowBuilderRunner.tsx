import React, { useCallback, useMemo, useState } from "react";
import { Play } from "lucide-react";
import { FlowEditor, graphToSteps, stepsToGraph } from "@neokapi/flow-editor";
import type { FlowSpec, FlowTrace, ToolInfo } from "@neokapi/flow-editor";
import { tools as toolReference } from "@neokapi/reference-data";
import type { ReferenceEntry } from "@neokapi/reference-data";
import { useLabRuntime } from "./useLabRuntime";
import type { LabRuntimeAssets } from "./useLabRuntime";
import FileSource from "./FileSource";
import type { FileSourceValue } from "./FileSource";
import OutputView from "./OutputView";
import { SAMPLES } from "./samples";
import { LAB_SCENARIOS, type LabScenario } from "./labScenarios";
import { ensureLocalNer, localNerLoaded } from "./localNer";
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
        lines.push(`      ${key}: ${formatScalar(value)}`);
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
        lines.push(`          ${key}: ${formatScalar(value)}`);
      }
    }
  }
  return lines.join("\n") + "\n";
}

// Render a config value as a YAML scalar. Strings are quoted so values like
// locale codes survive unambiguously; lists/objects emit as JSON, which YAML
// accepts as flow notation.
function formatScalar(value: unknown): string {
  if (typeof value === "string") return JSON.stringify(value);
  if (typeof value === "boolean" || typeof value === "number") return String(value);
  return JSON.stringify(value);
}

export interface FlowBuilderRunnerProps {
  assets: LabRuntimeAssets | null;
  defaultSampleId?: string;
  sampleIds?: string[];
  /** Scenario preselected in the picker (default: the first). */
  defaultScenarioId?: string;
  /** Restrict the scenario picker (default: all). */
  scenarioIds?: string[];
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

  const sampleFor = useCallback(
    (sampleId?: string): FileSourceValue => {
      const s =
        SAMPLES.find((x) => x.id === (sampleId ?? defaultSampleId)) ??
        SAMPLES.find((x) => x.id === "support-reply") ??
        SAMPLES[0];
      return { filename: s.filename, content: s.content, label: s.label };
    },
    [defaultSampleId],
  );
  const [file, setFile] = useState<FileSourceValue>(() => sampleFor(initialScenario.sampleId));

  // The editor is controlled: `flow` is the source of truth and onChange feeds
  // the graph the learner edits (add / remove / reorder tool nodes) back in.
  const [flow, setFlow] = useState<FlowSpec>(() => {
    const graph = stepsToGraph({ steps: initialScenario.steps });
    return graphToSteps(graph.nodes);
  });

  const [trace, setTrace] = useState<FlowTrace | null>(null);
  const [outPath, setOutPath] = useState<string | null>(null);
  const [outVersion, setOutVersion] = useState(0);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const selectScenario = useCallback(
    (s: LabScenario) => {
      setScenario(s);
      setPresets(s.presets);
      setFile(sampleFor(s.sampleId));
      const graph = stepsToGraph({ steps: s.steps });
      setFlow(graphToSteps(graph.nodes));
      setTrace(null);
      setOutPath(null);
      setError(null);
    },
    [sampleFor],
  );

  // Editing the flow invalidates the loaded run review (the trace no longer
  // matches the designed nodes).
  const handleFlowChange = useCallback((next: FlowSpec) => {
    setFlow(next);
    setTrace(null);
  }, []);

  const runFlow = useCallback(
    async (spec: FlowSpec) => {
      if (!runtime.ready) return;
      const steps = spec.steps.filter((s) => s.tool);
      if (steps.length === 0) {
        setError("add at least one tool to the flow before running");
        setTrace(null);
        return;
      }
      setBusy(true);
      setError(null);
      setNotice(null);

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
          await ensureLocalNer(setNotice);
        } catch (err) {
          setError(`failed to load the on-device NER model: ${String(err)}`);
          setBusy(false);
          return;
        }
      }
      setNotice(null);

      const recipe = buildRecipe({ steps }, presets);
      runtime.writeFile("flow.kapi", recipe);
      const inPath = runtime.writeFile(file.filename, file.bytes ?? file.content);
      const out = `/project/flow-out-${file.filename}`;
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
        setTrace(res.trace as FlowTrace);
        setOutPath(out);
        setOutVersion((v) => v + 1);
      } else {
        setError(res.error ?? "the run produced no trace");
        setTrace(null);
      }
      setBusy(false);
    },
    [runtime.ready, runtime.writeFile, runtime.trace, file, presets],
  );

  const stepCount = flow.steps.filter((s) => s.tool).length;

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
            <ToggleGroup
              type="single"
              variant="outline"
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
        <p className="text-sm leading-relaxed text-muted-foreground">{scenario.description}</p>

        <FileSource value={file} onChange={setFile} sampleIds={sampleIds} />

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

        {/* Sized container — FlowEditor lays out as `h-full`, so the host must
          give it explicit dimensions or it collapses to zero height. */}
        <div className={styles.editorFrame}>
          <FlowEditor
            flow={flow}
            tools={toolInfos}
            onChange={handleFlowChange}
            onGetSchema={handleGetSchema}
            onGetDoc={handleGetDoc}
            onRun={(spec) => void runFlow(spec)}
            runDisabled={!runtime.ready || busy}
            trace={trace ?? undefined}
            onTraceDismiss={() => setTrace(null)}
            projectPresets={presets}
          />
        </div>

        <div className={shared.pickerRow}>
          <span className={shared.pickerLabel}>
            {stepCount} tool{stepCount !== 1 ? "s" : ""} in this flow
          </span>
          <button
            className={shared.runButton}
            onClick={() => void runFlow(flow)}
            disabled={!runtime.ready || busy}
          >
            <Play size={14} /> Run flow
          </button>
        </div>

        <div className={`${shared.statusBar} ${error ? shared.statusError : ""}`}>
          {runtime.status === "booting" && "Booting kapi (first run downloads ~13 MB)…"}
          {runtime.status === "error" && `Failed to start: ${runtime.error}`}
          {runtime.ready && busy && (notice ?? "Running your flow…")}
          {runtime.ready && !busy && error && `Error: ${error}`}
          {runtime.ready &&
            !busy &&
            !error &&
            trace &&
            "Run complete — scrub the transport under the canvas and click a node to inspect what it did."}
        </div>

        {/* The sink side: what the flow wrote, in the shared output viewer. */}
        {outPath && trace && <OutputView runtime={runtime} path={outPath} version={outVersion} />}

        {!trace && !error && (
          <div className={shared.emptyHint}>
            Edit the flow above, then press Run flow — the run plays back on the same nodes you
            designed.
          </div>
        )}
      </div>
    </PortalThemeProvider>
  );
}
