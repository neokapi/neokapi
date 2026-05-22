/**
 * Shared story helpers for tool/format schema browsing.
 *
 * Each tool / format gets its own .stories.tsx wrapper that defers to
 * the components here so the per-story file is just metadata + a
 * one-line render. Without this every story re-implements `findSchema`,
 * a `ToolConfig` / `ConfigEditor`, and a side-by-side compare panel.
 */
import { useState } from "react";
import { SchemaForm, Button } from "@neokapi/ui-primitives";

import { formatSchemas, toolSchemas, type SchemaEntry } from "./reference-data";

export type SchemaSource = "builtIn" | "bridge";

const tools = {
  builtIn: toolSchemas.builtIn,
  bridge: toolSchemas.bridge,
};
const formats = {
  builtIn: formatSchemas.builtIn,
  bridge: formatSchemas.bridge,
};

function findIn(pool: SchemaEntry[], name: string): SchemaEntry | undefined {
  return pool.find((s) => s["x-name"] === name);
}

interface SchemaPanelProps {
  schema: SchemaEntry | undefined;
  /** Render the schema's own description above the form. Tools do, formats don't. */
  showDescription?: boolean;
}

function SchemaPanel({ schema, showDescription = false }: SchemaPanelProps) {
  const [values, setValues] = useState<Record<string, unknown>>({});
  if (!schema) return <p className="text-sm text-muted-foreground">Schema not found.</p>;
  const hasProps = !!schema.properties && Object.keys(schema.properties).length > 0;
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
        {showDescription && schema.description && (
          <p className="text-sm text-muted-foreground mb-3">{schema.description}</p>
        )}
        {hasProps ? (
          <SchemaForm schema={schema} values={values} onChange={setValues} />
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

interface ConfigStoryProps {
  schemaName: string;
  source: SchemaSource;
}

/** Renders a tool's schema-driven config form + the raw schema/value JSON. */
export function ToolConfig({ schemaName, source }: ConfigStoryProps) {
  return <SchemaPanel schema={findIn(tools[source], schemaName)} showDescription />;
}

/** Same shape as ToolConfig, sourced from format-schemas.json. */
export function FormatConfig({ schemaName, source }: ConfigStoryProps) {
  return <SchemaPanel schema={findIn(formats[source], schemaName)} />;
}

interface FormatCompareProps {
  nativeName: string;
  okapiName: string;
}

/** Side-by-side schema comparison: neokapi native vs okapi-bridge implementation. */
export function FormatCompare({ nativeName, okapiName }: FormatCompareProps) {
  const ns = findIn(formats.builtIn, nativeName);
  const os = findIn(formats.bridge, okapiName);
  const [nv, setNv] = useState<Record<string, unknown>>({});
  const [ov, setOv] = useState<Record<string, unknown>>({});
  const [showSchemas, setShowSchemas] = useState(false);
  if (!ns || !os) return <p className="text-sm text-muted-foreground">Schemas not found.</p>;
  const np = Object.keys(ns.properties || {}).length;
  const op = Object.keys(os.properties || {}).length;
  return (
    <div style={{ maxWidth: 1200 }}>
      <div className="mb-4 flex items-center gap-3">
        <span className="rounded-full px-2 py-0.5 text-[10px] bg-emerald-500/10 text-emerald-500">
          native: {np} params
        </span>
        <span className="rounded-full px-2 py-0.5 text-[10px] bg-blue-500/10 text-blue-500">
          okapi: {op} params
        </span>
        <Button
          variant="ghost"
          size="xs"
          onClick={() => setShowSchemas(!showSchemas)}
          className="ml-auto"
        >
          {showSchemas ? "Hide" : "Show"} schemas
        </Button>
      </div>
      <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 24 }}>
        <CompareColumn
          label="neokapi native"
          dotClass="bg-emerald-500"
          schema={ns}
          count={np}
          values={nv}
          setValues={setNv}
          showSchema={showSchemas}
        />
        <CompareColumn
          label="okapi bridge"
          dotClass="bg-blue-500"
          schema={os}
          count={op}
          values={ov}
          setValues={setOv}
          showSchema={showSchemas}
        />
      </div>
    </div>
  );
}

interface CompareColumnProps {
  label: string;
  dotClass: string;
  schema: SchemaEntry;
  count: number;
  values: Record<string, unknown>;
  setValues: (v: Record<string, unknown>) => void;
  showSchema: boolean;
}

function CompareColumn({
  label,
  dotClass,
  schema,
  count,
  values,
  setValues,
  showSchema,
}: CompareColumnProps) {
  return (
    <div>
      <h3 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-3 flex items-center gap-2">
        <span className={`inline-block w-2 h-2 rounded-full ${dotClass}`} />
        {label}
      </h3>
      <div className="rounded-lg border p-4">
        {count > 0 ? (
          <SchemaForm schema={schema} values={values} onChange={setValues} />
        ) : (
          <p className="text-sm text-muted-foreground">No parameters.</p>
        )}
      </div>
      {showSchema && (
        <pre className="mt-2 rounded bg-muted p-3 text-xs text-muted-foreground overflow-auto max-h-80">
          {JSON.stringify(schema, null, 2)}
        </pre>
      )}
    </div>
  );
}
