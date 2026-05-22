/**
 * Browsers: Tool Browser
 *
 * Browse ALL tools (built-in + bridge) with search, category filter,
 * and click-through to schema detail with live config editor.
 */
import { useState, useMemo } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { SchemaForm, Button, Card, Input } from "@neokapi/ui-primitives";
import type { ComponentSchema } from "@neokapi/ui-primitives";
import { toolSchemas, toolList } from "../_lib/reference-data";

const allSchemas = toolSchemas.all as unknown as ComponentSchema[];

interface ToolEntry {
  name: string;
  displayName: string;
  description: string;
  category: string;
  source: string;
  tags: string[];
  inputs: string[];
  schema?: ComponentSchema;
}

const CATEGORY_COLORS: Record<string, string> = {
  translation: "bg-blue-500/10 text-blue-500",
  translate: "bg-blue-500/10 text-blue-500",
  quality: "bg-emerald-500/10 text-emerald-500",
  validate: "bg-emerald-500/10 text-emerald-500",
  analysis: "bg-amber-500/10 text-amber-500",
  transform: "bg-purple-500/10 text-purple-500",
  enrich: "bg-cyan-500/10 text-cyan-500",
  convert: "bg-orange-500/10 text-orange-500",
  pipeline: "bg-pink-500/10 text-pink-500",
  other: "bg-gray-500/10 text-gray-500",
};

function buildToolEntries(): ToolEntry[] {
  const entries: ToolEntry[] = [];

  for (const t of toolList.builtIn) {
    const schema = allSchemas.find(
      (s) => (s as unknown as Record<string, unknown>)["x-name"] === t.name,
    );
    entries.push({
      name: t.name,
      displayName: t.name,
      description: t.description || "",
      category: t.category || "other",
      source: "built-in",
      tags: [],
      inputs: [],
      schema: schema || undefined,
    });
  }

  for (const t of toolList.bridge) {
    const schema = allSchemas.find(
      (s) => (s as unknown as Record<string, unknown>)["x-name"] === t.name,
    );
    entries.push({
      name: t.name,
      displayName: t.display_name || t.name,
      description: t.description || "",
      category: t.category || "other",
      source: "okapi",
      tags: t.tags || [],
      inputs: t.inputs || [],
      schema: schema || undefined,
    });
  }

  return entries;
}

function ToolBrowser() {
  const tools = useMemo(buildToolEntries, []);
  const [search, setSearch] = useState("");
  const [categoryFilter, setCategoryFilter] = useState<string>("all");
  const [sourceFilter, setSourceFilter] = useState<"all" | "built-in" | "okapi">("all");
  const [selected, setSelected] = useState<ToolEntry | null>(null);
  const [configValues, setConfigValues] = useState<Record<string, unknown>>({});

  const categories = useMemo(() => {
    const cats = new Set(tools.map((t) => t.category));
    return Array.from(cats).sort();
  }, [tools]);

  const filtered = useMemo(() => {
    return tools.filter((t) => {
      if (sourceFilter !== "all" && t.source !== sourceFilter) return false;
      if (categoryFilter !== "all" && t.category !== categoryFilter) return false;
      if (search) {
        const q = search.toLowerCase();
        return (
          t.name.toLowerCase().includes(q) ||
          t.displayName.toLowerCase().includes(q) ||
          t.description.toLowerCase().includes(q) ||
          t.tags.some((tag) => tag.toLowerCase().includes(q))
        );
      }
      return true;
    });
  }, [tools, search, categoryFilter, sourceFilter]);

  // Group by category
  const grouped = useMemo(() => {
    const groups: Record<string, ToolEntry[]> = {};
    for (const t of filtered) {
      const cat = t.category || "other";
      if (!groups[cat]) groups[cat] = [];
      groups[cat].push(t);
    }
    return Object.entries(groups).sort(([a], [b]) => a.localeCompare(b));
  }, [filtered]);

  if (selected) {
    return (
      <div style={{ maxWidth: 1100 }}>
        <Button
          variant="link"
          size="sm"
          onClick={() => {
            setSelected(null);
            setConfigValues({});
          }}
          className="mb-4 px-0"
        >
          &larr; Back to tool list
        </Button>
        <Card className="p-4 mb-4">
          <div className="flex items-center gap-2 mb-1">
            <h3 className="text-lg font-semibold">{selected.displayName}</h3>
            <span
              className={`rounded-full px-2 py-0.5 text-[10px] ${CATEGORY_COLORS[selected.category] || CATEGORY_COLORS.other}`}
            >
              {selected.category}
            </span>
            <span
              className={`rounded-full px-2 py-0.5 text-[10px] ${
                selected.source === "built-in"
                  ? "bg-emerald-500/10 text-emerald-500"
                  : "bg-blue-500/10 text-blue-500"
              }`}
            >
              {selected.source === "built-in" ? "native" : "okapi"}
            </span>
          </div>
          {selected.description && (
            <p className="text-sm text-muted-foreground">{selected.description}</p>
          )}
          {selected.tags.length > 0 && (
            <div className="flex gap-1 mt-2">
              {selected.tags.map((t) => (
                <span key={t} className="rounded bg-muted px-1.5 py-0.5 text-[10px]">
                  #{t}
                </span>
              ))}
            </div>
          )}
        </Card>
        <div
          style={{
            display: "grid",
            gridTemplateColumns: selected.schema ? "1fr 1fr" : "1fr",
            gap: 24,
          }}
        >
          <div>
            {selected.schema && Object.keys(selected.schema.properties || {}).length > 0 ? (
              <SchemaForm
                schema={selected.schema}
                values={configValues}
                onChange={setConfigValues}
              />
            ) : (
              <p className="text-sm text-muted-foreground">
                {selected.schema
                  ? "This tool has no configurable parameters."
                  : "No schema available."}
              </p>
            )}
          </div>
          {selected.schema && (
            <div style={{ minWidth: 0 }}>
              <h4 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-2">
                Schema (JSON)
              </h4>
              <pre className="rounded bg-muted p-3 text-xs text-muted-foreground overflow-auto max-h-96">
                {JSON.stringify(selected.schema, null, 2)}
              </pre>
              <h4 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mt-4 mb-2">
                Config Values
              </h4>
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
        <Input
          type="text"
          placeholder="Search tools by name, description, or tag..."
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          className="flex-1"
        />
        <select
          value={sourceFilter}
          onChange={(e) => setSourceFilter(e.target.value as "all" | "built-in" | "okapi")}
          className="rounded-md border bg-background px-2 py-2 text-xs"
        >
          <option value="all">All sources</option>
          <option value="built-in">Built-in</option>
          <option value="okapi">Okapi</option>
        </select>
      </div>

      <div className="flex flex-wrap gap-1 mb-4">
        <Button
          variant="ghost"
          size="xs"
          onClick={() => setCategoryFilter("all")}
          className={
            categoryFilter === "all"
              ? "bg-primary/10 text-primary font-medium"
              : "text-muted-foreground"
          }
        >
          All ({filtered.length})
        </Button>
        {categories.map((cat) => {
          const count = tools.filter(
            (t) => t.category === cat && (sourceFilter === "all" || t.source === sourceFilter),
          ).length;
          return (
            <Button
              key={cat}
              variant="ghost"
              size="xs"
              onClick={() => setCategoryFilter(cat)}
              className={
                categoryFilter === cat
                  ? "bg-primary/10 text-primary font-medium"
                  : "text-muted-foreground"
              }
            >
              {cat} ({count})
            </Button>
          );
        })}
      </div>

      {grouped.map(([category, catTools]) => (
        <div key={category} className="mb-6">
          <h3 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-2">
            {category} ({catTools.length})
          </h3>
          <div className="space-y-1">
            {catTools.map((t) => (
              <Button
                key={t.name}
                variant="outline"
                onClick={() => setSelected(t)}
                className="w-full h-auto whitespace-normal rounded-lg p-3 text-left flex-col items-start hover:border-primary/30 hover:bg-primary/5"
              >
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-2">
                    <code className="text-sm font-medium">{t.name}</code>
                    <span
                      className={`rounded-full px-1.5 py-0.5 text-[9px] ${
                        t.source === "built-in"
                          ? "bg-emerald-500/10 text-emerald-500"
                          : "bg-blue-500/10 text-blue-500"
                      }`}
                    >
                      {t.source === "built-in" ? "native" : "okapi"}
                    </span>
                  </div>
                  {!t.schema && (
                    <span className="text-[10px] text-muted-foreground/50">no schema</span>
                  )}
                </div>
                {t.description && (
                  <p className="text-xs text-muted-foreground mt-0.5 line-clamp-1">
                    {t.description}
                  </p>
                )}
              </Button>
            ))}
          </div>
        </div>
      ))}
    </div>
  );
}

const meta: Meta<typeof ToolBrowser> = {
  title: "Formats & Tools/Browsers/Tool Browser",
  component: ToolBrowser,
  tags: ["autodocs"],
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof ToolBrowser>;

export const AllTools: Story = {};
