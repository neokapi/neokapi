import React, { useCallback, useMemo, useState } from "react";
import { Play } from "lucide-react";
import { FlowEditor, graphToSteps, stepsToGraph } from "@neokapi/flow-editor";
import type { FlowSpec, ToolInfo } from "@neokapi/flow-editor";
import { tools as toolReference } from "@neokapi/reference-data";
import type { ReferenceEntry } from "@neokapi/reference-data";
import { useLabRuntime } from "./useLabRuntime";
import type { LabRuntimeAssets } from "./useLabRuntime";
import FileSource from "./FileSource";
import type { FileSourceValue } from "./FileSource";
import FlowTracePlayer from "./FlowTracePlayer";
import { SAMPLES } from "./samples";
import type { FlowTrace } from "@neokapi/ui-primitives/preview";
import { PortalThemeProvider } from "@neokapi/ui-primitives";
import shared from "./styles.module.css";
import styles from "./FlowBuilderRunner.module.css";

// Only tools that run in the browser WASM build are offered in the palette:
// the offline tools (pseudo-translate, word-count, term-check, search-replace,
// redact) plus the demo-provider-backed AI tools (ai-translate, qa-check).
// Listing anything else would let a learner build a flow that cannot run here.
const BROWSER_SAFE_TOOLS = [
  "search-replace",
  "redact",
  "pseudo-translate",
  "word-count",
  "term-check",
  "ai-translate",
  "qa-check",
];

// Transformer tools — they rewrite the source (AD-006). Transformers are
// ordinary ordered steps; the flow editor badges them and its placement pass
// validates their position.
const TRANSFORMER_TOOLS = new Set(["search-replace", "redact"]);

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
// names, descriptions and IO contracts stay truthful to the live engine. We
// keep only the browser-safe ids; categories pass through unchanged (already
// canonical). Exported for unit tests.
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
      isSourceTransform: TRANSFORMER_TOOLS.has(entry.id),
      recoverable: entry.recoverable,
    });
  }
  // Preserve the curated order rather than the dataset's order.
  return BROWSER_SAFE_TOOLS.map((id) => byId.get(id)).filter((t): t is ToolInfo => !!t);
}

// The starter flow a learner opens with. The transformers run first as plain
// ordered steps: search-replace normalises US→British spelling; redact hides
// the brand name and person name. Translation and QA then work the settled
// source. Exported for unit tests.
export const STARTER_STEPS: FlowSpec["steps"] = [
  {
    tool: "search-replace",
    config: { pairs: [{ search: "color", replace: "colour" }], source: true, target: false },
  },
  {
    tool: "redact",
    config: {
      detectors: ["rules"],
      rules: [
        { term: "Acme Corp", category: "org" },
        { term: "Jane Doe", category: "person" },
      ],
    },
  },
  { tool: "ai-translate" },
  { tool: "qa-check" },
];

// Serialize a FlowSpec into a minimal `.kapi` recipe with a single `lab` flow.
// `config:` is emitted only when a step actually carries config, so a
// freshly-added tool stays as a bare `- tool: <id>` line. Transformers are
// ordinary ordered steps — there is no separate block for them.
// Exported for unit tests.
export function buildRecipe(spec: FlowSpec): string {
  const lines: string[] = [
    "version: v1",
    "name: Lab",
    "defaults:",
    "  source_language: en",
    "flows:",
    "  lab:",
  ];

  lines.push("    steps:");
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

// Render a config value as a YAML scalar. The lab's tools take simple scalars;
// strings are quoted so values like locale codes survive unambiguously.
function formatScalar(value: unknown): string {
  if (typeof value === "string") return JSON.stringify(value);
  if (typeof value === "boolean" || typeof value === "number") return String(value);
  return JSON.stringify(value);
}

export interface FlowBuilderRunnerProps {
  assets: LabRuntimeAssets | null;
  defaultSampleId?: string;
  sampleIds?: string[];
}

// FlowBuilderRunner lets a learner assemble a flow in the visual node editor and
// then run it live: on Run it serializes the graph to a `.kapi` recipe, writes
// it into the WASM filesystem, runs `kapi run lab` with tracing on, and hands
// the resulting FlowTrace to <FlowTracePlayer> to step through. Builder and
// runner, side by side — the editor-built flow is the same flow that executes.
export default function FlowBuilderRunner({
  assets,
  defaultSampleId,
  sampleIds,
}: FlowBuilderRunnerProps): React.ReactElement {
  const runtime = useLabRuntime(assets);

  const toolInfos = useMemo(() => buildToolInfos(), []);

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

  const initialSample =
    SAMPLES.find((s) => s.id === defaultSampleId) ??
    SAMPLES.find((s) => s.id === "support-reply") ??
    SAMPLES[0];
  const [file, setFile] = useState<FileSourceValue>({
    filename: initialSample.filename,
    content: initialSample.content,
    label: initialSample.label,
  });

  // The editor is controlled: `flow` is the source of truth and onChange feeds
  // the graph the learner edits (add / remove / reorder tool nodes) back in.
  const [flow, setFlow] = useState<FlowSpec>(() => {
    const graph = stepsToGraph({ steps: STARTER_STEPS });
    return graphToSteps(graph.nodes);
  });

  const [trace, setTrace] = useState<FlowTrace | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

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
      const recipe = buildRecipe({ steps });
      runtime.writeFile("flow.kapi", recipe);
      const inPath = runtime.writeFile(file.filename, file.bytes ?? file.content);
      const res = await runtime.trace([
        "run",
        "lab",
        "-p",
        "/project/flow.kapi",
        "-i",
        inPath,
        "-o",
        `/project/flow-out-${file.filename}`,
        "--target-lang",
        "fr",
      ]);
      if (res.ok && res.trace) {
        setTrace(res.trace);
      } else {
        setError(res.error ?? "the run produced no trace");
        setTrace(null);
      }
      setBusy(false);
    },
    [runtime.ready, runtime.writeFile, runtime.trace, file],
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
        <FileSource value={file} onChange={setFile} sampleIds={sampleIds} />

        {/* Sized container — FlowEditor lays out as `h-full`, so the host must
          give it explicit dimensions or it collapses to zero height. */}
        <div className={styles.editorFrame}>
          <FlowEditor
            flow={flow}
            tools={toolInfos}
            onChange={setFlow}
            onGetSchema={handleGetSchema}
            onGetDoc={handleGetDoc}
            onRun={(spec) => void runFlow(spec)}
            runDisabled={!runtime.ready || busy}
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
          {runtime.ready && busy && "Running your flow…"}
          {runtime.ready && !busy && error && `Error: ${error}`}
        </div>

        {trace ? (
          <FlowTracePlayer trace={trace} showDescription={false} />
        ) : (
          !error && (
            <div className={shared.emptyHint}>
              Edit the flow above, then press Run flow to watch it execute step by step.
            </div>
          )
        )}
      </div>
    </PortalThemeProvider>
  );
}
