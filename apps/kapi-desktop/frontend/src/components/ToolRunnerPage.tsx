import { useState, useEffect, useMemo } from "react";
import {
  Play,
  FileInput,
  Search,
  BookOpen,
  ExternalLink,
  Wrench,
  Shield,
  ArrowRightLeft,
  Repeat,
  Sparkles,
  Layers,
  ChevronRight,
  Settings2,
  Tag,
  Lock,
  Clock,
} from "lucide-react";
import type { ToolInfo, PluginDocs, PluginDocsSummary, StepDoc } from "../types/api";
import type { ComponentSchema } from "@neokapi/ui-primitives";
import {
  Badge,
  Button,
  SchemaForm,
  Card,
  CardContent,
  Label,
  Input,
  ScrollArea,
  LoadingSpinner,
} from "@neokapi/ui-primitives";
import { t } from "@neokapi/kapi-react/runtime";
import { api } from "../hooks/useApi";
import { useSchemaFormHost } from "../hooks/useSchemaFormHost";
import { useError } from "./ErrorBanner";

// Category metadata for visual treatment. Labels resolve through `t()`
// per-render so they pick up the active locale; if we baked them into
// this module-level object they'd freeze at the fallback language.
const categoryIcons: Record<string, { icon: typeof Wrench; color: string }> = {
  translate: { icon: ArrowRightLeft, color: "text-blue-500 bg-blue-500/10" },
  validate: { icon: Shield, color: "text-emerald-500 bg-emerald-500/10" },
  transform: { icon: Repeat, color: "text-amber-500 bg-amber-500/10" },
  convert: { icon: ArrowRightLeft, color: "text-purple-500 bg-purple-500/10" },
  enrich: { icon: Sparkles, color: "text-rose-500 bg-rose-500/10" },
  pipeline: { icon: Layers, color: "text-cyan-500 bg-cyan-500/10" },
  utility: { icon: Wrench, color: "text-gray-500 bg-gray-500/10" },
};

function categoryLabel(cat: string): string {
  switch (cat) {
    case "translate":
      return t("Translation");
    case "validate":
      return t("Quality & Validation");
    case "transform":
      return t("Transform");
    case "convert":
      return t("Conversion");
    case "enrich":
      return t("Enrichment");
    case "pipeline":
      return t("Pipeline");
    default:
      return t("Utility");
  }
}

function categoryMeta(cat: string) {
  const visual = categoryIcons[cat] ?? categoryIcons.utility;
  return { ...visual, label: categoryLabel(cat) };
}

export interface ToolRunnerPageProps {
  /** Pre-loaded docs for Storybook. */
  docs?: PluginDocs | null;
  /** Pre-loaded tools for Storybook. */
  tools?: ToolInfo[];
  /** When set, tools are scoped to the project's declared plugins. */
  tabID?: string;
}

export function ToolRunnerPage({
  docs: propDocs,
  tools: propTools,
  tabID,
}: ToolRunnerPageProps = {}) {
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
    const toolsPromise = tabID ? api.listProjectTools(tabID) : api.listTools();
    Promise.all([toolsPromise, propDocs ? Promise.resolve(null) : api.getPluginDocsSummary()])
      .then(([t, summary]) => {
        if (t) setTools(t);
        if (summary) setDocsSummary(summary);
      })
      .catch((err) => showError("Failed to load tools", err))
      .finally(() => setLoading(false));
  }, [showError, propDocs, propTools, tabID]);

  // Group tools by category. Category strings the UI doesn't have a
  // label / icon for collapse into `utility` so we don't render N
  // separate "Utility" chips for each unknown category emitted by the
  // registry (other, analysis, text-processing, …). Filter + group
  // must share this normalisation so clicking a chip always yields
  // the same set of tools it counted.
  const normalizeCategory = (raw: string | undefined): string =>
    raw && categoryIcons[raw] ? raw : "utility";

  const categories = useMemo(() => {
    const cats = new Map<string, ToolInfo[]>();
    for (const tool of tools) {
      const cat = normalizeCategory(tool.category);
      if (!cats.has(cat)) cats.set(cat, []);
      cats.get(cat)!.push(tool);
    }
    return cats;
  }, [tools]);

  // Filter tools
  const filteredTools = useMemo(() => {
    let result = tools;
    if (filterCategory) {
      result = result.filter((t) => normalizeCategory(t.category) === filterCategory);
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
            <Search
              size={13}
              className="absolute left-2.5 top-1/2 -translate-y-1/2 text-muted-foreground"
            />
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
              const meta = categoryMeta(cat);
              // The button body is NOT extracted as a block (translate="no")
              // because `meta.label` is already a `t()`-resolved string from
              // `categoryMeta()`. Extracting would wrap the whole body in a
              // second translation, producing `▒ ▒ Utility ▒ (32) ▒` in
              // pseudo. The count is numeric — no translation needed.
              return (
                <Button
                  key={cat}
                  translate="no"
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
        <ScrollArea className="flex-1">
          <div className="p-2">
            {loading ? (
              <LoadingSpinner size="sm" text="Loading tools..." className="px-2 py-4" />
            ) : (
              <div className="space-y-0.5">
                {filteredTools.map((tool) => {
                  const cat = tool.category || "utility";
                  const meta = categoryMeta(cat);
                  const Icon = meta.icon;
                  const hasStepDoc =
                    resolveStepDoc(tool.name, docs) || docsSummary?.stepIDs?.includes(tool.name);
                  const isSelected = selectedTool === tool.name;

                  return (
                    <div
                      key={tool.name}
                      role="button"
                      tabIndex={0}
                      onClick={() => setSelectedTool(tool.name)}
                      onKeyDown={(e) => {
                        if (e.key === "Enter" || e.key === " ") setSelectedTool(tool.name);
                      }}
                      className={`cursor-pointer rounded-lg px-3 py-2.5 text-left transition-colors ${
                        isSelected
                          ? "bg-accent border border-primary/20 shadow-sm"
                          : "hover:bg-accent/50 border border-transparent"
                      }`}
                    >
                      <div className="flex items-start gap-2.5">
                        <div className={`mt-0.5 shrink-0 p-1 rounded ${meta.color}`}>
                          <Icon size={12} />
                        </div>
                        <div className="flex-1 min-w-0">
                          <div className="flex items-center gap-1.5 flex-wrap">
                            <span
                              className={`text-xs font-semibold ${isSelected ? "text-primary" : "text-foreground"}`}
                            >
                              {tool.display_name || tool.name}
                            </span>
                            {tool.source && tool.source !== "built-in" && (
                              <Badge
                                variant="secondary"
                                className="text-[8px] px-1 py-px bg-violet-500/10 text-violet-600 dark:text-violet-400 shrink-0"
                              >
                                {tool.source}
                              </Badge>
                            )}
                            {tool.has_schema && (
                              <Settings2 size={9} className="text-muted-foreground shrink-0" />
                            )}
                            {hasStepDoc && (
                              <BookOpen size={9} className="text-primary/50 shrink-0" />
                            )}
                          </div>
                          <p className="text-[10px] text-muted-foreground line-clamp-2 mt-0.5">
                            {tool.description}
                          </p>
                          {tool.tags && tool.tags.length > 0 && (
                            <div className="flex flex-wrap gap-1 mt-1">
                              {tool.tags.slice(0, 3).map((tag) => (
                                <Badge
                                  key={tag}
                                  variant="secondary"
                                  className="text-[8px] px-1 py-px"
                                >
                                  {tag}
                                </Badge>
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
                    </div>
                  );
                })}
                {filteredTools.length === 0 && !loading && (
                  <p className="px-3 py-4 text-xs text-muted-foreground text-center">
                    {search ? t("No tools match your search.") : t("No tools available.")}
                  </p>
                )}
              </div>
            )}
          </div>
        </ScrollArea>
      </div>

      {/* Right panel: tool detail */}
      <ScrollArea className="flex-1">
        {selectedTool && selectedToolInfo ? (
          <ToolDetail tool={selectedToolInfo} docs={docs} />
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
      </ScrollArea>
    </div>
  );
}

// --- Tool Detail Panel ---

function ToolDetail({ tool, docs }: { tool: ToolInfo; docs: PluginDocs | null }) {
  const [schema, setSchema] = useState<ComponentSchema | null>(null);
  const [config, setConfig] = useState<Record<string, unknown>>({});
  const [loadingSchema, setLoadingSchema] = useState(false);
  const [targetLang, setTargetLang] = useState("");

  const { showError: _showError } = useError();

  // Step documentation — pre-loaded (Storybook) or fetched on demand
  const [stepDoc, setStepDoc] = useState<StepDoc | undefined>(() =>
    resolveStepDoc(tool.name, docs),
  );
  const cat = tool.category || "utility";
  const meta = categoryMeta(cat);
  const Icon = meta.icon;

  // Native file/folder dialogs + credential vault for schema-form path /
  // credential widgets.
  const schemaHost = useSchemaFormHost();

  // Fetch step doc on demand if not pre-loaded
  useEffect(() => {
    setStepDoc(resolveStepDoc(tool.name, docs));
    if (docs) return;
    api
      .getStepDoc(tool.name)
      .then((d) => {
        if (d) setStepDoc(d);
      })
      .catch(() => {});
  }, [tool.name, docs]);

  // Load schema when tool changes — always try, schema may come from plugin schemas
  useEffect(() => {
    setConfig({});
    setSchema(null);
    setLoadingSchema(true);
    api
      .getToolSchema(tool.name)
      .then((s) => {
        if (s) setSchema(s as ComponentSchema);
      })
      .catch(() => {})
      .finally(() => setLoadingSchema(false));
  }, [tool.name]);

  // Ad-hoc tool execution from the desktop runner is not wired up yet: it
  // needs a dedicated RunTool(toolName, inputPaths, targetLang, config)
  // backend method (RunFlow requires a project tab and isn't a fit). Until
  // that API exists the runner controls render in a clearly disabled
  // "coming soon" state rather than as live controls that always error.

  return (
    <div className="p-6">
      {/* Header */}
      <div className="flex items-start gap-3 mb-5">
        <div className={`p-2 rounded-lg ${meta.color}`}>
          <Icon size={20} />
        </div>
        <div className="flex-1 min-w-0">
          <h2 className="text-lg font-semibold text-foreground">
            {tool.display_name || tool.name}
          </h2>
          <p className="text-sm text-muted-foreground mt-0.5">{tool.description}</p>
          <div className="flex items-center gap-2 mt-2">
            {/* meta.label is pre-resolved via t() in categoryMeta(). */}
            <span
              className={`text-[10px] px-2 py-0.5 rounded-full font-medium ${meta.color}`}
              translate="no"
            >
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
          <Card>
            <CardContent className="p-4">
              <SchemaForm
                schema={schema}
                values={config}
                onChange={setConfig}
                paramDocs={stepDoc?.parameters}
                host={schemaHost}
              />
            </CardContent>
          </Card>
        )}

        {/* Runner controls — execution is not yet available from the desktop
            app (see note above), so every control here is rendered disabled
            with an explanatory banner instead of as a live-but-broken control. */}
        <Card>
          <CardContent className="p-4 space-y-3">
            <div
              className="flex items-start gap-2 rounded-md border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-amber-700 dark:text-amber-300"
              role="status"
            >
              <Clock size={14} className="mt-0.5 shrink-0" />
              <div className="text-xs">
                <p className="font-medium">{t("Running tools here is coming soon")}</p>
                <p className="mt-0.5 opacity-80">
                  {t(
                    "Run this tool from the kapi CLI for now. In-app execution is still being wired up.",
                  )}
                </p>
              </div>
            </div>

            <fieldset disabled className="space-y-3 opacity-60">
              <div>
                <Label htmlFor="tool-files" className="mb-1 block">
                  Input Files
                </Label>
                <Button
                  id="tool-files"
                  type="button"
                  variant="outline"
                  disabled
                  title={t("In-app execution is coming soon")}
                  className="flex items-center gap-2 border-dashed text-muted-foreground w-full"
                >
                  <FileInput size={14} />
                  Select files...
                </Button>
              </div>

              {tool.requires?.includes("target-language") && (
                <div>
                  <Label htmlFor="tool-target-lang" className="mb-1 block">
                    Target Language
                  </Label>
                  <Input
                    id="tool-target-lang"
                    type="text"
                    value={targetLang}
                    onChange={(e) => setTargetLang(e.target.value)}
                    placeholder="e.g. fr-FR"
                    disabled
                    className="w-48"
                  />
                </div>
              )}

              <Button type="button" disabled title={t("In-app execution is coming soon")}>
                <Play size={14} />
                {t("Run {name}", { name: tool.display_name || tool.name })}
              </Button>
            </fieldset>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}

// --- Step Metadata Panel ---

function portLabel(f: { type: string; side?: string; optional?: boolean }): string {
  const side = f.side ? `@${f.side}` : "";
  return `${f.type}${side}${f.optional ? " (opt)" : ""}`;
}

function ToolMetadataPanel({ schema }: { schema: ComponentSchema }) {
  const toolMeta = schema.toolMeta;
  if (!toolMeta) return null;

  const consumes = toolMeta.consumes ?? [];
  const produces = toolMeta.produces ?? [];

  if (consumes.length === 0 && produces.length === 0) return null;

  return (
    <div className="flex flex-wrap items-center gap-2 text-[10px]">
      {consumes.map((f) => (
        <span
          key={`in-${f.type}-${f.side ?? ""}`}
          className="flex items-center gap-1 rounded bg-blue-500/10 px-2 py-0.5 text-blue-600 dark:text-blue-400"
        >
          <FileInput size={9} />
          Consumes: {portLabel(f)}
        </span>
      ))}
      {produces.map((f) => (
        <span
          key={`out-${f.type}-${f.side ?? ""}`}
          className="flex items-center gap-1 rounded bg-emerald-500/10 px-2 py-0.5 text-emerald-600 dark:text-emerald-400"
        >
          <Play size={9} />
          Produces: {portLabel(f)}
        </span>
      ))}
    </div>
  );
}

// --- Utility: resolve step doc by tool name ---

function resolveStepDoc(toolName: string, docs: PluginDocs | null): StepDoc | undefined {
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
