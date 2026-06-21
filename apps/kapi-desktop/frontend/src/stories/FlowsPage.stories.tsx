import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { Workflow, Plus, Play, Trash2, X } from "lucide-react";
import { Button, Card } from "@neokapi/ui-primitives";
import type { FlowSpec, FlowInfo } from "../types/api";
import { FlowEditor } from "@neokapi/flow-editor";
import type { ToolInfo } from "@neokapi/flow-editor";
import { FlowsPage, type FlowListItem } from "../components/FlowsPage";
import { toolsMetadata } from "./_lib/reference-data";

const tools = toolsMetadata as ToolInfo[];

const SAMPLE_FLOWS: Record<string, FlowSpec> = {
  "translate-and-qa": {
    description: "Translate with AI then run quality checks",
    steps: [
      { tool: "translate", label: "Translate" },
      { tool: "qa", label: "Quality Check" },
    ],
  },
  "full-pipeline": {
    description: "Complete localization pipeline",
    steps: [
      { tool: "recycle", label: "TM Leverage" },
      { tool: "translate", label: "AI Translate" },
      {
        tool: "",
        parallel: [
          { tool: "qa", label: "QA Check" },
          { tool: "brand-vocab-check", label: "Brand Check" },
        ],
      },
      { tool: "word-count", label: "Word Count" },
    ],
  },
  "pseudo-validate": {
    steps: [{ tool: "pseudo-translate" }, { tool: "qa" }],
  },
};

const SAMPLE_FLOW_LIST: FlowListItem[] = Object.entries(SAMPLE_FLOWS).map(([name, spec]) => ({
  id: name,
  name,
  description: spec.description ?? "",
  source: "user",
  stepCount: spec.steps.length,
}));

function SimulatedFlowsPage() {
  const [flows, setFlows] = useState(SAMPLE_FLOWS);
  const [selected, setSelected] = useState<string | null>(null);

  const flowList: FlowInfo[] = Object.entries(flows).map(([name, spec]) => ({
    name,
    description: spec.description ?? "",
    step_count: spec.steps.length,
    valid: true,
  }));

  if (selected && flows[selected]) {
    return (
      <div style={{ height: 700, display: "flex", flexDirection: "column" }}>
        <div className="flex items-center gap-3 px-6 py-3 border-b border-border shrink-0">
          <Button variant="ghost" size="icon-xs" onClick={() => setSelected(null)}>
            <X size={16} />
          </Button>
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
        <Button size="sm">
          <Plus size={12} />
          New Flow
        </Button>
      </div>
      <div className="grid grid-cols-1 md:grid-cols-2 gap-2">
        {flowList.map((flow) => (
          <Card
            key={flow.name}
            className="group p-4 transition-all hover:border-primary/30 hover:shadow-md cursor-pointer"
            onClick={() => setSelected(flow.name)}
          >
            <div className="flex items-start gap-3">
              <Workflow
                size={18}
                className="mt-0.5 text-muted-foreground group-hover:text-primary transition-colors shrink-0"
              />
              <div className="flex-1 min-w-0">
                <div className="text-sm font-semibold text-foreground group-hover:text-primary transition-colors truncate">
                  {flow.name}
                </div>
                {flow.description && (
                  <div className="text-[11px] text-muted-foreground truncate mt-0.5">
                    {flow.description}
                  </div>
                )}
                <div className="flex items-center gap-3 mt-2 text-[11px] text-muted-foreground">
                  <span>
                    {flow.step_count} step{flow.step_count !== 1 ? "s" : ""}
                  </span>
                </div>
              </div>
              <div
                className="flex gap-1 opacity-0 group-hover:opacity-100 transition-opacity"
                onClick={(e) => e.stopPropagation()}
              >
                <Button variant="ghost" size="icon-xs" title="Run">
                  <Play size={12} />
                </Button>
                <Button
                  variant="ghost"
                  size="icon-xs"
                  className="hover:bg-destructive/10 hover:text-destructive"
                  title="Delete"
                >
                  <Trash2 size={12} />
                </Button>
              </div>
            </div>
          </Card>
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

/**
 * Real component with pre-loaded flows (no Wails API calls).
 */
export const WithFlows: StoryObj<typeof FlowsPage> = {
  render: () => <FlowsPage flows={SAMPLE_FLOW_LIST} />,
};

/**
 * Real component with empty flows list.
 */
export const Empty: StoryObj<typeof FlowsPage> = {
  render: () => <FlowsPage flows={[]} />,
};

/**
 * Ad-hoc flow list with a project open — each flow gains an "Add to project"
 * action (hover a card) that copies it into the open project's recipe.
 */
export const AdoptIntoProject: StoryObj<typeof FlowsPage> = {
  render: () => (
    <FlowsPage flows={SAMPLE_FLOW_LIST} adoptTabID="tab-1" adoptProjectName="Acme App" />
  ),
};
