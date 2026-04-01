/**
 * Browsers: Format Browser
 *
 * Browse ALL formats (built-in + bridge) with search, filter by source,
 * and click-through to schema detail with live config editor.
 */
import { useState, useMemo } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { SchemaForm } from "@neokapi/flow-editor";
import { FormatConfigEditor } from "../../components/FormatConfigEditor";
import type { ComponentSchema } from "@neokapi/flow-editor";
import formatSchemas from "../fixtures/format-schemas.json";
import formatList from "../fixtures/format-list.json";
import pluginDocs from "../fixtures/plugin-docs.json";
import type { PluginDocs } from "../../types/api";

const docs = pluginDocs as unknown as PluginDocs;
const allSchemas = formatSchemas.all as unknown as ComponentSchema[];

interface FormatEntry {
  name: string;
  displayName: string;
  source: string;
  extensions: string[];
  mimeTypes: string[];
  schema?: ComponentSchema;
}

function buildFormatEntries(): FormatEntry[] {
  const entries: FormatEntry[] = [];

  for (const f of formatList.builtIn) {
    const schema = allSchemas.find(
      (s) => (s as Record<string, unknown>)["x-name"] === f.name,
    );
    entries.push({
      name: f.name,
      displayName: f.display_name || f.name,
      source: "built-in",
      extensions: f.extensions || [],
      mimeTypes: f.mime_types || [],
      schema: schema || undefined,
    });
  }

  for (const f of formatList.bridge) {
    const schema = allSchemas.find(
      (s) => (s as Record<string, unknown>)["x-name"] === f.name,
    );
    entries.push({
      name: f.name,
      displayName: f.display_name || f.name,
      source: "okapi",
      extensions: f.extensions || [],
      mimeTypes: f.mime_types || [],
      schema: schema || undefined,
    });
  }

  return entries;
}

function FormatBrowser() {
  const formats = useMemo(buildFormatEntries, []);
  const [search, setSearch] = useState("");
  const [sourceFilter, setSourceFilter] = useState<"all" | "built-in" | "okapi">("all");
  const [selected, setSelected] = useState<FormatEntry | null>(null);
  const [configValues, setConfigValues] = useState<Record<string, unknown>>({});

  const filtered = useMemo(() => {
    return formats.filter((f) => {
      if (sourceFilter !== "all" && f.source !== sourceFilter) return false;
      if (search) {
        const q = search.toLowerCase();
        return (
          f.name.toLowerCase().includes(q) ||
          f.displayName.toLowerCase().includes(q) ||
          f.extensions.some((e) => e.toLowerCase().includes(q)) ||
          f.mimeTypes.some((m) => m.toLowerCase().includes(q))
        );
      }
      return true;
    });
  }, [formats, search, sourceFilter]);

  const builtInCount = formats.filter((f) => f.source === "built-in").length;
  const okapiCount = formats.filter((f) => f.source === "okapi").length;

  if (selected) {
    return (
      <div style={{ maxWidth: 1100 }}>
        <button
          onClick={() => { setSelected(null); setConfigValues({}); }}
          className="text-sm text-primary hover:underline mb-4"
        >
          &larr; Back to format list
        </button>
        <div style={{ display: "grid", gridTemplateColumns: selected.schema ? "1fr 1fr" : "1fr", gap: 24 }}>
          <div>
            {selected.schema ? (
              <FormatConfigEditor
                schema={selected.schema}
                values={configValues}
                onChange={setConfigValues}
                title={selected.displayName}
              />
            ) : (
              <p className="text-muted-foreground">No schema available for this format.</p>
            )}
          </div>
          {selected.schema && (
            <div style={{ minWidth: 0 }}>
              <h4 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-2">Schema (JSON)</h4>
              <pre className="rounded bg-muted p-3 text-xs text-muted-foreground overflow-auto max-h-96">
                {JSON.stringify(selected.schema, null, 2)}
              </pre>
              <h4 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mt-4 mb-2">Config Values</h4>
              <pre className="rounded bg-muted p-3 text-xs text-muted-foreground overflow-auto max-h-40">
                {JSON.stringify(configValues, null, 2)}
              </pre>
            </div>
          )}
        </div>
      </div>
    );
  }

  return (
    <div style={{ maxWidth: 800 }}>
      <div className="flex items-center gap-3 mb-4">
        <input
          type="text"
          placeholder="Search formats by name, extension, or MIME type..."
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          className="flex-1 rounded-md border bg-background px-3 py-2 text-sm"
        />
        <div className="flex gap-1">
          {(["all", "built-in", "okapi"] as const).map((s) => (
            <button
              key={s}
              onClick={() => setSourceFilter(s)}
              className={`rounded-md px-3 py-1.5 text-xs font-medium transition ${
                sourceFilter === s
                  ? "bg-primary/10 text-primary"
                  : "text-muted-foreground hover:text-foreground"
              }`}
            >
              {s === "all" ? `All (${formats.length})` : s === "built-in" ? `Built-in (${builtInCount})` : `Okapi (${okapiCount})`}
            </button>
          ))}
        </div>
      </div>

      <div className="text-xs text-muted-foreground mb-3">
        {filtered.length} format{filtered.length !== 1 ? "s" : ""}
      </div>

      <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
        {filtered.map((f) => (
          <button
            key={f.name}
            onClick={() => setSelected(f)}
            className="rounded-lg border p-3 text-left transition hover:border-primary/30 hover:bg-primary/5"
          >
            <div className="flex items-center justify-between">
              <span className="text-sm font-medium">{f.displayName}</span>
              <span className={`rounded-full px-2 py-0.5 text-[10px] ${
                f.source === "built-in"
                  ? "bg-emerald-500/10 text-emerald-500"
                  : "bg-blue-500/10 text-blue-500"
              }`}>
                {f.source === "built-in" ? "native" : "okapi"}
              </span>
            </div>
            <div className="flex flex-wrap gap-1 mt-1.5">
              {f.extensions.slice(0, 4).map((ext) => (
                <code key={ext} className="rounded bg-muted px-1 py-0.5 text-[10px] text-muted-foreground">{ext}</code>
              ))}
              {f.extensions.length > 4 && (
                <span className="text-[10px] text-muted-foreground">+{f.extensions.length - 4}</span>
              )}
            </div>
            {!f.schema && (
              <span className="text-[10px] text-muted-foreground/50 mt-1 block">No schema</span>
            )}
          </button>
        ))}
      </div>
    </div>
  );
}

const meta: Meta<typeof FormatBrowser> = {
  title: "Formats & Tools/Browsers/Format Browser",
  component: FormatBrowser,
  tags: ["autodocs"],
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof FormatBrowser>;

export const AllFormats: Story = {};
