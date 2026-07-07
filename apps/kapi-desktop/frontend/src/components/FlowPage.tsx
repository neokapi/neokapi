import { useCallback, useRef, useReducer } from "react";
import { useQuery } from "@tanstack/react-query";
import type { FlowSpec } from "../types/api";
import { FlowEditor } from "@neokapi/flow-editor";
import type { ToolInfo, ToolDoc, ComponentSchema } from "@neokapi/flow-editor";
import { api } from "../hooks/useApi";
import { qk } from "../lib/queryKeys";
import { useInvalidateOnEvent } from "../hooks/useInvalidateOnEvent";
import { useJobFeed } from "../context/JobFeedContext";

interface FlowPageProps {
  flowName: string;
  flow: FlowSpec;
  onChange: (spec: FlowSpec) => void;
  onRun?: (flowName: string, spec: FlowSpec) => void;
  readOnly?: boolean;
  /** When set, tools are scoped to the project's declared plugins. */
  tabID?: string;
}

export function FlowPage({ flowName, flow, onChange, onRun, readOnly, tabID }: FlowPageProps) {
  const { hasActive } = useJobFeed();
  const schemasRef = useRef<Record<string, ComponentSchema | null>>({});
  const docsRef = useRef<Record<string, ToolDoc | null>>({});
  const fetchingRef = useRef<Set<string>>(new Set());
  const [, forceUpdate] = useReducer((x: number) => x + 1, 0);

  // Tools list as react-query server state. `select` maps the backend's raw
  // ToolInfo to the flow-editor's shape, so the raw cache entry is shared with
  // ToolRunnerPage under the same key. The "registries-changed" event
  // invalidates it (tools may have been added/removed by a plugin change).
  const toolsQuery = useQuery({
    queryKey: tabID ? qk.projectTools(tabID) : qk.tools(),
    queryFn: () => (tabID ? api.listProjectTools(tabID) : api.listTools()),
    select: (result): ToolInfo[] =>
      (result ?? []).map((t) => ({
        name: t.name,
        display_name: t.display_name,
        description: t.description,
        category: t.category,
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
      })),
  });
  const tools: ToolInfo[] = toolsQuery.data ?? [];

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

  const handleGetDoc = useCallback((toolName: string): ToolDoc | null => {
    if (toolName in docsRef.current) {
      return docsRef.current[toolName] ?? null;
    }
    const fetchKey = `doc:${toolName}`;
    if (fetchingRef.current.has(fetchKey)) return null;
    fetchingRef.current.add(fetchKey);
    void api.getStepDoc(toolName).then((result) => {
      fetchingRef.current.delete(fetchKey);
      if (result) {
        docsRef.current[toolName] = {
          displayName: result.filterName,
          overview: result.overview,
          parameters: result.parameters,
          limitations: result.limitations,
          processingNotes: result.processingNotes,
          examples: result.examples,
          wikiUrl: result.wikiUrl,
        };
      } else {
        docsRef.current[toolName] = null;
      }
      forceUpdate();
    });
    return null;
  }, []);

  return (
    <FlowEditor
      key={flowName}
      flow={flow}
      tools={tools}
      onChange={onChange}
      onRun={onRun ? (spec) => onRun(flowName, spec) : undefined}
      runDisabled={hasActive}
      onGetSchema={handleGetSchema}
      onGetDoc={handleGetDoc}
      readOnly={readOnly}
    />
  );
}
