/**
 * Schema Language: Property Types
 *
 * Demonstrates how each JSON Schema property type renders in the SchemaForm.
 * This is the foundation — every parameter in a format or tool schema
 * maps to one of these types.
 */
import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { SchemaForm } from "@neokapi/flow-editor";
import type { ComponentSchema } from "@neokapi/flow-editor";

function SchemaStory({ schema, description }: { schema: ComponentSchema; description?: string }) {
  const [values, setValues] = useState<Record<string, unknown>>({});
  return (
    <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 24, maxWidth: 900  }}>
      <div>
        {description && (
          <p className="text-sm text-muted-foreground mb-4">{description}</p>
        )}
        <SchemaForm schema={schema} values={values} onChange={setValues} />
      </div>
      <div style={{ minWidth: 0 }}>
        <h4 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-2">Schema (JSON)</h4>
        <pre className="rounded bg-muted p-3 text-xs text-muted-foreground overflow-auto max-h-60">
          {JSON.stringify(schema, null, 2)}
        </pre>
        <h4 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mt-4 mb-2">Values</h4>
        <pre className="rounded bg-muted p-3 text-xs text-muted-foreground overflow-auto max-h-40">
          {JSON.stringify(values, null, 2)}
        </pre>
      </div>
    </div>
  );
}

const meta: Meta<typeof SchemaStory> = {
  title: "Schema Language/Property Types",
  component: SchemaStory,
  tags: ["autodocs"],
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof SchemaStory>;

export const StringProperty: Story = {
  name: "string — Text Input",
  args: {
    description: "A simple string property renders as a text input. The `description` becomes helper text below the field.",
    schema: {
      title: "String Properties",
      type: "object",
      properties: {
        name: { type: "string", title: "Name", description: "A simple text field" },
        pattern: { type: "string", title: "Regex Pattern", description: "With placeholder text", "ui:placeholder": "e.g., ^[A-Z].*" },
        notes: { type: "string", title: "Notes", description: "Multiline when ui:widget is set to 'textarea'", "ui:widget": "textarea" },
      },
    },
  },
};

export const BooleanProperty: Story = {
  name: "boolean — Checkbox",
  args: {
    description: "Boolean properties render as checkboxes. The `default` value is shown when no value is set.",
    schema: {
      title: "Boolean Properties",
      type: "object",
      properties: {
        enabled: { type: "boolean", title: "Enabled", description: "A simple on/off toggle", default: true },
        extractAll: { type: "boolean", title: "Extract All Pairs", description: "Extract all string key-value pairs as translatable blocks", default: false },
      },
    },
  },
};

export const NumberProperty: Story = {
  name: "number / integer — Numeric Input",
  args: {
    description: "Number properties render as numeric inputs. `minimum` and `maximum` set bounds. `default` provides the initial value.",
    schema: {
      title: "Number Properties",
      type: "object",
      properties: {
        threshold: { type: "number", title: "Similarity Threshold", description: "Minimum match score (0.0-1.0)", default: 0.7, minimum: 0, maximum: 1 },
        maxSegments: { type: "integer", title: "Max Segments", description: "Maximum number of segments to process", default: 1000, minimum: 1 },
      },
    },
  },
};

export const EnumProperty: Story = {
  name: "enum — Dropdown Select",
  args: {
    description: "String properties with `enum` render as dropdown selects. Use `ui:enum-labels` for display names and `ui:enum-descriptions` for tooltips.",
    schema: {
      title: "Enum Properties",
      type: "object",
      properties: {
        outputFormat: {
          type: "string",
          title: "Output Format",
          description: "Choose the output format",
          enum: ["json", "yaml", "xml"],
          default: "json",
        },
        severity: {
          type: "string",
          title: "Severity Level",
          description: "With custom labels and descriptions",
          enum: ["error", "warning", "info"],
          default: "warning",
          "ui:enum-labels": { error: "Error (fail build)", warning: "Warning (report only)", info: "Informational" },
          "ui:enum-descriptions": {
            error: "Stops processing and reports the issue as a build failure",
            warning: "Reports the issue but continues processing",
            info: "Logs the issue for information purposes only",
          },
        },
      },
    },
  },
};

export const ObjectProperty: Story = {
  name: "object — Nested Fields",
  args: {
    description: "Object properties create nested field groups. For shallow nesting (depth 1), fields are shown inline. Deeper nesting uses drill-down navigation.",
    schema: {
      title: "Object Properties",
      type: "object",
      properties: {
        parser: {
          type: "object",
          title: "Parser Settings",
          description: "Settings for the document parser",
          properties: {
            assumeWellformed: { type: "boolean", title: "Assume Well-formed", description: "Skip validation of input structure", default: false },
            encoding: { type: "string", title: "Input Encoding", description: "Override auto-detected encoding", default: "UTF-8" },
          },
        },
        output: {
          type: "object",
          title: "Output",
          description: "Output generation settings",
          properties: {
            indent: { type: "integer", title: "Indent Spaces", default: 2, minimum: 0, maximum: 8 },
            trailingNewline: { type: "boolean", title: "Trailing Newline", default: true },
          },
        },
      },
    },
  },
};

export const ArrayProperty: Story = {
  name: "array — List Editor",
  args: {
    description: "Array properties render as lists with add/remove controls. The `items` schema defines the type of each element.",
    schema: {
      title: "Array Properties",
      type: "object",
      properties: {
        extensions: {
          type: "array",
          title: "File Extensions",
          description: "List of file extensions to process",
          items: { type: "string" },
        },
        rules: {
          type: "array",
          title: "Extraction Rules",
          description: "Regex patterns for content extraction",
          items: { type: "string" },
        },
      },
    },
  },
};
