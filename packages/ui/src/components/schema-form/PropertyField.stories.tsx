import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { PropertyField } from "./PropertyField";
import type { PropertySchema } from "./types";

// PropertyField is the single-field renderer that SchemaForm composes for each
// property. It is a controlled input (value in, onChange out); these stories
// wrap it in local state so each widget is interactive, and cover the common
// primitive widget branches (text, number, boolean, enum select).

function FieldHarness({
  name,
  schema,
  initial,
}: {
  name: string;
  schema: PropertySchema;
  initial?: unknown;
}) {
  const [value, setValue] = useState<unknown>(initial);
  return (
    <div className="w-80 rounded-lg border p-4">
      <PropertyField name={name} schema={schema} value={value} onChange={setValue} />
      <pre className="mt-4 rounded bg-muted p-2 text-xs text-muted-foreground">
        {JSON.stringify(value ?? schema.default ?? null)}
      </pre>
    </div>
  );
}

const meta: Meta<typeof PropertyField> = {
  title: "Formats & Tools/Schema/PropertyField",
  component: PropertyField,
  parameters: { layout: "centered" },
};

export default meta;
type Story = StoryObj<typeof PropertyField>;

export const Text: Story = {
  render: () => (
    <FieldHarness
      name="prefix"
      schema={{ type: "string", title: "Prefix", description: "Prefix added to translations" }}
      initial="["
    />
  ),
};

export const Number: Story = {
  render: () => (
    <FieldHarness
      name="expansionPercent"
      schema={{
        type: "integer",
        title: "Expansion Percent",
        description: "Expand text length %",
        default: 30,
        minimum: 0,
        maximum: 200,
      }}
    />
  ),
};

export const Boolean: Story = {
  render: () => (
    <FieldHarness
      name="applyAccents"
      schema={{ type: "boolean", title: "Apply Accents", default: true }}
    />
  ),
};

export const EnumSelect: Story = {
  render: () => (
    <FieldHarness
      name="tone"
      schema={{
        type: "string",
        title: "Tone",
        description: "Rewrite tone",
        enum: ["formal", "neutral", "casual"],
        default: "neutral",
      }}
    />
  ),
};

export const WithError: Story = {
  render: () => (
    <div className="w-80 rounded-lg border p-4">
      <PropertyField
        name="apiKey"
        schema={{ type: "string", title: "API Key", description: "Provider credential" }}
        value=""
        onChange={() => {}}
        error="API key is required"
      />
    </div>
  ),
};
