import { useCallback, useRef, useReducer } from "react";
import { useQuery } from "@tanstack/react-query";
import { LinearFlowEditor } from "@neokapi/ui-primitives";
import type { ComponentSchema } from "@neokapi/ui-primitives";
import { FlowTemplateLibrary } from "@neokapi/flow-editor";
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
  // same key. The "registries-changed" event invalidates it (a plugin change
  // may have added or removed tools).
  const toolsQuery = useQuery({
    queryKey: tabID ? qk.projectTools(tabID) : qk.tools(),
    queryFn: () => (tabID ? api.listProjectTools(tabID) : api.listTools()),
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

  return (
    <LinearFlowEditor
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
