import { useState, useEffect, useCallback, DragEvent, useMemo, Fragment } from "react";
import { t } from "@neokapi/kapi-react/runtime";
import {
  Plus,
  FileText,
  RefreshCw,
  Loader2,
  Upload,
  EyeOff,
  Eye,
  Globe,
  Pencil,
  Settings2,
  ChevronDown,
  ChevronUp,
  ChevronRight,
  ArrowRight,
  Layers,
} from "lucide-react";
import {
  Button,
  Badge,
  Card,
  Label,
  GlobInput,
  TargetPathInput,
  LocaleSelect,
  MultiLocaleSelect,
  FormatSelect,
  ItemCard,
  ConfirmDeleteButton,
  LocalePill,
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from "@neokapi/ui-primitives";
import type {
  KapiProject,
  ContentCollection,
  ContentItem,
  FormatSpec,
  FormatInfo,
} from "../types/api";
import { isBareEntry, effectiveItems } from "../types/api";
import { api, type OutputFileInfo } from "../hooks/useApi";
import { TranslationStatusPanel } from "./TranslationStatusPanel";
import { FilePreview } from "./FilePreview";
import { useError } from "./ErrorBanner";
import { useShortenHome } from "../hooks/useShortenHome";
import { useWailsEvent } from "../hooks/useWailsEvent";
import { useLocales } from "../hooks/useLocales";

interface FileMatch {
  path: string;
  format: string;
  relative: string;
  pattern: string;
  collection: string;
}

interface ProjectFile {
  path: string;
  relative: string;
  format: string;
  size: number;
  is_dir: boolean;
}

export interface ContentPageProps {
  project: KapiProject;
  projectPath: string;
  onUpdate: (project: KapiProject) => void;
  tabID: string;
  /** Pre-loaded formats for Storybook — skips api.listFormats(). */
  formatList?: FormatInfo[];
  /** Pre-loaded base path for Storybook — skips api.getBasePath(). */
  basePath?: string;
}

function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

/** Get the format name from a FormatSpec, or empty string. */
function formatName(f?: FormatSpec): string {
  return f?.name ?? "";
}

export function ContentPage({
  project,
  projectPath: _projectPath,
  onUpdate,
  tabID,
  formatList: propFormats,
  basePath: propBasePath,
}: ContentPageProps) {
  const { showError } = useError();
  const { locales } = useLocales();
  const shortenHome = useShortenHome();
  const [matches, setMatches] = useState<FileMatch[]>([]);
  const [projectFiles, setProjectFiles] = useState<ProjectFile[]>([]);
  const [basePath, setBasePath] = useState(propBasePath ?? "");
  const [scanning, setScanning] = useState(false);
  const [formats, setFormats] = useState<FormatInfo[]>(propFormats ?? []);
  const [hideUnmatched, setHideUnmatched] = useState(false);
  const [dragging, setDragging] = useState(false);
  const [formatPresets, setFormatPresets] = useState<
    Record<string, Array<{ name: string; description: string }>>
  >({});
  const [expandedConfig, setExpandedConfig] = useState<Set<string>>(new Set());
  const [expandedLangs, setExpandedLangs] = useState<Set<number>>(new Set());
  // Generated output files keyed by their source file's relative path (issue #5),
  // plus the set of source rows whose outputs are expanded.
  const [outputs, setOutputs] = useState<Record<string, OutputFileInfo[]>>({});
  const [expandedOutputs, setExpandedOutputs] = useState<Set<string>>(new Set());
  // Preview target: the file whose content is shown in the PreviewKit sheet.
  const [preview, setPreview] = useState<{ path: string; relative: string } | null>(null);

  const content = project.content ?? [];

  // Collect unique format names used across content for preset loading.
  const usedFormats = useMemo(() => {
    const fmts = new Set<string>();
    for (const coll of content) {
      for (const item of effectiveItems(coll)) {
        const fn = formatName(item.format);
        if (fn) fmts.add(fn);
      }
    }
    return [...fmts];
  }, [content]);

  const hasPreloadedData = !!(propFormats && propBasePath);

  // Load format presets whenever used formats change.
  useEffect(() => {
    if (hasPreloadedData) return;
    for (const fmt of usedFormats) {
      if (formatPresets[fmt]) continue;
      void api.listFormatPresets(fmt).then((presets) => {
        if (presets) {
          setFormatPresets((prev) => ({ ...prev, [fmt]: presets }));
        }
      });
    }
  }, [usedFormats, hasPreloadedData]); // eslint-disable-line react-hooks/exhaustive-deps

  // Load available formats and base path on mount.
  useEffect(() => {
    if (!propFormats) {
      api
        .listFormats()
        .then((f) => {
          if (f) setFormats(f);
        })
        .catch((err) => showError("Failed to load formats", err));
    }
    if (!propBasePath) {
      api
        .getBasePath(tabID)
        .then((b) => {
          if (b) setBasePath(b);
        })
        .catch((err) => showError("Failed to get base path", err));
    }
  }, [tabID, showError, propFormats, propBasePath]);

  const rescanFiles = useCallback(async () => {
    if (hasPreloadedData) return;
    setScanning(true);
    try {
      await api.updateProject(tabID, project);
      const [matched, allFiles, outs] = await Promise.all([
        api.matchContent(tabID),
        api.listProjectFiles(tabID),
        api.listOutputs(tabID),
      ]);
      setMatches(matched ?? []);
      setProjectFiles(allFiles ?? []);
      setOutputs(outs ?? {});
    } catch (err) {
      showError("Failed to scan files", err);
    } finally {
      setScanning(false);
    }
  }, [tabID, project, showError, hasPreloadedData]);

  const refreshOutputs = useCallback(() => {
    if (hasPreloadedData) return;
    void api
      .listOutputs(tabID)
      .then((outs) => {
        if (outs) setOutputs(outs);
      })
      .catch(() => {
        /* outputs are best-effort */
      });
  }, [tabID, hasPreloadedData]);

  useEffect(() => {
    void rescanFiles();
  }, [rescanFiles, content.length]);

  useWailsEvent("project-files-changed", (data) => {
    if (data === tabID) void rescanFiles();
  });

  // A flow run wrote an output file — refresh so it appears beneath its source
  // immediately, even while the run is still in progress (issue #5).
  useWailsEvent("outputs-changed", () => refreshOutputs());

  // --- Project update helpers ---
  const updateContent = (newContent: ContentCollection[]) => {
    onUpdate({ ...project, content: newContent });
  };

  const handleAddCollection = () => {
    updateContent([...content, { name: "New Collection", items: [{ path: "" }] }]);
  };

  const handleUpdateCollection = (index: number, coll: ContentCollection) => {
    const updated = [...content];
    updated[index] = coll;
    updateContent(updated);
  };

  const handleDeleteCollection = (index: number) => {
    updateContent(content.filter((_, i) => i !== index));
  };

  const handleAddFiles = async () => {
    const added = await api.addFilesDialog(tabID, "");
    if (added && added.length > 0) void rescanFiles();
  };

  const handleDrop = useCallback(
    async (e: DragEvent) => {
      e.preventDefault();
      setDragging(false);
      const items = e.dataTransfer?.files;
      if (!items || items.length === 0) return;
      for (let i = 0; i < items.length; i++) {
        const file = items[i];
        const path = (file as unknown as { path?: string }).path;
        if (path) {
          await api.copyFileToProject(tabID, path, "");
        }
      }
      void rescanFiles();
    },
    [tabID, rescanFiles],
  );

  const handleDragOver = useCallback((e: DragEvent) => {
    e.preventDefault();
    setDragging(true);
  }, []);

  const handleDragLeave = useCallback((e: DragEvent) => {
    e.preventDefault();
    setDragging(false);
  }, []);

  // --- Build unified file list ---
  const matchedSet = new Set(matches.map((m) => m.relative));
  const collectionMap = new Map<string, FileMatch[]>();
  for (const m of matches) {
    const key = m.collection || "";
    const arr = collectionMap.get(key) ?? [];
    arr.push(m);
    collectionMap.set(key, arr);
  }
  // Relative paths of every known output file, so they surface as children of
  // their source rather than getting dumped into "Other files" (issue #5).
  const outputSet = new Set<string>();
  for (const list of Object.values(outputs)) {
    for (const o of list) outputSet.add(o.relative);
  }
  const unmatchedFiles = projectFiles.filter(
    (f) => !f.is_dir && !matchedSet.has(f.relative) && !outputSet.has(f.relative),
  );
  const collectionNames = [...collectionMap.keys()].sort((a, b) => {
    if (!a) return 1;
    if (!b) return -1;
    return a.localeCompare(b);
  });
  const totalFiles = projectFiles.filter((f) => !f.is_dir).length;

  // --- Item editing helpers ---
  const renderItemEditor = (
    item: ContentItem,
    onItemChange: (item: ContentItem) => void,
    configKey: string,
  ) => {
    const fmt = formatName(item.format);
    const presetOptions = fmt ? (formatPresets[fmt] ?? []) : [];
    const hasConfig = item.format?.config && Object.keys(item.format.config).length > 0;
    const isExpanded = expandedConfig.has(configKey);

    return (
      <div className="space-y-2">
        <div>
          <Label className="mb-0.5 block text-xs text-muted-foreground">Path pattern</Label>
          <GlobInput
            value={item.path}
            onChange={(v) => onItemChange({ ...item, path: v })}
            placeholder="src/locales/en/*.json"
          />
        </div>
        <div className="grid grid-cols-2 gap-2">
          <div>
            <Label className="mb-0.5 block text-xs text-muted-foreground">Format</Label>
            <FormatSelect
              value={fmt}
              onChange={(newFmt) => {
                onItemChange({
                  ...item,
                  format: newFmt ? { name: newFmt } : undefined,
                });
                if (newFmt && !formatPresets[newFmt]) {
                  void api.listFormatPresets(newFmt).then((p) => {
                    if (p) setFormatPresets((prev) => ({ ...prev, [newFmt]: p }));
                  });
                }
              }}
              formats={formats}
            />
          </div>
          <div>
            <Label className="mb-0.5 block text-xs text-muted-foreground">Target path</Label>
            <TargetPathInput
              value={item.target ?? ""}
              onChange={(v) => onItemChange({ ...item, target: v || undefined })}
              placeholder="src/locales/{lang}/*.json"
            />
          </div>
        </div>

        {/* Exec extractor command — shortcut for format:exec's
            config.command field so users don't have to open the
            Format Config JSON editor for the common case. */}
        {fmt === "exec" && (
          <div>
            <Label className="mb-0.5 block text-xs text-muted-foreground">Extractor command</Label>
            <input
              type="text"
              value={
                typeof item.format?.config?.command === "string" ? item.format.config.command : ""
              }
              onChange={(e) =>
                onItemChange({
                  ...item,
                  format: {
                    ...item.format!,
                    config: {
                      ...item.format?.config,
                      command: e.target.value || undefined,
                    },
                  },
                })
              }
              placeholder="vp kapi-react extract --stream"
              className="w-full rounded-md border border-input bg-background px-2 py-1 font-mono text-xs outline-none focus-visible:ring-2 focus-visible:ring-ring"
            />
            <p className="mt-0.5 text-xs text-muted-foreground">
              `kapi extract -p` runs this command; NUL-separated paths on stdin, NDJSON blocks on
              stdout.
            </p>
          </div>
        )}

        {/* Format preset */}
        {fmt && fmt !== "exec" && (
          <div>
            <Label className="mb-0.5 block text-xs text-muted-foreground">Format Preset</Label>
            <Select
              value={item.format?.preset || "__default__"}
              onValueChange={(v) =>
                onItemChange({
                  ...item,
                  format: { ...item.format!, preset: v === "__default__" ? undefined : v },
                })
              }
            >
              <SelectTrigger className="h-8 text-xs">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="__default__">Default</SelectItem>
                {presetOptions.map((p) => (
                  <SelectItem key={p.name} value={p.name} translate="no">
                    {p.name}
                    {p.description ? ` \u2014 ${p.description}` : ""}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        )}

        {/* Inline format config */}
        {fmt && (
          <div>
            <Button
              variant="ghost"
              size="xs"
              onClick={() => {
                setExpandedConfig((prev) => {
                  const next = new Set(prev);
                  if (next.has(configKey)) next.delete(configKey);
                  else next.add(configKey);
                  return next;
                });
              }}
              className="h-auto px-0 text-muted-foreground hover:text-foreground"
            >
              {isExpanded ? <ChevronUp size={10} /> : <ChevronDown size={10} />}
              <Settings2 size={10} />
              Format Config
              {hasConfig && (
                <span className="ml-1 rounded bg-primary/10 px-1.5 py-0.5 text-primary">
                  {Object.keys(item.format!.config!).length}
                </span>
              )}
            </Button>
            {isExpanded && (
              <div className="mt-1.5">
                <textarea
                  value={hasConfig ? JSON.stringify(item.format!.config, null, 2) : ""}
                  onChange={(e) => {
                    const val = e.target.value.trim();
                    if (!val) {
                      onItemChange({
                        ...item,
                        format: { ...item.format!, config: undefined },
                      });
                      return;
                    }
                    try {
                      const parsed = JSON.parse(val);
                      onItemChange({
                        ...item,
                        format: { ...item.format!, config: parsed },
                      });
                    } catch {
                      // Don't update on invalid JSON — user is still typing.
                    }
                  }}
                  placeholder='{"key": "value"}'
                  rows={4}
                  className="w-full rounded border border-input bg-transparent px-2 py-1 font-mono text-xs outline-none focus:ring-1 focus:ring-ring"
                />
                <p className="mt-0.5 text-xs text-muted-foreground">
                  JSON config passed to the format reader/writer.
                </p>
              </div>
            )}
          </div>
        )}
      </div>
    );
  };

  return (
    <div className="flex h-full flex-col overflow-hidden p-6">
      <div className="mb-4 flex items-baseline justify-between">
        <h1 className="text-xl font-semibold">Content</h1>
        {basePath && (
          <p className="text-xs text-muted-foreground">
            All paths relative to {shortenHome(basePath)}
          </p>
        )}
      </div>

      {content.some((c) => c.archive) && (
        <section className="mb-4">
          <h2 className="mb-2 flex items-center gap-2 text-sm font-semibold uppercase tracking-wider text-muted-foreground">
            Translation state
          </h2>
          <TranslationStatusPanel tabID={tabID} />
        </section>
      )}

      <div className="grid min-h-0 flex-1 grid-cols-2 gap-6">
        {/* Left column: File Patterns */}
        <section className="flex min-h-0 flex-col overflow-auto">
          <div className="mb-3 flex items-center justify-between">
            <h2 className="flex items-center gap-2 text-sm font-semibold uppercase tracking-wider text-muted-foreground">
              <FileText size={14} />
              File Patterns
            </h2>
            <Button
              variant="outline"
              size="sm"
              onClick={handleAddCollection}
              aria-label="Add content collection"
            >
              <Plus size={12} />
              Add Collection
            </Button>
          </div>

          {content.length > 0 ? (
            <div className="space-y-3">
              {content.map((coll, ci) => {
                if (isBareEntry(coll)) {
                  // Bare entry — render as a simple card.
                  const item: ContentItem = {
                    path: coll.path ?? "",
                    format: coll.format,
                    target: coll.target,
                  };
                  return (
                    <ItemCard key={ci} className="relative">
                      <div className="absolute right-2 top-2 opacity-0 group-hover:opacity-100">
                        <ConfirmDeleteButton
                          onDelete={() => handleDeleteCollection(ci)}
                          mode="icon"
                        />
                      </div>
                      {renderItemEditor(
                        item,
                        (updated) =>
                          handleUpdateCollection(ci, {
                            path: updated.path,
                            format: updated.format,
                            target: updated.target,
                          }),
                        `bare-${ci}`,
                      )}
                    </ItemCard>
                  );
                }

                // Collection — render as a grouped card.
                return (
                  <ItemCard key={ci} className="p-0 overflow-hidden">
                    <div className="flex items-center justify-between border-b border-border px-4 py-3">
                      <div className="flex items-center gap-2">
                        <Layers size={14} className="text-primary" />
                        <input
                          type="text"
                          value={coll.name ?? ""}
                          onChange={(e) =>
                            handleUpdateCollection(ci, {
                              ...coll,
                              name: e.target.value || undefined,
                            })
                          }
                          placeholder="Collection name"
                          className="bg-transparent text-sm font-medium outline-none placeholder:text-muted-foreground/50"
                        />
                      </div>
                      <div className="flex items-center gap-1.5">
                        <button
                          onClick={() =>
                            setExpandedLangs((prev) => {
                              const next = new Set(prev);
                              if (next.has(ci)) next.delete(ci);
                              else next.add(ci);
                              return next;
                            })
                          }
                          className="flex items-center gap-1 rounded px-1.5 py-0.5 text-xs text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
                          title="Edit language overrides"
                        >
                          <Globe size={10} className="shrink-0" />
                          <LocalePill
                            locale={String(
                              coll.source_language || project.defaults?.source_language || "?",
                            )}
                          />
                          <span>&rarr;</span>
                          {(() => {
                            const targets = (
                              coll.target_languages ??
                              project.defaults?.target_languages ??
                              []
                            ).map(String);
                            if (targets.length === 0) {
                              return <span className="text-muted-foreground">?</span>;
                            }
                            // Few targets: show them as pills. Many: collapse to a
                            // count with the full list available on hover (issue #6).
                            if (targets.length <= 2) {
                              return (
                                <span className="flex items-center gap-1">
                                  {targets.map((l) => (
                                    <LocalePill key={l} locale={l} />
                                  ))}
                                </span>
                              );
                            }
                            return (
                              <Badge
                                variant="secondary"
                                className="px-1.5 py-0 text-[10px] font-normal"
                                title={targets.join(", ")}
                              >
                                {t("{count} languages", { count: targets.length })}
                              </Badge>
                            );
                          })()}
                          {(coll.source_language || coll.target_languages) && (
                            <Badge variant="secondary" className="ml-0.5 px-1 py-0 text-[9px]">
                              override
                            </Badge>
                          )}
                          <Pencil size={8} className="ml-0.5 shrink-0" />
                        </button>
                        <div className="opacity-0 group-hover:opacity-100">
                          <ConfirmDeleteButton
                            onDelete={() => handleDeleteCollection(ci)}
                            mode="icon"
                          />
                        </div>
                      </div>
                    </div>

                    {/* Collection language overrides (expanded) */}
                    {expandedLangs.has(ci) && (
                      <div className="border-b border-border px-4 py-3">
                        <div className="grid grid-cols-2 gap-3">
                          <div>
                            <Label className="mb-0.5 block text-xs text-muted-foreground">
                              Source override
                            </Label>
                            <LocaleSelect
                              value={coll.source_language ?? ""}
                              onChange={(v) =>
                                handleUpdateCollection(ci, {
                                  ...coll,
                                  source_language: v || undefined,
                                })
                              }
                              locales={locales}
                              placeholder={
                                project.defaults?.source_language
                                  ? t("Inherit ({source})", {
                                      source: project.defaults.source_language,
                                    })
                                  : t("Select source...")
                              }
                            />
                          </div>
                          <div>
                            <Label className="mb-0.5 block text-xs text-muted-foreground">
                              Target overrides
                            </Label>
                            <MultiLocaleSelect
                              value={coll.target_languages ?? []}
                              onChange={(v) =>
                                handleUpdateCollection(ci, {
                                  ...coll,
                                  target_languages: v.length > 0 ? v : undefined,
                                })
                              }
                              locales={locales}
                              placeholder={
                                project.defaults?.target_languages?.length
                                  ? t("Inherit ({targets})", {
                                      targets: project.defaults.target_languages.join(", "),
                                    })
                                  : t("Add targets...")
                              }
                            />
                          </div>
                        </div>
                      </div>
                    )}

                    {/* Items */}
                    <div className="space-y-0 divide-y divide-border">
                      {(coll.items ?? []).map((item, ii) => (
                        <div key={ii} className="group/item relative px-4 py-3">
                          <div className="absolute right-2 top-2 opacity-0 group-hover/item:opacity-100">
                            <ConfirmDeleteButton
                              onDelete={() => {
                                const newItems = (coll.items ?? []).filter((_, j) => j !== ii);
                                if (newItems.length === 0) {
                                  handleDeleteCollection(ci);
                                } else {
                                  handleUpdateCollection(ci, { ...coll, items: newItems });
                                }
                              }}
                              mode="icon"
                            />
                          </div>
                          {renderItemEditor(
                            item,
                            (updated) => {
                              const newItems = [...(coll.items ?? [])];
                              newItems[ii] = updated;
                              handleUpdateCollection(ci, { ...coll, items: newItems });
                            },
                            `coll-${ci}-${ii}`,
                          )}
                        </div>
                      ))}
                    </div>

                    {/* Add item button */}
                    <div className="border-t border-border px-4 py-2">
                      <Button
                        variant="ghost"
                        size="xs"
                        onClick={() =>
                          handleUpdateCollection(ci, {
                            ...coll,
                            items: [...(coll.items ?? []), { path: "" }],
                          })
                        }
                        className="text-muted-foreground"
                      >
                        <Plus size={10} />
                        Add another pattern
                      </Button>
                    </div>
                  </ItemCard>
                );
              })}
            </div>
          ) : (
            <div className="rounded-xl border border-dashed border-border p-6 text-center">
              <FileText size={20} className="mx-auto mb-2 text-muted-foreground/50" />
              <p className="text-sm text-muted-foreground">
                No content patterns. Add a collection to map your source files.
              </p>
            </div>
          )}
        </section>

        {/* Right column: Files */}
        <section className="flex min-h-0 flex-col overflow-auto">
          <div className="mb-3 flex items-center justify-between">
            <h2 className="flex items-center gap-2 text-sm font-semibold uppercase tracking-wider text-muted-foreground">
              Files
              <span className="text-xs font-normal">
                ({matches.length} matched
                {!hideUnmatched && unmatchedFiles.length > 0 && `, ${unmatchedFiles.length} other`}
                {totalFiles > 0 && ` of ${totalFiles} total`})
              </span>
            </h2>
            <div className="flex items-center gap-2">
              <Button
                variant="outline"
                size="sm"
                onClick={() => setHideUnmatched(!hideUnmatched)}
                className={hideUnmatched ? "bg-accent" : ""}
                aria-label={hideUnmatched ? "Show all files" : "Hide unmatched files"}
                title={hideUnmatched ? "Show all files" : "Hide unmatched files"}
              >
                {hideUnmatched ? <Eye size={12} /> : <EyeOff size={12} />}
                {hideUnmatched ? t("Show all") : t("Matched only")}
              </Button>
              <Button variant="outline" size="sm" onClick={handleAddFiles} aria-label="Add files">
                <Plus size={12} />
                Add Files
              </Button>
              <Button
                variant="outline"
                size="icon-sm"
                onClick={rescanFiles}
                disabled={scanning}
                aria-label="Rescan files"
              >
                {scanning ? (
                  <Loader2 size={12} className="animate-spin" />
                ) : (
                  <RefreshCw size={12} />
                )}
              </Button>
            </div>
          </div>

          <div
            onDrop={handleDrop}
            onDragOver={handleDragOver}
            onDragLeave={handleDragLeave}
            className={`min-h-[120px] rounded-lg border-2 transition-colors ${
              dragging ? "border-primary bg-primary/5" : "border-transparent"
            }`}
          >
            {matches.length === 0 && (hideUnmatched || projectFiles.length === 0) ? (
              <div className="flex flex-col items-center justify-center py-12 text-center">
                <Upload size={24} className="mb-3 text-muted-foreground/50" />
                <p className="text-sm text-muted-foreground">
                  {content.length > 0
                    ? t("No files matched the configured patterns.")
                    : t("Drop files here or click Add Files to add them to the project.")}
                </p>
              </div>
            ) : (
              <div className="space-y-4">
                {collectionNames.map((collName) => {
                  const collFiles = collectionMap.get(collName) ?? [];
                  return (
                    <div key={collName || "__uncollected"}>
                      {collName && (
                        <h3 className="mb-1.5 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                          {collName}
                          <span className="ml-1.5 font-normal">({collFiles.length})</span>
                        </h3>
                      )}
                      <Card>
                        <table className="w-full text-xs">
                          <thead>
                            <tr className="border-b border-border text-left text-muted-foreground">
                              <th className="px-3 py-2 font-medium">File</th>
                              <th className="px-3 py-2 font-medium">Format</th>
                              <th className="px-3 py-2 font-medium">Pattern</th>
                            </tr>
                          </thead>
                          <tbody>
                            {collFiles.map((m, i) => {
                              const outs = outputs[m.relative] ?? [];
                              const isOpen = expandedOutputs.has(m.relative);
                              const present = outs.filter((o) => o.exists).length;
                              return (
                                <Fragment key={i}>
                                  <tr
                                    onClick={() =>
                                      setPreview({ path: m.path, relative: m.relative })
                                    }
                                    className="cursor-pointer border-b border-border last:border-0 hover:bg-accent/30"
                                    title={t("Preview {file}", { file: m.relative })}
                                  >
                                    <td className="px-3 py-1.5">
                                      <span className="flex items-center gap-1.5 font-mono">
                                        {outs.length > 0 ? (
                                          <button
                                            onClick={(e) => {
                                              e.stopPropagation();
                                              setExpandedOutputs((prev) => {
                                                const next = new Set(prev);
                                                if (next.has(m.relative)) next.delete(m.relative);
                                                else next.add(m.relative);
                                                return next;
                                              });
                                            }}
                                            className="shrink-0 text-muted-foreground hover:text-foreground"
                                            title={isOpen ? t("Hide outputs") : t("Show outputs")}
                                            aria-label={
                                              isOpen ? t("Hide outputs") : t("Show outputs")
                                            }
                                          >
                                            {isOpen ? (
                                              <ChevronDown size={12} />
                                            ) : (
                                              <ChevronRight size={12} />
                                            )}
                                          </button>
                                        ) : (
                                          <FileText
                                            size={12}
                                            className="shrink-0 text-muted-foreground"
                                          />
                                        )}
                                        {m.relative}
                                      </span>
                                    </td>
                                    <td className="px-3 py-1.5">
                                      <Badge variant="secondary">{m.format || "unknown"}</Badge>
                                    </td>
                                    <td className="px-3 py-1.5 text-muted-foreground">
                                      <span className="flex items-center justify-between gap-2">
                                        <span>{m.pattern}</span>
                                        {outs.length > 0 && (
                                          <Badge
                                            variant="outline"
                                            className="shrink-0 text-[10px] font-normal"
                                          >
                                            {t("{present}/{total} outputs", {
                                              present,
                                              total: outs.length,
                                            })}
                                          </Badge>
                                        )}
                                      </span>
                                    </td>
                                  </tr>
                                  {isOpen &&
                                    outs.map((o) => (
                                      <tr
                                        key={`${i}-${o.relative}`}
                                        onClick={
                                          o.exists
                                            ? () =>
                                                setPreview({
                                                  path: o.path,
                                                  relative: o.relative,
                                                })
                                            : undefined
                                        }
                                        className={`border-b border-border last:border-0 ${
                                          o.exists
                                            ? "cursor-pointer hover:bg-accent/30"
                                            : "opacity-60"
                                        }`}
                                        title={
                                          o.exists
                                            ? t("Inspect {file}", { file: o.relative })
                                            : t("Not generated yet — run a flow to create it")
                                        }
                                      >
                                        <td className="py-1 pl-9 pr-3">
                                          <span className="flex items-center gap-1.5 font-mono text-muted-foreground">
                                            <ArrowRight size={10} className="shrink-0 opacity-50" />
                                            <LocalePill locale={o.lang} />
                                            <span>{o.relative}</span>
                                          </span>
                                        </td>
                                        <td className="px-3 py-1">
                                          {o.exists ? (
                                            <Badge variant="secondary">{o.format || "—"}</Badge>
                                          ) : (
                                            <span className="text-[10px] text-muted-foreground">
                                              {t("pending")}
                                            </span>
                                          )}
                                        </td>
                                        <td className="px-3 py-1 text-right text-muted-foreground">
                                          {o.exists ? formatSize(o.size) : ""}
                                        </td>
                                      </tr>
                                    ))}
                                </Fragment>
                              );
                            })}
                          </tbody>
                        </table>
                      </Card>
                    </div>
                  );
                })}

                {!hideUnmatched && unmatchedFiles.length > 0 && (
                  <div>
                    {matches.length > 0 && (
                      <h3 className="mb-1.5 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                        Other files
                        <span className="ml-1.5 font-normal">({unmatchedFiles.length})</span>
                      </h3>
                    )}
                    <Card>
                      <table className="w-full text-xs">
                        <thead>
                          <tr className="border-b border-border text-left text-muted-foreground">
                            <th className="px-3 py-2 font-medium">File</th>
                            <th className="px-3 py-2 font-medium">Format</th>
                            <th className="px-3 py-2 text-right font-medium">Size</th>
                          </tr>
                        </thead>
                        <tbody>
                          {unmatchedFiles.map((f) => (
                            <tr
                              key={f.relative}
                              onClick={
                                f.format
                                  ? () => setPreview({ path: f.path, relative: f.relative })
                                  : undefined
                              }
                              className={`border-b border-border last:border-0 text-muted-foreground hover:bg-accent/30 ${
                                f.format ? "cursor-pointer" : ""
                              }`}
                              title={
                                f.format ? t("Preview {file}", { file: f.relative }) : undefined
                              }
                            >
                              <td className="px-3 py-1.5">
                                <span className="flex items-center gap-1.5 font-mono">
                                  <FileText size={12} className="shrink-0" />
                                  {f.relative}
                                </span>
                              </td>
                              <td className="px-3 py-1.5">
                                {f.format ? (
                                  <Badge variant="secondary">{f.format}</Badge>
                                ) : (
                                  <span>&mdash;</span>
                                )}
                              </td>
                              <td className="px-3 py-1.5 text-right">{formatSize(f.size)}</td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </Card>
                  </div>
                )}
              </div>
            )}
          </div>
        </section>
      </div>

      <FilePreview
        tabID={tabID}
        filePath={preview?.path ?? null}
        filename={preview?.relative ?? ""}
        onClose={() => setPreview(null)}
      />
    </div>
  );
}
