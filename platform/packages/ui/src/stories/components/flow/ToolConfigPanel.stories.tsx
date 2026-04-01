import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  ToolConfigPanel,
  type ToolConfigPanelProps,
  type ComponentSchema,
} from "../../../components/flow/ToolConfigPanel";

const meta: Meta<typeof ToolConfigPanel> = {
  title: "Workspace/Flow/ToolConfigPanel",
  component: ToolConfigPanel,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 420, padding: 16 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof ToolConfigPanel>;

const sampleTMs = [
  { name: "project-memory", entryCount: 12450 },
  { name: "legacy-tm", entryCount: 85000 },
];

const sampleTermbases = [
  { name: "glossary", entryCount: 340 },
  { name: "brand-terms", entryCount: 52 },
];

function StatefulPanel(
  props: Omit<ToolConfigPanelProps, "onChange"> & { initialConfig?: Record<string, unknown> },
) {
  const [config, setConfig] = useState(props.initialConfig ?? props.config);
  return <ToolConfigPanel {...props} config={config} onChange={setConfig} />;
}

// --- Quality Check Step ---

const qualityCheckSchema: ComponentSchema = {
  $id: "quality-check",
  title: "Quality Check",
  description: "Compare source and target for quality.",
  properties: {
    outputPath: {
      type: "string",
      title: "Report Output",
      default: "${rootDir}/qa-report.html",
      "x-path": { type: "file", role: "output", accepts: ["html"] },
    },
    termsPath: {
      type: "string",
      title: "Terminology File",
      "x-path": { type: "file", role: "input", resourceKind: "termbase" },
    },
    checkTerms: {
      type: "boolean",
      title: "Check terms",
      default: false,
    },
    leadingWS: {
      type: "boolean",
      title: "Check leading whitespace",
      default: true,
    },
    trailingWS: {
      type: "boolean",
      title: "Check trailing whitespace",
      default: true,
    },
    emptyTarget: {
      type: "boolean",
      title: "Flag empty targets",
      default: true,
    },
    codeDifference: {
      type: "boolean",
      title: "Check inline code differences",
      default: true,
    },
  },
};

export const QualityCheckStep: Story = {
  render: () => (
    <StatefulPanel
      schema={qualityCheckSchema}
      config={{
        termsPath: "termbase:glossary",
        checkTerms: true,
        leadingWS: true,
        trailingWS: true,
        emptyTarget: true,
      }}
      resources={{ termbase: sampleTermbases }}
    />
  ),
};

// --- Batch Translation Step ---

const batchTranslationSchema: ComponentSchema = {
  $id: "batch-translation",
  title: "Batch Translation",
  description: "Creates translations from an external program.",
  properties: {
    command: {
      type: "string",
      title: "Command line",
      description: "Command line to execute the batch translation",
    },
    tmDirectory: {
      type: "string",
      title: "TM Directory",
      "x-path": { type: "directory", role: "input", resourceKind: "tm" },
    },
    tmxPath: {
      type: "string",
      title: "TMX Output",
      "x-path": { type: "file", role: "output", accepts: ["tmx"] },
    },
    srxPath: {
      type: "string",
      title: "SRX Rules",
      description: "Segmentation rules file",
      "x-path": { type: "file", role: "input", resourceKind: "srx", accepts: ["srx"] },
    },
    makeTMX: {
      type: "boolean",
      title: "Create TMX document",
      default: false,
    },
    blockSize: {
      type: "integer",
      title: "Block size",
      description: "Maximum text units per batch",
      default: 1000,
    },
  },
};

export const BatchTranslationStep: Story = {
  render: () => (
    <StatefulPanel
      schema={batchTranslationSchema}
      config={{
        command: "apertium -f html en-fr",
        tmDirectory: "tm:project-memory",
        makeTMX: true,
        blockSize: 1000,
      }}
      resources={{ tm: sampleTMs }}
    />
  ),
};

// --- Segmentation Step ---

const segmentationSchema: ComponentSchema = {
  $id: "segmentation",
  title: "Segmentation",
  description: "Apply SRX segmentation.",
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
    overwriteSegmentation: {
      type: "boolean",
      title: "Overwrite existing segmentation",
      default: false,
    },
    forceSegmentedOutput: {
      type: "boolean",
      title: "Force segmented output",
      default: true,
    },
  },
};

export const SegmentationStep: Story = {
  render: () => (
    <StatefulPanel
      schema={segmentationSchema}
      config={{ segmentSource: true, forceSegmentedOutput: true }}
    />
  ),
};

// --- Search and Replace Step ---

const searchReplaceSchema: ComponentSchema = {
  $id: "search-and-replace",
  title: "Search and Replace",
  description: "Performs search and replace.",
  properties: {
    replacementsPath: {
      type: "string",
      title: "Replacements File",
      "x-path": { type: "file", role: "input" },
    },
    logPath: {
      type: "string",
      title: "Log Output",
      default: "${rootDir}/replacementsLog.txt",
      "x-path": { type: "file", role: "output", accepts: ["txt"] },
    },
    regEx: {
      type: "boolean",
      title: "Use regular expressions",
      default: false,
    },
    ignoreCase: {
      type: "boolean",
      title: "Ignore case",
      default: false,
    },
    target: {
      type: "boolean",
      title: "Apply to target",
      default: true,
    },
    source: {
      type: "boolean",
      title: "Apply to source",
      default: false,
    },
    saveLog: {
      type: "boolean",
      title: "Save log",
      default: false,
    },
  },
};

export const SearchAndReplaceStep: Story = {
  render: () => (
    <StatefulPanel
      schema={searchReplaceSchema}
      config={{
        replacementsPath: "./replacements.txt",
        target: true,
        regEx: true,
      }}
    />
  ),
};

// --- Read-Only Mode ---

export const ReadOnly: Story = {
  render: () => (
    <StatefulPanel
      schema={qualityCheckSchema}
      config={{
        termsPath: "termbase:glossary",
        checkTerms: true,
        leadingWS: true,
      }}
      resources={{ termbase: sampleTermbases }}
      readOnly
    />
  ),
};
