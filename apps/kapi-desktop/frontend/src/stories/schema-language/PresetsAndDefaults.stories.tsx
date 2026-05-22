/**
 * Schema Language: Presets and Defaults
 *
 * Demonstrates how presets, defaults, and the preset indicator dot work
 * in the schema form system.
 */
import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { SchemaForm } from "@neokapi/ui-primitives";
import type { ComponentSchema } from "@neokapi/ui-primitives";
import { presets } from "../_lib/reference-data";

function PresetStory({
  schema,
  description,
  presetValues,
}: {
  schema: ComponentSchema;
  description?: string;
  presetValues?: Record<string, unknown>;
}) {
  const [values, setValues] = useState<Record<string, unknown>>({});
  return (
    <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 24, maxWidth: 900 }}>
      <div>
        {description && <p className="text-sm text-muted-foreground mb-4">{description}</p>}
        <SchemaForm
          schema={schema}
          values={values}
          onChange={setValues}
          presetValues={presetValues}
        />
      </div>
      <div style={{ minWidth: 0 }}>
        <h4 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-2">
          Current Values
        </h4>
        <pre className="rounded bg-muted p-3 text-xs text-muted-foreground overflow-auto max-h-32">
          {JSON.stringify(values, null, 2)}
        </pre>
        {presetValues && (
          <>
            <h4 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mt-4 mb-2">
              Active Preset Values
            </h4>
            <pre className="rounded bg-muted p-3 text-xs text-muted-foreground overflow-auto max-h-32">
              {JSON.stringify(presetValues, null, 2)}
            </pre>
          </>
        )}
      </div>
    </div>
  );
}

const meta: Meta<typeof PresetStory> = {
  title: "Formats & Tools/Schema/Presets & Defaults",
  component: PresetStory,
  tags: ["autodocs"],
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof PresetStory>;

export const DefaultValues: Story = {
  name: "default — Property Defaults",
  args: {
    description:
      "Each property can have a `default` value. When the user hasn't set a value, the default is shown as placeholder/initial state. The SchemaForm tracks which values differ from defaults.",
    schema: {
      title: "Properties with Defaults",
      type: "object",
      properties: {
        encoding: { type: "string", title: "Encoding", default: "UTF-8" },
        extractAll: { type: "boolean", title: "Extract All", default: true },
        maxSegments: { type: "integer", title: "Max Segments", default: 1000, minimum: 1 },
        outputFormat: {
          type: "string",
          title: "Format",
          default: "json",
          enum: ["json", "yaml", "xml"],
        },
      },
    },
  },
};

export const PresetIndicator: Story = {
  name: "presetValues — Modified Indicator Dot",
  args: {
    description:
      "When `presetValues` is provided, a colored dot appears next to fields that differ from the preset. This helps users see what they've customized. Try changing field values to see the dot appear.",
    presetValues: {
      encoding: "UTF-8",
      extractAll: true,
      maxSegments: 1000,
    },
    schema: {
      title: "Preset Comparison",
      type: "object",
      properties: {
        encoding: { type: "string", title: "Encoding", default: "UTF-8" },
        extractAll: { type: "boolean", title: "Extract All", default: true },
        maxSegments: { type: "integer", title: "Max Segments", default: 1000, minimum: 1 },
      },
    },
  },
};

// Find a format with real presets
const formatWithPresets = Object.entries(presets as Record<string, Record<string, unknown>>).find(
  ([, v]) => Object.keys(v).length > 1,
);

export const RealPresets: Story = {
  name: "Real Example: Bridge Format Presets",
  args: {
    description: formatWithPresets
      ? `Real presets for format "${formatWithPresets[0]}". Presets are extracted from Okapi filter configurations during the transform stage and stored as separate JSON files.`
      : "No multi-preset format found in fixtures.",
    presetValues: formatWithPresets
      ? (Object.values(formatWithPresets[1])[0] as Record<string, unknown>)
      : undefined,
    schema: {
      title: formatWithPresets ? `Presets for ${formatWithPresets[0]}` : "No presets",
      type: "object",
      properties: {
        info: {
          type: "string",
          title: "Available presets",
          description: formatWithPresets ? Object.keys(formatWithPresets[1]).join(", ") : "none",
        },
      },
    },
  },
};
