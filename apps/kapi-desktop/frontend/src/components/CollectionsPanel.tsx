import { useState, useEffect, useCallback, useRef, DragEvent, useMemo, Fragment } from "react";
import { PieChart, Pie, Cell, ResponsiveContainer } from "recharts";
import { t } from "@neokapi/kapi-react/runtime";
import {
  Plus,
  FileText,
  RefreshCw,
  Loader2,
  Upload,
  Pencil,
  Settings2,
  ChevronDown,
  ChevronRight,
  ArrowRight,
  Layers,
  Check,
  Files,
  PackageOpen,
  AlertTriangle,
  Filter,
  Play,
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
  ConfirmDeleteButton,
  LocalePill,
  Checkbox,
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
} from "@neokapi/ui-primitives";
import type {
  KapiProject,
  ContentCollection,
  ContentItem,
  FormatSpec,
  FlowSpec,
  FlowInfo,
  FormatInfo,
  FormatDefaults,
  ProjectStatus,
  CollectionStatus,
  ConvergenceReport,
  LocaleCoverage,
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
import { useActiveFilter } from "../context/ActiveFilterContext";
import { useJobFeed } from "../context/JobFeedContext";
import { filterLanguages } from "../lib/filter";

/** Run a project flow, optionally scoped to a subset of files (a collection). */
export type RunFlowHandler = (
  flowName: string,
  flow: FlowSpec,
  opts?: { scopePaths?: string[]; scopeLabel?: string },
) => void;

// Palette for the block-distribution "cake" — each collection gets a slice and a
// matching row dot (theme chart vars, cycled for >5 collections).
const CHART_COLORS = [
  "var(--chart-1)",
  "var(--chart-2)",
  "var(--chart-3)",
  "var(--chart-4)",
  "var(--chart-5)",
];
const collectionColor = (idx: number) => CHART_COLORS[idx % CHART_COLORS.length];

// A coverage tint from 0% (muted) to 100% (primary), for the heatmap tiles.
const coverageTint = (p: number) => `color-mix(in oklch, var(--primary) ${p}%, var(--muted))`;

// Ship-gate stage colours, shared by the per-language timeline + the row cells.
const STAGE_COLOR: Record<string, string> = {
  shippable: "oklch(0.62 0.17 150)", // green
  review: "oklch(0.72 0.15 80)", // amber
  translated: "var(--primary)", // blue
  none: "var(--muted-foreground)",
};

// The ship-gate ladder rung for a (collection, locale) scope, derived from the
// convergence report. `pct` is the translated coverage shown as a secondary
// figure; the label/colour convey how far along the gate ladder it is.
interface Rung {
  key: "shippable" | "review" | "draft" | "none";
  label: string;
  short: string;
  color: string;
  pct: number;
}
function rungFor(lc?: LocaleCoverage): Rung {
  const translated = lc?.pct?.translated ?? 0;
  if (!lc || translated === 0) {
    return { key: "none", label: "—", short: "—", color: "var(--muted-foreground)", pct: 0 };
  }
  if (lc.shippable) {
    return {
      key: "shippable",
      label: "Shippable",
      short: "Ship",
      color: "oklch(0.62 0.17 150)",
      pct: translated,
    };
  }
  if ((lc.pct?.reviewed ?? 0) > 0) {
    return {
      key: "review",
      label: "In review",
      short: "Review",
      color: "oklch(0.72 0.15 80)",
      pct: translated,
    };
  }
  return { key: "draft", label: "Draft", short: "Draft", color: "var(--primary)", pct: translated };
}

// Above this many target languages the per-language bar columns get cramped, so
// the coverage layout switches to the compact heatmap (issue #1068 review).
const HEATMAP_LANG_THRESHOLD = 5;

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

export interface CollectionsPanelProps {
  project: KapiProject;
  onUpdate: (project: KapiProject) => void;
  tabID: string;
  /** The project's flows, offered as a per-collection "Run" menu on each card. */
  flows?: Record<string, FlowSpec>;
  /** Run a flow scoped to a single collection's files. */
  onRunFlow?: RunFlowHandler;
  /** Pre-loaded formats for Storybook — skips api.listFormats(). */
  formatList?: FormatInfo[];
  /** Pre-loaded base path for Storybook — skips api.getBasePath(). */
  basePath?: string;
  /** Pre-loaded status for Storybook/tests — skips api.getProjectStatus(). */
  status?: ProjectStatus;
  /** Pre-loaded convergence for Storybook/tests — skips api.getConvergence(). */
  convergence?: ConvergenceReport;
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

/**
 * The status label the backend keys a collection's coverage under — the
 * collection name, or "(unnamed)" for a bare entry / a name-less collection.
 * Mirrors `collectionLabel` in the Go backend (status.go).
 */
function statusLabelOf(coll: ContentCollection): string {
  if (isBareEntry(coll)) return "(unnamed)";
  return coll.name && coll.name.length > 0 ? coll.name : "(unnamed)";
}

/**
 * Collapse a source file's per-locale outputs into one templated path, replacing
 * the locale path segment with {lang} — e.g. output/de-DE/docs/api-reference.md
 * → output/{lang}/docs/api-reference.md. All locales share the same shape, so
 * one templated line stands in for the lot; expand it for the per-locale detail.
 */
function templatedOutputPath(outs: OutputFileInfo[]): string {
  const o = outs[0];
  if (!o) return "";
  return o.relative
    .split("/")
    .map((seg) => (seg === o.lang ? "{lang}" : seg))
    .join("/");
}

/** A compact inline coverage cell for the project-wide strip: "loc ▮▮▯ 78%". */
function StripBar({ label, pct, color }: { label: string; pct: number; color?: string }) {
  return (
    <span className="flex min-w-40 flex-1 items-center gap-2">
      <span className="w-14 shrink-0 text-xs text-muted-foreground" translate="no">
        {label}
      </span>
      <span className="h-1.5 flex-1 overflow-hidden rounded-full bg-accent">
        <span
          className="block h-full rounded-full bg-primary transition-all"
          style={{ width: `${pct}%`, ...(color ? { background: color } : {}) }}
        />
      </span>
      <span className="w-9 shrink-0 text-right text-[11px] tabular-nums text-muted-foreground">
        {pct}%
      </span>
    </span>
  );
}

/** TimelineItem: one language's overall standing for the completeness timeline,
 *  plus its per-collection breakdown (revealed as dots on hover). */
interface TimelineItem {
  lang: string;
  pct: number; // overall translated coverage, 0–100
  stage: keyof typeof STAGE_COLOR;
  byCollection: { name: string; pct: number; stage: keyof typeof STAGE_COLOR }[];
}

/** LanguageTimeline plots each language on a 0→100% completeness axis as a dot
 *  on the line, with a vertical stem + arrow to its tag. Tags alternate above and
 *  below the line and stack into lanes where languages cluster, so stems stay
 *  short. Colours encode the ship-gate stage. Hovering a tag expands the language
 *  into a dot per collection (others dim) with a breakdown popover. */
function LanguageTimeline({ items }: { items: TimelineItem[] }) {
  const ref = useRef<HTMLDivElement>(null);
  const [width, setWidth] = useState(0);
  const [hovered, setHovered] = useState<string | null>(null);
  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const ro = new ResizeObserver((entries) => setWidth(entries[0].contentRect.width));
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  const PAD = 38; // horizontal inset so the 0% / 100% tags don't clip
  const CHIP = 62; // min horizontal gap before a tag stacks into the next lane
  const ROW = 22; // extra stem length per lane
  const BASE = 16; // shortest stem (lane 0)
  const TAG_H = 16; // tag height
  const color = (s: string) => STAGE_COLOR[s] ?? STAGE_COLOR.none;
  const xOf = (pct: number) => PAD + (pct / 100) * usable;

  // Greedy lane assignment on an ascending copy, then alternate lanes above/below
  // the line (even → above, odd → below) so each side needs half the stem height.
  const usable = Math.max(0, width - 2 * PAD);
  const sorted = [...items].sort((a, b) => a.pct - b.pct);
  const laneLast: number[] = [];
  const placed = sorted.map((it) => {
    const x = xOf(it.pct);
    let lane = 0;
    while (lane < laneLast.length && x - laneLast[lane] < CHIP) lane++;
    laneLast[lane] = x;
    const above = lane % 2 === 0;
    const sideLane = Math.floor(lane / 2);
    return { it, x, above, sideLane };
  });
  const sideExtent = (above: boolean) => {
    const ls = placed.filter((p) => p.above === above).map((p) => p.sideLane);
    return ls.length ? TAG_H + BASE + Math.max(...ls) * ROW : 0;
  };
  const aboveExtent = width > 0 ? sideExtent(true) : TAG_H + BASE;
  const belowExtent = width > 0 ? sideExtent(false) : 0;
  const axisY = aboveExtent + 8;
  const scaleY = axisY + belowExtent + 8;
  const height = scaleY + 12;
  const hoveredItem = placed.find((p) => p.it.lang === hovered)?.it;

  return (
    <div>
      <div className="mb-1 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
        {t("Completeness by language")}
      </div>
      <div ref={ref} className="relative" style={{ height }}>
        {/* faint gridlines */}
        {width > 0 &&
          [25, 50, 75].map((g) => (
            <div
              key={g}
              className="absolute w-px bg-border/40"
              style={{ left: xOf(g), top: 0, height: scaleY }}
            />
          ))}
        {/* the timeline itself */}
        <div className="absolute h-px bg-border" style={{ left: PAD, right: PAD, top: axisY }} />
        {/* each language: tag → stem → arrow → dot on the line */}
        {width > 0 &&
          placed.map(({ it, x, above, sideLane }) => {
            const c = color(it.stage);
            const stem = BASE + sideLane * ROW;
            const dim = hovered && hovered !== it.lang ? 0.25 : 1;
            const tagTop = above ? axisY - stem - TAG_H : axisY + stem;
            return (
              // Static wrapper: opacity dims the whole group; children still
              // anchor to the (positioned) timeline container.
              <span key={it.lang} style={{ opacity: dim, transition: "opacity 120ms" }}>
                {/* stem */}
                <span
                  className="absolute -translate-x-1/2"
                  style={{
                    left: x,
                    top: above ? axisY - stem : axisY,
                    width: 1,
                    height: stem - 4,
                    background: `color-mix(in oklch, ${c} 55%, var(--border))`,
                  }}
                />
                {/* arrowhead pointing at the dot */}
                <span
                  className="absolute -translate-x-1/2"
                  style={{
                    left: x,
                    top: above ? axisY - 8 : axisY + 4,
                    width: 0,
                    height: 0,
                    borderLeft: "3px solid transparent",
                    borderRight: "3px solid transparent",
                    ...(above
                      ? { borderTop: `4px solid ${c}` }
                      : { borderBottom: `4px solid ${c}` }),
                  }}
                />
                {/* dot on the line */}
                <span
                  className="absolute rounded-full"
                  style={{
                    left: x,
                    top: axisY,
                    width: 8,
                    height: 8,
                    transform: "translate(-50%, -50%)",
                    background: c,
                    border: "2px solid var(--card)",
                  }}
                />
                {/* tag */}
                <span
                  className="absolute -translate-x-1/2 cursor-default"
                  style={{ left: x, top: tagTop }}
                  onMouseEnter={() => setHovered(it.lang)}
                  onMouseLeave={() => setHovered(null)}
                >
                  <span
                    className="inline-flex items-center gap-1 whitespace-nowrap rounded-full border px-1.5 py-0.5 text-[10px] font-medium shadow-sm"
                    style={{
                      borderColor: c,
                      background: `color-mix(in oklch, ${c} 12%, var(--card))`,
                    }}
                    title={`${it.lang}: ${it.pct}% translated`}
                  >
                    <span translate="no">{it.lang}</span>
                    <span className="tabular-nums text-muted-foreground">{it.pct}</span>
                  </span>
                </span>
              </span>
            );
          })}
        {/* hover expansion: a dot per collection on the line + breakdown popover */}
        {width > 0 && hoveredItem && hoveredItem.byCollection.length > 0 && (
          <>
            {hoveredItem.byCollection.map((cc, i) => (
              <span
                key={`${cc.name}-${i}`}
                className="absolute rounded-full"
                style={{
                  left: xOf(cc.pct),
                  top: axisY,
                  width: 7,
                  height: 7,
                  transform: "translate(-50%, -50%)",
                  background: color(cc.stage),
                  border: "1.5px solid var(--card)",
                  boxShadow: "0 0 0 1px var(--border)",
                }}
                title={`${cc.name}: ${cc.pct}%`}
              />
            ))}
            <div
              className="absolute z-10 -translate-x-1/2 rounded-md border border-border bg-popover p-1.5 text-[10px] shadow-md"
              style={{
                left: Math.min(Math.max(xOf(hoveredItem.pct), 70), Math.max(70, width - 70)),
                top: scaleY + 2,
                minWidth: 120,
              }}
            >
              <div className="mb-0.5 font-medium" translate="no">
                {hoveredItem.lang}
              </div>
              {hoveredItem.byCollection.map((cc, i) => (
                <div key={`${cc.name}-${i}`} className="flex items-center gap-1.5">
                  <span
                    className="size-1.5 shrink-0 rounded-full"
                    style={{ background: color(cc.stage) }}
                  />
                  <span className="flex-1 truncate text-muted-foreground">{cc.name}</span>
                  <span className="tabular-nums">{cc.pct}%</span>
                </div>
              ))}
            </div>
          </>
        )}
        {/* % scale under the line */}
        {width > 0 &&
          [0, 50, 100].map((tk) => (
            <span
              key={tk}
              className="absolute -translate-x-1/2 text-[9px] text-muted-foreground"
              style={{ left: xOf(tk), top: scaleY }}
            >
              {tk}%
            </span>
          ))}
      </div>
      {/* legend */}
      <div className="mt-1 flex flex-wrap gap-x-3 gap-y-1 text-[10px] text-muted-foreground">
        {(
          [
            ["shippable", t("Shippable")],
            ["review", t("In review")],
            ["translated", t("Translated")],
          ] as const
        ).map(([k, label]) => (
          <span key={k} className="flex items-center gap-1">
            <span className="size-2 rounded-full" style={{ background: color(k) }} />
            {label}
          </span>
        ))}
      </div>
    </div>
  );
}

/**
 * CollectionsPanel is the collection-centric spine of the project home: one card
 * per content collection carrying its own stats (file count, block count,
 * coverage bar), expandable to its matched-file table and editable inline. It
 * folds together what used to be the standalone Content page and the Home
 * page's read-only Content Overview (issue #1068) — collections, files,
 * patterns, languages, coverage and the unmatched "Other files" all live here.
 */
export function CollectionsPanel({
  project,
  onUpdate,
  tabID,
  flows,
  onRunFlow,
  formatList: propFormats,
  basePath: propBasePath,
  status: propStatus,
  convergence: propConvergence,
}: CollectionsPanelProps) {
  const { showError } = useError();
  const { locales } = useLocales();
  const { hasActive } = useJobFeed();
  const shortenHome = useShortenHome();
  const {
    active: activeFilter,
    setActive: setActiveFilter,
    enabled: filterEnabled,
  } = useActiveFilter();
  const [matches, setMatches] = useState<FileMatch[]>([]);
  const [projectFiles, setProjectFiles] = useState<ProjectFile[]>([]);
  const [basePath, setBasePath] = useState(propBasePath ?? "");
  const [scanning, setScanning] = useState(false);
  const [extracting, setExtracting] = useState(false);
  const [formats, setFormats] = useState<FormatInfo[]>(propFormats ?? []);
  const [status, setStatus] = useState<ProjectStatus | null>(propStatus ?? null);
  // Ship-gate ladder standing per (collection, locale) — drives the coverage
  // cells (Shippable / In review / Draft / —) and the project-wide strip.
  const [convergence, setConvergence] = useState<ConvergenceReport | null>(propConvergence ?? null);
  // Flow validity (unknown tools, undeclared plugins) so we never offer to run a
  // broken flow — the run menus disable invalid flows with the reason.
  const [flowValidation, setFlowValidation] = useState<Record<string, FlowInfo>>({});
  const [dragging, setDragging] = useState(false);
  // configKey of the content item whose format-config modal is open (one at a time).
  const [dialogKey, setDialogKey] = useState<string | null>(null);
  // Per-collection-card UI state: which cards are expanded (file table visible)
  // and which are in edit mode (config editor over the files). Keyed by index.
  const [expanded, setExpanded] = useState<Set<number>>(new Set());
  const [editing, setEditing] = useState<Set<number>>(new Set());
  // Collections ticked for a batch run (keyed by index into project.content).
  const [selected, setSelected] = useState<Set<number>>(new Set());
  const [otherCollapsed, setOtherCollapsed] = useState(false);
  // Generated output files keyed by their source file's relative path (issue #5),
  // plus the set of source rows whose outputs are expanded.
  const [outputs, setOutputs] = useState<Record<string, OutputFileInfo[]>>({});
  const [expandedOutputs, setExpandedOutputs] = useState<Set<string>>(new Set());
  // Second level under a source row: the templated output/{lang}/… line expands
  // to its per-locale output files.
  const [expandedTemplate, setExpandedTemplate] = useState<Set<string>>(new Set());
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
  const flowNames = Object.keys(flows ?? {});

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
  // modal's default selection).
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

  const refreshStatus = useCallback(() => {
    if (propStatus) return;
    void api
      .getProjectStatus(tabID)
      .then((s) => {
        if (s) setStatus(s);
      })
      .catch(() => {
        /* status is best-effort */
      });
  }, [tabID, propStatus]);

  const refreshConvergence = useCallback(() => {
    if (propConvergence) return;
    void api
      .getConvergence(tabID)
      .then((c) => {
        if (c) setConvergence(c);
      })
      .catch(() => {
        /* convergence is best-effort */
      });
  }, [tabID, propConvergence]);

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

  // Scan files + load coverage on mount and whenever the collection set changes.
  useEffect(() => {
    void rescanFiles();
  }, [rescanFiles, content.length]);

  useEffect(() => {
    refreshStatus();
    refreshConvergence();
  }, [refreshStatus, refreshConvergence, project.content]);

  // Validate the project's flows so the run menus can disable broken ones.
  useEffect(() => {
    if (!tabID || !onRunFlow) return;
    void api.listFlows(tabID).then((fl) => {
      if (!fl) return;
      const map: Record<string, FlowInfo> = {};
      for (const f of fl) map[f.name] = f;
      setFlowValidation(map);
    });
  }, [tabID, onRunFlow, project.flows]);

  useWailsEvent("project-files-changed", (data) => {
    if (data === tabID) void rescanFiles();
  });

  // A flow run wrote an output file — refresh so it appears beneath its source
  // immediately, even while the run is still in progress (issue #5).
  useWailsEvent("outputs-changed", () => refreshOutputs());

  // An extraction completed (e.g. from another surface) — refresh coverage.
  useWailsEvent("project:extracted", () => {
    refreshStatus();
    refreshConvergence();
  });

  // Re-extract reads every source file into the block store (refreshing block
  // counts + coverage) and re-scans the file tables in one go.
  const handleExtract = useCallback(async () => {
    if (!tabID || hasPreloadedData) return;
    setExtracting(true);
    try {
      await api.runExtract(tabID);
      refreshStatus();
      refreshConvergence();
      await rescanFiles();
    } catch (err) {
      showError("Extraction failed", err);
    } finally {
      setExtracting(false);
    }
  }, [tabID, hasPreloadedData, refreshStatus, refreshConvergence, rescanFiles, showError]);

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

  // ── Active-filter narrowing (bug #1 — consistent everywhere) ───────────────
  // The active filter narrows which collections are shown. We track which
  // collections it hides so a visible "show all" affordance keeps anything from
  // vanishing without explanation. Filtering by collection name; an empty
  // collection dimension shows all. Indices are preserved so editing still
  // targets the right project.content entry.
  const filterCollections = activeFilter?.collections ?? [];
  const collectionVisible = useCallback(
    (coll: ContentCollection) =>
      filterCollections.length === 0 || filterCollections.includes(statusLabelOf(coll)),
    [filterCollections],
  );
  const visibleContent = content
    .map((coll, ci) => ({ coll, ci }))
    .filter(({ coll }) => collectionVisible(coll));
  const hiddenCount = content.length - visibleContent.length;
  const filterActive = filterEnabled && !!activeFilter;

  // ── Per-collection stats (block counts + coverage), keyed by status label ──
  const statusByLabel = useMemo(() => {
    const map = new Map<string, CollectionStatus>();
    for (const c of status?.collections ?? []) map.set(c.name, c);
    return map;
  }, [status]);

  // ── Project-wide coverage strip (over the visible collections) ─────────────
  const visibleStatuses = visibleContent
    .map(({ coll }) => statusByLabel.get(statusLabelOf(coll)))
    .filter((c): c is CollectionStatus => !!c);
  const totalBlocks = visibleStatuses.reduce((s, c) => s + c.blockCount, 0);
  const stripLangs = Array.from(
    new Set(visibleStatuses.flatMap((c) => filterLanguages(c.targetLanguages, activeFilter))),
  );
  const stripCoverage = stripLangs.map((lang) => {
    let translated = 0;
    let total = 0;
    for (const c of visibleStatuses) {
      if (!c.targetLanguages.includes(lang)) continue;
      translated += c.coverage?.[lang] ?? 0;
      total += c.blockCount;
    }
    return { lang, pct: total > 0 ? Math.round((translated / total) * 100) : 0 };
  });
  const hasData = !!status?.hasData;

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

        {/* Format configuration — schema-driven modal */}
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
                "This pattern auto-detects a format per file. Tune any of them here — settings apply project-wide.",
              )}
              formats={matchedFormats}
              allFormats={formats}
              allowAdd
              filterExtension={globExtension(item.path)}
              values={projectFormatValues}
              onChange={updateProjectFormat}
              scopeNote={t(
                "Stored in the project's defaults.formats — shared by every content item.",
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

  const openCard = (ci: number) => setExpanded((prev) => new Set(prev).add(ci));

  // The glob patterns a content entry declares, and the matched files for them.
  const patternsOf = (coll: ContentCollection) =>
    isBareEntry(coll) ? [coll.path ?? ""] : (coll.items ?? []).map((i) => i.path);
  const filesForEntry = (coll: ContentCollection) => {
    const pats = new Set(patternsOf(coll).filter(Boolean));
    return matches.filter((m) => pats.has(m.pattern));
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
          const templated = templatedOutputPath(outs);
          const tOpen = expandedTemplate.has(m.relative);
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
              {/* One templated output line stands in for every locale; expand
                  it for the per-locale files. */}
              {isOpen && outs.length > 0 && (
                <tr
                  onClick={() =>
                    setExpandedTemplate((prev) => {
                      const next = new Set(prev);
                      if (next.has(m.relative)) next.delete(m.relative);
                      else next.add(m.relative);
                      return next;
                    })
                  }
                  className="cursor-pointer border-b border-border last:border-0 hover:bg-accent/30"
                  title={tOpen ? t("Hide per-language outputs") : t("Show per-language outputs")}
                >
                  <td className="py-1 pl-9 pr-3">
                    <span className="flex items-center gap-1.5 font-mono text-muted-foreground">
                      {tOpen ? (
                        <ChevronDown size={11} className="shrink-0" />
                      ) : (
                        <ChevronRight size={11} className="shrink-0" />
                      )}
                      <ArrowRight size={10} className="shrink-0 opacity-50" />
                      <span translate="no">{templated}</span>
                    </span>
                  </td>
                  <td className="px-3 py-1">
                    <Badge variant="secondary">{m.format || "—"}</Badge>
                  </td>
                  <td className="px-3 py-1 text-right">
                    <Badge variant="outline" className="text-[10px] font-normal">
                      {t("{present}/{total} generated", { present, total: outs.length })}
                    </Badge>
                  </td>
                </tr>
              )}
              {isOpen &&
                tOpen &&
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
                    <td className="py-1 pl-16 pr-3">
                      <span className="flex items-center gap-1.5 font-mono text-muted-foreground">
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
          const fileExpanded = expandedArchives.has(f.path);
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
                      fileExpanded ? (
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
              {archive && fileExpanded && (
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

  // ── Batch selection → run a flow across the ticked collections ─────────────
  // Selecting is only meaningful when there are flows to run; the union of the
  // selected collections' matched files is the run scope.
  const selectable = !!onRunFlow && flowNames.length > 0;
  const visibleIndices = visibleContent.map((v) => v.ci);
  const allVisibleSelected =
    visibleIndices.length > 0 && visibleIndices.every((i) => selected.has(i));
  const selectedPaths = Array.from(
    new Set(
      visibleContent
        .filter((v) => selected.has(v.ci))
        .flatMap(({ coll }) => filesForEntry(coll).map((m) => m.path)),
    ),
  );
  const toggleSelect = (ci: number) => toggle(setSelected, ci);
  const clearSelection = () => setSelected(new Set());
  const toggleSelectAll = () =>
    setSelected(allVisibleSelected ? new Set() : new Set(visibleIndices));

  // The single flow picker in the section header is scope-aware: it runs across
  // the ticked collections when any are selected, else across the whole project
  // (the runner narrows "all" by the active filter). This is what folds the old
  // standalone "Run Flows" list into the collection surface (issue #1068).
  const hasSelection = selected.size > 0;
  const runReady = hasSelection ? selectedPaths.length > 0 : matches.length > 0;
  const flowValid = (name: string) => flowValidation[name]?.valid !== false;
  const flowRunTitle = (name: string) => {
    const v = flowValidation[name];
    if (v && v.valid === false) {
      return t("Cannot run: {issues}", {
        issues: (v.issues ?? []).map((i) => i.message).join("; "),
      });
    }
    if (!runReady) return t("No matched files to run on");
    return undefined;
  };
  const runFlowScoped = (name: string, spec: FlowSpec) => {
    if (hasSelection) {
      onRunFlow?.(name, spec, {
        scopePaths: selectedPaths,
        scopeLabel: t("{count} collections", { count: selected.size }),
      });
      clearSelection();
    } else {
      onRunFlow?.(name, spec);
    }
  };

  // ── Aligned coverage layout + colour-coded cake (issue #1068 review) ────────
  // Collections are coloured by their position in the displayed list so a row's
  // dot matches its cake slice. The coverage columns are the union of the
  // displayed collections' target languages (narrowed by the filter); with many
  // languages the per-language bars give way to a compact heatmap.
  const columnLangs = filterLanguages(
    Array.from(
      new Set(
        visibleContent.flatMap(({ coll }) =>
          (coll.target_languages ?? project.defaults?.target_languages ?? []).map(String),
        ),
      ),
    ),
    activeFilter,
  );
  const heatmap = columnLangs.length >= HEATMAP_LANG_THRESHOLD;
  // Ship-gate ladder standing per (collection, locale), keyed the same way the
  // convergence report reports it (collection "" for bare/unnamed entries).
  const covScope = useMemo(() => {
    const m = new Map<string, LocaleCoverage>();
    for (const lc of convergence?.locales ?? []) {
      m.set(`${lc.collection ?? ""} ${lc.locale}`, lc);
    }
    return m;
  }, [convergence]);
  const hasGates = covScope.size > 0;
  const scopeCov = (coll: ContentCollection, lang: string): LocaleCoverage | undefined => {
    const name = isBareEntry(coll) ? "" : (coll.name ?? "");
    return covScope.get(`${name} ${lang}`) ?? covScope.get(` ${lang}`);
  };
  // Coverage columns render once there's either extracted data or gate standing.
  const showCoverageCols = (hasData || hasGates) && columnLangs.length > 0;
  // Per-language standing for the completeness timeline: overall translated
  // coverage (position) + the ship-gate stage (colour), aggregated across every
  // (collection, locale) scope for that language, weighted by unit count.
  const scopeStage = (lc: LocaleCoverage): TimelineItem["stage"] => {
    const tr = lc.pct?.translated ?? 0;
    if (tr === 0) return "none";
    if (lc.shippable) return "shippable";
    return (lc.pct?.reviewed ?? 0) > 0 ? "review" : "translated";
  };
  const timelineItems: TimelineItem[] | null = hasGates
    ? columnLangs.map((lang) => {
        let total = 0;
        let tSum = 0;
        let rSum = 0;
        let shippableUnits = 0;
        const byCollection: TimelineItem["byCollection"] = [];
        for (const lc of convergence?.locales ?? []) {
          if (lc.locale !== lang) continue;
          total += lc.total;
          tSum += (lc.total * (lc.pct?.translated ?? 0)) / 100;
          rSum += (lc.total * (lc.pct?.reviewed ?? 0)) / 100;
          if (lc.shippable) shippableUnits += lc.total;
          byCollection.push({
            name: lc.collection || t("(unnamed)"),
            pct: Math.round(lc.pct?.translated ?? 0),
            stage: scopeStage(lc),
          });
        }
        byCollection.sort((a, b) => b.pct - a.pct);
        const pct = total > 0 ? Math.round((tSum / total) * 100) : 0;
        const reviewed = total > 0 ? Math.round((rSum / total) * 100) : 0;
        const stage: TimelineItem["stage"] =
          total === 0
            ? "none"
            : shippableUnits / total >= 0.999
              ? "shippable"
              : reviewed > 0
                ? "review"
                : pct > 0
                  ? "translated"
                  : "none";
        return { lang, pct, stage, byCollection };
      })
    : null;
  // Per-(collection, language) translated coverage %, or null when the
  // collection doesn't target that language / nothing extracted. Used as the
  // fallback view before any convergence (ship-gate) data is available.
  const covPct = (coll: ContentCollection, lang: string): number | null => {
    const cs = statusByLabel.get(statusLabelOf(coll));
    if (!cs || cs.blockCount === 0 || !cs.targetLanguages.includes(lang)) return null;
    return Math.round(((cs.coverage?.[lang] ?? 0) / cs.blockCount) * 100);
  };
  // One coverage cell: a ship-gate rung (Shippable / In review / Draft / —) with
  // the translated % as a secondary figure once convergence is available; the
  // translated-only bar/tile before then.
  const langCell = (coll: ContentCollection, lang: string) => {
    if (hasGates) {
      const r = rungFor(scopeCov(coll, lang));
      if (r.key === "none") {
        return <span className="text-center text-[10px] text-muted-foreground/40">&mdash;</span>;
      }
      return heatmap ? (
        <span
          className="flex items-center justify-center gap-1 text-[10px]"
          title={`${lang}: ${r.label} · ${r.pct}% translated`}
        >
          <span className="size-2 shrink-0 rounded-full" style={{ background: r.color }} />
          <span className="tabular-nums text-muted-foreground">{r.pct}</span>
        </span>
      ) : (
        <span
          className="flex flex-col items-center gap-0.5"
          title={`${lang}: ${r.label} · ${r.pct}% translated`}
        >
          <span className="text-[10px] font-medium leading-none" style={{ color: r.color }}>
            {r.label}
          </span>
          <span className="text-[10px] tabular-nums text-muted-foreground">{r.pct}%</span>
        </span>
      );
    }
    const p = covPct(coll, lang);
    if (p === null) {
      return <span className="text-center text-[10px] text-muted-foreground/40">&mdash;</span>;
    }
    return heatmap ? (
      <span
        className="flex h-6 items-center justify-center rounded text-[10px] font-medium tabular-nums"
        style={{
          background: coverageTint(p),
          color: p > 55 ? "var(--primary-foreground)" : "var(--muted-foreground)",
        }}
        title={`${lang}: ${p}%`}
      >
        {p}
      </span>
    ) : (
      <span className="flex flex-col items-center gap-1" title={`${lang}: ${p}%`}>
        <span className="h-1.5 w-full overflow-hidden rounded-full bg-accent">
          <span className="block h-full rounded-full bg-primary" style={{ width: `${p}%` }} />
        </span>
        <span className="text-[10px] tabular-nums text-muted-foreground">{p}%</span>
      </span>
    );
  };
  // Cake slices: one per displayed collection with blocks, coloured by position.
  const cake = visibleContent.map(({ coll }, idx) => ({
    name: statusLabelOf(coll),
    value: statusByLabel.get(statusLabelOf(coll))?.blockCount ?? 0,
    fill: collectionColor(idx),
  }));
  const cakeSlices = cake.filter((d) => d.value > 0);

  // Grid template shared by the header + every row so columns line up. The
  // coverage block is N language columns (bars or heatmap tiles), or a single
  // flexible spacer before the actions when there's nothing to show.
  const coverageCols = showCoverageCols
    ? `repeat(${columnLangs.length}, minmax(${heatmap ? 40 : 60}px, 1fr))`
    : "1fr";
  const gridCols = `${selectable ? "24px " : ""}minmax(150px,1.6fr) 52px 62px ${coverageCols} auto`;

  return (
    <section className="mb-8">
      {/* Section header — Collections is the spine; actions live here. */}
      <div className="mb-3 flex flex-wrap items-center gap-2">
        <h2 className="flex items-center gap-2 text-sm font-semibold uppercase tracking-wider text-muted-foreground">
          <FileText size={14} />
          {t("Collections")}
        </h2>
        {basePath && (
          <span className="hidden text-xs text-muted-foreground sm:inline">
            {t("relative to {base}", { base: shortenHome(basePath) })}
          </span>
        )}
        <div className="ml-auto flex items-center gap-2">
          {/* Scope-aware flow runner — folds the old "Run Flows" list in here.
              Runs across the ticked collections, or the whole project (narrowed
              by the active filter) when nothing is selected. */}
          {selectable &&
            (flowNames.length === 1 ? (
              <Button
                size="sm"
                disabled={hasActive || !runReady || !flowValid(flowNames[0])}
                title={flowRunTitle(flowNames[0])}
                onClick={() => runFlowScoped(flowNames[0], flows![flowNames[0]])}
                aria-label={
                  hasSelection
                    ? t("Run {flow} on selected collections", { flow: flowNames[0] })
                    : t("Run {flow} on all collections", { flow: flowNames[0] })
                }
              >
                <Play size={12} />
                {hasSelection ? t("Run on selected") : t("Run {flow}", { flow: flowNames[0] })}
              </Button>
            ) : (
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button size="sm" disabled={hasActive || !runReady} aria-label={t("Run a flow")}>
                    <Play size={12} />
                    {hasSelection ? t("Run on selected") : t("Run flow")}
                    <ChevronDown size={12} />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end">
                  <DropdownMenuLabel>
                    {hasSelection
                      ? t("Run on {count} collections", { count: selected.size })
                      : t("Run on all collections")}
                  </DropdownMenuLabel>
                  {flowNames.map((fn) => (
                    <DropdownMenuItem
                      key={fn}
                      disabled={!runReady || !flowValid(fn)}
                      title={flowRunTitle(fn)}
                      onClick={() => runFlowScoped(fn, flows![fn])}
                    >
                      <Play size={12} />
                      {fn}
                    </DropdownMenuItem>
                  ))}
                </DropdownMenuContent>
              </DropdownMenu>
            ))}
          <Button
            variant="outline"
            size="sm"
            onClick={handleAddCollection}
            aria-label="Add content collection"
          >
            <Plus size={12} />
            {t("Add Collection")}
          </Button>
          <Button variant="outline" size="sm" onClick={handleAddFiles} aria-label="Add files">
            <Plus size={12} />
            {t("Add Files")}
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={() => void handleExtract()}
            disabled={extracting || scanning}
            aria-label={hasData ? "Re-extract content" : "Run extract"}
          >
            {extracting ? <Loader2 size={12} className="animate-spin" /> : <RefreshCw size={12} />}
            {hasData ? t("Re-extract") : t("Extract")}
          </Button>
        </div>
      </div>

      {/* Active-filter escape hatch (bug #1) — collections never vanish silently. */}
      {filterActive && (
        <div className="mb-3 flex items-center gap-2 rounded-md border border-border bg-muted/40 px-3 py-1.5 text-xs">
          <Filter size={12} className="shrink-0 text-muted-foreground" />
          <span className="text-muted-foreground">
            {hiddenCount > 0
              ? t("Filtered by {name} — {count} collection(s) hidden", {
                  name: activeFilter.name,
                  count: hiddenCount,
                })
              : t("Filtered by {name}", { name: activeFilter.name })}
          </span>
          <Button
            variant="link"
            size="xs"
            className="ml-auto h-auto px-0"
            onClick={() => void setActiveFilter("")}
          >
            {t("Show all")}
          </Button>
        </div>
      )}

      {/* Stale store banner (bug #2) — counts produced by an older kapi. */}
      {status?.stale && (
        <div className="mb-3 flex items-center gap-2 rounded-md border border-amber-500/30 bg-amber-500/5 px-3 py-2 text-xs">
          <AlertTriangle size={13} className="shrink-0 text-amber-500" />
          <span className="text-muted-foreground">
            {t("These counts were produced by an earlier version of kapi and may be out of date.")}
          </span>
          <Button
            variant="outline"
            size="xs"
            className="ml-auto"
            onClick={() => void handleExtract()}
            disabled={extracting}
          >
            {extracting ? <Loader2 size={11} className="animate-spin" /> : <RefreshCw size={11} />}
            {t("Re-extract")}
          </Button>
        </div>
      )}

      {/* Colour-coded "cake": block distribution per collection (slices match
          the row dots below) + project-wide coverage per language. */}
      {content.length > 0 &&
        (hasData ? (
          <Card className="mb-3 p-4">
            <div className="grid gap-6 sm:grid-cols-[auto_1fr] sm:items-center">
              <div className="flex items-center gap-3">
                {cakeSlices.length > 0 ? (
                  <div className="h-28 w-28 shrink-0">
                    <ResponsiveContainer width="100%" height="100%">
                      <PieChart>
                        <Pie
                          data={cakeSlices}
                          dataKey="value"
                          nameKey="name"
                          innerRadius="56%"
                          outerRadius="100%"
                          paddingAngle={cakeSlices.length > 1 ? 2 : 0}
                          strokeWidth={0}
                        >
                          {cakeSlices.map((d) => (
                            <Cell key={d.name} fill={d.fill} />
                          ))}
                        </Pie>
                      </PieChart>
                    </ResponsiveContainer>
                  </div>
                ) : (
                  <div className="flex h-28 w-28 shrink-0 items-center justify-center rounded-full border border-dashed text-[10px] text-muted-foreground">
                    {t("No blocks")}
                  </div>
                )}
                <ul className="space-y-1 text-xs">
                  <li className="font-medium text-foreground">
                    {t("{count} blocks", { count: totalBlocks })}
                  </li>
                  {cake.map((d, idx) => (
                    <li key={d.name} className="flex items-center gap-1.5">
                      <span
                        className="size-2 shrink-0 rounded-[2px]"
                        style={{ background: collectionColor(idx) }}
                      />
                      <span className="truncate text-muted-foreground">{d.name}</span>
                      <span className="tabular-nums text-foreground">{d.value}</span>
                    </li>
                  ))}
                </ul>
              </div>
              {timelineItems && timelineItems.length > 0 ? (
                <LanguageTimeline items={timelineItems} />
              ) : (
                stripCoverage.length > 0 && (
                  <div className="space-y-1.5">
                    <div className="mb-1 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
                      {t("Coverage across collections")}
                    </div>
                    <div className="flex flex-wrap gap-x-6 gap-y-1.5">
                      {stripCoverage.map((p) => (
                        <StripBar key={p.lang} label={p.lang} pct={p.pct} />
                      ))}
                    </div>
                  </div>
                )
              )}
            </div>
          </Card>
        ) : (
          <Card className="mb-3 flex items-center gap-3 p-4">
            <PackageOpen size={18} className="shrink-0 text-muted-foreground/50" />
            <div className="flex-1 text-xs text-muted-foreground">
              {t("Nothing extracted yet — run extract to read your content and analyze coverage.")}
            </div>
            <Button
              size="sm"
              onClick={() => void handleExtract()}
              disabled={extracting || scanning}
            >
              {extracting ? (
                <>
                  <Loader2 size={12} className="animate-spin" />
                  {t("Extracting...")}
                </>
              ) : (
                t("Run extract")
              )}
            </Button>
          </Card>
        ))}

      {/* Archive collections get the translation-state panel (unchanged). */}
      {content.some((c) => c.archive) && (
        <div className="mb-4">
          <TranslationStatusPanel tabID={tabID} />
        </div>
      )}

      {/* Selection bar — appears once collections are ticked. The run action
          itself lives in the scope-aware "Run" picker in the section header
          (which switches to "Run on selected" while a selection is active). */}
      {selectable && selected.size > 0 && (
        <div className="sticky top-2 z-10 mb-3 flex flex-wrap items-center gap-2 rounded-md border border-primary/40 bg-primary/10 px-3 py-2 text-xs shadow-sm backdrop-blur">
          <span className="font-medium">{t("{count} selected", { count: selected.size })}</span>
          <span className="text-muted-foreground">
            {t("{count} files", { count: selectedPaths.length })}
          </span>
          <span className="text-muted-foreground">·</span>
          <span className="text-muted-foreground">{t("run via Run on selected, above")}</span>
          <div className="ml-auto flex items-center gap-2">
            <Button variant="ghost" size="xs" onClick={toggleSelectAll}>
              {allVisibleSelected ? t("Deselect all") : t("Select all")}
            </Button>
            <Button variant="ghost" size="xs" onClick={clearSelection}>
              {t("Clear")}
            </Button>
          </div>
        </div>
      )}

      {/* The collection cards + unmatched "Other files", with drop-to-add. */}
      <div
        onDrop={handleDrop}
        onDragOver={handleDragOver}
        onDragLeave={handleDragLeave}
        className={`rounded-lg border-2 transition-colors ${
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
          <div className="overflow-hidden rounded-lg border border-border">
            {/* Column header — shares the row grid so everything lines up. */}
            {visibleContent.length > 0 && (
              <div
                className="grid items-center gap-x-3 border-b border-border bg-muted/30 px-3 py-2 text-[10px] font-medium uppercase tracking-wide text-muted-foreground"
                style={{ gridTemplateColumns: gridCols }}
              >
                {selectable && <span />}
                <span>{t("Collection")}</span>
                <span className="text-right">{t("Files")}</span>
                <span className="text-right">{t("Blocks")}</span>
                {showCoverageCols ? (
                  columnLangs.map((l) => (
                    <span key={l} className="text-center normal-case" translate="no">
                      {heatmap ? l.split("-")[0] : l}
                    </span>
                  ))
                ) : (
                  <span>{hasData ? "" : t("Coverage")}</span>
                )}
                <span />
              </div>
            )}

            {visibleContent.map(({ coll, ci }, idx) => {
              const isEditing = editing.has(ci);
              const isOpen = expanded.has(ci);
              const files = filesForEntry(coll);
              const bare = isBareEntry(coll);
              const title = bare ? coll.path || t("Files") : coll.name || t("Untitled collection");
              const cs = statusByLabel.get(statusLabelOf(coll));
              return (
                <div key={ci} className="border-b border-border last:border-0">
                  <div
                    className="grid items-center gap-x-3 px-3 py-2.5 hover:bg-accent/20"
                    style={{ gridTemplateColumns: gridCols }}
                  >
                    {selectable && (
                      <Checkbox
                        checked={selected.has(ci)}
                        onCheckedChange={() => toggleSelect(ci)}
                        aria-label={t("Select {collection}", { collection: title })}
                        className="shrink-0"
                      />
                    )}
                    {/* Name cell — chevron + colour dot (matches cake) + name. */}
                    <button
                      onClick={() => toggle(setExpanded, ci)}
                      className="flex min-w-0 items-center gap-2 text-left"
                      aria-label={isOpen ? t("Collapse") : t("Expand")}
                      aria-expanded={isOpen}
                    >
                      {isOpen ? (
                        <ChevronDown size={13} className="shrink-0 text-muted-foreground" />
                      ) : (
                        <ChevronRight size={13} className="shrink-0 text-muted-foreground" />
                      )}
                      {/* The collection's cake colour lives on the icon itself
                          (matches its slice) instead of a separate dot. */}
                      <Layers
                        size={13}
                        className="shrink-0"
                        style={{ color: collectionColor(idx) }}
                      />
                      <span className="truncate text-sm font-medium" title={title}>
                        {title}
                      </span>
                    </button>
                    <span className="text-right text-xs tabular-nums text-muted-foreground">
                      {files.length}
                    </span>
                    <span className="text-right text-xs tabular-nums">
                      {hasData && cs ? cs.blockCount : "—"}
                    </span>
                    {showCoverageCols ? (
                      columnLangs.map((l) => <Fragment key={l}>{langCell(coll, l)}</Fragment>)
                    ) : (
                      <span />
                    )}
                    {/* Actions — per-collection Run, Edit, delete (icon-only). */}
                    <span className="flex items-center justify-end gap-1">
                      {onRunFlow &&
                        files.length > 0 &&
                        flowNames.length > 0 &&
                        (flowNames.length === 1 ? (
                          <Button
                            variant="ghost"
                            size="icon-sm"
                            disabled={hasActive}
                            onClick={() =>
                              onRunFlow(flowNames[0], flows![flowNames[0]], {
                                scopePaths: files.map((m) => m.path),
                                scopeLabel: title,
                              })
                            }
                            aria-label={t("Run {flow} on {collection}", {
                              flow: flowNames[0],
                              collection: title,
                            })}
                          >
                            <Play size={13} />
                          </Button>
                        ) : (
                          <DropdownMenu>
                            <DropdownMenuTrigger asChild>
                              <Button
                                variant="ghost"
                                size="icon-sm"
                                disabled={hasActive}
                                aria-label={t("Run a flow on {collection}", { collection: title })}
                              >
                                <Play size={13} />
                              </Button>
                            </DropdownMenuTrigger>
                            <DropdownMenuContent align="end">
                              <DropdownMenuLabel>
                                {t("Run on {collection}", { collection: title })}
                              </DropdownMenuLabel>
                              {flowNames.map((fn) => (
                                <DropdownMenuItem
                                  key={fn}
                                  onClick={() =>
                                    onRunFlow(fn, flows![fn], {
                                      scopePaths: files.map((m) => m.path),
                                      scopeLabel: title,
                                    })
                                  }
                                >
                                  <Play size={12} />
                                  {fn}
                                </DropdownMenuItem>
                              ))}
                            </DropdownMenuContent>
                          </DropdownMenu>
                        ))}
                      <Button
                        variant={isEditing ? "secondary" : "ghost"}
                        size="icon-sm"
                        onClick={() => {
                          openCard(ci); // editing implies the body is open
                          toggle(setEditing, ci);
                        }}
                        aria-label={isEditing ? t("Done editing") : t("Edit collection")}
                      >
                        {isEditing ? <Check size={13} /> : <Pencil size={13} />}
                      </Button>
                      <ConfirmDeleteButton
                        onDelete={() => handleDeleteCollection(ci)}
                        mode="icon"
                      />
                    </span>
                  </div>

                  {isOpen && (
                    <div className="border-t border-border bg-muted/10">
                      {/* Editor slides in over the output; both stay visible. */}
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
                                  openCard(ci);
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
                </div>
              );
            })}

            {/* Other files — unmatched, not owned by any collection. Hidden while
                a collection filter is active (they belong to no collection). */}
            {!filterCollections.length && unmatchedFiles.length > 0 && (
              <div className="border-b border-border last:border-0">
                <button
                  onClick={() => setOtherCollapsed((v) => !v)}
                  className="flex w-full items-center gap-2 px-3 py-2.5 text-left hover:bg-accent/20"
                  aria-label={otherCollapsed ? t("Expand") : t("Collapse")}
                >
                  {otherCollapsed ? (
                    <ChevronRight size={13} className="shrink-0 text-muted-foreground" />
                  ) : (
                    <ChevronDown size={13} className="shrink-0 text-muted-foreground" />
                  )}
                  <Files size={13} className="shrink-0 text-muted-foreground" />
                  <span className="text-sm font-medium">{t("Other files")}</span>
                  <Badge variant="secondary" className="text-[10px] font-normal">
                    {t("{count} files", { count: unmatchedFiles.length })}
                  </Badge>
                </button>
                {!otherCollapsed && (
                  <div className="border-t border-border bg-muted/10">
                    {unmatchedTable(unmatchedFiles)}
                  </div>
                )}
              </div>
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
    </section>
  );
}
