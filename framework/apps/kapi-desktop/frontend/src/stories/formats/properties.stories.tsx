import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { SchemaForm, Button } from "@neokapi/ui-primitives";

import type { ComponentSchema } from "@neokapi/ui-primitives";
import formatSchemas from "../fixtures/format-schemas.json";

type SE = ComponentSchema & { "x-name": string; "x-source": string };
const builtIn = formatSchemas.builtIn as unknown as SE[];
const bridge = formatSchemas.bridge as unknown as SE[];
function findSchema(name: string, source: "builtIn" | "bridge"): SE | undefined {
  return source === "builtIn" ? builtIn.find((s) => s["x-name"] === name) : bridge.find((s) => s["x-name"] === name);
}

function ConfigEditor({ schemaName, source }: { schemaName: string; source: "builtIn" | "bridge" }) {
  const schema = findSchema(schemaName, source);
  const [values, setValues] = useState<Record<string, unknown>>({});
  if (!schema) return <p className="text-sm text-muted-foreground">Schema not found.</p>;
  const hasProps = schema.properties && Object.keys(schema.properties).length > 0;
  return (
    <div style={{ display: "grid", gridTemplateColumns: hasProps ? "1fr 1fr" : "1fr", gap: 24, maxWidth: 1100 }}>
      <div>
        {hasProps ? (source === "builtIn"
          ? <SchemaForm schema={schema} values={values} onChange={setValues} />
          : <SchemaForm schema={schema} values={values} onChange={setValues} />
        ) : <p className="text-sm text-muted-foreground">No configurable parameters.</p>}
      </div>
      <div style={{ minWidth: 0 }}>
        <h4 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-2">Schema</h4>
        <pre className="rounded bg-muted p-3 text-xs text-muted-foreground overflow-auto max-h-[600px]">{JSON.stringify(schema, null, 2)}</pre>
        {hasProps && (<>
          <h4 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mt-4 mb-2">Values</h4>
          <pre className="rounded bg-muted p-3 text-xs text-muted-foreground overflow-auto max-h-40">{JSON.stringify(values, null, 2)}</pre>
        </>)}
      </div>
    </div>
  );
}

function CompareEditor({ nativeName, okapiName }: { nativeName: string; okapiName: string }) {
  const ns = findSchema(nativeName, "builtIn");
  const os = findSchema(okapiName, "bridge");
  const [nv, setNv] = useState<Record<string, unknown>>({});
  const [ov, setOv] = useState<Record<string, unknown>>({});
  const [showSchemas, setShowSchemas] = useState(false);
  if (!ns || !os) return <p className="text-sm text-muted-foreground">Schemas not found.</p>;
  const np = Object.keys(ns.properties || {}).length;
  const op = Object.keys(os.properties || {}).length;
  return (
    <div style={{ maxWidth: 1200 }}>
      <div className="mb-4 flex items-center gap-3">
        <span className="rounded-full px-2 py-0.5 text-[10px] bg-emerald-500/10 text-emerald-500">native: {np} params</span>
        <span className="rounded-full px-2 py-0.5 text-[10px] bg-blue-500/10 text-blue-500">okapi: {op} params</span>
        <Button variant="ghost" size="xs" onClick={() => setShowSchemas(!showSchemas)} className="ml-auto">{showSchemas ? "Hide" : "Show"} schemas</Button>
      </div>
      <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 24 }}>
        <div>
          <h3 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-3 flex items-center gap-2"><span className="inline-block w-2 h-2 rounded-full bg-emerald-500" />neokapi native</h3>
          <div className="rounded-lg border p-4">{np > 0 ? <SchemaForm schema={ns} values={nv} onChange={setNv} /> : <p className="text-sm text-muted-foreground">No parameters.</p>}</div>
          {showSchemas && <pre className="mt-2 rounded bg-muted p-3 text-xs text-muted-foreground overflow-auto max-h-80">{JSON.stringify(ns, null, 2)}</pre>}
        </div>
        <div>
          <h3 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-3 flex items-center gap-2"><span className="inline-block w-2 h-2 rounded-full bg-blue-500" />okapi bridge</h3>
          <div className="rounded-lg border p-4">{op > 0 ? <SchemaForm schema={os} values={ov} onChange={setOv} /> : <p className="text-sm text-muted-foreground">No parameters.</p>}</div>
          {showSchemas && <pre className="mt-2 rounded bg-muted p-3 text-xs text-muted-foreground overflow-auto max-h-80">{JSON.stringify(os, null, 2)}</pre>}
        </div>
      </div>
    </div>
  );
}

const meta: Meta = {
  title: "Formats & Tools/Formats/Java Properties Format",
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj;

export const NativeConfig: Story = {
  name: "Native Configuration",
  render: () => <ConfigEditor schemaName="properties" source="builtIn" />,
};
export const OkapiConfig: Story = {
  name: "Okapi Configuration",
  render: () => <ConfigEditor schemaName="okf_properties" source="bridge" />,
};
export const Compare: Story = {
  name: "Side by Side",
  render: () => <CompareEditor nativeName="properties" okapiName="okf_properties" />,
};
