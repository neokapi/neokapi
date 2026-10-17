import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { t } from "@neokapi/i18n-react/runtime";
import {
  Button,
  EmptyState,
  Popover,
  PopoverContent,
  PopoverTrigger,
  SimpleTooltip,
  StatRow,
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
  ViewTab,
  ViewTabGroup,
  cn,
  type StatItem,
} from "@neokapi/ui-primitives";
import { Download, Info, Link2, Pin, PinOff, Redo2, SearchX, Undo2 } from "lucide-react";
import { ATLAS_STYLE } from "./colors";
import { CompareView } from "./components/CompareView";
import { DimensionFlow } from "./components/DimensionFlow";
import { MapView } from "./components/MapView";
import { MatrixView } from "./components/MatrixView";
import { RasterView } from "./components/RasterView";
import { RecordInspector } from "./components/RecordInspector";
import { RecordsTable } from "./components/RecordsTable";
import { ScopeChips } from "./components/ScopeChips";
import { SelectionPane } from "./components/SelectionPane";
import { TimeBrush } from "./components/TimeBrush";
import { formatCount, formatDay } from "./labels";
import { geoAxis, preparedStateAt, prepareAtlas } from "./model/atlas";
import { type Pick, cohort, evalContext, previewSet } from "./model/filter";
import { eventsInWindow } from "./model/histogram";
import { type Investigation, exportInvestigation } from "./model/investigation";
import {
  type AtlasQuery,
  type AtlasView,
  type QueryHistory,
  canGoBack,
  canGoForward,
  defaultQuery,
  goBack,
  goForward,
  hasFilters,
  pushQuery,
  replaceQuery,
  serialiseQuery,
  startHistory,
  validateQuery,
  withoutFilters,
} from "./model/query";
import { NEEDS_PERSON } from "./model/state";
import { transitions } from "./model/transitions";
import type { AtlasData } from "./types";

export interface ContextAtlasProps {
  /** The atlas answer: `kapi context atlas --json`. */
  data: AtlasData;
  /** A query to open with (from a link or a saved investigation); validated against the data. */
  initialQuery?: unknown;
  /** A record to open in the detail pane. */
  initialRecord?: string;
  /** Called with each committed query, for a host that keeps it in the URL. */
  onQueryChange?: (query: AtlasQuery) => void;
  /** The command-line equivalent, shown as a chip in the header (`kapi context atlas`). */
  commandHint?: string;
  /** Open an evidence path in the host (an editor, a file view). */
  onOpenEvidence?: (path: string) => void;
  /** Receive an exported investigation; without it the export downloads as JSON. */
  onExport?: (investigation: Investigation) => void;
  /** Turn a query fragment (`q=...`) into a link to copy; without it there is no link action. */
  linkFor?: (fragment: string) => string;
  /** The action an empty atlas offers; a CLI chip for `kapi context note` when omitted. */
  emptyAction?: ReactNode;
  className?: string;
}

const VIEWS: { id: AtlasView; label: () => string }[] = [
  { id: "flow", label: () => t("Flow", "atlas view") },
  { id: "matrix", label: () => t("Matrix", "atlas view") },
  { id: "time", label: () => t("Over time", "atlas view") },
  { id: "map", label: () => t("Map", "atlas view") },
  { id: "records", label: () => t("Records", "atlas view") },
];

function CommandChip({ command }: { command: string }) {
  return (
    <code className="rounded-md border bg-muted/50 px-2 py-1 font-mono text-[11px] text-muted-foreground">
      {command}
    </code>
  );
}

function download(inv: Investigation) {
  const blob = new Blob([JSON.stringify(inv, null, 2)], { type: "application/json" });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = `context-investigation-${inv.generated_at.slice(0, 10)}.json`;
  a.click();
  URL.revokeObjectURL(url);
}

/**
 * The context atlas: one query, linked views. A time brush sets the window
 * and the snapshot; filters retained in any view narrow every view (OR within
 * an axis, AND across axes); hovering a mark previews the records it holds in
 * every view without changing the query. The detail pane shows the selected
 * record, or the selection's transitions and questions to start from.
 */
export function ContextAtlas({
  data,
  initialQuery,
  initialRecord,
  onQueryChange,
  commandHint,
  onOpenEvidence,
  onExport,
  linkFor,
  emptyAction,
  className,
}: ContextAtlasProps) {
  // `units` reads the data as it came: a content cell weighs its unit count.
  const units = useMemo(() => prepareAtlas(data), [data]);
  const [history, setHistory] = useState<QueryHistory>(() =>
    startHistory(
      initialQuery === undefined ? defaultQuery(units) : validateQuery(initialQuery, units),
    ),
  );
  // New data: keep the query where it still makes sense.
  const seen = useRef(units);
  useEffect(() => {
    if (seen.current === units) return;
    seen.current = units;
    setHistory((h) => startHistory(validateQuery(h.present, units)));
  }, [units]);

  const query = history.present;
  // Every view counts in the query's measure.
  const atlas = useMemo(
    () => (query.measure === "units" ? units : prepareAtlas(data, { measure: "records" })),
    [units, data, query.measure],
  );
  const notify = useRef(onQueryChange);
  notify.current = onQueryChange;
  const onQuery = useCallback((next: AtlasQuery, opts?: { transient?: boolean }) => {
    setHistory((h) => (opts?.transient ? replaceQuery(h, next) : pushQuery(h, next)));
    if (!opts?.transient) notify.current?.(next);
  }, []);
  const move = (fn: (h: QueryHistory) => QueryHistory) => {
    const next = fn(history);
    setHistory(next);
    notify.current?.(next.present);
  };

  const ctx = useMemo(() => evalContext(atlas, query), [atlas, query]);
  const foreground = useMemo(() => cohort(atlas, query, ctx), [atlas, query, ctx]);
  const universe = useMemo(() => cohort(atlas, query, ctx, query.flowAxes), [atlas, query, ctx]);

  const [picks, setPicks] = useState<Pick[] | null>(null);
  const preview = useMemo(
    () => (picks ? previewSet(atlas, foreground, picks, ctx) : null),
    [atlas, foreground, picks, ctx],
  );

  const [selected, setSelected] = useState<string | null>(initialRecord ?? null);
  const [pinned, setPinned] = useState<AtlasQuery | null>(null);
  const [comparing, setComparing] = useState(false);
  const [copied, setCopied] = useState(false);

  const counts = useMemo(() => {
    let weight = 0;
    let needs = 0;
    for (const i of foreground) {
      const p = atlas.records[i];
      weight += units.records[i].weight;
      if (NEEDS_PERSON.has(preparedStateAt(p, ctx.snapshotMs))) needs += p.weight;
    }
    const total = units.records.reduce((s, p) => s + p.weight, 0);
    const weighted = units.weighted;
    return {
      records: foreground.length,
      weight,
      total,
      weighted,
      needs,
      changed: transitions(atlas, foreground, ctx).changedWeight,
      events: eventsInWindow(atlas, query.clock, ctx.startMs, ctx.endMs),
    };
  }, [atlas, units, foreground, ctx, query.clock]);

  const stats: StatItem[] = [
    { id: "records", value: counts.records, label: t("records selected", "atlas count") },
    ...(counts.weighted
      ? [
          {
            id: "units",
            value: counts.weight,
            label: t("units, of {total}", "atlas count: weighted records, of the whole atlas", {
              total: formatCount(counts.total),
            }),
            hint: t(
              "A content cell counts its units; every other record counts one.",
              "atlas count hint",
            ),
          },
        ]
      : []),
    {
      id: "needs",
      value: counts.needs,
      label: t("need a person", "atlas count"),
      tone: counts.needs > 0 ? ("warning" as const) : ("default" as const),
      hint: t(
        "Suggested, contested, stale or awaiting approval at the snapshot.",
        "atlas count hint",
      ),
    },
    {
      id: "changed",
      value: counts.changed,
      label: t("changed state", "atlas count"),
      hint: t(
        "State at the snapshot differs from the state at the window's start.",
        "atlas count hint",
      ),
    },
    {
      id: "events",
      value: counts.events,
      label: t("events in window", "atlas count"),
      tone: "muted" as const,
    },
  ];

  const exportNow = () => {
    const inv = exportInvestigation(atlas, query, { pinned, selected });
    if (onExport) onExport(inv);
    else download(inv);
  };

  const copyLink = async () => {
    if (!linkFor) return;
    try {
      await navigator.clipboard?.writeText(linkFor(serialiseQuery(query)));
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {
      // The clipboard can be refused; nothing else depends on it.
    }
  };

  if (atlas.records.length === 0) {
    return (
      <div
        className={cn("context-atlas flex flex-col gap-4", className)}
        data-testid="context-atlas"
      >
        <EmptyState
          icon={<Info className="mx-auto size-6" />}
          title={t("No context recorded yet", "atlas empty state")}
          description={t(
            "Rules, notes, terms, content memory and content appear here as kapi records them, with their history over time.",
            "atlas empty state",
          )}
          action={emptyAction ?? <CommandChip command="kapi context note" />}
        />
        {data.notes?.length > 0 && <Notes notes={data.notes} />}
      </div>
    );
  }

  const hasMap = !!geoAxis(atlas);
  const views = VIEWS.filter((v) => v.id !== "map" || hasMap);
  const view = query.view === "map" && !hasMap ? "flow" : query.view;
  const viewProps = { atlas, query, ctx, foreground, preview, onPreview: setPicks, onQuery };

  return (
    <div
      className={cn("context-atlas flex flex-col gap-4", className)}
      data-testid="context-atlas"
      onKeyDown={(e) => {
        if (e.key === "Escape") setPicks(null);
      }}
    >
      <style>{ATLAS_STYLE}</style>

      <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
        <div className="flex min-w-0 items-baseline gap-2">
          {data.project && <span className="truncate text-sm font-medium">{data.project}</span>}
          <span className="text-[11px] text-muted-foreground tabular-nums">
            {t("{from} to {to}", "the atlas's span", {
              from: formatDay(atlas.firstDay),
              to: formatDay(atlas.lastDay, true),
            })}
          </span>
          {data.notes?.length > 0 && <NotesInfo notes={data.notes} />}
        </div>
        <div className="ml-auto flex flex-wrap items-center gap-1.5">
          <SimpleTooltip content={t("Back", "query history")}>
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={t("Back", "query history")}
              disabled={!canGoBack(history)}
              onClick={() => move(goBack)}
            >
              <Undo2 aria-hidden />
            </Button>
          </SimpleTooltip>
          <SimpleTooltip content={t("Forward", "query history")}>
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={t("Forward", "query history")}
              disabled={!canGoForward(history)}
              onClick={() => move(goForward)}
            >
              <Redo2 aria-hidden />
            </Button>
          </SimpleTooltip>
          {pinned ? (
            <>
              <Button variant="outline" size="sm" onClick={() => setComparing(true)}>
                Compare with A
              </Button>
              <SimpleTooltip content={t("Unpin A", "compare")}>
                <Button
                  variant="ghost"
                  size="icon-sm"
                  aria-label={t("Unpin A", "compare")}
                  onClick={() => setPinned(null)}
                >
                  <PinOff aria-hidden />
                </Button>
              </SimpleTooltip>
            </>
          ) : (
            <Button variant="outline" size="sm" onClick={() => setPinned(query)}>
              <Pin aria-hidden />
              Pin A
            </Button>
          )}
          {linkFor && (
            <SimpleTooltip
              content={
                copied ? t("Copied", "copy link") : t("Copy a link to this query", "copy link")
              }
            >
              <Button
                variant="ghost"
                size="icon-sm"
                aria-label={t("Copy link", "copy link")}
                onClick={copyLink}
              >
                <Link2 aria-hidden />
              </Button>
            </SimpleTooltip>
          )}
          <SimpleTooltip content={t("Export the investigation as JSON", "export")}>
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={t("Export investigation", "export")}
              onClick={exportNow}
            >
              <Download aria-hidden />
            </Button>
          </SimpleTooltip>
          {commandHint && <CommandChip command={commandHint} />}
        </div>
      </div>

      <div className="rounded-xl border bg-card p-3">
        <TimeBrush atlas={atlas} query={query} onQuery={onQuery} />
      </div>

      <ScopeChips atlas={atlas} query={query} onQuery={onQuery} />
      <div className="flex flex-wrap items-end justify-between gap-4">
        <StatRow items={stats} />
        {units.weighted && (
          <div className="flex items-center gap-1.5">
            <span className="text-[11px] text-muted-foreground">Count</span>
            <ViewTabGroup aria-label={t("What the views count", "atlas measure")}>
              <ViewTab
                active={query.measure === "records"}
                onClick={() => onQuery({ ...query, measure: "records" })}
              >
                Records
              </ViewTab>
              <ViewTab
                active={query.measure === "units"}
                onClick={() => onQuery({ ...query, measure: "units" })}
              >
                Units
              </ViewTab>
            </ViewTabGroup>
          </div>
        )}
      </div>

      <div className="grid items-start gap-4 lg:grid-cols-[minmax(0,1fr)_320px]">
        <Tabs
          value={view}
          onValueChange={(v) => onQuery({ ...query, view: v as AtlasView })}
          className="min-w-0"
        >
          <TabsList variant="line">
            {views.map((v) => (
              <TabsTrigger key={v.id} value={v.id}>
                {v.label()}
              </TabsTrigger>
            ))}
          </TabsList>
          <div className="min-w-0 rounded-xl border bg-card p-4">
            {foreground.length === 0 && view !== "flow" ? (
              <EmptyState
                icon={<SearchX className="mx-auto size-6" />}
                title={t("No records match this scope", "atlas empty selection")}
                description={t(
                  "Nothing recorded by the snapshot passes every filter.",
                  "atlas empty selection",
                )}
                action={
                  hasFilters(query) ? (
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => onQuery(withoutFilters(query))}
                    >
                      Clear filters
                    </Button>
                  ) : undefined
                }
              />
            ) : (
              <>
                <TabsContent value="flow">
                  {foreground.length === 0 && !query.surrounding ? (
                    <EmptyState
                      icon={<SearchX className="mx-auto size-6" />}
                      title={t("No records match this scope", "atlas empty selection")}
                      description={t(
                        "Nothing recorded by the snapshot passes every filter.",
                        "atlas empty selection",
                      )}
                      action={
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={() => onQuery(withoutFilters(query))}
                        >
                          Clear filters
                        </Button>
                      }
                    />
                  ) : (
                    <DimensionFlow {...viewProps} universe={universe} />
                  )}
                </TabsContent>
                <TabsContent value="matrix">
                  <MatrixView {...viewProps} />
                </TabsContent>
                <TabsContent value="time">
                  <RasterView {...viewProps} />
                </TabsContent>
                {hasMap && (
                  <TabsContent value="map">
                    <MapView {...viewProps} />
                  </TabsContent>
                )}
                <TabsContent value="records">
                  <RecordsTable
                    atlas={atlas}
                    query={query}
                    ctx={ctx}
                    foreground={foreground}
                    selected={selected}
                    onSelect={setSelected}
                  />
                </TabsContent>
              </>
            )}
          </div>
        </Tabs>

        <aside
          className="rounded-xl border bg-card p-4 lg:sticky lg:top-4"
          aria-label={t("Detail", "atlas detail pane")}
        >
          {selected && atlas.byId.has(selected) ? (
            <RecordInspector
              atlas={atlas}
              recordId={selected}
              ctx={ctx}
              onClose={() => setSelected(null)}
              onOpenEvidence={onOpenEvidence}
            />
          ) : (
            <SelectionPane
              atlas={atlas}
              query={query}
              ctx={ctx}
              foreground={foreground}
              onQuery={onQuery}
              onSelect={setSelected}
            />
          )}
        </aside>
      </div>

      {pinned && (
        <CompareView
          atlas={atlas}
          pinned={pinned}
          query={query}
          open={comparing}
          onOpenChange={setComparing}
          onRestore={() => {
            onQuery(pinned);
            setComparing(false);
          }}
          onSwap={() => {
            const a = pinned;
            setPinned(query);
            onQuery(a);
          }}
          onExport={exportNow}
        />
      )}
    </div>
  );
}

/**
 * What the read could and could not see, behind an information mark beside
 * the span: a reader who wonders why a history is short asks here, and the
 * page does not narrate it to everyone else.
 */
function NotesInfo({ notes }: { notes: string[] }) {
  return (
    <Popover>
      <SimpleTooltip content={t("What this read covers", "atlas notes")}>
        <PopoverTrigger asChild>
          <Button
            variant="ghost"
            size="icon-sm"
            className="size-5 text-muted-foreground"
            aria-label={t("What this read covers", "atlas notes")}
          >
            <Info aria-hidden className="size-3.5" />
          </Button>
        </PopoverTrigger>
      </SimpleTooltip>
      <PopoverContent align="start" className="w-96 p-3">
        <Notes notes={notes} />
      </PopoverContent>
    </Popover>
  );
}

function Notes({ notes }: { notes: string[] }) {
  return (
    <ul className="flex flex-col gap-1" data-testid="atlas-notes">
      {notes.map((n) => (
        <li key={n} className="flex items-start gap-2 text-[11px] text-muted-foreground">
          <Info aria-hidden className="mt-px size-3.5 shrink-0" />
          {n}
        </li>
      ))}
    </ul>
  );
}
