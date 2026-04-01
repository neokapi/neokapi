/**
 * Schema Language: Format Metadata
 *
 * Demonstrates x-format, presets, and how format-specific metadata
 * translates to UI elements in the FormatConfigEditor.
 */
import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { SchemaForm } from "@neokapi/flow-editor";
import { FormatConfigEditor } from "../../components/FormatConfigEditor";
import type { ComponentSchema } from "@neokapi/flow-editor";
import formatSchemas from "../fixtures/format-schemas.json";

function SchemaStory({ schema, description }: { schema: ComponentSchema; description?: string }) {
  const [values, setValues] = useState<Record<string, unknown>>({});
  return (
    <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 24, maxWidth: 900 }}>
      <div>
        {description && <p className="text-sm text-muted-foreground mb-4">{description}</p>}
        <FormatConfigEditor schema={schema} values={values} onChange={setValues} title={schema.title} />
      </div>
      <div>
        <h4 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-2">x-format metadata</h4>
        <pre className="rounded bg-muted p-3 text-xs text-muted-foreground overflow-auto max-h-40">
          {JSON.stringify(schema["x-format"] || {}, null, 2)}
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
  title: "Schema Language/Format Metadata",
  component: SchemaStory,
  tags: ["autodocs"],
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof SchemaStory>;

export const FormatIdentification: Story = {
  name: "x-format — Format ID, Extensions, MIME Types",
  args: {
    description:
      'The `x-format` block identifies a format: its ID, supported file extensions, and MIME types. The FormatConfigEditor renders these as badges in the header.',
    schema: {
      title: "JSON Format",
      description: "Configuration for the JSON file format reader/writer",
      type: "object",
      "x-format": {
        id: "json",
        extensions: [".json", ".jsonc", ".json5"],
        mimeTypes: ["application/json"],
      },
      properties: {
        extractAllPairs: { type: "boolean", title: "Extract All Pairs", default: true },
      },
    },
  },
};

export const RealBuiltInFormat: Story = {
  name: "Real Example: Built-in JSON Schema",
  args: {
    description: "A real built-in format schema generated from the neokapi Go codebase. Shows how the actual schema renders.",
    schema: (formatSchemas.builtIn.find((f: Record<string, unknown>) => f["x-name"] === "json") ?? {
      title: "JSON (not found)",
      type: "object",
      properties: {},
    }) as unknown as ComponentSchema,
  },
};

export const RealBridgeFormat: Story = {
  name: "Real Example: Okapi Bridge HTML Schema",
  args: {
    description: "A real Okapi bridge format schema with x-format metadata, x-editor hints, and complex nested properties.",
    schema: (formatSchemas.bridge.find((f: Record<string, unknown>) => f["x-name"] === "okf_html") ?? {
      title: "HTML (not found)",
      type: "object",
      properties: {},
    }) as unknown as ComponentSchema,
  },
};

export const RefResolution: Story = {
  name: "$ref + $defs — Schema References",
  args: {
    description:
      'Properties can use `$ref` to reference shared definitions in `$defs`. The SchemaForm resolves these at render time. This is common in Okapi bridge schemas where multiple properties share the same structure.',
    schema: {
      title: "Schema with $defs",
      type: "object",
      $defs: {
        ruleEntry: {
          type: "object",
          properties: {
            pattern: { type: "string", title: "Pattern" },
            action: { type: "string", title: "Action", enum: ["extract", "skip", "protect"] },
          },
        },
      },
      properties: {
        extractionRules: { title: "Extraction Rules", $ref: "#/$defs/ruleEntry" },
        protectionRules: { title: "Protection Rules", $ref: "#/$defs/ruleEntry" },
      },
    } as unknown as ComponentSchema,
  },
};
