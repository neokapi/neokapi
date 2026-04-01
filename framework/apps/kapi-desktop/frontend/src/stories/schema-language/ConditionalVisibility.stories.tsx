/**
 * Schema Language: Conditional Visibility
 *
 * Demonstrates ui:visible for showing/hiding fields based on other field values,
 * and ui:enabled.enabledBy for enabling/disabling fields.
 */
import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { SchemaForm } from "@neokapi/flow-editor";
import type { ComponentSchema } from "@neokapi/flow-editor";

function SchemaStory({ schema, description, initialValues }: { schema: ComponentSchema; description?: string; initialValues?: Record<string, unknown> }) {
  const [values, setValues] = useState<Record<string, unknown>>(initialValues ?? {});
  return (
    <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 24, maxWidth: 900 }}>
      <div>
        {description && <p className="text-sm text-muted-foreground mb-4">{description}</p>}
        <SchemaForm schema={schema} values={values} onChange={setValues} />
      </div>
      <div>
        <h4 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-2">Current Values</h4>
        <pre className="rounded bg-muted p-3 text-xs text-muted-foreground overflow-auto max-h-40">
          {JSON.stringify(values, null, 2)}
        </pre>
        <h4 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mt-4 mb-2">Schema</h4>
        <pre className="rounded bg-muted p-3 text-xs text-muted-foreground overflow-auto max-h-60">
          {JSON.stringify(schema, null, 2)}
        </pre>
      </div>
    </div>
  );
}

const meta: Meta<typeof SchemaStory> = {
  title: "Schema Language/Conditional Visibility",
  component: SchemaStory,
  tags: ["autodocs"],
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof SchemaStory>;

export const ShowIfBoolean: Story = {
  name: "ui:visible — Toggle Field Visibility",
  args: {
    description:
      'The `ui:visible` rule on a property controls its visibility based on another field\'s value. Toggle "Use Code Finder" to show/hide the rules field.',
    schema: {
      title: "Conditional Fields",
      type: "object",
      properties: {
        useCodeFinder: { type: "boolean", title: "Use Code Finder", description: "Enable inline code detection", default: false },
        codeFinderRules: {
          type: "string",
          title: "Code Finder Rules",
          description: "Regex patterns for inline codes (one per line)",
          "ui:widget": "textarea",
          "ui:visible": { field: "useCodeFinder", eq: true },
        },
      },
    },
  },
};

export const ShowIfEnum: Story = {
  name: "ui:visible — Enum-Driven Visibility",
  args: {
    description:
      'Fields can be shown conditionally based on an enum value. Select different output modes to see different options appear.',
    initialValues: { outputMode: "file" },
    schema: {
      title: "Mode-Dependent Fields",
      type: "object",
      properties: {
        outputMode: {
          type: "string",
          title: "Output Mode",
          enum: ["file", "stdout", "api"],
          default: "file",
        },
        outputPath: {
          type: "string",
          title: "Output Path",
          description: "File path for output",
          "ui:visible": { field: "outputMode", eq: "file" },
        },
        apiEndpoint: {
          type: "string",
          title: "API Endpoint",
          description: "URL to POST results to",
          "ui:placeholder": "https://api.example.com/results",
          "ui:visible": { field: "outputMode", eq: "api" },
        },
        apiKey: {
          type: "string",
          title: "API Key",
          "ui:visible": { field: "outputMode", eq: "api" },
        },
      },
    },
  },
};

export const ShowIfEmpty: Story = {
  name: "ui:visible empty — Show When Field is Unset",
  args: {
    description:
      'Setting `empty: true` in ui:visible shows the field when the referenced field is empty or unset. Clear the "Override Path" to see the default path message.',
    schema: {
      title: "Empty-Based Visibility",
      type: "object",
      properties: {
        overridePath: { type: "string", title: "Override Path", description: "Custom output path (leave empty for default)" },
        defaultPathInfo: {
          type: "string",
          title: "Default Path",
          description: "Using auto-generated path based on input file",
          "ui:visible": { field: "overridePath", empty: true },
        },
      },
    },
  },
};

export const EditorEnabledBy: Story = {
  name: "ui:enabled.enabledBy — Master/Slave Fields",
  args: {
    description:
      'The `ui:enabled.enabledBy` metadata from Okapi controls whether a field is enabled or disabled based on a master field. The field remains visible but grayed out when disabled.',
    schema: {
      title: "Enabled-By Dependencies",
      type: "object",
      properties: {
        useTranslation: { type: "boolean", title: "Enable Translation", default: false },
        sourceLanguage: {
          type: "string",
          title: "Source Language",
          default: "en",
          "ui:enabled": { field: "useTranslation", eq: true },
        },
        targetLanguage: {
          type: "string",
          title: "Target Language",
          "ui:placeholder": "e.g., fr",
          "ui:enabled": { field: "useTranslation", eq: true },
        },
      },
    },
  },
};
