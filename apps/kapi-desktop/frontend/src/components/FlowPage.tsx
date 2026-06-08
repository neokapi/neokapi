import { useEffect, useState, useCallback, useRef, useReducer } from "react";
import type { FlowSpec } from "../types/api";
import { useWailsEvent } from "../hooks/useWailsEvent";
import { FlowEditor } from "@neokapi/flow-editor";
import type { ToolInfo, ToolDoc, ComponentSchema } from "@neokapi/flow-editor";
import { api } from "../hooks/useApi";
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
  const [tools, setTools] = useState<ToolInfo[]>([]);
  const schemasRef = useRef<Record<string, ComponentSchema | null>>({});
  const docsRef = useRef<Record<string, ToolDoc | null>>({});
  const fetchingRef = useRef<Set<string>>(new Set());
  const [, forceUpdate] = useReducer((x: number) => x + 1, 0);

  const loadTools = useCallback(() => {
    // Use project-scoped tools when in project mode.
    const promise = tabID ? api.listProjectTools(tabID) : api.listTools();
    void promise.then((result) => {
      if (result) {
        setTools(
          result.map((t) => ({
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
          })),
        );
      }
    });
  }, [tabID]);

  useEffect(() => {
    loadTools();
  }, [loadTools]);

  // Refresh when plugins change (tools may have been added/removed).
  useWailsEvent("registries-changed", loadTools);

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
