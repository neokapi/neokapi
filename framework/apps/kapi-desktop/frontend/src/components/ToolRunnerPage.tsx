import { useState, useEffect, useMemo, useCallback } from "react";
import {
  Play,
  FileInput,
  Loader2,
  Search,
  BookOpen,
  ExternalLink,
  Wrench,
  Zap,
  Shield,
  ArrowRightLeft,
  Repeat,
  Sparkles,
  Layers,
  ChevronRight,
  Settings2,
  Tag,
  Lock,
} from "lucide-react";
import type { ToolInfo, PluginDocs, PluginDocsSummary, StepDoc } from "../types/api";
import type { ComponentSchema } from "@neokapi/ui-primitives";
import { Button, SchemaForm } from "@neokapi/ui-primitives";
import { api } from "../hooks/useApi";
import { useError } from "./ErrorBanner";

// Category metadata for visual treatment
const categoryMeta: Record<
  string,
  { icon: typeof Wrench; color: string; label: string }
> = {
  translate: {
    icon: ArrowRightLeft,
    color: "text-blue-500 bg-blue-500/10",
    label: "Translation",
  },
  validate: {
    icon: Shield,
    color: "text-emerald-500 bg-emerald-500/10",
    label: "Quality & Validation",
  },
  transform: {
    icon: Repeat,
    color: "text-amber-500 bg-amber-500/10",
    label: "Transform",
  },
  convert: {
    icon: ArrowRightLeft,
    color: "text-purple-500 bg-purple-500/10",
    label: "Conversion",
  },
  enrich: {
    icon: Sparkles,
    color: "text-rose-500 bg-rose-500/10",
    label: "Enrichment",
  },
  pipeline: {
    icon: Layers,
    color: "text-cyan-500 bg-cyan-500/10",
    label: "Pipeline",
  },
  utility: {
    icon: Wrench,
    color: "text-gray-500 bg-gray-500/10",
    label: "Utility",
  },
};

export interface ToolRunnerPageProps {
  /** Pre-loaded docs for Storybook. */
  docs?: PluginDocs | null;
  /** Pre-loaded tools for Storybook. */
  tools?: ToolInfo[];
}

export function ToolRunnerPage({ docs: propDocs, tools: propTools }: ToolRunnerPageProps = {}) {
  const [tools, setTools] = useState<ToolInfo[]>(propTools ?? []);
  const [loading, setLoading] = useState(!propTools);
  const [docs] = useState<PluginDocs | null>(propDocs ?? null);
  const [docsSummary, setDocsSummary] = useState<PluginDocsSummary | null>(null);
  const [selectedTool, setSelectedTool] = useState<string | null>(null);
  const [search, setSearch] = useState("");
  const [filterCategory, setFilterCategory] = useState<string | null>(null);

  const { showError } = useError();

  useEffect(() => {
    if (propTools) return;
    Promise.all([
      api.listTools(),
      propDocs ? Promise.resolve(null) : api.getPluginDocsSummary(),
    ])
      .then(([t, summary]) => {
        if (t) setTools(t);
        if (summary) setDocsSummary(summary);
      })
      .catch((err) => showError("Failed to load tools", err))
      .finally(() => setLoading(false));
  }, [showError, propDocs, propTools]);

  // Group tools by category
  const categories = useMemo(() => {
    const cats = new Map<string, ToolInfo[]>();
    for (const tool of tools) {
      const cat = tool.category || "utility";
      if (!cats.has(cat)) cats.set(cat, []);
      cats.get(cat)!.push(tool);
    }
    return cats;
  }, [tools]);

  // Filter tools
  const filteredTools = useMemo(() => {
    let result = tools;
    if (filterCategory) {
      result = result.filter((t) => (t.category || "utility") === filterCategory);
    }
    if (search) {
      const q = search.toLowerCase();
      result = result.filter(
        (t) =>
          t.name.toLowerCase().includes(q) ||
          t.description.toLowerCase().includes(q) ||
          t.tags?.some((tag) => tag.toLowerCase().includes(q)),
      );
    }
    return result;
  }, [tools, search, filterCategory]);

  const selectedToolInfo = tools.find((t) => t.name === selectedTool);

  return (
    <div className="flex h-full">
      {/* Left panel: tool browser */}
      <div className="w-72 shrink-0 border-r border-border flex flex-col overflow-hidden">
        {/* Search */}
        <div className="p-3 border-b border-border">
          <div className="relative">
            <Search size={13} className="absolute left-2.5 top-1/2 -translate-y-1/2 text-muted-foreground" />
            <input
              type="text"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder="Search tools..."
              className="w-full rounded-md border border-input bg-transparent pl-8 pr-3 py-1.5 text-xs outline-none focus:ring-1 focus:ring-ring"
            />
          </div>

          {/* Category filter chips */}
          <div className="flex flex-wrap gap-1 mt-2">
            <Button
              variant={!filterCategory ? "default" : "secondary"}
              size="xs"
              onClick={() => setFilterCategory(null)}
            >
              All ({tools.length})
            </Button>
            {Array.from(categories.entries()).map(([cat, catTools]) => {
              const meta = categoryMeta[cat] || categoryMeta.utility;
              return (
                <Button
                  key={cat}
                  variant={filterCategory === cat ? "default" : "secondary"}
                  size="xs"
                  onClick={() => setFilterCategory(filterCategory === cat ? null : cat)}
                >
                  {meta.label} ({catTools.length})
                </Button>
              );
            })}
          </div>
        </div>

        {/* Tool list */}
        <div className="flex-1 overflow-y-auto p-2">
          {loading ? (
            <div className="flex items-center gap-2 px-2 py-4 text-sm text-muted-foreground">
              <Loader2 size={14} className="animate-spin" />
              Loading tools...
            </div>
          ) : (
            <div className="space-y-0.5">
              {filteredTools.map((tool) => {
                const cat = tool.category || "utility";
                const meta = categoryMeta[cat] || categoryMeta.utility;
                const Icon = meta.icon;
                const hasStepDoc = resolveStepDoc(tool.name, docs) || docsSummary?.stepIDs?.includes(tool.name);
                const isSelected = selectedTool === tool.name;

                return (
                  <Button
                    key={tool.name}
                    variant="ghost"
                    onClick={() => setSelectedTool(tool.name)}
                    className={`w-full h-auto rounded-lg px-3 py-2.5 text-left ${
                      isSelected
                        ? "bg-accent border border-primary/20 shadow-sm"
                        : "hover:bg-accent/50 border border-transparent"
                    }`}
                  >
                    <div className="flex items-start gap-2.5">
                      <div className={`mt-0.5 p-1 rounded ${meta.color}`}>
                        <Icon size={12} />
                      </div>
                      <div className="flex-1 min-w-0">
                        <div className="flex items-center gap-1.5">
                          <span className={`text-xs font-semibold truncate ${isSelected ? "text-primary" : "text-foreground"}`}>
                            {tool.display_name || tool.name}
                          </span>
                          {tool.source && tool.source !== "built-in" && (
                            <span className="text-[8px] px-1 py-px rounded bg-violet-500/10 text-violet-600 dark:text-violet-400 shrink-0 font-medium">
                              {tool.source}
                            </span>
                          )}
                          {tool.has_schema && (
                            <Settings2 size={9} className="text-muted-foreground shrink-0" />
                          )}
                          {hasStepDoc && (
                            <BookOpen size={9} className="text-primary/50 shrink-0" />
                          )}
                        </div>
                        <p className="text-[10px] text-muted-foreground line-clamp-1 mt-0.5">
                          {tool.description}
                        </p>
                        {tool.tags && tool.tags.length > 0 && (
                          <div className="flex gap-1 mt-1">
                            {tool.tags.slice(0, 3).map((tag) => (
                              <span
                                key={tag}
                                className="text-[8px] px-1 py-px rounded bg-muted text-muted-foreground"
                              >
                                {tag}
                              </span>
                            ))}
                          </div>
                        )}
                      </div>
                      <ChevronRight
                        size={12}
                        className={`mt-1 shrink-0 transition-colors ${
                          isSelected ? "text-primary" : "text-muted-foreground/30"
                        }`}
                      />
                    </div>
                  </Button>
                );
              })}
              {filteredTools.length === 0 && !loading && (
                <p className="px-3 py-4 text-xs text-muted-foreground text-center">
                  {search ? "No tools match your search." : "No tools available."}
                </p>
              )}
            </div>
          )}
        </div>
      </div>

      {/* Right panel: tool detail */}
      <div className="flex-1 overflow-y-auto">
        {selectedTool && selectedToolInfo ? (
          <ToolDetail
            tool={selectedToolInfo}
            docs={docs}
          />
        ) : (
          <div className="flex h-full items-center justify-center text-muted-foreground">
            <div className="text-center">
              <Wrench size={32} className="mx-auto mb-3 opacity-20" />
              <p className="text-sm">Select a tool to view details and run it</p>
              <p className="text-xs mt-1 opacity-60">
                {tools.length} tools available across {categories.size} categories
              </p>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

// --- Tool Detail Panel ---

function ToolDetail({
  tool,
  docs,
}: {
  tool: ToolInfo;
  docs: PluginDocs | null;
}) {
  const [schema, setSchema] = useState<ComponentSchema | null>(null);
  const [config, setConfig] = useState<Record<string, unknown>>({});
  const [loadingSchema, setLoadingSchema] = useState(false);
  const [targetLang, setTargetLang] = useState("");
  const [running, setRunning] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const { showError } = useError();

  // Step documentation — pre-loaded (Storybook) or fetched on demand
  const [stepDoc, setStepDoc] = useState<StepDoc | undefined>(
    () => resolveStepDoc(tool.name, docs),
  );
  const cat = tool.category || "utility";
  const meta = categoryMeta[cat] || categoryMeta.utility;
  const Icon = meta.icon;

  // Fetch step doc on demand if not pre-loaded
  useEffect(() => {
    setStepDoc(resolveStepDoc(tool.name, docs));
    if (docs) return;
    api.getStepDoc(tool.name).then((d) => {
      if (d) setStepDoc(d);
    }).catch(() => {});
  }, [tool.name, docs]);

  // Load schema when tool changes — always try, schema may come from plugin schemas
  useEffect(() => {
    setConfig({});
    setSchema(null);
    setError(null);
    setLoadingSchema(true);
    api.getToolSchema(tool.name)
      .then((s) => { if (s) setSchema(s as ComponentSchema); })
      .catch(() => {})
      .finally(() => setLoadingSchema(false));
  }, [tool.name]);

  // TODO: RunFlow requires a project tab — ad-hoc tool execution needs a
  // dedicated RunTool(toolName, inputPaths, targetLang, config) backend method.
  // For now the Run button will error until that API exists.
  const handleRun = useCallback(async () => {
    if (!targetLang && tool.requires?.includes("target-language")) return;
    setRunning(true);
    setError(null);
    try {
      await api.runFlow("", tool.name, [], targetLang);
    } catch (e) {
      setError(String(e));
    } finally {
      setRunning(false);
    }
  }, [tool.name, tool.requires, targetLang]);

  return (
    <div className="p-6">
      {/* Header */}
      <div className="flex items-start gap-3 mb-5">
        <div className={`p-2 rounded-lg ${meta.color}`}>
          <Icon size={20} />
        </div>
        <div className="flex-1 min-w-0">
          <h2 className="text-lg font-semibold text-foreground">{tool.display_name || tool.name}</h2>
          <p className="text-sm text-muted-foreground mt-0.5">
            {tool.description}
          </p>
          <div className="flex items-center gap-2 mt-2">
            <span className={`text-[10px] px-2 py-0.5 rounded-full font-medium ${meta.color}`}>
              {meta.label}
            </span>
            {tool.tags?.map((tag) => (
              <span
                key={tag}
                className="text-[10px] px-1.5 py-0.5 rounded bg-muted text-muted-foreground flex items-center gap-0.5"
              >
                <Tag size={8} />
                {tag}
              </span>
            ))}
            {tool.requires?.map((req) => (
              <span
                key={req}
                className="text-[10px] px-1.5 py-0.5 rounded bg-amber-500/10 text-amber-600 dark:text-amber-400 flex items-center gap-0.5"
              >
                <Lock size={8} />
                {req}
              </span>
            ))}
            {stepDoc?.wikiUrl && (
              <a
                href={stepDoc.wikiUrl}
                target="_blank"
                rel="noopener noreferrer"
                className="text-[10px] text-primary/70 hover:text-primary transition-colors flex items-center gap-0.5 ml-auto"
              >
                <ExternalLink size={9} />
                Wiki
              </a>
            )}
          </div>
        </div>
      </div>

      {/* Configuration form + run controls */}
      <div className="space-y-4">
          {/* Step metadata (I/O types, pipeline params) */}
          {!loadingSchema && schema && <ToolMetadataPanel schema={schema} />}

          {loadingSchema && (
            <div className="py-4 text-center text-sm text-muted-foreground animate-pulse">
              Loading configuration...
            </div>
          )}
          {!loadingSchema && schema && (
            <div className="rounded-lg border border-border bg-card p-4">
              <SchemaForm
                schema={schema}
                values={config}
                onChange={setConfig}
                paramDocs={stepDoc?.parameters}
              />
            </div>
          )}

          {/* Runner controls */}
          <div className="rounded-lg border border-border bg-card p-4 space-y-3">
            <div>
              <label className="mb-1 block text-xs font-medium" htmlFor="tool-files">
                Input Files
              </label>
              <Button
                id="tool-files"
                variant="outline"
                className="flex items-center gap-2 border-dashed text-muted-foreground hover:border-primary/40 hover:text-primary w-full"
              >
                <FileInput size={14} />
                Select files...
              </Button>
            </div>

            {tool.requires?.includes("target-language") && (
              <div>
                <label className="mb-1 block text-xs font-medium" htmlFor="tool-target-lang">
                  Target Language
                </label>
                <input
                  id="tool-target-lang"
                  type="text"
                  value={targetLang}
                  onChange={(e) => setTargetLang(e.target.value)}
                  placeholder="e.g. fr-FR"
                  className="w-48 rounded-md border border-input bg-transparent px-3 py-1.5 text-sm outline-none focus:ring-1 focus:ring-ring"
                />
              </div>
            )}

            {error && (
              <p className="text-xs text-destructive" role="alert">
                {error}
              </p>
            )}

            <Button
              onClick={handleRun}
              disabled={running || (tool.requires?.includes("target-language") && !targetLang)}
            >
              {running ? <Loader2 size={14} className="animate-spin" /> : <Play size={14} />}
              {running ? "Running..." : `Run ${tool.display_name || tool.name}`}
            </Button>
          </div>
      </div>
    </div>
  );
}

// --- Step Metadata Panel ---

const ioTypeLabels: Record<string, string> = {
  "filter-events": "Filter Events",
  "raw-document": "Raw Document",
  file: "File",
};

function ToolMetadataPanel({ schema }: { schema: ComponentSchema }) {
  const toolMeta = schema.toolMeta;
  if (!toolMeta) return null;

  const hasInputs = toolMeta.inputs && toolMeta.inputs.length > 0;
  const hasOutputs = toolMeta.outputs && toolMeta.outputs.length > 0;

  if (!hasInputs && !hasOutputs) return null;

  return (
    <div className="flex flex-wrap items-center gap-2 text-[10px]">
      {hasInputs && toolMeta.inputs!.map((input) => (
        <span key={input} className="flex items-center gap-1 rounded bg-blue-500/10 px-2 py-0.5 text-blue-600 dark:text-blue-400">
          <FileInput size={9} />
          In: {ioTypeLabels[input] || input}
        </span>
      ))}
      {hasOutputs && toolMeta.outputs!.map((output) => (
        <span key={output} className="flex items-center gap-1 rounded bg-emerald-500/10 px-2 py-0.5 text-emerald-600 dark:text-emerald-400">
          <Play size={9} />
          Out: {ioTypeLabels[output] || output}
        </span>
      ))}
    </div>
  );
}

// --- Utility: resolve step doc by tool name ---

function resolveStepDoc(
  toolName: string,
  docs: PluginDocs | null,
): StepDoc | undefined {
  if (!docs?.steps) return undefined;

  // Direct match
  if (docs.steps[toolName]) return docs.steps[toolName];

  // Try common name transforms: "pseudo-translate" → "pseudo_translate", etc.
  const hyphenated = toolName.replace(/_/g, "-");
  if (docs.steps[hyphenated]) return docs.steps[hyphenated];

  const underscored = toolName.replace(/-/g, "_");
  if (docs.steps[underscored]) return docs.steps[underscored];

  return undefined;
}
