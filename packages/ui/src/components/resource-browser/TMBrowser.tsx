import { useState, useEffect, useCallback, useRef, useMemo } from "react";
import type { Run } from "@neokapi/kapi-format";
import type { TMAdapter } from "./adapters";
import type {
  TMEntryDTO,
  TMFacets,
  EntityAnnotationDTO,
  EntityPatternRequest,
  VariantInputDTO,
} from "./types";
import { BulkActionBar } from "./BulkActionBar";
import { TMSearchBar, type FilterToken } from "./TMSearchBar";
import { TMFacetSidebar, EMPTY_FACETS, type FacetSelection } from "./TMFacetSidebar";
import { TMBrowserToolbar } from "./TMBrowserToolbar";
import { TMEntryList } from "./TMEntryList";
import { TMAddEntryDialog } from "./TMAddEntryDialog";
import { EntityAnnotationDialog } from "./EntityAnnotationDialog";
import { resolveLocaleName, type LocaleInfo } from "../ui/locale-select";
import { Button } from "../ui/button";
import {
  buildSearchBarFilterFields,
  buildSearchFilter,
  variantForInput,
  PAGE_SIZE,
  type ViewMode,
} from "./tm-browser-helpers";

interface TMBrowserProps {
  adapter: TMAdapter;
  /** Default source locale for the bilingual toggle. */
  sourceLocale?: string;
  /** Default target locale candidates for the bilingual toggle. */
  targetLocales?: string[];
  /** Locale list for the add-entry form's locale selectors. If omitted, plain text inputs are used. */
  locales?: LocaleInfo[];
  /**
   * When set, scope the multi-language view to these locales (and bias the
   * bilingual target to the first). Used by the desktop's Active Filter to focus
   * the browser on the languages you're working with; the user can still adjust
   * the display filter. Omitted/empty = show all.
   */
  scopeLocales?: string[];
  onError?: (message: string, details?: unknown) => void;
}

export function TMBrowser({
  adapter,
  sourceLocale: propSourceLocale = "",
  targetLocales: propTargetLocales = [],
  locales,
  scopeLocales,
  onError,
}: TMBrowserProps) {
  const [viewMode, setViewMode] = useState<ViewMode>("multilang");

  const [entries, setEntries] = useState<TMEntryDTO[]>([]);
  const [totalCount, setTotalCount] = useState(0);
  const [searchText, setSearchText] = useState("");
  const [debouncedSearch, setDebouncedSearch] = useState("");
  const [page, setPage] = useState(0);
  const [loading, setLoading] = useState(true);
  const [initialLoadDone, setInitialLoadDone] = useState(false);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [showAnnotateDialog, setShowAnnotateDialog] = useState(false);
  const [confirmBulkDelete, setConfirmBulkDelete] = useState(false);

  const [facets, setFacets] = useState<TMFacets | null>(null);
  const [facetsLoading, setFacetsLoading] = useState(false);
  const [facetSelection, setFacetSelection] = useState<FacetSelection>(EMPTY_FACETS);

  // Bilingual locale pair controlled from the top bar. When null we pick
  // defaults from the first two facet locales (or props) once facets land.
  const [bilingualSrc, setBilingualSrc] = useState<string | null>(propSourceLocale || null);
  const [bilingualTgt, setBilingualTgt] = useState<string | null>(propTargetLocales[0] ?? null);

  // Multi-language "show" filter — hides specific locales in the multilang view.
  // undefined = show all; array = show only those. Seeded from scopeLocales (the
  // desktop Active Filter) when provided.
  const [displayLocales, setDisplayLocales] = useState<string[] | undefined>(
    scopeLocales?.length ? scopeLocales : undefined,
  );

  // Re-apply the scope when it changes (switching the Active Filter).
  const scopeKey = scopeLocales?.join(",") ?? "";
  useEffect(() => {
    setDisplayLocales(scopeKey ? scopeKey.split(",") : undefined);
  }, [scopeKey]);

  // Marked entities from search bar for entity-value filtering.
  const [markedEntities, setMarkedEntities] = useState<EntityAnnotationDTO[]>([]);

  // Add entry form state.
  const [showAddForm, setShowAddForm] = useState(false);
  const [addSource, setAddSource] = useState("");
  const [addTarget, setAddTarget] = useState("");
  const [addSrcLocale, setAddSrcLocale] = useState(propSourceLocale);
  const [addTgtLocale, setAddTgtLocale] = useState(propTargetLocales[0] ?? "");

  // Filter tokens from the search bar.
  const [filterTokens, setFilterTokens] = useState<FilterToken[]>([]);

  const tokenLocale = useMemo(
    () => filterTokens.find((t) => t.key === "language")?.value ?? "",
    [filterTokens],
  );

  // Available locales — derived from facets (preferred) or entry data.
  const allLocales = useMemo(() => {
    if (facets && facets.locales.length > 0) {
      return facets.locales.map((l) => l.locale);
    }
    const set = new Set<string>();
    for (const e of entries) {
      for (const l of Object.keys(e.variants)) set.add(l);
    }
    return [...set].sort();
  }, [facets, entries]);

  // Auto-initialise bilingual pair once facets arrive.
  useEffect(() => {
    if (allLocales.length === 0) return;
    if (!bilingualSrc) setBilingualSrc(allLocales[0]);
    if (!bilingualTgt && allLocales.length > 1) setBilingualTgt(allLocales[1]);
  }, [allLocales, bilingualSrc, bilingualTgt]);

  // Merge user-provided locale info with locales from data.
  const mergedLocales = useMemo(() => {
    const known = new Map((locales ?? []).map((l) => [l.code, l]));
    for (const l of allLocales) {
      if (!known.has(l)) known.set(l, { code: l, displayName: resolveLocaleName(l) });
    }
    return [...known.values()];
  }, [locales, allLocales]);

  // Toggle a single locale in the multi-language display-filter row.
  const toggleDisplayLocale = useCallback(
    (locale: string) => {
      setDisplayLocales((prev) => {
        if (prev === undefined) return allLocales.filter((l) => l !== locale);
        return prev.includes(locale) ? prev.filter((l) => l !== locale) : [...prev, locale];
      });
    },
    [allLocales],
  );

  // Compute effective locales: in bilingual mode we require both source/target,
  // in multilang mode we apply the selected language filter (if any).
  const anyLocale = useMemo(() => {
    if (tokenLocale) return tokenLocale;
    if (facetSelection.locales.length === 1) return facetSelection.locales[0];
    return "";
  }, [tokenLocale, facetSelection.locales]);

  const requireLocale = useMemo(() => {
    if (viewMode === "bilingual" && bilingualTgt) return bilingualTgt;
    return "";
  }, [viewMode, bilingualTgt]);

  const handleSearchSubmit = useCallback((val: string) => {
    setSearchText(val);
    setDebouncedSearch(val);
    setPage(0);
  }, []);

  // Build search filter from facet selection + tokens + marked entities.
  const searchFilter = useMemo(
    () => buildSearchFilter(facetSelection, filterTokens, markedEntities),
    [facetSelection, filterTokens, markedEntities],
  );

  // Refs for stable callbacks (avoid re-creating fetchEntries on every keystroke).
  const adapterRef = useRef(adapter);
  const anyLocaleRef = useRef(anyLocale);
  const requireLocaleRef = useRef(requireLocale);
  const filterRef = useRef(searchFilter);
  adapterRef.current = adapter;
  anyLocaleRef.current = anyLocale;
  requireLocaleRef.current = requireLocale;
  filterRef.current = searchFilter;

  const fetchEntries = useCallback(async (q: string, p: number) => {
    setLoading(true);
    try {
      const a = adapterRef.current;
      const f = filterRef.current;
      const hasFilter = Object.keys(f).length > 0;

      const result =
        hasFilter && a.searchFiltered
          ? await a.searchFiltered(
              q,
              anyLocaleRef.current,
              requireLocaleRef.current,
              f,
              p * PAGE_SIZE,
              PAGE_SIZE,
            )
          : await a.search(
              q,
              anyLocaleRef.current,
              requireLocaleRef.current,
              p * PAGE_SIZE,
              PAGE_SIZE,
            );
      setEntries(result.entries ?? []);
      setTotalCount(result.total_count);
    } finally {
      setLoading(false);
      setInitialLoadDone(true);
    }
  }, []);

  const fetchFacets = useCallback(async () => {
    const a = adapterRef.current;
    setFacetsLoading(true);
    try {
      if (a.getFacetsFiltered) {
        const data = await a.getFacetsFiltered(
          debouncedSearch,
          anyLocaleRef.current,
          requireLocaleRef.current,
          filterRef.current,
        );
        setFacets(data);
      } else if (a.getFacets) {
        const data = await a.getFacets();
        setFacets(data);
      }
    } catch {
      // Facets are non-critical.
    } finally {
      setFacetsLoading(false);
    }
  }, [debouncedSearch]);

  useEffect(() => {
    void fetchEntries(debouncedSearch, page);
  }, [fetchEntries, debouncedSearch, page, anyLocale, requireLocale, viewMode, searchFilter]);

  useEffect(() => {
    void fetchFacets();
  }, [fetchFacets, debouncedSearch, anyLocale, requireLocale, searchFilter]);

  useEffect(() => {
    setPage(0);
    setSelected(new Set());
  }, [viewMode, facetSelection, filterTokens]);

  // --- Selection ---
  const toggleSelect = useCallback((id: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }, []);

  const deselectAll = useCallback(() => {
    setSelected(new Set());
    setConfirmBulkDelete(false);
  }, []);

  const visibleIds = useMemo(() => entries.map((e) => e.id), [entries]);
  const allVisibleSelected = visibleIds.length > 0 && visibleIds.every((id) => selected.has(id));
  const someVisibleSelected = visibleIds.some((id) => selected.has(id));

  const toggleSelectAll = useCallback(() => {
    setSelected((prev) => {
      if (allVisibleSelected) {
        const next = new Set(prev);
        for (const id of visibleIds) next.delete(id);
        return next;
      }
      return new Set([...prev, ...visibleIds]);
    });
  }, [allVisibleSelected, visibleIds]);

  // --- CRUD ---
  const handleEditVariant = useCallback(
    async (entry: TMEntryDTO, locale: string, runs: Run[]) => {
      try {
        const nextVariants: Record<string, VariantInputDTO> = {};
        for (const [l, v] of Object.entries(entry.variants)) {
          if (l === locale) {
            nextVariants[l] = variantForInput(runs);
          } else {
            nextVariants[l] = variantForInput(v.runs);
          }
        }
        await adapter.updateEntry({
          entry_id: entry.id,
          variants: nextVariants,
          hint_src_lang: entry.hint_src_lang,
          project_id: entry.project_id,
          note: entry.note,
        });
        void fetchEntries(debouncedSearch, page);
        void fetchFacets();
      } catch (err) {
        onError?.("Failed to save TM entry", err);
      }
    },
    [adapter, fetchEntries, fetchFacets, debouncedSearch, page, onError],
  );

  const handleDelete = useCallback(
    async (id: string) => {
      try {
        await adapter.deleteEntry(id);
        setSelected((prev) => {
          const next = new Set(prev);
          next.delete(id);
          return next;
        });
        void fetchEntries(debouncedSearch, page);
        void fetchFacets();
      } catch (err) {
        onError?.("Failed to delete TM entry", err);
      }
    },
    [adapter, fetchEntries, fetchFacets, debouncedSearch, page, onError],
  );

  const handleBulkDelete = useCallback(async () => {
    if (!confirmBulkDelete) {
      setConfirmBulkDelete(true);
      return;
    }
    try {
      await adapter.deleteEntries([...selected]);
      setSelected(new Set());
      setConfirmBulkDelete(false);
      void fetchEntries(debouncedSearch, page);
      void fetchFacets();
    } catch (err) {
      onError?.("Failed to delete TM entries", err);
    }
  }, [
    adapter,
    selected,
    confirmBulkDelete,
    fetchEntries,
    fetchFacets,
    debouncedSearch,
    page,
    onError,
  ]);

  const handleAdd = useCallback(async () => {
    if (!addSource.trim() || !addTarget.trim() || !addSrcLocale || !addTgtLocale) return;
    try {
      const variants: Record<string, VariantInputDTO> = {
        [addSrcLocale]: { text: addSource },
        [addTgtLocale]: { text: addTarget },
      };
      await adapter.addEntry({
        variants,
        hint_src_lang: addSrcLocale,
      });
      setAddSource("");
      setAddTarget("");
      setShowAddForm(false);
      void fetchEntries(debouncedSearch, page);
      void fetchFacets();
    } catch (err) {
      onError?.("Failed to add TM entry", err);
    }
  }, [
    adapter,
    addSource,
    addTarget,
    addSrcLocale,
    addTgtLocale,
    fetchEntries,
    fetchFacets,
    debouncedSearch,
    page,
    onError,
  ]);

  const handleAnnotateEntities = useCallback(
    async (patterns: EntityPatternRequest[]) => {
      if (!adapter.annotateEntities) throw new Error("Adapter does not support entity annotation");
      const result = await adapter.annotateEntities({ entry_ids: [...selected], patterns });
      void fetchEntries(debouncedSearch, page);
      void fetchFacets();
      return result;
    },
    [adapter, selected, fetchEntries, fetchFacets, debouncedSearch, page],
  );

  const isEmpty = entries.length === 0;

  // Build filter fields from facet data for the search bar's filter dropdown.
  const searchBarFilterFields = useMemo(() => buildSearchBarFilterFields(facets), [facets]);

  // Bilingual visible locales = exactly the two selected.
  const bilingualVisible = useMemo(() => {
    if (!bilingualSrc || !bilingualTgt) return undefined;
    return [bilingualTgt];
  }, [bilingualSrc, bilingualTgt]);

  const sourceLocaleForDialogDefault = bilingualSrc ?? propSourceLocale;
  const targetLocaleForDialogDefault = bilingualTgt ?? propTargetLocales[0] ?? "";
  useEffect(() => {
    if (!addSrcLocale && sourceLocaleForDialogDefault)
      setAddSrcLocale(sourceLocaleForDialogDefault);
    if (!addTgtLocale && targetLocaleForDialogDefault)
      setAddTgtLocale(targetLocaleForDialogDefault);
  }, [sourceLocaleForDialogDefault, targetLocaleForDialogDefault, addSrcLocale, addTgtLocale]);

  const handleClearSearch = useCallback(() => {
    setSearchText("");
    setDebouncedSearch("");
  }, []);

  return (
    <div data-testid="tm-browser">
      {/* Google-style search bar */}
      <div className="mb-4">
        <TMSearchBar
          value={searchText}
          onChange={handleSearchSubmit}
          filters={filterTokens}
          onFiltersChange={setFilterTokens}
          filterFields={searchBarFilterFields}
          onLookup={adapter.lookup}
          onEntitiesChange={setMarkedEntities}
          sourceLocale={bilingualSrc ?? ""}
          targetLocale={bilingualTgt ?? ""}
        />
      </div>

      <div className="flex gap-4">
        {/* Left: facet filters */}
        {adapter.getFacets && (
          <div className="w-56 shrink-0">
            <TMFacetSidebar
              facets={facets}
              selection={facetSelection}
              onSelectionChange={setFacetSelection}
              loading={facetsLoading}
            />
          </div>
        )}

        {/* Right: results */}
        <div className="flex-1 min-w-0">
          {/* Toolbar row: count + Add Entry */}
          <div className="flex items-center gap-2 mb-2">
            <div className="text-[12px] text-muted-foreground flex items-center gap-2">
              <span>
                {totalCount} {totalCount === 1 ? "entry" : "entries"}
                {debouncedSearch && " matching"}
              </span>
              {loading && initialLoadDone && (
                <span className="inline-block w-3 h-3 border-2 border-current border-t-transparent rounded-full animate-spin opacity-50" />
              )}
            </div>
            <Button
              size="sm"
              onClick={() => setShowAddForm(true)}
              className="ml-auto whitespace-nowrap"
            >
              Add Entry
            </Button>
          </div>

          {/* Selection + locale controls + view toggle */}
          <TMBrowserToolbar
            isEmpty={isEmpty}
            allVisibleSelected={allVisibleSelected}
            someVisibleSelected={someVisibleSelected}
            onToggleSelectAll={toggleSelectAll}
            viewMode={viewMode}
            onViewModeChange={setViewMode}
            allLocales={allLocales}
            bilingualSrc={bilingualSrc}
            bilingualTgt={bilingualTgt}
            onBilingualSrcChange={setBilingualSrc}
            onBilingualTgtChange={setBilingualTgt}
            displayLocales={displayLocales}
            onToggleDisplayLocale={toggleDisplayLocale}
            onDisplayLocalesChange={setDisplayLocales}
          />

          {/* Loading skeleton + empty state + entries + pagination */}
          <TMEntryList
            entries={entries}
            loading={loading}
            initialLoadDone={initialLoadDone}
            searchQuery={debouncedSearch}
            onClearSearch={handleClearSearch}
            hintLocale={viewMode === "bilingual" ? bilingualSrc : null}
            visibleLocales={viewMode === "bilingual" ? bilingualVisible : displayLocales}
            selected={selected}
            onToggleSelect={toggleSelect}
            onEditVariant={(entry, locale, runs) => void handleEditVariant(entry, locale, runs)}
            onDelete={(id) => void handleDelete(id)}
            page={page}
            totalCount={totalCount}
            onPageChange={setPage}
          />
        </div>
      </div>

      {/* Bulk action bar */}
      <BulkActionBar
        selectedCount={selected.size}
        onDelete={handleBulkDelete}
        confirmDelete={confirmBulkDelete}
        onAnnotateEntities={
          adapter.annotateEntities ? () => setShowAnnotateDialog(true) : undefined
        }
        onDeselectAll={deselectAll}
      />

      {/* Entity annotation dialog */}
      <EntityAnnotationDialog
        open={showAnnotateDialog}
        onClose={() => setShowAnnotateDialog(false)}
        selectedCount={selected.size}
        initialPattern={searchText}
        onApply={handleAnnotateEntities}
      />

      {/* Add entry dialog */}
      <TMAddEntryDialog
        open={showAddForm}
        onOpenChange={setShowAddForm}
        source={addSource}
        onSourceChange={setAddSource}
        target={addTarget}
        onTargetChange={setAddTarget}
        srcLocale={addSrcLocale}
        onSrcLocaleChange={setAddSrcLocale}
        tgtLocale={addTgtLocale}
        onTgtLocaleChange={setAddTgtLocale}
        locales={mergedLocales}
        onSubmit={() => void handleAdd()}
      />
    </div>
  );
}
