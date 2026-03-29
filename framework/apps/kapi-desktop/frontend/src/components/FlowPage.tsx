import { useState, useEffect } from "react";
import { Plus, Trash2 } from "lucide-react";
import type { KapiProject, FlowSpec } from "../types/api";
import { FlowEditor } from "@neokapi/flow-editor";
import type { ToolInfo } from "@neokapi/flow-editor";
import { api } from "../hooks/useApi";

interface FlowPageProps {
  project: KapiProject;
  onUpdate: (project: KapiProject) => void;
  onRunFlow?: (flowName: string, flow: FlowSpec) => void;
}

export function FlowPage({ project, onUpdate, onRunFlow }: FlowPageProps) {
  const [selectedFlow, setSelectedFlow] = useState<string | null>(null);
  const [tools, setTools] = useState<ToolInfo[]>([]);
  const flowNames = Object.keys(project.flows ?? {});

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

  const handleAddFlow = () => {
    let counter = flowNames.length + 1;
    let name = `flow-${counter}`;
    while (project.flows?.[name]) {
      counter++;
      name = `flow-${counter}`;
    }
    const newFlow: FlowSpec = {
      steps: [{ tool: "pseudo-translate" }],
    };
    onUpdate({
      ...project,
      flows: { ...project.flows, [name]: newFlow },
    });
    setSelectedFlow(name);
  };

  const handleDeleteFlow = (name: string) => {
    const flows = { ...project.flows };
    delete flows[name];
    onUpdate({ ...project, flows });
    if (selectedFlow === name) setSelectedFlow(null);
  };

  const handleFlowChange = (flowName: string, spec: FlowSpec) => {
    onUpdate({
      ...project,
      flows: { ...project.flows, [flowName]: spec },
    });
  };

  const selectedSpec = selectedFlow ? project.flows?.[selectedFlow] : null;

  return (
    <div className="flex h-full">
      {/* Flow list sidebar */}
      <div className="w-48 shrink-0 border-r border-border p-3">
        <div className="mb-3 flex items-center justify-between">
          <h2 className="text-sm font-medium">Flows</h2>
          <button
            onClick={handleAddFlow}
            className="rounded p-1 text-muted-foreground hover:bg-accent hover:text-foreground"
            aria-label="New flow"
          >
            <Plus size={14} />
          </button>
        </div>
        <div className="space-y-0.5">
          {flowNames.map((name) => (
            <div
              key={name}
              className={`group flex items-center gap-1 rounded px-2 py-1.5 text-sm ${
                selectedFlow === name
                  ? "bg-accent text-accent-foreground"
                  : "text-muted-foreground hover:bg-accent/50"
              }`}
            >
              <button
                onClick={() => setSelectedFlow(name)}
                className="flex-1 truncate text-left"
              >
                {name}
              </button>
              <button
                onClick={() => handleDeleteFlow(name)}
                className="rounded p-0.5 opacity-0 hover:text-destructive group-hover:opacity-100"
                aria-label={`Delete flow ${name}`}
              >
                <Trash2 size={12} />
              </button>
            </div>
          ))}
          {flowNames.length === 0 && (
            <p className="px-2 text-xs text-muted-foreground">
              No flows yet. Click + to add one.
            </p>
          )}
        </div>
      </div>

      {/* Visual flow editor */}
      <div className="flex-1">
        {selectedSpec && selectedFlow ? (
          <FlowEditor
            flow={selectedSpec}
            tools={tools}
            onChange={(updated) => handleFlowChange(selectedFlow, updated)}
            onRun={onRunFlow ? (spec) => onRunFlow(selectedFlow, spec) : undefined}
          />
        ) : (
          <div className="flex h-full items-center justify-center text-muted-foreground">
            <p className="text-sm">Select a flow or create a new one</p>
          </div>
        )}
      </div>
    </div>
  );
}
