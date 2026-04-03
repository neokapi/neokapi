import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { SchemaForm } from "@neokapi/ui-primitives";

import type { ComponentSchema } from "@neokapi/ui-primitives";
import formatSchemas from "../fixtures/format-schemas.json";

type SE = ComponentSchema & { "x-name": string; "x-source": string };
const builtIn = formatSchemas.builtIn as unknown as SE[];
const bridge = formatSchemas.bridge as unknown as SE[];
function findSchema(name: string, source: "builtIn" | "bridge"): SE | undefined {
  return source === "builtIn"
    ? builtIn.find((s) => s["x-name"] === name)
    : bridge.find((s) => s["x-name"] === name);
}

function ConfigEditor({
  schemaName,
  source,
}: {
  schemaName: string;
  source: "builtIn" | "bridge";
}) {
  const schema = findSchema(schemaName, source);
  const [values, setValues] = useState<Record<string, unknown>>({});
  if (!schema) return <p className="text-sm text-muted-foreground">Schema not found.</p>;
  const hasProps = schema.properties && Object.keys(schema.properties).length > 0;
  return (
    <div
      style={{
        display: "grid",
        gridTemplateColumns: hasProps ? "1fr 1fr" : "1fr",
        gap: 24,
        maxWidth: 1100,
      }}
    >
      <div>
        {hasProps ? (
          source === "builtIn" ? (
            <SchemaForm schema={schema} values={values} onChange={setValues} />
          ) : (
            <SchemaForm schema={schema} values={values} onChange={setValues} />
          )
        ) : (
          <p className="text-sm text-muted-foreground">No configurable parameters.</p>
        )}
      </div>
      <div style={{ minWidth: 0 }}>
        <h4 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-2">
          Schema
        </h4>
        <pre className="rounded bg-muted p-3 text-xs text-muted-foreground overflow-auto max-h-[600px]">
          {JSON.stringify(schema, null, 2)}
        </pre>
        {hasProps && (
          <>
            <h4 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mt-4 mb-2">
              Values
            </h4>
            <pre className="rounded bg-muted p-3 text-xs text-muted-foreground overflow-auto max-h-40">
              {JSON.stringify(values, null, 2)}
            </pre>
          </>
        )}
      </div>
    </div>
  );
}

const meta: Meta = {
  title: "Formats & Tools/Formats/Plain Text Filter (basetable)",
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj;

export const OkapiConfig: Story = {
  name: "Configuration",
  render: () => <ConfigEditor schemaName="okf_basetable" source="bridge" />,
};
