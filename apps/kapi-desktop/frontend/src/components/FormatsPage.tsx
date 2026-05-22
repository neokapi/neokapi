import { useState, useEffect, useCallback, useMemo } from "react";
import { t } from "@neokapi/kapi-react/runtime";
import { useWailsEvent } from "../hooks/useWailsEvent";
import Markdown from "react-markdown";
import remarkGfm from "remark-gfm";
import {
  FileText,
  ArrowLeft,
  FileInput,
  FileOutput,
  Plug,
  Settings2,
  Save,
  Play,
  ChevronDown,
  ChevronRight,
  X,
  BookOpen,
} from "lucide-react";
import type { FormatInfo, PluginDocs, PluginDocsSummary, FilterDoc } from "../types/api";
import type { ComponentSchema } from "@neokapi/ui-primitives";
import {
  Button,
  SchemaForm,
  Card,
  CardContent,
  Skeleton,
  Input,
  Tabs,
  TabsList,
  TabsTrigger,
  TabsContent,
  ScrollArea,
  PageHeader,
  ItemCard,
} from "@neokapi/ui-primitives";
import { api } from "../hooks/useApi";
import { useSchemaFormHost } from "../hooks/useSchemaFormHost";
import { useError } from "./ErrorBanner";
import { DocsPanel } from "./DocsPanel";

export interface FormatsPageProps {
  /** Pre-loaded docs for Storybook — in real app, individual docs fetched on demand. */
  docs?: PluginDocs | null;
  /** Pre-loaded formats for Storybook. */
  formats?: FormatInfo[];
  /** Pre-loaded schemas keyed by format name, for Storybook. */
  schemas?: Record<string, ComponentSchema>;
  /** Pre-loaded presets keyed by format name, for Storybook. */
  presets?: Record<string, PresetInfoItem[]>;
  /** Force loading/skeleton state (for Storybook). */
  forceLoading?: boolean;
}

export function FormatsPage({
  docs: propDocs,
  formats: propFormats,
  schemas: propSchemas,
  presets: propPresets,
  forceLoading = false,
}: FormatsPageProps = {}) {
  const [formats, setFormats] = useState<FormatInfo[]>(propFormats ?? []);
  const [loading, setLoading] = useState(forceLoading || !propFormats);
  const [selectedFormat, setSelectedFormat] = useState<string | null>(null);
  const [search, setSearch] = useState("");
  const [docs] = useState<PluginDocs | null>(propDocs ?? null);
  const [docsSummary, setDocsSummary] = useState<PluginDocsSummary | null>(null);

  const { showError } = useError();

  useEffect(() => {
    if (propFormats || forceLoading) return;
    Promise.all([api.listFormats(), propDocs ? Promise.resolve(null) : api.getPluginDocsSummary()])
      .then(([f, summary]) => {
        if (f) setFormats(f);
        if (summary) setDocsSummary(summary);
      })
      .catch((err) => showError("Failed to load formats", err))
      .finally(() => setLoading(false));
  }, [showError, propDocs, propFormats]);

  // Refresh when plugins change (formats may have been added/removed).
  useWailsEvent("registries-changed", () => {
    api
      .listFormats()
      .then((f) => {
        if (f) setFormats(f);
      })
      .catch(() => {});
    api
      .getPluginDocsSummary()
      .then((s) => {
        if (s) setDocsSummary(s);
      })
      .catch(() => {});
  });

  // Total documented count for display
  const documentedCount = propDocs
    ? Object.keys(propDocs.filters).length
    : (docsSummary?.filterIDs?.length ?? 0);

  const filtered = search
    ? formats.filter(
        (f) =>
          f.name.toLowerCase().includes(search.toLowerCase()) ||
          f.display_name?.toLowerCase().includes(search.toLowerCase()) ||
          f.extensions?.some((e) => e.toLowerCase().includes(search.toLowerCase())),
      )
    : formats;

  // Group by source
  const builtIn = filtered.filter((f) => f.source === "built-in" || !f.source);
  const plugin = filtered.filter((f) => f.source && f.source !== "built-in");

  if (selectedFormat) {
    return (
      <FormatDetail
        formatName={selectedFormat}
        formatInfo={formats.find((f) => f.name === selectedFormat)}
        docs={docs}
        propSchema={propSchemas?.[selectedFormat]}
        propPresets={propPresets?.[selectedFormat]}
        onBack={() => setSelectedFormat(null)}
      />
    );
  }

  return (
    <div className="p-6">
      <PageHeader
        title="Formats"
        actions={
          documentedCount > 0 ? (
            <span className="text-[10px] text-muted-foreground">
              {documentedCount} documented formats
            </span>
          ) : undefined
        }
      />

      {/* Search */}
      <div className="relative mb-4">
        <input
          type="text"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="Search formats by name or extension..."
          className="w-full rounded-md border border-input bg-transparent pl-8 pr-3 py-2 text-sm outline-none focus:ring-1 focus:ring-ring"
        />
        <svg
          className="absolute left-2.5 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-muted-foreground"
          xmlns="http://www.w3.org/2000/svg"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="2"
          strokeLinecap="round"
          strokeLinejoin="round"
        >
          <circle cx="11" cy="11" r="8" />
          <path d="m21 21-4.3-4.3" />
        </svg>
      </div>

      {loading && (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-2">
          {[0, 1, 2, 3, 4, 5].map((i) => (
            <ItemCard key={i}>
              <div className="flex items-start gap-3">
                <Skeleton className="mt-0.5 h-5 w-5 shrink-0 rounded" />
                <div className="min-w-0 flex-1">
                  <Skeleton className="h-4 w-2/3" />
                  <div className="mt-2 flex gap-1">
                    <Skeleton className="h-4 w-10 rounded" />
                    <Skeleton className="h-4 w-10 rounded" />
                    <Skeleton className="h-4 w-12 rounded" />
                  </div>
                  <Skeleton className="mt-2 h-3 w-24" />
                </div>
              </div>
            </ItemCard>
          ))}
        </div>
      )}

      {!loading && (
        <>
          {builtIn.length > 0 && (
            <FormatSection
              title="Built-in Formats"
              formats={builtIn}
              docs={docs}
              docsSummary={docsSummary}
              onSelect={setSelectedFormat}
            />
          )}
          {plugin.length > 0 && (
            <FormatSection
              title="Plugin Formats"
              formats={plugin}
              docs={docs}
              docsSummary={docsSummary}
              onSelect={setSelectedFormat}
            />
          )}
          {filtered.length === 0 && (
            <div className="py-12 text-center text-muted-foreground">
              <p className="text-sm">
                {search ? t("No formats match your search.") : t("No formats available.")}
              </p>
            </div>
          )}
        </>
      )}
    </div>
  );
}

function FormatSection({
  title,
  formats,
  docs,
  docsSummary,
  onSelect,
}: {
  title: string;
  formats: FormatInfo[];
  docs: PluginDocs | null;
  docsSummary?: PluginDocsSummary | null;
  onSelect: (name: string) => void;
}) {
  const documentedIDs = useMemo(() => {
    if (docs) return new Set(Object.keys(docs.filters));
    if (docsSummary?.filterIDs) return new Set(docsSummary.filterIDs);
    return new Set<string>();
  }, [docs, docsSummary]);
  return (
    <div className="mb-6">
      <h2 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-2">
        {title}
      </h2>
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-2">
        {formats.map((f) => {
          const filterDoc = resolveFilterDoc(f.name, docs);
          const hasDocs = filterDoc || documentedIDs.has(f.name);
          return (
            <ItemCard key={f.name} clickable onClick={() => onSelect(f.name)}>
              <div className="flex items-start gap-3">
                <FileText
                  size={18}
                  className="mt-0.5 text-muted-foreground group-hover:text-primary transition-colors shrink-0"
                />
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2">
                    <span className="text-sm font-semibold text-foreground group-hover:text-primary transition-colors truncate">
                      {f.display_name || f.name}
                    </span>
                    {f.display_name && (
                      <span className="text-[10px] px-1.5 py-px rounded bg-muted text-muted-foreground font-mono shrink-0">
                        {f.name}
                      </span>
                    )}
                    {f.source && f.source !== "built-in" && (
                      <Plug size={10} className="text-muted-foreground shrink-0" />
                    )}
                  </div>

                  {/* Doc overview snippet (only when pre-loaded, e.g. Storybook) */}
                  {filterDoc && (
                    <p className="mt-1 text-[11px] leading-snug text-muted-foreground line-clamp-2">
                      {filterDoc.overview}
                    </p>
                  )}

                  {f.extensions && f.extensions.length > 0 && (
                    <div className="flex flex-wrap gap-1 mt-1.5">
                      {f.extensions.map((ext) => (
                        <span
                          key={ext}
                          className="text-[10px] px-1.5 py-px rounded bg-muted text-muted-foreground font-mono"
                        >
                          {ext}
                        </span>
                      ))}
                    </div>
                  )}
                  <div className="flex items-center gap-2 mt-1.5 text-[10px] text-muted-foreground">
                    {f.has_reader && (
                      <span className="flex items-center gap-0.5">
                        <FileInput size={9} /> Read
                      </span>
                    )}
                    {f.has_writer && (
                      <span className="flex items-center gap-0.5">
                        <FileOutput size={9} /> Write
                      </span>
                    )}
                    {f.has_schema && (
                      <span className="flex items-center gap-0.5">
                        <Settings2 size={9} /> Configurable
                      </span>
                    )}
                    {hasDocs && (
                      <span className="flex items-center gap-0.5">
                        <BookOpen size={9} /> Docs
                      </span>
                    )}
                  </div>
                </div>
              </div>
            </ItemCard>
          );
        })}
      </div>
    </div>
  );
}

// --- Preset Info type ---

interface PresetInfoItem {
  name: string;
  description: string;
  format: string;
  config?: Record<string, unknown>;
  source?: string;
}

// --- Format Part Info type ---

interface FormatPartInfo {
  type: string;
  id: string;
  summary: string;
  source_text?: string;
  properties?: Record<string, string>;
}

function FormatDetail({
  formatName,
  formatInfo,
  docs,
  propSchema,
  propPresets,
  onBack,
}: {
  formatName: string;
  formatInfo?: FormatInfo;
  docs: PluginDocs | null;
  propSchema?: ComponentSchema;
  propPresets?: PresetInfoItem[];
  onBack: () => void;
}) {
  const [schema, setSchema] = useState<ComponentSchema | null>(propSchema ?? null);
  const [presets, setPresets] = useState<PresetInfoItem[]>(propPresets ?? []);
  const [loadingSchema, setLoadingSchema] = useState(!propSchema);
  const [config, setConfig] = useState<Record<string, unknown>>({});
  const [selectedPreset, setSelectedPreset] = useState<string | null>(null);
  const [activeTab, setActiveTab] = useState<string>("editor");

  // Phase 2: Save-as-preset dialog
  const [showSavePreset, setShowSavePreset] = useState(false);
  const [savePresetName, setSavePresetName] = useState("");
  const [saving, setSaving] = useState(false);

  // Phase 3: YAML config view
  const [yamlText, setYamlText] = useState("");
  const [yamlError, setYamlError] = useState<string | null>(null);

  // Phase 4: Format runner
  const [runnerParts, setRunnerParts] = useState<FormatPartInfo[] | null>(null);
  const [runnerLoading, setRunnerLoading] = useState(false);

  // Filter documentation — pre-loaded (Storybook) or fetched on demand
  const [filterDoc, setFilterDoc] = useState<FilterDoc | undefined>(() =>
    resolveFilterDoc(formatName, docs),
  );

  // Native file/folder dialogs + credential vault for schema-form path /
  // credential widgets.
  const schemaHost = useSchemaFormHost();

  const { showError } = useError();

  // Fetch filter doc on demand if not pre-loaded
  useEffect(() => {
    if (docs) return; // Already have pre-loaded docs
    api
      .getFilterDoc(formatName)
      .then((d) => {
        if (d) setFilterDoc(d);
      })
      .catch(() => {});
  }, [formatName, docs]);

  // Get preset values for modified-from-preset indicator
  const presetValues = useMemo(() => {
    if (!selectedPreset) return undefined;
    const preset = presets.find((p) => p.name === selectedPreset);
    return preset?.config;
  }, [selectedPreset, presets]);

  // Load schema and presets (skip if pre-loaded via props)
  const loadPresets = useCallback(() => {
    if (propPresets) return Promise.resolve();
    return api.listFormatPresets(formatName).then((p) => {
      if (p) setPresets(p as PresetInfoItem[]);
    });
  }, [formatName, propPresets]);

  useEffect(() => {
    if (propSchema) return;
    setLoadingSchema(true);
    Promise.all([api.getFormatSchema(formatName), loadPresets()])
      .then(([s]) => {
        if (s) setSchema(s as ComponentSchema);
      })
      .catch((err) => showError("Failed to load format details", err))
      .finally(() => setLoadingSchema(false));
  }, [formatName, showError, loadPresets, propSchema]);

  // Phase 3: Update YAML when switching to YAML tab or config changes
  useEffect(() => {
    if (activeTab === "yaml") {
      api
        .renderFormatConfig(formatName, config, "yaml")
        .then((text) => {
          if (text !== null) {
            setYamlText(text);
            setYamlError(null);
          }
        })
        .catch(() => {
          // Fall back to JSON stringify
          setYamlText(JSON.stringify(config, null, 2));
        });
    }
  }, [activeTab, config, formatName]);

  const handlePresetSelect = useCallback(
    (presetName: string) => {
      const preset = presets.find((p) => p.name === presetName);
      if (preset?.config) {
        setConfig(preset.config);
        setSelectedPreset(presetName);
      }
    },
    [presets],
  );

  // Phase 2: Save preset handler
  const handleSavePreset = useCallback(async () => {
    if (!savePresetName.trim()) return;
    setSaving(true);
    try {
      await api.saveFormatPreset(formatName, savePresetName.trim(), config);
      await loadPresets();
      setSelectedPreset(savePresetName.trim());
      setShowSavePreset(false);
      setSavePresetName("");
    } catch (err) {
      showError("Failed to save preset", err);
    } finally {
      setSaving(false);
    }
  }, [formatName, savePresetName, config, loadPresets, showError]);

  // Phase 2: Delete preset handler
  const handleDeletePreset = useCallback(
    async (presetName: string) => {
      try {
        await api.deleteFormatPreset(formatName, presetName);
        await loadPresets();
        if (selectedPreset === presetName) {
          setSelectedPreset(null);
        }
      } catch (err) {
        showError("Failed to delete preset", err);
      }
    },
    [formatName, loadPresets, selectedPreset, showError],
  );

  // Phase 3: YAML text change handler (parse back to config)
  const handleYamlChange = useCallback((text: string) => {
    setYamlText(text);
    try {
      const parsed = JSON.parse(text);
      if (typeof parsed === "object" && parsed !== null) {
        setConfig(parsed);
        setYamlError(null);
      }
    } catch {
      // Try parsing as YAML-like (simple key: value pairs)
      setYamlError("Edit the config in JSON format to sync changes back to the editor");
    }
  }, []);

  // Phase 4: Run format reader
  const handleRunReader = useCallback(async () => {
    setRunnerLoading(true);
    try {
      const parts = await api.runFormatReaderDialog(formatName, config);
      if (parts) {
        setRunnerParts(parts as FormatPartInfo[]);
      }
    } catch (err) {
      showError("Failed to run format reader", err);
    } finally {
      setRunnerLoading(false);
    }
  }, [formatName, config, showError]);

  const hasDocumentation = !!filterDoc;

  return (
    <div className="p-6">
      {/* Header */}
      <div className="flex items-center gap-3 mb-6">
        <Button variant="ghost" size="icon-xs" onClick={onBack}>
          <ArrowLeft size={16} />
        </Button>
        <FileText size={20} className="text-primary" />
        <div className="flex-1 min-w-0">
          <h1 className="text-lg font-semibold">
            {filterDoc?.filterName || formatInfo?.display_name || formatName}
          </h1>
          <div className="flex items-center gap-2 mt-0.5">
            {formatInfo?.extensions?.map((ext) => (
              <span
                key={ext}
                className="text-[10px] px-1.5 py-px rounded bg-muted text-muted-foreground font-mono"
              >
                {ext}
              </span>
            ))}
            {formatInfo?.mime_types?.map((mt) => (
              <span
                key={mt}
                className="text-[10px] px-1.5 py-px rounded bg-muted text-muted-foreground"
              >
                {mt}
              </span>
            ))}
          </div>
        </div>
      </div>

      {/* Overview from docs */}
      {filterDoc && (
        <div className="mb-6 rounded-lg border border-primary/15 bg-primary/[0.03] px-4 py-3 text-[13px] leading-relaxed text-foreground/85 [&_a]:text-primary/80 [&_a]:underline [&_a]:underline-offset-2 [&_code]:px-1 [&_code]:py-px [&_code]:rounded [&_code]:bg-muted [&_code]:text-[0.9em] [&_code]:font-mono">
          <Markdown remarkPlugins={[remarkGfm]}>{filterDoc.overview}</Markdown>
        </div>
      )}

      {/* Capabilities */}
      <div className="flex gap-3 mb-6">
        {formatInfo?.has_reader && (
          <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
            <FileInput size={12} className="text-primary" /> Reader
          </div>
        )}
        {formatInfo?.has_writer && (
          <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
            <FileOutput size={12} className="text-primary" /> Writer
          </div>
        )}
        {formatInfo?.source && formatInfo.source !== "built-in" && (
          <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
            <Plug size={12} /> {formatInfo.source}
          </div>
        )}
        {filterDoc?.wikiUrl && (
          <a
            href={filterDoc.wikiUrl}
            target="_blank"
            rel="noopener noreferrer"
            className="flex items-center gap-1.5 text-xs text-primary/70 hover:text-primary transition-colors ml-auto"
          >
            <BookOpen size={12} /> Wiki
          </a>
        )}
      </div>

      {/* Presets */}
      {presets.length > 0 && (
        <div className="mb-6">
          <div className="flex items-center justify-between mb-2">
            <h2 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider">
              Presets
            </h2>
            <Button
              variant="ghost"
              size="xs"
              onClick={() => setShowSavePreset(true)}
              className="px-0 h-auto text-[10px] text-muted-foreground hover:text-foreground"
            >
              <Save size={10} /> Save as Preset
            </Button>
          </div>
          <div className="flex flex-wrap gap-2">
            {presets.map((p) => (
              <div key={p.name} className="relative group">
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => handlePresetSelect(p.name)}
                  className={
                    selectedPreset === p.name
                      ? "border-primary bg-primary/10 text-primary"
                      : "text-muted-foreground hover:border-primary/30 hover:text-foreground"
                  }
                >
                  {p.name}
                  {p.source && <span className="ml-1 text-[9px] opacity-60">({p.source})</span>}
                  {p.description && (
                    <span className="ml-1 text-muted-foreground">— {p.description}</span>
                  )}
                </Button>
                {p.source === "user" && (
                  <Button
                    variant="destructive"
                    size="icon-xs"
                    onClick={(e: React.MouseEvent) => {
                      e.stopPropagation();
                      void handleDeletePreset(p.name);
                    }}
                    className="absolute -top-1.5 -right-1.5 hidden group-hover:flex items-center justify-center w-4 h-4 rounded-full"
                    title="Delete preset"
                  >
                    <X size={8} />
                  </Button>
                )}
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Save Preset Dialog */}
      {showSavePreset && (
        <Card className="mb-6">
          <CardContent className="p-4">
            <h3 className="text-sm font-semibold mb-2">Save as Preset</h3>
            <div className="flex gap-2">
              <Input
                type="text"
                value={savePresetName}
                onChange={(e) => setSavePresetName(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter") void handleSavePreset();
                }}
                placeholder="Preset name..."
                className="flex-1"
                autoFocus
              />
              <Button
                size="sm"
                onClick={handleSavePreset}
                disabled={saving || !savePresetName.trim()}
              >
                {saving ? t("Saving...") : t("Save")}
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={() => {
                  setShowSavePreset(false);
                  setSavePresetName("");
                }}
              >
                Cancel
              </Button>
            </div>
          </CardContent>
        </Card>
      )}

      {/* No presets yet — show Save as Preset shortcut */}
      {presets.length === 0 && schema && !loadingSchema && (
        <div className="mb-4">
          <Button
            variant="ghost"
            size="xs"
            onClick={() => setShowSavePreset(true)}
            className="px-0 h-auto text-muted-foreground hover:text-foreground"
          >
            <Save size={10} /> Save current configuration as a preset
          </Button>
        </div>
      )}

      {/* Configuration schema */}
      {loadingSchema && (
        <div className="py-8 text-center text-sm text-muted-foreground animate-pulse">
          Loading configuration schema...
        </div>
      )}

      {!loadingSchema && (schema || hasDocumentation) && (
        <Tabs defaultValue={schema ? "editor" : "docs"} onValueChange={setActiveTab}>
          <TabsList variant="line">
            {schema && (
              <>
                <TabsTrigger value="editor">
                  <Settings2 size={12} />
                  Editor
                </TabsTrigger>
                <TabsTrigger value="yaml">Config (YAML)</TabsTrigger>
              </>
            )}
            {hasDocumentation && (
              <TabsTrigger value="docs">
                <BookOpen size={12} />
                Documentation
              </TabsTrigger>
            )}
          </TabsList>

          {schema && (
            <TabsContent value="editor">
              <div className="flex gap-4">
                <Card className="max-w-xl flex-1">
                  <CardContent className="p-4">
                    <SchemaForm
                      schema={schema}
                      values={config}
                      onChange={setConfig}
                      presetValues={presetValues}
                      host={schemaHost}
                    />
                  </CardContent>
                </Card>

                {/* Contextual parameter help sidebar */}
                {filterDoc &&
                  filterDoc.parameters &&
                  Object.keys(filterDoc.parameters).length > 0 && (
                    <div className="w-80 shrink-0 hidden xl:block">
                      <div className="sticky top-4">
                        <h3 className="text-[11px] font-semibold text-muted-foreground uppercase tracking-wider mb-2 flex items-center gap-1.5">
                          <BookOpen size={11} />
                          Parameter Reference
                        </h3>
                        <DocsPanel doc={filterDoc} inline />
                      </div>
                    </div>
                  )}
              </div>
            </TabsContent>
          )}

          {schema && (
            <TabsContent value="yaml">
              <div className="max-w-xl">
                <textarea
                  value={yamlText}
                  onChange={(e) => handleYamlChange(e.target.value)}
                  className="w-full min-h-[300px] rounded-lg border border-border bg-card p-4 font-mono text-xs text-foreground outline-none focus:ring-1 focus:ring-ring resize-y"
                  spellCheck={false}
                />
                {yamlError && <p className="mt-1 text-[10px] text-amber-500">{yamlError}</p>}
              </div>
            </TabsContent>
          )}

          {hasDocumentation && filterDoc && (
            <TabsContent value="docs">
              <div className="max-w-2xl">
                <DocsPanel doc={filterDoc} />
              </div>
            </TabsContent>
          )}
        </Tabs>
      )}

      {!loadingSchema && !schema && !hasDocumentation && (
        <div className="py-8 text-center text-sm text-muted-foreground">
          This format has no configurable parameters.
        </div>
      )}

      {/* Ad-hoc Format Runner */}
      {!loadingSchema && formatInfo?.has_reader && (
        <div className="mt-8">
          <h2 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-3">
            Test
          </h2>
          <div className="flex items-center gap-2 mb-3">
            <Button
              variant="outline"
              size="sm"
              onClick={handleRunReader}
              disabled={runnerLoading}
              className="bg-card hover:border-primary/30 hover:text-primary"
            >
              <Play size={12} />
              {runnerLoading ? t("Running...") : t("Open File...")}
            </Button>
            {runnerParts !== null && (
              <span className="text-[10px] text-muted-foreground">
                {t("{count} part(s) extracted", { count: runnerParts.length })}
              </span>
            )}
          </div>

          {/* Runner results */}
          {runnerParts !== null && runnerParts.length > 0 && (
            <Card className="max-w-2xl overflow-hidden">
              <ScrollArea className="max-h-[400px]">
                <div className="divide-y divide-border">
                  {runnerParts.map((part, i) => (
                    <PartRow key={i} part={part} />
                  ))}
                </div>
              </ScrollArea>
            </Card>
          )}

          {runnerParts !== null && runnerParts.length === 0 && (
            <div className="py-4 text-center text-sm text-muted-foreground">
              No parts extracted from the file.
            </div>
          )}
        </div>
      )}
    </div>
  );
}

// --- Part Row for Runner Results ---

const partTypeBadgeColors: Record<string, string> = {
  Block: "bg-blue-500/15 text-blue-600 dark:text-blue-400",
  LayerStart: "bg-emerald-500/15 text-emerald-600 dark:text-emerald-400",
  LayerEnd: "bg-emerald-500/10 text-emerald-500/70",
  Data: "bg-gray-500/15 text-gray-600 dark:text-gray-400",
  GroupStart: "bg-purple-500/15 text-purple-600 dark:text-purple-400",
  GroupEnd: "bg-purple-500/10 text-purple-500/70",
  Media: "bg-amber-500/15 text-amber-600 dark:text-amber-400",
};

function PartRow({ part }: { part: FormatPartInfo }) {
  const [expanded, setExpanded] = useState(false);
  const hasProps = part.properties && Object.keys(part.properties).length > 0;
  const hasDetails = hasProps || part.source_text;

  return (
    <div className="px-3 py-2">
      <div className="flex items-center gap-2">
        {/* Type badge */}
        <span
          className={`text-[9px] font-semibold px-1.5 py-0.5 rounded ${
            partTypeBadgeColors[part.type] || "bg-muted text-muted-foreground"
          }`}
        >
          {part.type}
        </span>

        {/* ID */}
        {part.id && (
          <span className="text-[10px] font-mono text-muted-foreground truncate max-w-[120px]">
            {part.id}
          </span>
        )}

        {/* Summary — preview of parsed content, not UI copy. */}
        <span className="flex-1 text-xs text-foreground truncate" translate="no">
          {part.summary}
        </span>

        {/* Expand toggle */}
        {hasDetails && (
          <Button
            variant="ghost"
            size="icon-xs"
            onClick={() => setExpanded((v) => !v)}
            className="h-5 w-5"
          >
            {expanded ? <ChevronDown size={12} /> : <ChevronRight size={12} />}
          </Button>
        )}
      </div>

      {/* Expanded details */}
      {expanded && (
        <div className="mt-2 ml-6 space-y-1">
          {part.source_text && (
            <div>
              <span className="text-[9px] font-semibold text-muted-foreground uppercase">
                Source:
              </span>
              <pre className="mt-0.5 text-[10px] text-foreground bg-muted/50 rounded p-2 whitespace-pre-wrap break-words max-h-32 overflow-auto">
                {part.source_text}
              </pre>
            </div>
          )}
          {hasProps && (
            <div>
              <span className="text-[9px] font-semibold text-muted-foreground uppercase">
                Properties:
              </span>
              <div className="mt-0.5 flex flex-wrap gap-1">
                {Object.entries(part.properties!).map(([k, v]) => (
                  <span
                    key={k}
                    className="text-[9px] px-1.5 py-0.5 rounded bg-muted text-muted-foreground font-mono"
                  >
                    {k}: {v}
                  </span>
                ))}
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

// --- Utility: resolve filter doc by format name ---

function resolveFilterDoc(formatName: string, docs: PluginDocs | null): FilterDoc | undefined {
  if (!docs) return undefined;

  // Direct match
  if (docs.filters[formatName]) return docs.filters[formatName];

  // Try alias resolution
  const aliased = docs.aliases?.[formatName];
  if (aliased && docs.filters[aliased]) return docs.filters[aliased];

  // Strip version suffix (e.g., "okf_html@1.48.0" → "okf_html")
  const bare = formatName.includes("@")
    ? formatName.slice(0, formatName.lastIndexOf("@"))
    : formatName;
  if (bare !== formatName) {
    if (docs.filters[bare]) return docs.filters[bare];
    const bareAliased = docs.aliases?.[bare];
    if (bareAliased && docs.filters[bareAliased]) return docs.filters[bareAliased];
  }

  return undefined;
}
