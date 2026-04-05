import { useState, useEffect, useCallback, DragEvent, useMemo } from "react";
import {
  Plus,
  Trash2,
  Globe,
  FileText,
  FolderOpen,
  RefreshCw,
  Loader2,
  X,
  Upload,
  EyeOff,
  Eye,
  Settings2,
  ChevronDown,
  ChevronUp,
  Layers,
} from "lucide-react";
import { Button, Badge, Card, Label, Input } from "@neokapi/ui-primitives";
import type { KapiProject, ContentCollection, ContentItem, FormatSpec } from "../types/api";
import { isBareEntry, effectiveItems } from "../types/api";
import { api } from "../hooks/useApi";
import { useError } from "./ErrorBanner";
import { useShortenHome } from "../hooks/useShortenHome";
import { useWailsEvent } from "../hooks/useWailsEvent";

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
  /** Pre-loaded presets for Storybook — skips api.listPresets(). */
  presetList?: Array<{ name: string; description: string }>;
  /** Pre-loaded format names for Storybook — skips api.listFormats(). */
  formatNames?: string[];
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
  presetList: propPresets,
  formatNames: propFormats,
  basePath: propBasePath,
}: ContentPageProps) {
  const { showError } = useError();
  const shortenHome = useShortenHome();
  const [matches, setMatches] = useState<FileMatch[]>([]);
  const [projectFiles, setProjectFiles] = useState<ProjectFile[]>([]);
  const [basePath, setBasePath] = useState(propBasePath ?? "");
  const [scanning, setScanning] = useState(false);
  const [presets, setPresets] = useState<Array<{ name: string; description: string }>>(
    propPresets ?? [],
  );
  const [formats, setFormats] = useState<string[]>(propFormats ?? []);
  const [hideUnmatched, setHideUnmatched] = useState(false);
  const [dragging, setDragging] = useState(false);
  const [formatPresets, setFormatPresets] = useState<
    Record<string, Array<{ name: string; description: string }>>
  >({});
  const [expandedConfig, setExpandedConfig] = useState<Set<string>>(new Set());

  const defaults = project.defaults ?? {};
  const content = project.content ?? [];
  const plugins = project.plugins ?? {};

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

  // Load available presets and formats on mount.
  useEffect(() => {
    if (!propPresets) {
      api
        .listPresets()
        .then((p) => {
          if (p) setPresets(p);
        })
        .catch((err) => showError("Failed to load presets", err));
    }
    if (!propFormats) {
      api
        .listFormats()
        .then((f) => {
          if (f) setFormats(f.map((x) => x.name));
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
  }, [tabID, showError, propPresets, propFormats, propBasePath]);

  const hasPreloadedData = !!(propPresets && propFormats && propBasePath);

  const rescanFiles = useCallback(async () => {
    if (hasPreloadedData) return;
    setScanning(true);
    try {
      await api.updateProject(tabID, project);
      const [matched, allFiles] = await Promise.all([
        api.matchContent(tabID),
        api.listProjectFiles(tabID),
      ]);
      setMatches(matched ?? []);
      setProjectFiles(allFiles ?? []);
    } catch (err) {
      showError("Failed to scan files", err);
    } finally {
      setScanning(false);
    }
  }, [tabID, project, showError, hasPreloadedData]);

  useEffect(() => {
    void rescanFiles();
  }, [rescanFiles, content.length]);

  useWailsEvent("project-files-changed", (data) => {
    if (data === tabID) void rescanFiles();
  });

  // --- Project update helpers ---
  const updateDefaults = (patch: Partial<typeof defaults>) => {
    onUpdate({ ...project, defaults: { ...defaults, ...patch } });
  };

  const updateContent = (newContent: ContentCollection[]) => {
    onUpdate({ ...project, content: newContent });
  };

  const handleAddBareEntry = () => {
    updateContent([...content, { path: "" }]);
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
  const unmatchedFiles = projectFiles.filter((f) => !f.is_dir && !matchedSet.has(f.relative));
  const directories = projectFiles.filter((f) => f.is_dir);
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
          <input
            type="text"
            value={item.path}
            onChange={(e) => onItemChange({ ...item, path: e.target.value })}
            placeholder="src/locales/en/*.json"
            className="w-full rounded border border-input bg-transparent px-2 py-1 text-xs outline-none focus:ring-1 focus:ring-ring"
          />
        </div>
        <div className="grid grid-cols-2 gap-2">
          <div>
            <Label className="mb-0.5 block text-xs text-muted-foreground">Format</Label>
            <select
              value={fmt}
              onChange={(e) => {
                const newFmt = e.target.value || undefined;
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
              className="w-full rounded border border-input bg-transparent px-2 py-1 text-xs outline-none focus:ring-1 focus:ring-ring"
            >
              <option value="">auto-detect</option>
              {formats.map((f) => (
                <option key={f} value={f}>
                  {f}
                </option>
              ))}
            </select>
          </div>
          <div>
            <Label className="mb-0.5 block text-xs text-muted-foreground">Target path</Label>
            <input
              type="text"
              value={item.target ?? ""}
              onChange={(e) => onItemChange({ ...item, target: e.target.value || undefined })}
              placeholder="src/locales/{lang}/*.json"
              className="w-full rounded border border-input bg-transparent px-2 py-1 text-xs outline-none focus:ring-1 focus:ring-ring"
            />
          </div>
        </div>

        {/* Format preset */}
        {fmt && (
          <div>
            <Label className="mb-0.5 block text-xs text-muted-foreground">Format Preset</Label>
            <select
              value={item.format?.preset ?? ""}
              onChange={(e) =>
                onItemChange({
                  ...item,
                  format: { ...item.format!, preset: e.target.value || undefined },
                })
              }
              className="w-full rounded border border-input bg-transparent px-2 py-1 text-xs outline-none focus:ring-1 focus:ring-ring"
            >
              <option value="">Default</option>
              {presetOptions.map((p) => (
                <option key={p.name} value={p.name}>
                  {p.name}
                  {p.description ? ` \u2014 ${p.description}` : ""}
                </option>
              ))}
            </select>
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
    <div className="p-6">
      <h1 className="mb-6 text-xl font-semibold">Content</h1>

      {/* Framework preset */}
      {presets.length > 0 && (
        <section className="mb-6">
          <h2 className="mb-3 flex items-center gap-2 text-sm font-semibold uppercase tracking-wider text-muted-foreground">
            Preset
          </h2>
          <div className="flex max-w-lg items-center gap-2">
            <select
              value={project.preset ?? ""}
              onChange={async (e) => {
                const name = e.target.value;
                if (name) {
                  const updated = await api.applyPreset(tabID, name);
                  if (updated) onUpdate(updated);
                } else {
                  onUpdate({ ...project, preset: undefined });
                }
              }}
              className="flex-1 rounded border border-input bg-transparent px-2 py-1.5 text-sm outline-none focus:ring-1 focus:ring-ring"
              aria-label="Framework preset"
            >
              <option value="">None (custom)</option>
              {presets.map((p) => (
                <option key={p.name} value={p.name}>
                  {p.name} — {p.description}
                </option>
              ))}
            </select>
            {project.preset && <Badge variant="secondary">{project.preset}</Badge>}
          </div>
        </section>
      )}

      {/* Languages (under defaults) */}
      <section className="mb-6">
        <h2 className="mb-3 flex items-center gap-2 text-sm font-semibold uppercase tracking-wider text-muted-foreground">
          <Globe size={14} />
          Languages
        </h2>
        <div className="grid max-w-lg grid-cols-2 gap-3">
          <div>
            <Label htmlFor="source-lang" className="mb-1 block text-xs text-muted-foreground">
              Source Language
            </Label>
            <Input
              id="source-lang"
              type="text"
              value={defaults.source_language ?? ""}
              onChange={(e) => updateDefaults({ source_language: e.target.value })}
              placeholder="en-US"
            />
          </div>
          <div>
            <Label className="mb-1 block text-xs text-muted-foreground">Target Languages</Label>
            <div className="flex flex-wrap items-center gap-1.5 rounded border border-input bg-transparent px-2 py-1.5">
              {(defaults.target_languages ?? []).map((lang) => (
                <span
                  key={lang}
                  className="flex items-center gap-1 rounded bg-accent px-2 py-0.5 text-xs"
                >
                  {lang}
                  <Button
                    variant="ghost"
                    size="icon-xs"
                    onClick={() =>
                      updateDefaults({
                        target_languages: defaults.target_languages?.filter((l) => l !== lang),
                      })
                    }
                    className="ml-0.5 h-4 w-4 rounded-full hover:text-destructive"
                    aria-label={`Remove ${lang}`}
                  >
                    <X size={10} />
                  </Button>
                </span>
              ))}
              <input
                type="text"
                placeholder={defaults.target_languages?.length ? "" : "Add language (e.g. fr-FR)"}
                className="min-w-[80px] flex-1 bg-transparent text-sm outline-none"
                onKeyDown={(e) => {
                  if (e.key === "Enter" || e.key === ",") {
                    e.preventDefault();
                    const val = e.currentTarget.value.trim();
                    if (val && !defaults.target_languages?.includes(val)) {
                      updateDefaults({
                        target_languages: [...(defaults.target_languages ?? []), val],
                      });
                      e.currentTarget.value = "";
                    }
                  }
                }}
              />
            </div>
          </div>
        </div>
      </section>

      {/* Plugins (map form) */}
      <section className="mb-6">
        <h2 className="mb-3 flex items-center gap-2 text-sm font-semibold uppercase tracking-wider text-muted-foreground">
          Plugins
        </h2>
        <div className="flex max-w-lg flex-wrap items-center gap-1.5 rounded border border-input bg-transparent px-2 py-1.5">
          {Object.entries(plugins).map(([name, spec]) => (
            <span
              key={name}
              className="flex items-center gap-1 rounded bg-accent px-2 py-0.5 text-xs"
            >
              {name}
              {spec.version && <span className="text-muted-foreground">{spec.version}</span>}
              {spec.framework_version && (
                <span className="text-muted-foreground">fw:{spec.framework_version}</span>
              )}
              <Button
                variant="ghost"
                size="icon-xs"
                onClick={() => {
                  const next = { ...plugins };
                  delete next[name];
                  onUpdate({ ...project, plugins: Object.keys(next).length ? next : undefined });
                }}
                className="ml-0.5 h-4 w-4 rounded-full hover:text-destructive"
                aria-label={`Remove ${name}`}
              >
                <X size={10} />
              </Button>
            </span>
          ))}
          <input
            type="text"
            placeholder={Object.keys(plugins).length ? "" : "Add plugin (e.g. okapi)"}
            className="min-w-[120px] flex-1 bg-transparent text-sm outline-none"
            onKeyDown={(e) => {
              if (e.key === "Enter" || e.key === ",") {
                e.preventDefault();
                const val = e.currentTarget.value.trim();
                if (val && !plugins[val]) {
                  onUpdate({
                    ...project,
                    plugins: { ...plugins, [val]: { version: "*" } },
                  });
                  e.currentTarget.value = "";
                }
              }
            }}
          />
        </div>
        <p className="mt-1 text-xs text-muted-foreground">
          Add plugins by name. Version ranges can be edited in the YAML directly.
        </p>
      </section>

      {/* Project root info */}
      {basePath && (
        <p className="mb-6 text-xs text-muted-foreground">
          All paths relative to {shortenHome(basePath)}
        </p>
      )}

      {/* Content collections and bare entries */}
      <section className="mb-6">
        <div className="mb-3 flex items-center justify-between">
          <h2 className="flex items-center gap-2 text-sm font-semibold uppercase tracking-wider text-muted-foreground">
            <FileText size={14} />
            File Patterns
          </h2>
          <div className="flex gap-1.5">
            <Button
              variant="outline"
              size="sm"
              onClick={handleAddBareEntry}
              aria-label="Add content pattern"
            >
              <Plus size={12} />
              Add Pattern
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={handleAddCollection}
              aria-label="Add content collection"
            >
              <Layers size={12} />
              Add Collection
            </Button>
          </div>
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
                  <div
                    key={ci}
                    className="group rounded-xl border border-border bg-background p-4 shadow-sm transition-colors hover:border-primary/20"
                  >
                    <div className="mb-3 flex items-start justify-between">
                      <div className="font-mono text-xs text-muted-foreground">
                        {item.path || (
                          <span className="italic text-muted-foreground/50">no path</span>
                        )}
                      </div>
                      <Button
                        variant="ghost"
                        size="icon-xs"
                        onClick={() => handleDeleteCollection(ci)}
                        className="opacity-0 hover:text-destructive group-hover:opacity-100"
                        aria-label={`Remove pattern ${ci + 1}`}
                      >
                        <Trash2 size={12} />
                      </Button>
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
                  </div>
                );
              }

              // Collection — render as a grouped card.
              return (
                <div
                  key={ci}
                  className="group rounded-xl border border-border bg-background shadow-sm transition-colors hover:border-primary/20"
                >
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
                      {coll.target_languages && coll.target_languages.length > 0 && (
                        <Badge variant="secondary">{coll.target_languages.join(", ")}</Badge>
                      )}
                      {coll.source_language && (
                        <Badge variant="secondary">src: {coll.source_language}</Badge>
                      )}
                      <Button
                        variant="ghost"
                        size="icon-xs"
                        onClick={() => handleDeleteCollection(ci)}
                        className="opacity-0 hover:text-destructive group-hover:opacity-100"
                        aria-label={`Remove collection ${coll.name}`}
                      >
                        <Trash2 size={12} />
                      </Button>
                    </div>
                  </div>

                  {/* Collection language overrides */}
                  <div className="border-b border-border px-4 py-2">
                    <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                      <span>Language overrides:</span>
                      <input
                        type="text"
                        value={coll.source_language ?? ""}
                        onChange={(e) =>
                          handleUpdateCollection(ci, {
                            ...coll,
                            source_language: e.target.value || undefined,
                          })
                        }
                        placeholder="source lang"
                        className="w-24 rounded border border-input bg-transparent px-1.5 py-0.5 text-xs outline-none"
                      />
                      <input
                        type="text"
                        value={coll.target_languages?.join(", ") ?? ""}
                        onChange={(e) => {
                          const val = e.target.value.trim();
                          handleUpdateCollection(ci, {
                            ...coll,
                            target_languages: val
                              ? val
                                  .split(",")
                                  .map((s) => s.trim())
                                  .filter(Boolean)
                              : undefined,
                          });
                        }}
                        placeholder="target langs (comma-separated)"
                        className="min-w-[160px] flex-1 rounded border border-input bg-transparent px-1.5 py-0.5 text-xs outline-none"
                      />
                    </div>
                  </div>

                  {/* Items */}
                  <div className="space-y-0 divide-y divide-border">
                    {(coll.items ?? []).map((item, ii) => (
                      <div key={ii} className="px-4 py-3">
                        <div className="mb-2 flex items-center justify-between">
                          <span className="font-mono text-xs text-muted-foreground">
                            {item.path || "no path"}
                          </span>
                          <Button
                            variant="ghost"
                            size="icon-xs"
                            onClick={() => {
                              const newItems = (coll.items ?? []).filter((_, j) => j !== ii);
                              if (newItems.length === 0) {
                                handleDeleteCollection(ci);
                              } else {
                                handleUpdateCollection(ci, { ...coll, items: newItems });
                              }
                            }}
                            className="opacity-0 hover:text-destructive group-hover:opacity-100"
                            aria-label={`Remove item ${ii + 1}`}
                          >
                            <Trash2 size={10} />
                          </Button>
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
                      Add item
                    </Button>
                  </div>
                </div>
              );
            })}
          </div>
        ) : (
          <div className="rounded-xl border border-dashed border-border p-6 text-center">
            <FileText size={20} className="mx-auto mb-2 text-muted-foreground/50" />
            <p className="text-sm text-muted-foreground">
              No content patterns. Add a pattern to map your source files.
            </p>
          </div>
        )}
      </section>

      {/* Unified files view */}
      <section>
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
              {hideUnmatched ? "Show all" : "Matched only"}
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
              {scanning ? <Loader2 size={12} className="animate-spin" /> : <RefreshCw size={12} />}
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
                  ? "No files matched the configured patterns."
                  : "Drop files here or click Add Files to add them to the project."}
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
                          {collFiles.map((m, i) => (
                            <tr
                              key={i}
                              className="border-b border-border last:border-0 hover:bg-accent/30"
                            >
                              <td className="px-3 py-1.5">
                                <span className="flex items-center gap-1.5 font-mono">
                                  <FileText size={12} className="shrink-0 text-muted-foreground" />
                                  {m.relative}
                                </span>
                              </td>
                              <td className="px-3 py-1.5">
                                <Badge variant="secondary">{m.format || "unknown"}</Badge>
                              </td>
                              <td className="px-3 py-1.5 text-muted-foreground">{m.pattern}</td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </Card>
                  </div>
                );
              })}

              {!hideUnmatched && (unmatchedFiles.length > 0 || directories.length > 0) && (
                <div>
                  {matches.length > 0 && (
                    <h3 className="mb-1.5 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                      Other files
                      <span className="ml-1.5 font-normal">
                        ({unmatchedFiles.length + directories.length})
                      </span>
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
                        {directories.map((f) => (
                          <tr
                            key={f.relative}
                            className="border-b border-border last:border-0 text-muted-foreground hover:bg-accent/30"
                          >
                            <td className="px-3 py-1.5">
                              <span className="flex items-center gap-1.5 font-mono">
                                <FolderOpen size={12} className="shrink-0" />
                                {f.relative}/
                              </span>
                            </td>
                            <td className="px-3 py-1.5">&mdash;</td>
                            <td className="px-3 py-1.5 text-right">&mdash;</td>
                          </tr>
                        ))}
                        {unmatchedFiles.map((f) => (
                          <tr
                            key={f.relative}
                            className="border-b border-border last:border-0 text-muted-foreground hover:bg-accent/30"
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
  );
}
