import { useEffect, useState, useCallback, useRef, useReducer } from "react";
import type { FlowSpec } from "../types/api";
import { useWailsEvent } from "../hooks/useWailsEvent";
import { FlowEditor } from "@neokapi/flow-editor";
import type { ToolInfo, ComponentSchema } from "@neokapi/flow-editor";
import { api } from "../hooks/useApi";

interface FlowPageProps {
  flowName: string;
  flow: FlowSpec;
  onChange: (spec: FlowSpec) => void;
  onRun?: (flowName: string, spec: FlowSpec) => void;
  readOnly?: boolean;
}

export function FlowPage({ flowName, flow, onChange, onRun, readOnly }: FlowPageProps) {
  const [tools, setTools] = useState<ToolInfo[]>([]);
  const schemasRef = useRef<Record<string, ComponentSchema | null>>({});
  const fetchingRef = useRef<Set<string>>(new Set());
  const [, forceUpdate] = useReducer((x: number) => x + 1, 0);

  const loadTools = useCallback(() => {
    api.listTools().then((result) => {
      if (result) {
        setTools(
          result.map((t) => ({
            name: t.name,
            description: t.description,
            category: t.category,
            has_schema: t.has_schema,
            inputs: t.inputs,
            outputs: t.outputs,
            tags: t.tags,
            requires: t.requires,
          })),
        );
      }
    });
  }, []);

  useEffect(() => { loadTools(); }, [loadTools]);

  // Refresh when plugins change (tools may have been added/removed).
  useWailsEvent("registries-changed", loadTools);

  const handleGetSchema = useCallback((toolName: string): ComponentSchema | null => {
    if (toolName in schemasRef.current) {
      return schemasRef.current[toolName] ?? null;
    }
    if (fetchingRef.current.has(toolName)) return null;
    fetchingRef.current.add(toolName);
    api.getToolSchema(toolName).then((result) => {
      fetchingRef.current.delete(toolName);
      schemasRef.current[toolName] = (result as ComponentSchema) ?? null;
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
      onGetSchema={handleGetSchema}
      readOnly={readOnly}
    />
  );
}
