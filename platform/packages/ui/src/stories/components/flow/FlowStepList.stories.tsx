import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  FlowStepList,
  type FlowStepInfo,
} from "../../../components/flow/FlowStepList";
import type { ComponentSchema } from "../../../components/flow/ToolConfigPanel";

const meta: Meta<typeof FlowStepList> = {
  title: "Workspace/Flow/FlowStepList",
  component: FlowStepList,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 480, padding: 16 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof FlowStepList>;

const sampleTMs = [
  { name: "project-memory", entryCount: 12450 },
  { name: "legacy-tm", entryCount: 85000 },
];

const sampleTermbases = [
  { name: "glossary", entryCount: 340 },
];

// Minimal schemas for demo.
const leveragingSchema: ComponentSchema = {
  $id: "leveraging",
  title: "Leveraging",
  description: "Leverage existing translations.",
  properties: {
    tmxPath: {
      type: "string",
      title: "Translation Memory",
      "x-path": { type: "file", role: "input", resourceKind: "tm", accepts: ["tmx"] },
    },
    threshold: {
      type: "integer",
      title: "Match threshold",
      description: "Minimum similarity score (0-100)",
      default: 95,
    },
    fillTarget: {
      type: "boolean",
      title: "Fill target with match",
      default: true,
    },
  },
};

const qualityCheckSchema: ComponentSchema = {
  $id: "quality-check",
  title: "Quality Check",
  description: "Compare source and target for quality.",
  properties: {
    termsPath: {
      type: "string",
      title: "Terminology",
      "x-path": { type: "file", role: "input", resourceKind: "termbase" },
    },
    outputPath: {
      type: "string",
      title: "Report Output",
      default: "${rootDir}/qa-report.html",
      "x-path": { type: "file", role: "output", accepts: ["html"] },
    },
    leadingWS: {
      type: "boolean",
      title: "Check leading whitespace",
      default: true,
    },
    emptyTarget: {
      type: "boolean",
      title: "Flag empty targets",
      default: true,
    },
  },
};

const segmentationSchema: ComponentSchema = {
  $id: "segmentation",
  title: "Segmentation",
  description: "Apply SRX segmentation to text units.",
  properties: {
    segmentSource: {
      type: "boolean",
      title: "Segment source",
      default: true,
    },
    segmentTarget: {
      type: "boolean",
      title: "Segment target",
      default: false,
    },
  },
};

const schemas = new Map<string, ComponentSchema>([
  ["leveraging", leveragingSchema],
  ["quality-check", qualityCheckSchema],
  ["segmentation", segmentationSchema],
]);

const translateFlow: FlowStepInfo[] = [
  {
    tool: "leveraging",
    label: "TM Leverage",
    config: { tmxPath: "tm:project-memory", threshold: 95, fillTarget: true },
  },
  {
    tool: "segmentation",
    config: { segmentSource: true },
  },
  {
    tool: "quality-check",
    label: "QA Check",
    config: { termsPath: "termbase:glossary", leadingWS: true, emptyTarget: true },
  },
];

function StatefulList(props: { steps: FlowStepInfo[] }) {
  const [steps, setSteps] = useState(props.steps);

  const handleConfigChange = (index: number, config: Record<string, unknown>) => {
    const updated = [...steps];
    updated[index] = { ...updated[index], config };
    setSteps(updated);
  };

  return (
    <FlowStepList
      steps={steps}
      schemas={schemas}
      onStepConfigChange={handleConfigChange}
      resources={{ tm: sampleTMs, termbase: sampleTermbases }}
    />
  );
}

export const TranslateFlow: Story = {
  render: () => <StatefulList steps={translateFlow} />,
};

export const EmptyFlow: Story = {
  render: () => (
    <FlowStepList
      steps={[]}
      schemas={schemas}
      onStepConfigChange={() => {}}
    />
  ),
};

export const SingleStep: Story = {
  render: () => <StatefulList steps={[translateFlow[0]]} />,
};

export const WithResources: Story = {
  render: () => <StatefulList steps={translateFlow} />,
};
