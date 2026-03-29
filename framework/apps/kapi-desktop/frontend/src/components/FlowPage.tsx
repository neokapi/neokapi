import { useState } from "react";
import { Plus, Play, Trash2 } from "lucide-react";
import type { KapiProject, FlowSpec, FlowStep } from "../types/api";

interface FlowPageProps {
  project: KapiProject;
  onUpdate: (project: KapiProject) => void;
  onRunFlow?: (flowName: string, flow: FlowSpec) => void;
}

export function FlowPage({ project, onUpdate, onRunFlow }: FlowPageProps) {
  const [selectedFlow, setSelectedFlow] = useState<string | null>(null);
  const flowNames = Object.keys(project.flows ?? {});

  const handleAddFlow = () => {
    // Find a unique name to avoid overwriting existing flows.
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

  const handleUpdateStep = (
    flowName: string,
    stepIndex: number,
    step: FlowStep,
  ) => {
    const flow = project.flows?.[flowName];
    if (!flow) return;
    const steps = [...flow.steps];
    steps[stepIndex] = step;
    onUpdate({
      ...project,
      flows: { ...project.flows, [flowName]: { ...flow, steps } },
    });
  };

  const handleAddStep = (flowName: string) => {
    const flow = project.flows?.[flowName];
    if (!flow) return;
    onUpdate({
      ...project,
      flows: {
        ...project.flows,
        [flowName]: { ...flow, steps: [...flow.steps, { tool: "" }] },
      },
    });
  };

  const selectedSpec = selectedFlow ? project.flows?.[selectedFlow] : null;

  return (
    <div className="flex h-full">
      {/* Flow list sidebar */}
      <div className="w-56 shrink-0 border-r border-border p-3">
        <div className="mb-3 flex items-center justify-between">
          <h2 className="text-sm font-medium">Flows</h2>
          <button
            onClick={handleAddFlow}
            className="rounded p-1 text-muted-foreground hover:bg-accent hover:text-foreground"
            title="New flow"
          >
            <Plus size={14} />
          </button>
        </div>
        <div className="space-y-0.5">
          {flowNames.map((name) => (
            <div
              key={name}
              className={`flex items-center gap-1 rounded px-2 py-1.5 text-sm ${
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
                className="rounded p-0.5 opacity-0 hover:bg-destructive/10 hover:text-destructive group-hover:opacity-100"
                style={{ opacity: selectedFlow === name ? 1 : undefined }}
                title="Delete flow"
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

      {/* Flow editor area */}
      <div className="flex-1 p-6">
        {selectedSpec && selectedFlow ? (
          <div>
            <div className="mb-4 flex items-center gap-3">
              <h2 className="text-lg font-medium">{selectedFlow}</h2>
              <button
                onClick={() => onRunFlow?.(selectedFlow, selectedSpec)}
                className="flex items-center gap-1.5 rounded-md bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground hover:bg-primary/90"
                aria-label={`Run flow ${selectedFlow}`}
              >
                <Play size={12} />
                Run
              </button>
            </div>

            {/* Steps editor */}
            <div className="space-y-2">
              {selectedSpec.steps.map((step, i) => (
                <div
                  key={i}
                  className="flex items-center gap-2 rounded-lg border border-border p-3"
                >
                  <span className="flex h-6 w-6 items-center justify-center rounded-full bg-primary/10 text-xs font-medium text-primary">
                    {i + 1}
                  </span>
                  <input
                    type="text"
                    value={step.tool}
                    onChange={(e) =>
                      handleUpdateStep(selectedFlow, i, {
                        ...step,
                        tool: e.target.value,
                      })
                    }
                    placeholder="Tool name"
                    className="flex-1 rounded border border-input bg-transparent px-2 py-1 text-sm outline-none focus:ring-1 focus:ring-ring"
                  />
                </div>
              ))}
            </div>

            <button
              onClick={() => handleAddStep(selectedFlow)}
              className="mt-3 flex items-center gap-1.5 rounded-md border border-dashed border-border px-3 py-2 text-xs text-muted-foreground hover:border-primary hover:text-primary"
            >
              <Plus size={12} />
              Add step
            </button>
          </div>
        ) : (
          <div className="flex h-full items-center justify-center text-muted-foreground">
            <p className="text-sm">Select a flow or create a new one</p>
          </div>
        )}
      </div>
    </div>
  );
}
