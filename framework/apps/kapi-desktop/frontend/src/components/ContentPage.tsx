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
} from "lucide-react";
import { Button } from "@neokapi/ui-primitives";
import type { KapiProject, ContentEntry } from "../types/api";
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

interface ContentPageProps {
  project: KapiProject;
  projectPath: string;
  onUpdate: (project: KapiProject) => void;
  tabID: string;
}

function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

export function ContentPage({ project, projectPath, onUpdate, tabID }: ContentPageProps) {
  const { showError } = useError();
  const shortenHome = useShortenHome();
  const [matches, setMatches] = useState<FileMatch[]>([]);
  const [projectFiles, setProjectFiles] = useState<ProjectFile[]>([]);
  const [basePath, setBasePath] = useState("");
  const [scanning, setScanning] = useState(false);
  const [presets, setPresets] = useState<Array<{ name: string; description: string }>>([]);
  const [formats, setFormats] = useState<string[]>([]);
  const [hideUnmatched, setHideUnmatched] = useState(false);
  const [dragging, setDragging] = useState(false);
  // Format presets cache: format name → presets list.
  const [formatPresets, setFormatPresets] = useState<
    Record<string, Array<{ name: string; description: string }>>
  >({});
  // Track which cards have expanded config sections.
  const [expandedConfig, setExpandedConfig] = useState<Set<number>>(new Set());

  const content = project.content ?? [];

  // Collect unique format names used across content entries for preset loading.
  const usedFormats = useMemo(
    () => [...new Set(content.map((e) => e.format).filter(Boolean) as string[])],
    [content],
  );

  // Load format presets whenever used formats change.
  useEffect(() => {
    for (const fmt of usedFormats) {
      if (formatPresets[fmt]) continue;
      api.listFormatPresets(fmt).then((presets) => {
        if (presets) {
          setFormatPresets((prev) => ({ ...prev, [fmt]: presets }));
        }
      });
    }
  }, [usedFormats]); // eslint-disable-line react-hooks/exhaustive-deps

  // Load available presets and formats on mount.
  useEffect(() => {
    api
      .listPresets()
      .then((p) => {
        if (p) setPresets(p);
      })
      .catch((err) => showError("Failed to load presets", err));
    api
      .listFormats()
      .then((f) => {
        if (f) setFormats(f.map((x) => x.name));
      })
      .catch((err) => showError("Failed to load formats", err));
    api
      .getBasePath(tabID)
      .then((b) => {
        if (b) setBasePath(b);
      })
      .catch((err) => showError("Failed to get base path", err));
  }, [tabID, showError]);

  const rescanFiles = useCallback(async () => {
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
  }, [tabID, project, showError]);

  useEffect(() => {
    rescanFiles();
  }, [rescanFiles, content.length]);

  // Auto-refresh when files change on disk.
  useWailsEvent("project-files-changed", (data) => {
    if (data === tabID) rescanFiles();
  });

  const handleAddEntry = () => {
    onUpdate({
      ...project,
      content: [...content, { path: "" }],
    });
  };

  const handleUpdateEntry = (index: number, entry: ContentEntry) => {
    const updated = [...content];
    updated[index] = entry;
    onUpdate({ ...project, content: updated });
  };

  const handleDeleteEntry = (index: number) => {
    onUpdate({
      ...project,
      content: content.filter((_, i) => i !== index),
    });
  };

  const handleAddFiles = async () => {
    const added = await api.addFilesDialog(tabID, "");
    if (added && added.length > 0) rescanFiles();
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
      rescanFiles();
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

  // Map matched files by collection.
  const collectionMap = new Map<string, FileMatch[]>();
  for (const m of matches) {
    const key = m.collection || "";
    const arr = collectionMap.get(key) ?? [];
    arr.push(m);
    collectionMap.set(key, arr);
  }

  // Unmatched project files (not directories, not matched).
  const unmatchedFiles = projectFiles.filter((f) => !f.is_dir && !matchedSet.has(f.relative));
  // Directories from project files.
  const directories = projectFiles.filter((f) => f.is_dir);

  // Sorted collection names (empty string = uncollected, goes last).
  const collectionNames = [...collectionMap.keys()].sort((a, b) => {
    if (!a) return 1;
    if (!b) return -1;
    return a.localeCompare(b);
  });

  const totalFiles = projectFiles.filter((f) => !f.is_dir).length;

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
            {project.preset && (
              <span className="rounded bg-accent px-2 py-0.5 text-xs">{project.preset}</span>
            )}
          </div>
        </section>
      )}

      {/* Languages */}
      <section className="mb-6">
        <h2 className="mb-3 flex items-center gap-2 text-sm font-semibold uppercase tracking-wider text-muted-foreground">
          <Globe size={14} />
          Languages
        </h2>
        <div className="grid max-w-lg grid-cols-2 gap-3">
          <div>
            <label className="mb-1 block text-xs text-muted-foreground" htmlFor="source-lang">
              Source Language
            </label>
            <input
              id="source-lang"
              type="text"
              value={project.source_language ?? ""}
              onChange={(e) => onUpdate({ ...project, source_language: e.target.value })}
              placeholder="en-US"
              className="w-full rounded border border-input bg-transparent px-2 py-1.5 text-sm outline-none focus:ring-1 focus:ring-ring"
            />
          </div>
          <div>
            <label className="mb-1 block text-xs text-muted-foreground">Target Languages</label>
            <div className="flex flex-wrap items-center gap-1.5 rounded border border-input bg-transparent px-2 py-1.5">
              {(project.target_languages ?? []).map((lang) => (
                <span
                  key={lang}
                  className="flex items-center gap-1 rounded bg-accent px-2 py-0.5 text-xs"
                >
                  {lang}
                  <Button
                    variant="ghost"
                    size="icon-xs"
                    onClick={() =>
                      onUpdate({
                        ...project,
                        target_languages: project.target_languages?.filter((l) => l !== lang),
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
                placeholder={project.target_languages?.length ? "" : "Add language (e.g. fr-FR)"}
                className="min-w-[80px] flex-1 bg-transparent text-sm outline-none"
                onKeyDown={(e) => {
                  if (e.key === "Enter" || e.key === ",") {
                    e.preventDefault();
                    const val = e.currentTarget.value.trim();
                    if (val && !project.target_languages?.includes(val)) {
                      onUpdate({
                        ...project,
                        target_languages: [...(project.target_languages ?? []), val],
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

      {/* Plugins */}
      <section className="mb-6">
        <h2 className="mb-3 flex items-center gap-2 text-sm font-semibold uppercase tracking-wider text-muted-foreground">
          Plugins
        </h2>
        <div className="flex max-w-lg flex-wrap items-center gap-1.5 rounded border border-input bg-transparent px-2 py-1.5">
          {(project.plugins ?? []).map((plugin) => {
            const [name, ver] = plugin.includes("@")
              ? plugin.split("@", 2)
              : [plugin, ""];
            return (
              <span
                key={plugin}
                className="flex items-center gap-1 rounded bg-accent px-2 py-0.5 text-xs"
              >
                {name}
                {ver && <span className="text-muted-foreground">@{ver}</span>}
                <Button
                  variant="ghost"
                  size="icon-xs"
                  onClick={() =>
                    onUpdate({
                      ...project,
                      plugins: project.plugins?.filter((p) => p !== plugin),
                    })
                  }
                  className="ml-0.5 h-4 w-4 rounded-full hover:text-destructive"
                  aria-label={`Remove ${plugin}`}
                >
                  <X size={10} />
                </Button>
              </span>
            );
          })}
          <input
            type="text"
            placeholder={project.plugins?.length ? "" : "Add plugin (e.g. okapi@1.47.0)"}
            className="min-w-[120px] flex-1 bg-transparent text-sm outline-none"
            onKeyDown={(e) => {
              if (e.key === "Enter" || e.key === ",") {
                e.preventDefault();
                const val = e.currentTarget.value.trim();
                if (val && !project.plugins?.includes(val)) {
                  onUpdate({
                    ...project,
                    plugins: [...(project.plugins ?? []), val],
                  });
                  e.currentTarget.value = "";
                }
              }
            }}
          />
        </div>
        <p className="mt-1 text-xs text-muted-foreground">
          Pin plugin versions with name@version (e.g. okapi@1.47.0).
        </p>
      </section>

      {/* Project root info */}
      {basePath && (
        <p className="mb-6 text-xs text-muted-foreground">
          All paths relative to {shortenHome(basePath)}
        </p>
      )}

      {/* File patterns as cards */}
      <section className="mb-6">
        <div className="mb-3 flex items-center justify-between">
          <h2 className="flex items-center gap-2 text-sm font-semibold uppercase tracking-wider text-muted-foreground">
            <FileText size={14} />
            File Patterns
          </h2>
          <Button
            variant="outline"
            size="sm"
            onClick={handleAddEntry}
            aria-label="Add content pattern"
          >
            <Plus size={12} />
            Add Pattern
          </Button>
        </div>

        {content.length > 0 ? (
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            {content.map((entry, i) => {
              const matchCount = matches.filter((m) => m.pattern === entry.path).length;
              const presetOptions = entry.format ? formatPresets[entry.format] ?? [] : [];
              const hasConfig =
                entry.format_config && Object.keys(entry.format_config).length > 0;
              const isExpanded = expandedConfig.has(i);
              return (
                <div
                  key={i}
                  className="group rounded-xl border border-border bg-background p-4 shadow-sm transition-colors hover:border-primary/20"
                >
                  <div className="mb-3 flex items-start justify-between">
                    <div className="flex-1">
                      <input
                        type="text"
                        value={entry.collection ?? ""}
                        onChange={(e) =>
                          handleUpdateEntry(i, {
                            ...entry,
                            collection: e.target.value || undefined,
                          })
                        }
                        placeholder="Collection"
                        className="mb-1 w-full bg-transparent text-sm font-medium outline-none placeholder:text-muted-foreground/50"
                      />
                      <div className="font-mono text-xs text-muted-foreground">
                        {entry.path || (
                          <span className="italic text-muted-foreground/50">no path</span>
                        )}
                      </div>
                    </div>
                    <div className="flex items-center gap-1.5">
                      {hasConfig && (
                        <Settings2
                          size={12}
                          className="text-primary"
                          aria-label="Has format config"
                        />
                      )}
                      {matchCount > 0 && (
                        <span className="rounded-full bg-primary/10 px-2 py-0.5 text-xs text-primary">
                          {matchCount}
                        </span>
                      )}
                      <Button
                        variant="ghost"
                        size="icon-xs"
                        onClick={() => handleDeleteEntry(i)}
                        className="opacity-0 hover:text-destructive group-hover:opacity-100"
                        aria-label={`Remove pattern ${i + 1}`}
                      >
                        <Trash2 size={12} />
                      </Button>
                    </div>
                  </div>
                  <div className="space-y-2">
                    <div>
                      <label className="mb-0.5 block text-xs text-muted-foreground">
                        Path pattern
                      </label>
                      <input
                        type="text"
                        value={entry.path}
                        onChange={(e) => handleUpdateEntry(i, { ...entry, path: e.target.value })}
                        placeholder="src/locales/en/*.json"
                        className="w-full rounded border border-input bg-transparent px-2 py-1 text-xs outline-none focus:ring-1 focus:ring-ring"
                      />
                    </div>
                    <div className="grid grid-cols-2 gap-2">
                      <div>
                        <label className="mb-0.5 block text-xs text-muted-foreground">Format</label>
                        <select
                          value={entry.format ?? ""}
                          onChange={(e) => {
                            const fmt = e.target.value || undefined;
                            handleUpdateEntry(i, {
                              ...entry,
                              format: fmt,
                              // Clear preset when format changes.
                              format_preset: undefined,
                              format_config: undefined,
                            });
                            // Load presets for the new format.
                            if (fmt && !formatPresets[fmt]) {
                              api.listFormatPresets(fmt).then((p) => {
                                if (p)
                                  setFormatPresets((prev) => ({ ...prev, [fmt]: p }));
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
                        <label className="mb-0.5 block text-xs text-muted-foreground">
                          Target path
                        </label>
                        <input
                          type="text"
                          value={entry.target ?? ""}
                          onChange={(e) =>
                            handleUpdateEntry(i, {
                              ...entry,
                              target: e.target.value || undefined,
                            })
                          }
                          placeholder="src/locales/{lang}/*.json"
                          className="w-full rounded border border-input bg-transparent px-2 py-1 text-xs outline-none focus:ring-1 focus:ring-ring"
                        />
                      </div>
                    </div>

                    {/* Format preset — shown when a format is selected */}
                    {entry.format && (
                      <div>
                        <label className="mb-0.5 block text-xs text-muted-foreground">
                          Format Preset
                        </label>
                        <select
                          value={entry.format_preset ?? ""}
                          onChange={(e) =>
                            handleUpdateEntry(i, {
                              ...entry,
                              format_preset: e.target.value || undefined,
                            })
                          }
                          className="w-full rounded border border-input bg-transparent px-2 py-1 text-xs outline-none focus:ring-1 focus:ring-ring"
                        >
                          <option value="">Default</option>
                          {presetOptions.map((p) => (
                            <option key={p.name} value={p.name}>
                              {p.name}
                              {p.description ? ` — ${p.description}` : ""}
                            </option>
                          ))}
                        </select>
                      </div>
                    )}

                    {/* Inline format config — expandable */}
                    {entry.format && (
                      <div>
                        <Button
                          variant="ghost"
                          size="xs"
                          onClick={() => {
                            setExpandedConfig((prev) => {
                              const next = new Set(prev);
                              if (next.has(i)) next.delete(i);
                              else next.add(i);
                              return next;
                            });
                          }}
                          className="px-0 h-auto text-muted-foreground hover:text-foreground"
                        >
                          {isExpanded ? <ChevronUp size={10} /> : <ChevronDown size={10} />}
                          <Settings2 size={10} />
                          Format Config
                          {hasConfig && (
                            <span className="ml-1 rounded bg-primary/10 px-1.5 py-0.5 text-primary">
                              {Object.keys(entry.format_config!).length}
                            </span>
                          )}
                        </Button>
                        {isExpanded && (
                          <div className="mt-1.5">
                            <textarea
                              value={
                                hasConfig
                                  ? JSON.stringify(entry.format_config, null, 2)
                                  : ""
                              }
                              onChange={(e) => {
                                const val = e.target.value.trim();
                                if (!val) {
                                  handleUpdateEntry(i, {
                                    ...entry,
                                    format_config: undefined,
                                  });
                                  return;
                                }
                                try {
                                  const parsed = JSON.parse(val);
                                  handleUpdateEntry(i, {
                                    ...entry,
                                    format_config: parsed,
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
              ({matches.length} matched{!hideUnmatched && unmatchedFiles.length > 0 && `, ${unmatchedFiles.length} other`}{totalFiles > 0 && ` of ${totalFiles} total`})
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
            <Button
              variant="outline"
              size="sm"
              onClick={handleAddFiles}
              aria-label="Add files"
            >
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
                  ? "No files matched the configured patterns."
                  : "Drop files here or click Add Files to add them to the project."}
              </p>
            </div>
          ) : (
            <div className="space-y-4">
              {/* Collections with matched files */}
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
                    <div className="rounded-lg border border-border">
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
                                <span className="rounded bg-accent px-1.5 py-0.5">
                                  {m.format || "unknown"}
                                </span>
                              </td>
                              <td className="px-3 py-1.5 text-muted-foreground">{m.pattern}</td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  </div>
                );
              })}

              {/* Unmatched files */}
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
                  <div className="rounded-lg border border-border">
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
                                <span className="rounded bg-accent px-1.5 py-0.5">{f.format}</span>
                              ) : (
                                <span>&mdash;</span>
                              )}
                            </td>
                            <td className="px-3 py-1.5 text-right">{formatSize(f.size)}</td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                </div>
              )}
            </div>
          )}
        </div>
      </section>
    </div>
  );
}
