import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { ParameterField } from "./ParameterField";
import type { PropertySchema } from "./types";

// ParameterField renders a single schema-driven format/tool parameter and is a
// controlled input: it reads `value` and reports edits through `onChange`. The
// stories wrap it in local state so the widgets are interactive in Storybook.

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
      <ParameterField name={name} schema={schema} value={value} onChange={setValue} />
      <pre className="mt-4 rounded bg-muted p-2 text-xs text-muted-foreground">
        {JSON.stringify(value ?? schema.default ?? null)}
      </pre>
    </div>
  );
}

const meta: Meta<typeof ParameterField> = {
  title: "Formats & Tools/Schema/ParameterField",
  component: ParameterField,
  parameters: { layout: "centered" },
};

export default meta;
type Story = StoryObj<typeof ParameterField>;

export const TextField: Story = {
  render: () => (
    <FieldHarness
      name="prefix"
      schema={{ type: "string", description: "Prefix added to translations", default: "[" }}
    />
  ),
};

export const NumberField: Story = {
  render: () => (
    <FieldHarness
      name="expansionPercent"
      schema={{
        type: "integer",
        description: "Expand text length %",
        default: 30,
        minimum: 0,
        maximum: 200,
      }}
    />
  ),
};

export const BooleanField: Story = {
  render: () => (
    <FieldHarness
      name="applyAccents"
      schema={{ type: "boolean", description: "Apply diacritics to characters" }}
      initial={true}
    />
  ),
};

export const EnumField: Story = {
  render: () => (
    <FieldHarness
      name="tone"
      schema={{
        type: "string",
        description: "Rewrite tone",
        enum: ["formal", "neutral", "casual"],
        default: "neutral",
      }}
    />
  ),
};
