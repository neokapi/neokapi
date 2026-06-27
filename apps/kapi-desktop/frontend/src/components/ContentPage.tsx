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
  ChevronRight,
  ArrowRight,
  Layers,
  Check,
  Files,
} from "lucide-react";
import {
  Button,
  Badge,
  Label,
  GlobInput,
  TargetPathInput,
  LocaleSelect,
  MultiLocaleSelect,
  FormatSelect,
  ItemCard,
  ConfirmDeleteButton,
  LocalePill,
} from "@neokapi/ui-primitives";
import type {
  KapiProject,
  ContentCollection,
  ContentItem,
  FormatSpec,
  FormatInfo,
  FormatDefaults,
} from "../types/api";
import { isBareEntry } from "../types/api";
import { api, type OutputFileInfo } from "../hooks/useApi";
import { FormatConfigDialog, type FormatConfigValue } from "./FormatConfigDialog";
import { TranslationStatusPanel } from "./TranslationStatusPanel";
import { FilePreview } from "./FilePreview";
import { ArchiveEntries, isArchivePath } from "./ArchiveEntries";
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

/**
 * The single extension a glob targets (e.g. "input/*.json" → ".json"), or
 * undefined for a bare "*"/"**" or a brace alternation. Used to pre-filter the
 * format picker in the config modal.
 */
function globExtension(pattern: string): string | undefined {
  const seg = pattern.split("/").pop() ?? pattern;
  const m = /\*\.([A-Za-z0-9]+)$/.exec(seg);
  return m ? "." + m[1].toLowerCase() : undefined;
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
  // configKey of the content item whose format-config modal is open (one at a time).
  const [dialogKey, setDialogKey] = useState<string | null>(null);
  // Per-collection-card UI state: which cards are collapsed (body hidden) and
  // which are in edit mode (config editor vs the files view). Keyed by index.
  const [collapsed, setCollapsed] = useState<Set<number>>(new Set());
  const [editing, setEditing] = useState<Set<number>>(new Set());
  const [otherCollapsed, setOtherCollapsed] = useState(false);
  // Generated output files keyed by their source file's relative path (issue #5),
  // plus the set of source rows whose outputs are expanded.
  const [outputs, setOutputs] = useState<Record<string, OutputFileInfo[]>>({});
  const [expandedOutputs, setExpandedOutputs] = useState<Set<string>>(new Set());
  // Preview target: the file whose content is shown in the PreviewKit sheet.
  const [preview, setPreview] = useState<{ path: string; relative: string } | null>(null);
  // Archive rows that are expanded to show their inner entries, keyed by path.
  const [expandedArchives, setExpandedArchives] = useState<Set<string>>(new Set());
  // Per-entry preview target: a single file inside an archive container.
  const [archivePreview, setArchivePreview] = useState<{
    path: string;
    relative: string;
    entry: string;
  } | null>(null);

  const content = project.content ?? [];

  const hasPreloadedData = !!(propFormats && propBasePath);

  // Per-format config/preset stored in the project's defaults.formats, surfaced
  // to the modal for wildcard items (which auto-detect, so config lives once per
  // format at the project level rather than on a single item).
  const projectFormatValues = useMemo(() => {
    const out: Record<string, FormatConfigValue> = {};
    for (const [f, fd] of Object.entries(project.defaults?.formats ?? {})) {
      out[f] = { config: fd.config, preset: fd.preset };
    }
    return out;
  }, [project.defaults?.formats]);

  // Persist a per-format override into project defaults.formats (wildcard items).
  const updateProjectFormat = useCallback(
    (fmt: string, next: FormatConfigValue) => {
      const defaults = { ...project.defaults };
      const formats: Record<string, FormatDefaults> = { ...defaults.formats };
      const entry: FormatDefaults = { ...formats[fmt] };
      if (next.preset) entry.preset = next.preset;
      else delete entry.preset;
      if (next.config && Object.keys(next.config).length > 0) entry.config = next.config;
      else delete entry.config;
      if (Object.keys(entry).length === 0) delete formats[fmt];
      else formats[fmt] = entry;
      defaults.formats = Object.keys(formats).length > 0 ? formats : undefined;
      onUpdate({ ...project, defaults });
    },
    [project, onUpdate],
  );

  // Formats detected among the files a content item matches (for the wildcard
  // modal's default selection), and the glob's extension if it carries one.
  const matchedFormatsForItem = useCallback(
    (item: ContentItem) => {
      const set = new Set<string>();
      for (const m of matches) {
        if (m.pattern === item.path && m.format) set.add(m.format);
      }
      return [...set];
    },
    [matches],
  );

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

  // --- Build the "Other files" set: project files that no collection matched ---
  const matchedSet = new Set(matches.map((m) => m.relative));
  // Relative paths of every known output file, so generated files surface as
  // children of their source rather than getting dumped into "Other files".
  const outputSet = new Set<string>();
  for (const list of Object.values(outputs)) {
    for (const o of list) outputSet.add(o.relative);
  }
  const unmatchedFiles = projectFiles.filter(
    (f) => !f.is_dir && !matchedSet.has(f.relative) && !outputSet.has(f.relative),
  );

  // --- Item editing helpers ---
  const renderItemEditor = (
    item: ContentItem,
    onItemChange: (item: ContentItem) => void,
    configKey: string,
  ) => {
    const fmt = formatName(item.format);
    const hasConfig = item.format?.config && Object.keys(item.format.config).length > 0;
    const matchedFormats = fmt ? [] : matchedFormatsForItem(item);
    // A single explicit format → configure that format on the item. Otherwise the
    // item auto-detects (wildcard) → configure the matched formats project-wide.
    const isWildcard = !fmt;

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
              onChange={(newFmt) =>
                onItemChange({
                  ...item,
                  format: newFmt ? { name: newFmt } : undefined,
                })
              }
              formats={formats}
            />
          </div>
          <div>
            <Label className="mb-0.5 block text-xs text-muted-foreground">Target path</Label>
            <TargetPathInput
              value={item.target ?? ""}
              onChange={(v) => onItemChange({ ...item, target: v || undefined })}
              placeholder="output/{lang}  ·  or output/{lang}/{dir}/{name}.{ext}"
            />
          </div>
        </div>
        <div>
          <Label className="mb-0.5 block text-xs text-muted-foreground">
            Base{" "}
            <span className="font-normal text-muted-foreground/60">
              (optional — outputs mirror source paths relative to this; defaults to the path prefix
              before the first wildcard)
            </span>
          </Label>
          <GlobInput
            value={item.base ?? ""}
            onChange={(v) => onItemChange({ ...item, base: v || undefined })}
            placeholder="auto (e.g. input/docs)"
          />
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

        {/* Format configuration \u2014 schema-driven modal */}
        {(fmt || matchedFormats.length > 0) && (
          <div>
            <Button
              variant="ghost"
              size="xs"
              onClick={() => setDialogKey(configKey)}
              className="h-auto px-0 text-muted-foreground hover:text-foreground"
            >
              <Settings2 size={10} />
              {fmt ? (
                <>
                  {t("Configure {fmt}", { fmt })}
                  {(hasConfig || item.format?.preset) && (
                    <span className="ml-1 rounded bg-primary/10 px-1.5 py-0.5 text-primary">
                      {item.format?.preset
                        ? item.format.preset
                        : Object.keys(item.format!.config!).length}
                    </span>
                  )}
                </>
              ) : (
                t("Configure formats ({count})", { count: matchedFormats.length })
              )}
            </Button>
          </div>
        )}

        {dialogKey === configKey &&
          (isWildcard ? (
            <FormatConfigDialog
              open
              onOpenChange={(o) => !o && setDialogKey(null)}
              title={t("Configure formats")}
              description={t(
                "This pattern auto-detects a format per file. Tune any of them here \u2014 settings apply project-wide.",
              )}
              formats={matchedFormats}
              allFormats={formats}
              allowAdd
              filterExtension={globExtension(item.path)}
              values={projectFormatValues}
              onChange={updateProjectFormat}
              scopeNote={t(
                "Stored in the project's defaults.formats \u2014 shared by every content item.",
              )}
            />
          ) : (
            <FormatConfigDialog
              open
              onOpenChange={(o) => !o && setDialogKey(null)}
              title={t("Configure {fmt}", { fmt })}
              formats={[fmt]}
              allFormats={formats}
              values={{
                [fmt]: { config: item.format?.config, preset: item.format?.preset },
              }}
              onChange={(f, next) =>
                onItemChange({
                  ...item,
                  format: { name: f, preset: next.preset, config: next.config },
                })
              }
              scopeNote={t("Applies to this content item.")}
            />
          ))}
      </div>
    );
  };

  // ── Card helpers ─────────────────────────────────────────────────────────
  const toggle = (setSet: React.Dispatch<React.SetStateAction<Set<number>>>, key: number) =>
    setSet((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });

  // The glob patterns a content entry declares, and the matched files for them.
  const patternsOf = (coll: ContentCollection) =>
    isBareEntry(coll) ? [coll.path ?? ""] : (coll.items ?? []).map((i) => i.path);
  const filesForEntry = (coll: ContentCollection) => {
    const pats = new Set(patternsOf(coll).filter(Boolean));
    return matches.filter((m) => pats.has(m.pattern));
  };

  // Read-only source→targets summary shown in a collection card's header.
  const langSummary = (coll: ContentCollection) => {
    const source = String(coll.source_language || project.defaults?.source_language || "?");
    const targets = (coll.target_languages ?? project.defaults?.target_languages ?? []).map(String);
    const overridden = !!(coll.source_language || coll.target_languages);
    return (
      <span className="flex items-center gap-1 text-xs text-muted-foreground">
        <Globe size={10} className="shrink-0" />
        <LocalePill locale={source} />
        <span>&rarr;</span>
        {targets.length === 0 ? (
          <span>?</span>
        ) : targets.length <= 2 ? (
          targets.map((l) => <LocalePill key={l} locale={l} />)
        ) : (
          <Badge
            variant="secondary"
            className="px-1.5 py-0 text-[10px] font-normal"
            title={targets.join(", ")}
          >
            {t("{count} languages", { count: targets.length })}
          </Badge>
        )}
        {overridden && (
          <Badge variant="secondary" className="ml-0.5 px-1 py-0 text-[9px]">
            override
          </Badge>
        )}
      </span>
    );
  };

  // The editor body for a collection card (name, language overrides, patterns).
  const collectionEditor = (coll: ContentCollection, ci: number) => {
    if (isBareEntry(coll)) {
      const item: ContentItem = { path: coll.path ?? "", format: coll.format, target: coll.target };
      return renderItemEditor(
        item,
        (updated) =>
          handleUpdateCollection(ci, {
            path: updated.path,
            format: updated.format,
            target: updated.target,
          }),
        `bare-${ci}`,
      );
    }
    return (
      <div className="space-y-4">
        <div>
          <Label className="mb-0.5 block text-xs text-muted-foreground">Collection name</Label>
          <input
            type="text"
            value={coll.name ?? ""}
            onChange={(e) =>
              handleUpdateCollection(ci, { ...coll, name: e.target.value || undefined })
            }
            placeholder="Collection name"
            className="w-full rounded-md border border-input bg-background px-2 py-1 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
          />
        </div>
        <div className="grid grid-cols-2 gap-3">
          <div>
            <Label className="mb-0.5 block text-xs text-muted-foreground">Source override</Label>
            <LocaleSelect
              value={coll.source_language ?? ""}
              onChange={(v) =>
                handleUpdateCollection(ci, { ...coll, source_language: v || undefined })
              }
              locales={locales}
              placeholder={
                project.defaults?.source_language
                  ? t("Inherit ({source})", { source: project.defaults.source_language })
                  : t("Select source...")
              }
            />
          </div>
          <div>
            <Label className="mb-0.5 block text-xs text-muted-foreground">Target overrides</Label>
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
        <div>
          <Label className="mb-1 block text-xs text-muted-foreground">Patterns</Label>
          <div className="space-y-2">
            {(coll.items ?? []).map((item, ii) => (
              <div key={ii} className="group/item relative rounded-md border border-border p-3">
                <div className="absolute right-2 top-2 opacity-0 group-hover/item:opacity-100">
                  <ConfirmDeleteButton
                    onDelete={() => {
                      const newItems = (coll.items ?? []).filter((_, j) => j !== ii);
                      if (newItems.length === 0) handleDeleteCollection(ci);
                      else handleUpdateCollection(ci, { ...coll, items: newItems });
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
        </div>
      </div>
    );
  };

  // The matched-files table for a collection card (rows + output expansion).
  const matchedTable = (files: FileMatch[]) => (
    <table className="w-full text-xs">
      <thead>
        <tr className="border-b border-border text-left text-muted-foreground">
          <th className="px-3 py-2 font-medium">File</th>
          <th className="px-3 py-2 font-medium">Format</th>
          <th className="px-3 py-2 font-medium">Pattern</th>
        </tr>
      </thead>
      <tbody>
        {files.map((m, i) => {
          const outs = outputs[m.relative] ?? [];
          const isOpen = expandedOutputs.has(m.relative);
          const present = outs.filter((o) => o.exists).length;
          return (
            <Fragment key={i}>
              <tr
                onClick={() => setPreview({ path: m.path, relative: m.relative })}
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
                        aria-label={isOpen ? t("Hide outputs") : t("Show outputs")}
                      >
                        {isOpen ? <ChevronDown size={12} /> : <ChevronRight size={12} />}
                      </button>
                    ) : (
                      <FileText size={12} className="shrink-0 text-muted-foreground" />
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
                      <Badge variant="outline" className="shrink-0 text-[10px] font-normal">
                        {t("{present}/{total} outputs", { present, total: outs.length })}
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
                        ? () => setPreview({ path: o.path, relative: o.relative })
                        : undefined
                    }
                    className={`border-b border-border last:border-0 ${
                      o.exists ? "cursor-pointer hover:bg-accent/30" : "opacity-60"
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
                        <span className="text-[10px] text-muted-foreground">{t("pending")}</span>
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
  );

  // The unmatched-files table for the "Other files" card.
  const unmatchedTable = (files: ProjectFile[]) => (
    <table className="w-full text-xs">
      <thead>
        <tr className="border-b border-border text-left text-muted-foreground">
          <th className="px-3 py-2 font-medium">File</th>
          <th className="px-3 py-2 font-medium">Format</th>
          <th className="px-3 py-2 text-right font-medium">Size</th>
        </tr>
      </thead>
      <tbody>
        {files.map((f) => {
          // An archive is a namespace of files: clicking it expands an inner-entry
          // list rather than previewing the container as one document.
          const archive = isArchivePath(f.relative);
          const expanded = expandedArchives.has(f.path);
          const onRow = archive
            ? () =>
                setExpandedArchives((prev) => {
                  const next = new Set(prev);
                  if (next.has(f.path)) next.delete(f.path);
                  else next.add(f.path);
                  return next;
                })
            : f.format
              ? () => setPreview({ path: f.path, relative: f.relative })
              : undefined;
          return (
            <Fragment key={f.relative}>
              <tr
                onClick={onRow}
                className={`border-b border-border last:border-0 text-muted-foreground hover:bg-accent/30 ${
                  onRow ? "cursor-pointer" : ""
                }`}
                title={
                  archive
                    ? t("Browse entries in {file}", { file: f.relative })
                    : f.format
                      ? t("Preview {file}", { file: f.relative })
                      : undefined
                }
              >
                <td className="px-3 py-1.5">
                  <span className="flex items-center gap-1.5 font-mono">
                    {archive ? (
                      expanded ? (
                        <ChevronDown size={12} className="shrink-0" />
                      ) : (
                        <ChevronRight size={12} className="shrink-0" />
                      )
                    ) : (
                      <FileText size={12} className="shrink-0" />
                    )}
                    {f.relative}
                  </span>
                </td>
                <td className="px-3 py-1.5">
                  {f.format ? <Badge variant="secondary">{f.format}</Badge> : <span>&mdash;</span>}
                </td>
                <td className="px-3 py-1.5 text-right">{formatSize(f.size)}</td>
              </tr>
              {archive && expanded && (
                <tr className="border-b border-border last:border-0">
                  <td colSpan={3} className="px-3 py-1.5">
                    <ArchiveEntries
                      archivePath={f.path}
                      onSelect={(entry) =>
                        setArchivePreview({ path: f.path, relative: f.relative, entry })
                      }
                    />
                  </td>
                </tr>
              )}
            </Fragment>
          );
        })}
      </tbody>
    </table>
  );

  return (
    <div className="flex h-full flex-col overflow-hidden p-6">
      <div className="mb-4 flex items-center justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold">Content</h1>
          {basePath && (
            <p className="text-xs text-muted-foreground">
              {t("All paths relative to {base}", { base: shortenHome(basePath) })}
            </p>
          )}
        </div>
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={handleAddCollection}
            aria-label="Add content collection"
          >
            <Plus size={12} />
            Add Collection
          </Button>
          <Button variant="outline" size="sm" onClick={handleAddFiles} aria-label="Add files">
            <Plus size={12} />
            Add Files
          </Button>
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

      {content.some((c) => c.archive) && (
        <section className="mb-4">
          <h2 className="mb-2 flex items-center gap-2 text-sm font-semibold uppercase tracking-wider text-muted-foreground">
            Translation state
          </h2>
          <TranslationStatusPanel tabID={tabID} />
        </section>
      )}

      <div
        onDrop={handleDrop}
        onDragOver={handleDragOver}
        onDragLeave={handleDragLeave}
        className={`min-h-0 flex-1 overflow-auto rounded-lg border-2 transition-colors ${
          dragging ? "border-primary bg-primary/5" : "border-transparent"
        }`}
      >
        {content.length === 0 && unmatchedFiles.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-16 text-center">
            <Upload size={24} className="mb-3 text-muted-foreground/50" />
            <p className="text-sm text-muted-foreground">
              {t("Add a collection to map your source files, or drop files here.")}
            </p>
          </div>
        ) : (
          <div className="grid grid-cols-1 items-start gap-3 xl:grid-cols-2">
            {content.map((coll, ci) => {
              const isEditing = editing.has(ci);
              const isOpen = !collapsed.has(ci);
              const files = filesForEntry(coll);
              const bare = isBareEntry(coll);
              const title = bare ? coll.path || t("Files") : coll.name || t("Untitled collection");
              return (
                <ItemCard
                  key={ci}
                  className={`overflow-hidden p-0 ${isEditing ? "xl:col-span-2" : ""}`}
                >
                  <div className="flex items-center gap-2 px-4 py-3">
                    <button
                      onClick={() => toggle(setCollapsed, ci)}
                      className="shrink-0 text-muted-foreground hover:text-foreground"
                      aria-label={isOpen ? t("Collapse") : t("Expand")}
                    >
                      {isOpen ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
                    </button>
                    <Layers size={14} className="shrink-0 text-primary" />
                    <span className="truncate text-sm font-medium" title={title}>
                      {title}
                    </span>
                    {!bare && langSummary(coll)}
                    <Badge variant="secondary" className="shrink-0 text-[10px] font-normal">
                      {t("{count} files", { count: files.length })}
                    </Badge>
                    <div className="ml-auto flex shrink-0 items-center gap-1">
                      <Button
                        variant={isEditing ? "secondary" : "ghost"}
                        size="xs"
                        onClick={() => {
                          // Editing implies the body is open.
                          setCollapsed((prev) => {
                            const next = new Set(prev);
                            next.delete(ci);
                            return next;
                          });
                          toggle(setEditing, ci);
                        }}
                        aria-label={isEditing ? t("Done editing") : t("Edit collection")}
                      >
                        {isEditing ? <Check size={12} /> : <Pencil size={12} />}
                        {isEditing ? t("Done") : t("Edit")}
                      </Button>
                      <ConfirmDeleteButton
                        onDelete={() => handleDeleteCollection(ci)}
                        mode="icon"
                      />
                    </div>
                  </div>

                  {isOpen && (
                    <div className="border-t border-border">
                      {/* Editor slides in over the output; both stay visible,
                          separated by a distinct tint + accent rule. */}
                      {isEditing && (
                        <div className="animate-in slide-in-from-top-2 fade-in border-b-2 border-primary/40 bg-muted/40 p-4 shadow-inner duration-200">
                          <div className="mb-2 flex items-center gap-1.5 text-[11px] font-semibold uppercase tracking-wide text-primary">
                            <Pencil size={11} />
                            {t("Edit collection")}
                          </div>
                          {collectionEditor(coll, ci)}
                        </div>
                      )}

                      {/* Output — the matched files, always visible when expanded. */}
                      {files.length > 0 ? (
                        matchedTable(files)
                      ) : (
                        <p className="px-4 py-6 text-center text-xs text-muted-foreground">
                          {t("No files matched this collection's patterns.")}
                          {!isEditing && (
                            <>
                              {" "}
                              <button
                                onClick={() => {
                                  setCollapsed((prev) => {
                                    const next = new Set(prev);
                                    next.delete(ci);
                                    return next;
                                  });
                                  setEditing((prev) => new Set(prev).add(ci));
                                }}
                                className="text-primary hover:underline"
                              >
                                {t("Edit patterns")}
                              </button>
                            </>
                          )}
                        </p>
                      )}
                    </div>
                  )}
                </ItemCard>
              );
            })}

            {/* Other files — unmatched, not owned by any collection. */}
            {!hideUnmatched && unmatchedFiles.length > 0 && (
              <ItemCard className="overflow-hidden p-0">
                <div className="flex items-center gap-2 px-4 py-3">
                  <button
                    onClick={() => setOtherCollapsed((v) => !v)}
                    className="shrink-0 text-muted-foreground hover:text-foreground"
                    aria-label={otherCollapsed ? t("Expand") : t("Collapse")}
                  >
                    {otherCollapsed ? <ChevronRight size={14} /> : <ChevronDown size={14} />}
                  </button>
                  <Files size={14} className="shrink-0 text-muted-foreground" />
                  <span className="text-sm font-medium">{t("Other files")}</span>
                  <Badge variant="secondary" className="text-[10px] font-normal">
                    {t("{count} files", { count: unmatchedFiles.length })}
                  </Badge>
                </div>
                {!otherCollapsed && (
                  <div className="border-t border-border">{unmatchedTable(unmatchedFiles)}</div>
                )}
              </ItemCard>
            )}
          </div>
        )}
      </div>

      <FilePreview
        tabID={tabID}
        filePath={preview?.path ?? null}
        filename={preview?.relative ?? ""}
        onClose={() => setPreview(null)}
      />

      <FilePreview
        tabID={tabID}
        filePath={archivePreview?.path ?? null}
        filename={archivePreview ? `${archivePreview.relative}!${archivePreview.entry}` : ""}
        entryPath={archivePreview?.entry ?? null}
        onClose={() => setArchivePreview(null)}
      />
    </div>
  );
}
