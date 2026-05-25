import React, { useCallback, useEffect, useMemo, useState } from "react";
import { Play } from "lucide-react";
import { tools as toolDataset } from "@neokapi/reference-data";
import type { ComponentSchema, PropertySchema, ReferenceEntry } from "@neokapi/reference-data";
import { SchemaForm } from "@neokapi/ui-primitives";
import { useLabRuntime } from "./useLabRuntime";
import type { LabRuntimeAssets } from "./useLabRuntime";
import FileSource from "./FileSource";
import type { FileSourceValue } from "./FileSource";
import { SAMPLES } from "./samples";
import type { FlowTrace } from "./types";
import BlockResults from "./BlockResults";
import shared from "./styles.module.css";
import styles from "./ToolLab.module.css";

// Browser-safe tools with interesting config that run in the WASM build via the
// deterministic demo providers (no network, no credentials). pseudo-translate
// expands and brackets the source; case-transform rewrites case. Both are pure,
// offline transforms, so a learner can edit their config and see the effect at
// once. The trace node id of the single configured step is always "tool-0".
const ALLOWED_TOOL_IDS = ["pseudo-translate", "case-transform"] as const;

export interface ToolLabProps {
  /** WASM asset URLs from the host; null defers booting (e.g. during SSR). */
  assets: LabRuntimeAssets | null;
  /** Sample selected on first render (default: first sample). */
  defaultSampleId?: string;
  /** Restrict the offered samples. */
  sampleIds?: string[];
}

// Pull the curated tools out of the generated reference dataset, preserving the
// allowlist order. We read schema + doc from the same source the /tools page
// uses, so the form here matches the reference exactly.
function loadTools(): ReferenceEntry[] {
  const byId = new Map(toolDataset.entries.map((e) => [e.id, e]));
  return ALLOWED_TOOL_IDS.map((id) => byId.get(id)).filter(
    (e): e is ReferenceEntry => e !== undefined && e.schema !== undefined,
  );
}

// Seed form values from a schema's property defaults — mirrors the reference
// page's seedDefaults so the form opens on the tool's real defaults.
function seedDefaults(schema: ComponentSchema | undefined): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  for (const [key, prop] of Object.entries(schema?.properties ?? {})) {
    out[key] = prop.default ?? defaultForType(prop.type);
  }
  return out;
}

function defaultForType(type: string): unknown {
  switch (type) {
    case "boolean":
      return false;
    case "array":
      return [];
    case "object":
      return {};
    case "integer":
    case "number":
      return undefined;
    default:
      return "";
  }
}

function isEmpty(val: unknown): boolean {
  if (val === undefined || val === null || val === "") return true;
  if (Array.isArray(val)) return val.length === 0;
  if (typeof val === "object") return Object.keys(val as object).length === 0;
  return false;
}

// Render one config scalar as a YAML value. The recipe only carries the simple
// scalar params these tools expose (string/bool/integer), so quoting strings
// that could be misread as YAML scalars is sufficient.
function scalarYaml(val: unknown): string {
  if (typeof val === "boolean" || typeof val === "number") return String(val);
  const s = String(val);
  if (s === "" || /[:#{}[\],&*!|>'"%@`]/.test(s) || /^\s|\s$/.test(s)) {
    return JSON.stringify(s);
  }
  return s;
}

// Build the recipe YAML for a one-step flow. Only values that differ from their
// schema default (and are non-empty) become config keys, so the recipe stays
// minimal and the demo runs with the tool's natural defaults when untouched.
// `config:` is omitted entirely when nothing was changed.
function buildRecipe(
  toolId: string,
  values: Record<string, unknown>,
  schema: ComponentSchema | undefined,
): string {
  const props = schema?.properties ?? {};
  const changed = Object.entries(values).filter(([key, val]) => {
    const prop = props[key] as PropertySchema | undefined;
    if (!prop || isEmpty(val)) return false;
    if (prop.default !== undefined && JSON.stringify(prop.default) === JSON.stringify(val)) {
      return false;
    }
    return true;
  });

  const lines = [
    "version: v1",
    "name: Lab",
    "defaults:",
    "  source_language: en",
    "flows:",
    "  lab:",
    "    steps:",
    `      - tool: ${toolId}`,
  ];
  if (changed.length > 0) {
    lines.push("        config:");
    for (const [key, val] of changed) {
      lines.push(`          ${key}: ${scalarYaml(val)}`);
    }
  }
  return lines.join("\n") + "\n";
}

// ToolLab lets a learner pick a tool, edit its config in the live shared
// SchemaForm, run it on a file in WASM, and read a per-Block before/after table.
// It writes a one-step `.kapi` recipe carrying the edited config, runs it with
// tracing, and diffs each Block's initial source against the configured step's
// output.
export default function ToolLab({
  assets,
  defaultSampleId,
  sampleIds,
}: ToolLabProps): React.ReactElement {
  const runtime = useLabRuntime(assets);
  const tools = useMemo(() => loadTools(), []);

  const initialSample = SAMPLES.find((s) => s.id === defaultSampleId) ?? SAMPLES[0];
  const [file, setFile] = useState<FileSourceValue>({
    filename: initialSample.filename,
    content: initialSample.content,
    label: initialSample.label,
  });

  const [toolId, setToolId] = useState(tools[0]?.id ?? "");
  const tool = useMemo(() => tools.find((t) => t.id === toolId) ?? tools[0], [tools, toolId]);

  // Form values reset to the selected tool's schema defaults when the tool changes.
  const [values, setValues] = useState<Record<string, unknown>>(() => seedDefaults(tool?.schema));
  useEffect(() => {
    setValues(seedDefaults(tool?.schema));
  }, [tool]);

  const [trace, setTrace] = useState<FlowTrace | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const runTool = useCallback(async () => {
    if (!runtime.ready || !tool) return;
    setBusy(true);
    setError(null);
    const inPath = runtime.writeFile(file.filename, file.bytes ?? file.content);
    const recipe = buildRecipe(tool.id, values, tool.schema);
    const recipePath = runtime.writeFile("toollab.kapi", recipe);
    const outPath = `/project/toollab-out-${file.filename}`;
    const res = await runtime.trace([
      "run",
      "lab",
      "-p",
      recipePath,
      "-i",
      inPath,
      "-o",
      outPath,
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
  }, [runtime.ready, runtime.writeFile, runtime.trace, file, tool, values]);

  // Auto-run once the runtime is ready, and whenever the file, tool, or config
  // changes — runTool is a stable callback keyed on those inputs. A short debounce
  // keeps typing in a text field (e.g. the pseudo prefix) responsive rather than
  // firing a WASM run on every keystroke.
  useEffect(() => {
    if (!runtime.ready) return;
    const handle = setTimeout(() => void runTool(), 250);
    return () => clearTimeout(handle);
  }, [runtime.ready, runTool]);

  const blockCount = trace
    ? Object.values(trace.parts).filter((ss) => ss.initial.type === "Block").length
    : 0;

  if (!tool) {
    return <div className={shared.emptyHint}>No browser-safe tools available.</div>;
  }

  return (
    <div className={shared.explorer}>
      <FileSource value={file} onChange={setFile} sampleIds={sampleIds} />

      <div className={shared.pickerRow}>
        <label className={shared.pickerLabel}>Tool</label>
        <select
          className={shared.select}
          value={toolId}
          onChange={(e) => setToolId(e.target.value)}
        >
          {tools.map((t) => (
            <option key={t.id} value={t.id}>
              {t.displayName}
            </option>
          ))}
        </select>
        <button
          className={shared.runButton}
          onClick={() => void runTool()}
          disabled={!runtime.ready || busy}
        >
          <Play size={14} /> Run
        </button>
      </div>

      {/* The shared SchemaForm only inherits its design tokens inside a
          `.kapi-reference` scope (see the docs site's tailwind.css), so wrap it
          here just as the /tools reference pages do. */}
      <div className={`${styles.configPanel} kapi-reference`}>
        <SchemaForm
          schema={tool.schema!}
          values={values}
          onChange={setValues}
          paramDocs={tool.doc?.parameters}
          hideHeader
          compact
        />
      </div>

      <div className={`${shared.statusBar} ${error ? shared.statusError : ""}`}>
        {runtime.status === "booting" && "Booting kapi (first run downloads ~13 MB)…"}
        {runtime.status === "error" && `Failed to start: ${runtime.error}`}
        {runtime.ready && busy && "Running tool…"}
        {runtime.ready && !busy && error && `Error: ${error}`}
        {runtime.ready && !busy && !error && trace && (
          <span>
            {blockCount} {blockCount === 1 ? "block" : "blocks"}
          </span>
        )}
      </div>

      {trace ? (
        <BlockResults trace={trace} targetLocale="fr" />
      ) : (
        !error && !busy && <div className={shared.emptyHint}>Pick a tool and edit its config.</div>
      )}
    </div>
  );
}
