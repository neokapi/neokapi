// A content memory as a list of pairs beside a detail pane.
//
// An entry holds one sentence in several languages. The list has one row per
// pair, the entry's source text and one of its other languages, so an entry in
// three languages is two rows. The filter bar at the top of the list searches
// the store on Enter and narrows it by the facets the store counts (language,
// target language, project, entity, import, inline codes), as typed tokens or
// from its Filters menu. Find similar, beside it, looks up wording like a
// sentence instead, and lists the matches best first with their scores.
//
// The box at the start of a row marks its entry, as does x on the selected
// row, and Mark all shown marks every entry in view. While any entry is marked
// the detail pane holds what can be done to all of them; otherwise it holds the
// selected pair, with the toolbar's toggle between the pair and every language
// the entry holds.

import { useEffect, useMemo, useRef, useState } from "react";
import {
  keepPreviousData,
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { Check, Database } from "lucide-react";
import { t } from "@neokapi/i18n-react/runtime";
import type { Run } from "@neokapi/kapi-format";
import {
  Button,
  CodedTextDisplay,
  EmptyState,
  ErrorNotice,
  FilterBar,
  ListDetail,
  ListDetailEmpty,
  ListDetailItem,
  ListDetailList,
  LoadingSpinner,
  LocaleLabel,
  ToggleGroup,
  ToggleGroupItem,
  cn,
  type EntityAnnotationDTO,
  type FilterToken,
  type LocaleInfo,
  type LookupMemoryRequest,
  type MemoryAdapter,
  type MemoryEntryDTO,
  type MemoryMatchDTO,
  type VariantInputDTO,
} from "@neokapi/ui-primitives";
import { memoryFilterFields, memoryQueryParts, memoryTargets } from "../lib/memoryFilters";
import { pairsOf, variantInput, type MemoryPair } from "../lib/memoryPairs";
import { MemoryLookupBar } from "./MemoryLookupBar";
import { AddEntryDialog, BulkDetail, PairDetail, type MemoryView } from "./MemoryPairDetail";

export { pairsOf, sourceLocaleOf, type MemoryPair } from "../lib/memoryPairs";

/** How many entries one read of the store carries. */
export const MEMORY_PAGE = 100;

/** How the search at the top of the list asks the store. */
type SearchMode = "search" | "lookup";

const NO_MARKS: ReadonlyMap<string, MemoryEntryDTO> = new Map();

export interface MemoryListDetailProps {
  /** The store, through the host's adapter. */
  adapter: MemoryAdapter;
  /** Names the store in query keys, so two open stores never share a cache. */
  handle: string;
  /** The target languages to show, from the Active Filter. Absent shows every one. */
  targetLocales?: string[];
  /** Languages for the add-entry form. */
  locales?: LocaleInfo[];
  /** The project's source language, the add-entry form's default. */
  sourceLocale?: string;
  /** The add-entry dialog is open; the page header's button opens it. */
  adding?: boolean;
  onAddingChange?: (open: boolean) => void;
  /** The pair to open on, by its id. Absent opens with nothing selected. */
  defaultSelected?: string;
  /**
   * The state to open in, for Storybook: filter tokens, the detail's view, the
   * entries marked once they load, or a Find similar lookup already run.
   */
  initial?: {
    tokens?: FilterToken[];
    view?: MemoryView;
    marked?: string[];
    lookup?: string;
  };
  onError?: (message: string, details?: unknown) => void;
}

export function MemoryListDetail({
  adapter,
  handle,
  targetLocales,
  locales = [],
  sourceLocale = "",
  adding = false,
  onAddingChange,
  defaultSelected,
  initial,
  onError,
}: MemoryListDetailProps) {
  const qc = useQueryClient();
  const [mode, setMode] = useState<SearchMode>(initial?.lookup ? "lookup" : "search");
  const [search, setSearch] = useState("");
  const [tokens, setTokens] = useState<FilterToken[]>(initial?.tokens ?? []);
  const [lookupReq, setLookupReq] = useState<LookupMemoryRequest | null>(null);
  const [view, setView] = useState<MemoryView>(initial?.view ?? "pair");
  const [marked, setMarked] = useState<ReadonlyMap<string, MemoryEntryDTO>>(NO_MARKS);
  const [selectedId, setSelectedId] = useState<string | null>(defaultSelected ?? null);

  const parts = useMemo(() => memoryQueryParts(tokens, targetLocales), [tokens, targetLocales]);
  const targets = useMemo(() => memoryTargets(tokens, targetLocales), [tokens, targetLocales]);
  const filtered = Object.keys(parts.filter).length > 0;

  // The facets the store counts for what is in view, as the filter bar's fields.
  const facets = useQuery({
    queryKey: ["tm", "facets", handle, search, parts],
    queryFn: async () => {
      if (adapter.getFacetsFiltered) {
        return adapter.getFacetsFiltered(
          search,
          parts.anyLocale,
          parts.requireLocale,
          parts.filter,
        );
      }
      return adapter.getFacets ? adapter.getFacets() : null;
    },
    placeholderData: keepPreviousData,
  });
  const fields = useMemo(() => memoryFilterFields(facets.data), [facets.data]);
  const storeLocales = useMemo(
    () => (facets.data?.locales ?? []).map((l) => l.locale),
    [facets.data],
  );

  const list = useInfiniteQuery({
    queryKey: ["tm", "entries", handle, search, parts],
    queryFn: ({ pageParam }) =>
      filtered && adapter.searchFiltered
        ? adapter.searchFiltered(
            search,
            parts.anyLocale,
            parts.requireLocale,
            parts.filter,
            pageParam,
            MEMORY_PAGE,
          )
        : adapter.search(search, parts.anyLocale, parts.requireLocale, pageParam, MEMORY_PAGE),
    initialPageParam: 0,
    getNextPageParam: (last, pages) => {
      const loaded = pages.reduce((n, p) => n + (p.entries?.length ?? 0), 0);
      return (last.entries?.length ?? 0) > 0 && loaded < last.total_count ? loaded : undefined;
    },
    // The previous answer stays on screen while the next search is read.
    placeholderData: keepPreviousData,
  });
  const entries = useMemo(
    () => list.data?.pages.flatMap((p) => p.entries ?? []) ?? [],
    [list.data],
  );
  const total = list.data?.pages[0]?.total_count ?? 0;

  const lookup = useQuery({
    queryKey: ["tm", "lookup", handle, lookupReq],
    enabled: mode === "lookup" && !!lookupReq && !!adapter.lookup,
    queryFn: async () => (await adapter.lookup!(lookupReq!)) ?? [],
  });

  // The rows: the search's pairs, or in Find similar one pair per match.
  const { rows, matchOf } = useMemo(() => {
    if (mode === "search") {
      return {
        rows: entries.flatMap((e) => pairsOf(e, targets)),
        matchOf: new Map<string, MemoryMatchDTO>(),
      };
    }
    const byId = new Map<string, MemoryMatchDTO>();
    const out: MemoryPair[] = [];
    for (const m of lookup.data ?? []) {
      const into = lookupReq?.target_locale;
      const pair = pairsOf(m.entry).find((p) => p.target === into) ?? pairsOf(m.entry, targets)[0];
      if (!pair || byId.has(pair.id)) continue;
      byId.set(pair.id, m);
      out.push(pair);
    }
    return { rows: out, matchOf: byId };
  }, [mode, entries, targets, lookup.data, lookupReq]);

  const selected = rows.find((p) => p.id === selectedId) ?? null;

  const toggleMark = (entry: MemoryEntryDTO) =>
    setMarked((prev) => {
      const next = new Map(prev);
      if (next.has(entry.id)) next.delete(entry.id);
      else next.set(entry.id, entry);
      return next;
    });
  const shownEntries = useMemo(() => {
    const seen = new Map<string, MemoryEntryDTO>();
    for (const row of rows) seen.set(row.entry.id, row.entry);
    return [...seen.values()];
  }, [rows]);
  const allMarked = shownEntries.length > 0 && shownEntries.every((e) => marked.has(e.id));
  const toggleAll = () =>
    setMarked((prev) => {
      const next = new Map(prev);
      if (allMarked) for (const e of shownEntries) next.delete(e.id);
      else for (const e of shownEntries) next.set(e.id, e);
      return next;
    });

  const refresh = () => void qc.invalidateQueries({ queryKey: ["tm"] });
  const update = useMutation({
    mutationFn: ({
      entry,
      locale,
      runs,
    }: {
      entry: MemoryEntryDTO;
      locale: string;
      runs: Run[];
    }) => {
      const variants: Record<string, VariantInputDTO> = {};
      for (const [l, v] of Object.entries(entry.variants)) {
        variants[l] = variantInput(l === locale ? runs : v.runs);
      }
      return adapter.updateEntry({
        entry_id: entry.id,
        variants,
        hint_src_lang: entry.hint_src_lang,
        project_id: entry.project_id,
        note: entry.note,
      });
    },
    onError: (err) => onError?.(t("The entry could not be saved"), err),
    onSettled: refresh,
  });
  const remove = useMutation({
    mutationFn: (entry: MemoryEntryDTO) => adapter.deleteEntry(entry.id),
    onSuccess: () => setSelectedId(null),
    onError: (err) => onError?.(t("The entry could not be deleted"), err),
    onSettled: refresh,
  });
  const removeMarked = useMutation({
    mutationFn: (ids: string[]) => adapter.deleteEntries(ids),
    onSuccess: (_, ids) => {
      setMarked(NO_MARKS);
      if (selected && ids.includes(selected.entry.id)) setSelectedId(null);
    },
    onError: (err) => onError?.(t("The entries could not be deleted"), err),
    onSettled: refresh,
  });
  const add = useMutation({
    mutationFn: (req: { source: string; target: string; srcLocale: string; tgtLocale: string }) =>
      adapter.addEntry({
        variants: {
          [req.srcLocale]: { text: req.source },
          [req.tgtLocale]: { text: req.target },
        },
        hint_src_lang: req.srcLocale,
      }),
    onSuccess: () => onAddingChange?.(false),
    onError: (err) => onError?.(t("The entry could not be added"), err),
    onSettled: refresh,
  });

  const firstTarget =
    parts.requireLocale ||
    targetLocales?.[0] ||
    storeLocales.find((l) => l !== (sourceLocale || storeLocales[0])) ||
    "";

  const runLookup = (text: string, entities: EntityAnnotationDTO[]) => {
    setSelectedId(null);
    setLookupReq({
      text,
      entities,
      source_locale: sourceLocale || storeLocales[0] || "",
      target_locale: firstTarget,
      min_score: 0.5,
      max_results: 10,
    });
  };

  // The opening state a story asks for, applied once the store has answered.
  const opened = useRef(false);
  useEffect(() => {
    if (opened.current || !initial || entries.length === 0 || !facets.data) return;
    opened.current = true;
    if (initial.marked?.length) {
      const wanted = new Set(initial.marked);
      setMarked(new Map(entries.filter((e) => wanted.has(e.id)).map((e) => [e.id, e])));
    }
    if (initial.lookup) runLookup(initial.lookup, []);
  });

  const addDialog = (
    <AddEntryDialog
      open={adding}
      onOpenChange={(open) => onAddingChange?.(open)}
      locales={locales}
      defaultSource={sourceLocale || storeLocales[0] || ""}
      defaultTarget={firstTarget}
      busy={add.isPending}
      onSubmit={(req) => add.mutate(req)}
    />
  );

  if (list.error && !list.data) {
    return (
      <div className="p-6">
        <ErrorNotice error={list.error} title="The content memory could not be read" />
      </div>
    );
  }
  if (list.isLoading) {
    return (
      <div className="p-6">
        <LoadingSpinner />
      </div>
    );
  }
  if (entries.length === 0 && !search && tokens.length === 0 && mode === "search") {
    return (
      <div className="p-6">
        <EmptyState
          icon={<Database size={20} />}
          title="No approved wording yet"
          description="Each translation you approve is kept here with the file it came from, so the next run can reuse it."
          action={
            onAddingChange && (
              <Button size="sm" variant="outline" onClick={() => onAddingChange(true)}>
                Add entry
              </Button>
            )
          }
        />
        {addDialog}
      </div>
    );
  }

  const narrowed = mode === "search" && (!!search || tokens.length > 0);
  const listPane = (
    <>
      <div className="sticky top-0 z-10 space-y-1.5 bg-background px-2 pt-2 pb-1">
        {mode === "search" ? (
          <div data-slot="memory-filter-bar">
            <FilterBar
              filters={tokens}
              onFiltersChange={setTokens}
              search={search}
              onSearchChange={setSearch}
              fields={fields}
              placeholder={t("Search wording")}
            />
          </div>
        ) : (
          <MemoryLookupBar
            onLookup={runLookup}
            busy={lookup.isFetching}
            defaultText={initial?.lookup}
          />
        )}
        <div className="flex items-center gap-2 px-1 text-[11px] text-muted-foreground">
          {rows.length > 0 && (
            <button
              type="button"
              data-slot="memory-mark-all"
              onClick={toggleAll}
              className="rounded hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring/50 focus-visible:outline-none"
            >
              {allMarked ? <>Unmark all shown</> : <>Mark all shown</>}
            </button>
          )}
          {narrowed && (
            <span className="tabular-nums">
              {t("{count, plural, one {# entry matches} other {# entries match}}", {
                count: total,
              })}
            </span>
          )}
          {adapter.lookup && (
            <ToggleGroup
              type="single"
              variant="outline"
              size="sm"
              value={mode}
              onValueChange={(next) => {
                if (!next) return;
                setMode(next as SearchMode);
                setSelectedId(null);
              }}
              aria-label={t("How to search")}
              data-slot="memory-search-mode"
              className="ml-auto"
            >
              <ToggleGroupItem value="search" className="h-6 px-2 text-[11px]">
                Search
              </ToggleGroupItem>
              <ToggleGroupItem value="lookup" className="h-6 px-2 text-[11px]">
                Find similar
              </ToggleGroupItem>
            </ToggleGroup>
          )}
        </div>
      </div>
      {rows.length === 0 ? (
        <p className="px-4 py-6 text-center text-xs text-muted-foreground">
          {mode === "search" ? (
            <>No entry matches this search.</>
          ) : lookup.isFetching ? (
            <>Looking up similar wording.</>
          ) : lookupReq ? (
            <>No similar wording in this store.</>
          ) : (
            <>Type a sentence and press Enter to find wording like it.</>
          )}
        </p>
      ) : (
        <ListDetailList
          selectedId={selectedId}
          onSelect={setSelectedId}
          aria-label={t("Entries")}
          className="pt-0"
          onKeyDown={(e) => {
            // x marks the selected row's entry, as the box at its start does.
            if (e.key === "x" && !e.metaKey && !e.ctrlKey && !e.altKey && selected) {
              e.preventDefault();
              toggleMark(selected.entry);
            }
          }}
        >
          {rows.map((pair) => (
            <PairRow
              key={pair.id}
              pair={pair}
              marked={marked.has(pair.entry.id)}
              score={matchOf.get(pair.id)?.score}
              onToggleMark={() => toggleMark(pair.entry)}
            />
          ))}
        </ListDetailList>
      )}
      {mode === "search" && list.hasNextPage && (
        <div className="px-2 pb-3">
          <Button
            variant="ghost"
            size="sm"
            className="w-full text-muted-foreground"
            disabled={list.isFetchingNextPage}
            onClick={() => void list.fetchNextPage()}
          >
            {t("Show {count} more entries", {
              count: Math.min(MEMORY_PAGE, total - entries.length),
            })}
          </Button>
        </div>
      )}
    </>
  );

  let detail: React.ReactNode;
  if (marked.size > 0) {
    detail = (
      <BulkDetail
        entries={[...marked.values()]}
        adapter={adapter}
        search={search}
        deleting={removeMarked.isPending}
        onDelete={() => removeMarked.mutate([...marked.keys()])}
        onClear={() => setMarked(NO_MARKS)}
        onAnnotated={refresh}
      />
    );
  } else if (selected) {
    detail = (
      <PairDetail
        key={selected.id}
        pair={selected}
        adapter={adapter}
        search={search}
        saving={update.isPending}
        view={view}
        onViewChange={setView}
        match={matchOf.get(selected.id)}
        onSelect={setSelectedId}
        onSave={(locale, runs) => update.mutateAsync({ entry: selected.entry, locale, runs })}
        onDelete={() => remove.mutate(selected.entry)}
        onAnnotated={refresh}
      />
    );
  } else {
    detail = (
      <ListDetailEmpty
        title={t("Choose an entry to see it here")}
        description={t("The source and its translation in full, and where the entry came from.")}
      />
    );
  }

  return (
    <div data-testid="tm-browser" data-slot="memory-browser" className="h-full min-h-0">
      <ListDetail list={listPane} listWidth={380} detail={detail} />
      {addDialog}
    </div>
  );
}

/** One pair in the list: its mark, the source text on one line, and the target's language. */
function PairRow({
  pair,
  marked,
  score,
  onToggleMark,
}: {
  pair: MemoryPair;
  marked: boolean;
  /** The match score, in Find similar. */
  score?: number;
  onToggleMark: () => void;
}) {
  const variant = pair.entry.variants[pair.source];
  return (
    <ListDetailItem
      id={pair.id}
      data-slot="memory-pair"
      data-testid={`tm-entry-${pair.entry.id}${pair.target ? `-${pair.target}` : ""}`}
      data-marked={marked || undefined}
      // Rows off screen skip layout and paint, so a store of thousands of
      // entries scrolls as lightly as a short one.
      className="[contain-intrinsic-size:auto_36px] [content-visibility:auto]"
      leading={<MarkBox marked={marked} onToggle={onToggleMark} />}
      trailing={
        <>
          {score !== undefined && (
            <span
              data-slot="memory-match-score"
              className="text-[11px] text-muted-foreground tabular-nums"
            >
              {Math.round(score * 100)}%
            </span>
          )}
          {pair.target && <LocaleLabel locale={pair.target} compact />}
        </>
      }
    >
      <CodedTextDisplay
        runs={variant?.runs}
        text={variant?.text}
        locale={pair.source}
        className="[&_*]:whitespace-nowrap"
      />
      {marked && <span className="sr-only">{t("marked", "content memory entry")}</span>}
    </ListDetailItem>
  );
}

/**
 * The box that marks a row's entry. A click on it marks and leaves the
 * selection where it is; the list's x does the same from the keyboard, so the
 * box itself stays out of the tab order and out of the row's name.
 */
function MarkBox({ marked, onToggle }: { marked: boolean; onToggle: () => void }) {
  return (
    <span
      aria-hidden
      data-slot="memory-mark"
      onClick={(e) => {
        // A handled click leaves the row's own selection alone.
        e.preventDefault();
        onToggle();
      }}
      className={cn(
        "flex size-3.5 cursor-pointer items-center justify-center rounded-[4px] border",
        marked
          ? "border-primary bg-primary text-primary-foreground"
          : "border-muted-foreground/40 hover:border-foreground",
      )}
    >
      {marked && <Check className="size-3" />}
    </span>
  );
}
