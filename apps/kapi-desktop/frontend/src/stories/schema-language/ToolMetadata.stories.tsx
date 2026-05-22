/**
 * Schema Language: Tool Metadata
 *
 * Demonstrates toolMeta and x-component for tool identification,
 * categorization, I/O types, and how these appear in the tool browser.
 */
import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { SchemaForm } from "@neokapi/ui-primitives";
import type { ComponentSchema } from "@neokapi/ui-primitives";
import { toolSchemas } from "../_lib/reference-data";

function SchemaStory({ schema, description }: { schema: ComponentSchema; description?: string }) {
  const [values, setValues] = useState<Record<string, unknown>>({});
  const meta = schema.toolMeta as Record<string, unknown> | undefined;

  return (
    <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 24, maxWidth: 900 }}>
      <div>
        {description && <p className="text-sm text-muted-foreground mb-4">{description}</p>}

        {/* Tool metadata card */}
        {meta && (
          <div className="rounded-lg border p-4 mb-4 space-y-2">
            <div className="flex items-center gap-2">
              <span className="text-sm font-semibold">
                {(meta.displayName as string) || schema.title}
              </span>
              {typeof meta.category === "string" ? (
                <span className="rounded-full bg-primary/10 text-primary px-2 py-0.5 text-xs">
                  {meta.category}
                </span>
              ) : null}
            </div>
            {typeof meta.description === "string" ? (
              <p className="text-sm text-muted-foreground">{meta.description}</p>
            ) : null}
            <div className="flex flex-wrap gap-2 text-xs">
              {(meta.inputs as string[])?.map((i: string) => (
                <span key={i} className="rounded bg-muted px-1.5 py-0.5">
                  in: {i}
                </span>
              ))}
              {(meta.outputs as string[])?.map((o: string) => (
                <span key={o} className="rounded bg-muted px-1.5 py-0.5">
                  out: {o}
                </span>
              ))}
              {(meta.tags as string[])?.map((t: string) => (
                <span key={t} className="rounded bg-muted px-1.5 py-0.5">
                  #{t}
                </span>
              ))}
              {(meta.requires as string[])?.map((r: string) => (
                <span key={r} className="rounded bg-destructive/10 text-destructive px-1.5 py-0.5">
                  requires: {r}
                </span>
              ))}
            </div>
          </div>
        )}

        <SchemaForm schema={schema} values={values} onChange={setValues} />
      </div>
      <div style={{ minWidth: 0 }}>
        <h4 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-2">
          Tool Metadata
        </h4>
        <pre className="rounded bg-muted p-3 text-xs text-muted-foreground overflow-auto max-h-40">
          {JSON.stringify(meta || {}, null, 2)}
        </pre>
        <h4 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mt-4 mb-2">
          Schema
        </h4>
        <pre className="rounded bg-muted p-3 text-xs text-muted-foreground overflow-auto max-h-60">
          {JSON.stringify(schema, null, 2)}
        </pre>
      </div>
    </div>
  );
}

const meta: Meta<typeof SchemaStory> = {
  title: "Formats & Tools/Schema/Tool Metadata",
  component: SchemaStory,
  tags: ["autodocs"],
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof SchemaStory>;

export const ToolIdentification: Story = {
  name: "toolMeta — ID, Category, I/O, Tags",
  args: {
    description:
      "The `toolMeta` block identifies a tool: its display name, category (translate, validate, transform, etc.), input/output part types, tags, and required context (e.g., needs target-language).",
    schema: {
      title: "Word Count",
      type: "object",
      toolMeta: {
        id: "word-count",
        displayName: "Word Count",
        description: "Count translatable words and segments for cost estimation",
        category: "analysis",
        inputs: ["block"],
        outputs: ["block"],
        tags: ["analysis", "statistics"],
        requires: ["source-language"],
      },
      properties: {
        countWhitespace: { type: "boolean", title: "Count Whitespace", default: false },
        includeProtected: { type: "boolean", title: "Include Protected Content", default: false },
      },
    } as unknown as ComponentSchema,
  },
};

export const RealBuiltInTool: Story = {
  name: "Real Example: Built-in pseudo-translate",
  args: {
    description: "A real built-in tool schema from the neokapi Go codebase.",
    schema: (toolSchemas.builtIn.find((t) => t["x-name"] === "pseudo-translate") ?? {
      title: "pseudo-translate (not found)",
      type: "object",
      properties: {},
    }) as unknown as ComponentSchema,
  },
};

export const RealBridgeTool: Story = {
  name: "Real Example: Okapi Bridge search-and-replace",
  args: {
    description: "A real Okapi bridge tool schema with toolMeta derived from step-metadata.json.",
    schema: (toolSchemas.bridge.find((t) => t["x-name"] === "search-and-replace") ?? {
      title: "search-and-replace (not found)",
      type: "object",
      properties: {},
    }) as unknown as ComponentSchema,
  },
};

export const ToolCategories: Story = {
  name: "Categories — How Tools Are Classified",
  args: {
    description:
      "Tools are classified by category: translate, validate, transform, enrich, convert, pipeline, analysis. The ToolBrowser groups tools by category with distinct colors and icons.",
    schema: {
      title: "Category Examples",
      description:
        "The toolMeta.category field determines grouping and visual treatment in the tool browser.",
      type: "object",
      properties: {},
    },
  },
};
