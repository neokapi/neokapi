import { useMemo, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import type { ComponentSchema } from "@neokapi/ui-primitives";
import { FlowTemplateLibrary, FlowViewTabs, useToolSchemas } from "@neokapi/flow-editor";
import type { FlowTrace, ToolInfo as EditorToolInfo } from "@neokapi/flow-editor";
import type { FlowSpec, RunTraces, ToolInfo } from "../types/api";
import { api } from "../hooks/useApi";
import { qk } from "../lib/queryKeys";
import { useInvalidateOnEvent } from "../hooks/useInvalidateOnEvent";
import { useWailsEvent } from "../hooks/useWailsEvent";
import { useJobFeed } from "../context/JobFeedContext";
import { useSchemaFormHost } from "../hooks/useSchemaFormHost";
import { sameSteps, traceFileKey } from "../lib/runTraces";
import { RunTracePicker } from "./RunTracePicker";

/** A run pre-loaded for Storybook, in place of ListRunTraces / GetLastTrace. */
export interface PreloadedRun {
  run: RunTraces;
  /** Each retained file's trace, keyed by input file path. */
  traces: Record<string, FlowTrace>;
}

/**
 * Run events after which the retained traces may differ from what is cached:
 * a run starting (the backend keeps only the last run), a file's trace being
 * kept, and the run ending either way.
 */
const RUN_TRACE_EVENTS = new Set(["state", "trace", "complete", "error"]);

interface FlowPageProps {
  flowName: string;
  flow: FlowSpec;
  onChange: (spec: FlowSpec) => void;
  onRun?: (flowName: string, spec: FlowSpec) => void;
  readOnly?: boolean;
  /** When set, tools are scoped to the project's declared plugins. */
  tabID?: string;
  /** True when this is the project's default flow (project mode only). */
  isDefault?: boolean;
  /** Set/clear the project's default flow (project mode only). */
  onToggleDefault?: (next: boolean) => void;
  /** Rename the flow (project mode only). */
  onRename?: (next: string) => void;
  /** A pre-loaded run (Storybook), so the Run view shows without a backend. */
  preloadedRun?: PreloadedRun;
}

// The backend's raw ToolInfo mapped to the flow editor's shape: the diagram
// reads the IO contract and the transformer flags off it, the step editor the
// name, description and whether there are options. A stable function so the
// query's `select` keeps its result reference between renders.
function selectEditorTools(result: ToolInfo[] | null | undefined): EditorToolInfo[] {
  return (result ?? []).map((t) => ({
    name: t.name,
    display_name: t.display_name,
    description: t.description,
    category: t.category,
    source: t.source,
    has_schema: t.has_schema,
    tags: t.tags,
    requires: t.requires,
    cardinality: t.cardinality,
    default_locale: t.default_locale,
    consumes: t.consumes,
    produces: t.produces,
    side_effects: t.side_effects,
    isSourceTransform: t.is_source_transform,
    recoverable: t.recoverable,
  }));
}

/** A tool's option schema over the Wails binding; null outside Wails or without one. */
const fetchToolSchema = (toolName: string) =>
  api.getToolSchema(toolName).then((result) => (result as ComponentSchema | null) ?? null);

export function FlowPage({
  flowName,
  flow,
  onChange,
  onRun,
  readOnly,
  tabID,
  isDefault,
  onToggleDefault,
  onRename,
  preloadedRun,
}: FlowPageProps) {
  const { hasActive } = useJobFeed();
  const qc = useQueryClient();
  const host = useSchemaFormHost();
  const handleGetSchema = useToolSchemas(fetchToolSchema);

  // Tools list as react-query server state, shared with ToolRunnerPage under the
  // same key (the raw entry is cached; `select` shapes it for the editor). The
  // "registries-changed" event invalidates it (a plugin change may have added
  // or removed tools).
  const toolsQuery = useQuery({
    queryKey: tabID ? qk.projectTools(tabID) : qk.tools(),
    queryFn: () => (tabID ? api.listProjectTools(tabID) : api.listTools()),
    select: selectEditorTools,
  });
  const tools: EditorToolInfo[] = toolsQuery.data ?? [];

  useInvalidateOnEvent("registries-changed", [tabID ? qk.projectTools(tabID) : qk.tools()]);

  // The last run's retained traces. The backend keeps them from the most
  // recent run of whatever flow, so they belong here only while that run was
  // of this flow with these steps; an edit withholds them until it is undone.
  // Any run event that can change the set invalidates the family, and the
  // trace query below sits under the same key prefix.
  const runQuery = useQuery({
    queryKey: qk.runTraces(),
    queryFn: () => (preloadedRun ? preloadedRun.run : api.listRunTraces()),
  });
  useWailsEvent("flow:event", (data) => {
    const ev = data as { type?: unknown } | null;
    if (ev && typeof ev.type === "string" && RUN_TRACE_EVENTS.has(ev.type)) {
      void qc.invalidateQueries({ queryKey: qk.runTraces() });
    }
  });
  const run = runQuery.data ?? null;
  const runFiles = useMemo(
    () => (run && run.flow_name === flowName && sameSteps(run.steps, flow.steps) ? run.files : []),
    [run, flowName, flow.steps],
  );

  // The file replaying: the reader's pick while it is still retained, else
  // the file that completed last. A dismissal hides the run until the set
  // changes (a new file kept, or a new run).
  const [pickedKey, setPickedKey] = useState<string | null>(null);
  const [dismissedKey, setDismissedKey] = useState<string | null>(null);
  const newest = runFiles.length > 0 ? runFiles[runFiles.length - 1] : null;
  const selected = runFiles.find((f) => traceFileKey(f) === pickedKey) ?? newest;
  const runKey = newest ? `${runFiles.length}:${traceFileKey(newest)}` : null;
  const dismissed = runKey !== null && dismissedKey === runKey;

  const traceQuery = useQuery({
    queryKey: qk.runTrace(selected?.file_path ?? "", selected?.locale ?? ""),
    queryFn: () => {
      if (!selected) return null;
      if (preloadedRun) return preloadedRun.traces[selected.file_path] ?? null;
      return api.getLastTrace(selected.file_path, selected.locale ?? "");
    },
    enabled: selected !== null && !dismissed,
  });
  const trace = selected && !dismissed ? (traceQuery.data ?? undefined) : undefined;

  // Steps (authoring), Diagram (the read-only canvas), and Run once a trace
  // of this flow is retained: the canvas replays it, with the file picker in
  // the view bar when the run covered several files.
  return (
    <FlowViewTabs
      key={flowName}
      flowName={flowName}
      flow={flow}
      tools={tools}
      onChange={onChange}
      onGetSchema={handleGetSchema}
      host={host}
      onRun={onRun ? () => onRun(flowName, flow) : undefined}
      runDisabled={hasActive}
      readOnly={readOnly}
      isDefault={isDefault}
      onToggleDefault={onToggleDefault}
      onRename={onRename}
      templateLibrary={<FlowTemplateLibrary onSelect={onChange} />}
      trace={trace}
      onTraceDismiss={() => setDismissedKey(runKey)}
      runControls={
        trace && selected && run ? (
          <RunTracePicker
            files={runFiles}
            selected={selected}
            onSelect={(f) => setPickedKey(traceFileKey(f))}
            maxParts={run.max_parts}
          />
        ) : undefined
      }
    />
  );
}
