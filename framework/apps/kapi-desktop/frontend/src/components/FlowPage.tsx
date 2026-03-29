import { useEffect, useState } from "react";
import type { FlowSpec } from "../types/api";
import { FlowEditor } from "@neokapi/flow-editor";
import type { ToolInfo } from "@neokapi/flow-editor";
import { api } from "../hooks/useApi";

interface FlowPageProps {
  flowName: string;
  flow: FlowSpec;
  onChange: (spec: FlowSpec) => void;
  onRun?: (flowName: string, spec: FlowSpec) => void;
}

export function FlowPage({ flowName, flow, onChange, onRun }: FlowPageProps) {
  const [tools, setTools] = useState<ToolInfo[]>([]);

  useEffect(() => {
    api.listTools().then((result) => {
      if (result) {
        setTools(
          result.map((t) => ({
            name: t.name,
            description: t.description,
            category: t.category,
          })),
        );
      }
    });
  }, []);

  return (
    <FlowEditor
      key={flowName}
      flow={flow}
      tools={tools}
      onChange={onChange}
      onRun={onRun ? (spec) => onRun(flowName, spec) : undefined}
    />
  );
}
