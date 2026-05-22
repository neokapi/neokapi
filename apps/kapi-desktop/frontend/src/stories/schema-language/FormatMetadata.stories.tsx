/**
 * Schema Language: Format Metadata
 *
 * Demonstrates formatMeta, presets, and how format-specific metadata
 * translates to UI elements in the FormatConfigEditor.
 */
import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { FormatConfigEditor } from "../../components/FormatConfigEditor";
import type { ComponentSchema } from "@neokapi/ui-primitives";
import { formatSchemas } from "../_lib/reference-data";

function SchemaStory({ schema, description }: { schema: ComponentSchema; description?: string }) {
  const [values, setValues] = useState<Record<string, unknown>>({});
  return (
    <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 24, maxWidth: 900 }}>
      <div>
        {description && <p className="text-sm text-muted-foreground mb-4">{description}</p>}
        <FormatConfigEditor
          schema={schema}
          values={values}
          onChange={setValues}
          title={schema.title}
        />
      </div>
      <div style={{ minWidth: 0 }}>
        <h4 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-2">
          formatMeta
        </h4>
        <pre className="rounded bg-muted p-3 text-xs text-muted-foreground overflow-auto max-h-40">
          {JSON.stringify(schema.formatMeta || {}, null, 2)}
        </pre>
        <h4 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mt-4 mb-2">
          Values
        </h4>
        <pre className="rounded bg-muted p-3 text-xs text-muted-foreground overflow-auto max-h-40">
          {JSON.stringify(values, null, 2)}
        </pre>
      </div>
    </div>
  );
}

const meta: Meta<typeof SchemaStory> = {
  title: "Formats & Tools/Schema/Format Metadata",
  component: SchemaStory,
  tags: ["autodocs"],
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof SchemaStory>;

export const FormatIdentification: Story = {
  name: "formatMeta — Format ID, Extensions, MIME Types",
  args: {
    description:
      "The `formatMeta` block identifies a format: its ID, supported file extensions, and MIME types. The FormatConfigEditor renders these as badges in the header.",
    schema: {
      title: "JSON Format",
      description: "Configuration for the JSON file format reader/writer",
      type: "object",
      formatMeta: {
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
    description:
      "A real built-in format schema generated from the neokapi Go codebase. Shows how the actual schema renders.",
    schema: (formatSchemas.builtIn.find((f) => f["x-name"] === "json") ?? {
      title: "JSON (not found)",
      type: "object",
      properties: {},
    }) as unknown as ComponentSchema,
  },
};

export const RealBridgeFormat: Story = {
  name: "Real Example: Okapi Bridge HTML Schema",
  args: {
    description:
      "A real Okapi bridge format schema with formatMeta, ui:widget hints, and complex nested properties.",
    schema: (formatSchemas.bridge.find((f) => f["x-name"] === "okf_html") ?? {
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
      "Properties can use `$ref` to reference shared definitions in `$defs`. The SchemaForm resolves these at render time. This is common in Okapi bridge schemas where multiple properties share the same structure.",
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
