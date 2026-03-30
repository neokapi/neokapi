import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import {
  Workflow,
  Plus,
  Play,
  Trash2,
  X,
} from "lucide-react";
import type { FlowSpec, FlowInfo } from "../types/api";
import { FlowEditor } from "@neokapi/flow-editor";
import type { ToolInfo } from "@neokapi/flow-editor";
import toolsData from "./fixtures/tools-metadata.json";

const tools = toolsData as ToolInfo[];

const SAMPLE_FLOWS: Record<string, FlowSpec> = {
  "translate-and-qa": {
    description: "Translate with AI then run quality checks",
    steps: [
      { tool: "ai-translate", label: "Translate" },
      { tool: "qa-check", label: "Quality Check" },
    ],
  },
  "full-pipeline": {
    description: "Complete localization pipeline",
    steps: [
      { tool: "tm-leverage", label: "TM Leverage" },
      { tool: "ai-translate", label: "AI Translate" },
      {
        tool: "",
        parallel: [
          { tool: "qa-check", label: "QA Check" },
          { tool: "brand-vocab-check", label: "Brand Check" },
        ],
      },
      { tool: "word-count", label: "Word Count" },
    ],
  },
  "pseudo-validate": {
    steps: [
      { tool: "pseudo-translate" },
      { tool: "qa-check" },
    ],
  },
};

function SimulatedFlowsPage() {
  const [flows, setFlows] = useState(SAMPLE_FLOWS);
  const [selected, setSelected] = useState<string | null>(null);

  const flowList: FlowInfo[] = Object.entries(flows).map(([name, spec]) => ({
    name,
    description: spec.description ?? "",
    step_count: spec.steps.length,
  }));

  if (selected && flows[selected]) {
    return (
      <div style={{ height: 700, display: "flex", flexDirection: "column" }}>
        <div className="flex items-center gap-3 px-6 py-3 border-b border-border shrink-0">
          <button
            onClick={() => setSelected(null)}
            className="p-1 rounded hover:bg-accent text-muted-foreground hover:text-foreground transition-colors"
          >
            <X size={16} />
          </button>
          <Workflow size={16} className="text-muted-foreground" />
          <span className="text-sm font-semibold">{selected}</span>
        </div>
        <div style={{ flex: 1 }}>
          <FlowEditor
            flow={flows[selected]}
            tools={tools}
            onChange={(spec) => setFlows({ ...flows, [selected]: spec })}
          />
        </div>
      </div>
    );
  }

  return (
    <div className="p-6">
      <div className="mb-6 flex items-center justify-between">
        <h1 className="text-xl font-semibold">Project Flows</h1>
        <button className="flex items-center gap-1.5 rounded-md bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground hover:bg-primary/90">
          <Plus size={12} />
          New Flow
        </button>
      </div>
      <div className="grid grid-cols-1 md:grid-cols-2 gap-2">
        {flowList.map((flow) => (
          <div
            key={flow.name}
            className="group rounded-lg border border-border bg-card p-4 transition-all hover:border-primary/30 hover:shadow-md cursor-pointer"
            onClick={() => setSelected(flow.name)}
          >
            <div className="flex items-start gap-3">
              <Workflow size={18} className="mt-0.5 text-muted-foreground group-hover:text-primary transition-colors shrink-0" />
              <div className="flex-1 min-w-0">
                <div className="text-sm font-semibold text-foreground group-hover:text-primary transition-colors truncate">
                  {flow.name}
                </div>
                {flow.description && (
                  <div className="text-[11px] text-muted-foreground truncate mt-0.5">{flow.description}</div>
                )}
                <div className="flex items-center gap-3 mt-2 text-[11px] text-muted-foreground">
                  <span>{flow.step_count} step{flow.step_count !== 1 ? "s" : ""}</span>
                </div>
              </div>
              <div className="flex gap-1 opacity-0 group-hover:opacity-100 transition-opacity" onClick={(e) => e.stopPropagation()}>
                <button className="p-1.5 rounded hover:bg-accent text-muted-foreground hover:text-foreground transition-colors" title="Run">
                  <Play size={12} />
                </button>
                <button className="p-1.5 rounded hover:bg-destructive/10 text-muted-foreground hover:text-destructive transition-colors" title="Delete">
                  <Trash2 size={12} />
                </button>
              </div>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

const meta: Meta<typeof SimulatedFlowsPage> = {
  title: "Pages/FlowsPage",
  component: SimulatedFlowsPage,
  tags: ["autodocs"],
  parameters: {
    layout: "fullscreen",
    docs: {
      description: {
        component:
          "Flow management page with flow list, create/delete, and integrated flow editor. Supports both ad-hoc and project-scoped flows.",
      },
    },
  },
};

export default meta;
type Story = StoryObj<typeof SimulatedFlowsPage>;

export const Default: Story = {};
