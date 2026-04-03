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

const sampleTermbases = [
  { name: "glossary", entryCount: 340 },
  { name: "brand-terms", entryCount: 52 },
];

function StatefulPanel(
  props: Omit<ToolConfigPanelProps, "onChange"> & {
    initialConfig?: Record<string, unknown>;
  },
) {
  const [config, setConfig] = useState(props.initialConfig ?? props.config);
  return <ToolConfigPanel {...props} config={config} onChange={setConfig} />;
}

// --- Quality Check Step (nested groups, options, ui:enabled) ---

const qualityCheckSchema: ComponentSchema = {
  $id: "quality-check",
  title: "Quality Check",
  description: "Configurable quality assurance checks for bilingual content.",
  properties: {
    whitespace: {
      type: "object",
      title: "Whitespace",
      description: "Whitespace consistency checks",
      properties: {
        leadingWS: {
          type: "boolean",
          title: "Check Leading Whitespace",
          description:
            "Flag text units where leading whitespace differs between source and target.",
          default: true,
        },
        trailingWS: {
          type: "boolean",
          title: "Check Trailing Whitespace",
          description:
            "Flag text units where trailing whitespace differs between source and target.",
          default: true,
        },
      },
    },
    completeness: {
      type: "object",
      title: "Completeness",
      description: "Empty target/source checks",
      properties: {
        emptyTarget: {
          type: "boolean",
          title: "Warn on Empty Target",
          description: "Flag segments where the target is empty while the source is not.",
          default: true,
        },
        emptySource: {
          type: "boolean",
          title: "Warn on Empty Source",
          description: "Flag segments where the target is not empty while the source is empty.",
          default: true,
        },
      },
    },
    length: {
      type: "object",
      title: "Length",
      description: "Character length validation",
      properties: {
        checkMaxCharLength: {
          type: "boolean",
          title: "Check Maximum Character Length",
          description:
            "Flag target text longer than a given percentage of source character length.",
          default: true,
        },
        maxCharLengthBreak: {
          type: "integer",
          title: "Long Text Threshold",
          description: 'Character count above which text is considered "long".',
          default: 20,
          "ui:enabled": { field: "checkMaxCharLength", eq: true },
        },
        maxCharLengthAbove: {
          type: "integer",
          title: "Percentage for Long Text",
          description: "Maximum allowed percentage of source length for long text.",
          default: 200,
          "ui:enabled": { field: "checkMaxCharLength", eq: true },
        },
        maxCharLengthBelow: {
          type: "integer",
          title: "Percentage for Short Text",
          description: "Maximum allowed percentage of source length for short text.",
          default: 350,
          "ui:enabled": { field: "checkMaxCharLength", eq: true },
        },
      },
    },
    report: {
      type: "object",
      title: "Report",
      description: "Output report settings",
      properties: {
        outputPath: {
          type: "string",
          title: "Report File",
          description: "Path for the quality check report.",
          default: "${rootDir}/qa-report.html",
          "x-path": {
            type: "file",
            role: "output",
            accepts: ["html"],
            browseTitle: "Quality Check Report",
            forSaveAs: true,
          },
        },
        outputType: {
          type: "integer",
          title: "Report Format",
          description: "Format of the quality check report.",
          default: 0,
          options: [
            { value: 0, label: "HTML file" },
            { value: 1, label: "Tab-delimited file" },
            { value: 2, label: "XML file" },
          ],
        },
        autoOpen: {
          type: "boolean",
          title: "Open After Completion",
          default: true,
        },
      },
    },
    terminology: {
      type: "object",
      title: "Terminology",
      description: "Terminology verification",
      properties: {
        checkTerms: {
          type: "boolean",
          title: "Check Terminology",
          description: "Verify glossary terminology is used correctly.",
          default: false,
        },
        termsPath: {
          type: "string",
          title: "Terminology File",
          "x-path": {
            type: "file",
            role: "input",
            resourceKind: "termbase",
          },
          "ui:enabled": { field: "checkTerms", eq: true },
        },
      },
    },
  },
};

export const QualityCheckStep: Story = {
  render: () => (
    <StatefulPanel
      schema={qualityCheckSchema}
      config={{
        whitespace: { leadingWS: true, trailingWS: true },
        completeness: { emptyTarget: true, emptySource: true },
        length: {
          checkMaxCharLength: true,
          maxCharLengthBreak: 20,
          maxCharLengthAbove: 200,
          maxCharLengthBelow: 350,
        },
        report: { outputType: 0, autoOpen: true },
        terminology: { checkTerms: false },
      }}
      resources={{ termbase: sampleTermbases }}
    />
  ),
};

// --- Codes Removal Step (options from consolidated enum) ---

const codesRemovalSchema: ComponentSchema = {
  $id: "codes-removal",
  title: "Inline Codes Removal",
  properties: {
    mode: {
      type: "string",
      title: "Removal Mode",
      description: "What to remove from inline codes",
      default: "0",
      options: [
        { value: "0", label: "Remove marker, keep content" },
        { value: "1", label: "Remove content, keep marker" },
        { value: "2", label: "Remove marker and content" },
      ],
    },
    stripSource: {
      type: "boolean",
      title: "Strip Codes in Source",
      default: false,
    },
    stripTarget: {
      type: "boolean",
      title: "Strip Codes in Target",
      default: true,
    },
  },
};

export const CodesRemovalStep: Story = {
  render: () => (
    <StatefulPanel
      schema={codesRemovalSchema}
      config={{ mode: "0", stripSource: false, stripTarget: true }}
    />
  ),
};

// --- Segmentation Step (simple booleans) ---

const segmentationSchema: ComponentSchema = {
  $id: "segmentation",
  title: "Segmentation",
  properties: {
    segmentSource: {
      type: "boolean",
      title: "Segment Source Text",
      description: "Segment the source text using SRX rules.",
      default: true,
    },
    segmentTarget: {
      type: "boolean",
      title: "Segment Target Text",
      description: "Segment existing target text using SRX rules.",
      default: false,
    },
    overwriteSegmentation: {
      type: "boolean",
      title: "Overwrite Existing Segmentation",
      default: false,
    },
    forceSegmentedOutput: {
      type: "boolean",
      title: "Force Segmented Output",
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

// --- Read-Only Mode ---

export const ReadOnly: Story = {
  render: () => (
    <StatefulPanel
      schema={qualityCheckSchema}
      config={{
        whitespace: { leadingWS: true },
        terminology: { checkTerms: true },
      }}
      resources={{ termbase: sampleTermbases }}
      readOnly
    />
  ),
};
