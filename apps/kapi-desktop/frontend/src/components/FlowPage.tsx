import { useCallback, useRef, useReducer } from "react";
import { useQuery } from "@tanstack/react-query";
import type { ComponentSchema } from "@neokapi/ui-primitives";
import { FlowTemplateLibrary, FlowViewTabs } from "@neokapi/flow-editor";
import type { ToolInfo as EditorToolInfo } from "@neokapi/flow-editor";
import type { FlowSpec, ToolInfo } from "../types/api";
import { api } from "../hooks/useApi";
import { qk } from "../lib/queryKeys";
import { useInvalidateOnEvent } from "../hooks/useInvalidateOnEvent";
import { useJobFeed } from "../context/JobFeedContext";
import { useSchemaFormHost } from "../hooks/useSchemaFormHost";

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
}: FlowPageProps) {
  const { hasActive } = useJobFeed();
  const host = useSchemaFormHost();
  const schemasRef = useRef<Record<string, ComponentSchema | null>>({});
  const fetchingRef = useRef<Set<string>>(new Set());
  const [, forceUpdate] = useReducer((x: number) => x + 1, 0);

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

  const handleGetSchema = useCallback((toolName: string): ComponentSchema | null => {
    if (toolName in schemasRef.current) {
      return schemasRef.current[toolName] ?? null;
    }
    if (fetchingRef.current.has(toolName)) return null;
    fetchingRef.current.add(toolName);
    void api.getToolSchema(toolName).then((result) => {
      fetchingRef.current.delete(toolName);
      schemasRef.current[toolName] = (result as ComponentSchema) ?? null;
      forceUpdate();
    });
    return null;
  }, []);

  // Steps (authoring) and Diagram (the read-only canvas). The desktop records
  // no trace of a flow run, so there is no Run view here.
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
    />
  );
}
