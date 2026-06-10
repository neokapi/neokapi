import { useState, useEffect, useCallback, useRef, useMemo } from "react";
import { t } from "@neokapi/kapi-react/runtime";
import type { Run } from "@neokapi/kapi-format";
import { flattenRuns } from "@neokapi/kapi-format";
import type { TMAdapter } from "./adapters";
import type {
  TMEntryDTO,
  TMFacets,
  TMSearchFilter,
  EntityAnnotationDTO,
  EntityPatternRequest,
  VariantDTO,
  VariantInputDTO,
} from "./types";
import { LocalePill } from "./LocalePill";
import { BulkActionBar } from "./BulkActionBar";
import { Pagination } from "./Pagination";
import { TMSearchBar } from "./TMSearchBar";
import { TMFacetSidebar, EMPTY_FACETS, type FacetSelection } from "./TMFacetSidebar";
import { TMGroupedEntry } from "./TMGroupedEntry";
import { EntityAnnotationDialog } from "./EntityAnnotationDialog";
import { LocaleSelect, resolveLocaleName, type LocaleInfo } from "../ui/locale-select";
import { ItemCard } from "../ui/item-card";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from "../ui/dialog";
import { Button } from "../ui/button";
import { Input } from "../ui/input";
import { Label } from "../ui/label";
import { Checkbox } from "../ui/checkbox";
import { List, Languages } from "lucide-react";
import { cn } from "../../lib/utils";

type ViewMode = "bilingual" | "multilang";

interface TMBrowserProps {
  adapter: TMAdapter;
  /** Default source locale for the bilingual toggle. */
  sourceLocale?: string;
  /** Default target locale candidates for the bilingual toggle. */
  targetLocales?: string[];
  /** Locale list for the add-entry form's locale selectors. If omitted, plain text inputs are used. */
  locales?: LocaleInfo[];
  onError?: (message: string, details?: unknown) => void;
}

const PAGE_SIZE = 50;

/**
 * Builds a variant input from a Run sequence, deriving the flattened
 * plain text and attaching the runs when any inline content is present.
 */
function variantForInput(runs: Run[]): VariantInputDTO {
  const input: VariantInputDTO = { text: flattenRuns(runs) };
  if (runs.length > 0) input.runs = runs;
  return input;
}

export function TMBrowser({
  adapter,
  sourceLocale: propSourceLocale = "",
  targetLocales: propTargetLocales = [],
  locales,
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
  // undefined = show all; array = show only those.
  const [displayLocales, setDisplayLocales] = useState<string[] | undefined>(undefined);

  // Marked entities from search bar for entity-value filtering.
  const [markedEntities, setMarkedEntities] = useState<EntityAnnotationDTO[]>([]);

  // Add entry form state.
  const [showAddForm, setShowAddForm] = useState(false);
  const [addSource, setAddSource] = useState("");
  const [addTarget, setAddTarget] = useState("");
  const [addSrcLocale, setAddSrcLocale] = useState(propSourceLocale);
  const [addTgtLocale, setAddTgtLocale] = useState(propTargetLocales[0] ?? "");

  // Filter tokens from the search bar.
  const [filterTokens, setFilterTokens] = useState<Array<{ key: string; value: string }>>([]);

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
  const searchFilter = useMemo((): TMSearchFilter => {
    const filter: TMSearchFilter = {};
    const tokenProject = filterTokens.find((t) => t.key === "project")?.value;
    if (tokenProject) filter.project_id = tokenProject;
    else if (facetSelection.projects.length === 1) filter.project_id = facetSelection.projects[0];
    if (facetSelection.entityTypes.length > 0) filter.entity_types = facetSelection.entityTypes;
    if (facetSelection.sessionIds.length > 0) filter.session_ids = facetSelection.sessionIds;
    if (facetSelection.codeFilter === "has_codes") filter.has_codes = true;
    if (facetSelection.codeFilter === "no_codes") filter.has_codes = false;
    if (markedEntities.length > 0) {
      filter.entity_values = markedEntities.map((e) => ({ value: e.text, type: e.type }));
    }
    return filter;
  }, [facetSelection, filterTokens, markedEntities]);

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
  const searchBarFilterFields = useMemo(() => {
    if (!facets) return [];
    const fields: Array<{
      key: string;
      label: string;
      values?: Array<{ value: string; label: string }>;
    }> = [];
    if (facets.locales.length > 0) {
      fields.push({
        key: "language",
        label: t("Language"),
        values: facets.locales.map((l) => ({ value: l.locale, label: l.locale })),
      });
    }
    if (facets.projects.length > 0) {
      fields.push({
        key: "project",
        label: t("Project"),
        values: facets.projects.map((p) => ({
          value: p.project_id,
          label: p.project_id || t("No project"),
        })),
      });
    }
    return fields;
  }, [facets]);

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
          <div className="flex items-center gap-2 mb-2 pl-3 min-h-7">
            {!isEmpty && (
              <Checkbox
                checked={allVisibleSelected ? true : someVisibleSelected ? "indeterminate" : false}
                onCheckedChange={toggleSelectAll}
                aria-label="Select all visible entries"
                title={allVisibleSelected ? "Deselect all" : "Select all on this page"}
              />
            )}

            {viewMode === "bilingual" && allLocales.length > 1 && (
              <>
                <span className="inline-flex shrink-0 items-center px-1.5 py-px text-[10px] font-medium text-muted-foreground ml-3">
                  Pair:
                </span>
                <select
                  value={bilingualSrc ?? ""}
                  onChange={(e) => setBilingualSrc(e.target.value || null)}
                  className="text-[11px] rounded border border-input bg-background px-1.5 py-0.5"
                  aria-label="Bilingual source locale"
                >
                  <option value="">— src —</option>
                  {allLocales.map((l) => (
                    <option key={l} value={l}>
                      {l}
                    </option>
                  ))}
                </select>
                <span className="text-muted-foreground text-[11px]">→</span>
                <select
                  value={bilingualTgt ?? ""}
                  onChange={(e) => setBilingualTgt(e.target.value || null)}
                  className="text-[11px] rounded border border-input bg-background px-1.5 py-0.5"
                  aria-label="Bilingual target locale"
                >
                  <option value="">— tgt —</option>
                  {allLocales.map((l) => (
                    <option key={l} value={l}>
                      {l}
                    </option>
                  ))}
                </select>
              </>
            )}

            {viewMode === "multilang" && allLocales.length > 1 && (
              <>
                <span className="inline-flex shrink-0 items-center px-1.5 py-px text-[10px] font-medium text-muted-foreground ml-3">
                  Show:
                </span>
                {allLocales.map((locale) => {
                  const active = displayLocales === undefined || displayLocales.includes(locale);
                  return (
                    <button
                      key={locale}
                      onClick={() => toggleDisplayLocale(locale)}
                      className={cn(
                        "inline-flex items-center",
                        "transition-opacity",
                        !active && "opacity-30",
                      )}
                    >
                      <LocalePill locale={locale} />
                    </button>
                  );
                })}
                <button
                  onClick={() => setDisplayLocales(undefined)}
                  className="inline-flex shrink-0 items-center px-1.5 py-px rounded font-mono text-[10px] font-medium bg-muted text-muted-foreground hover:bg-muted/80 hover:text-foreground transition-colors"
                  title="Show all languages"
                >
                  All
                </button>
                <button
                  onClick={() => setDisplayLocales([])}
                  className="inline-flex shrink-0 items-center px-1.5 py-px rounded font-mono text-[10px] font-medium bg-muted text-muted-foreground hover:bg-muted/80 hover:text-foreground transition-colors"
                  title="Hide all languages"
                >
                  None
                </button>
              </>
            )}

            {/* View toggle */}
            <div className="flex rounded-md border border-input ml-auto">
              <Button
                variant="ghost"
                size="icon-xs"
                onClick={() => setViewMode("bilingual")}
                className={cn(
                  "rounded-r-none",
                  viewMode === "bilingual" && "bg-accent text-foreground",
                )}
                title="Bilingual view"
              >
                <List className="size-4" />
              </Button>
              <Button
                variant="ghost"
                size="icon-xs"
                onClick={() => setViewMode("multilang")}
                className={cn(
                  "rounded-l-none",
                  viewMode === "multilang" && "bg-accent text-foreground",
                )}
                title="Multi-language view"
              >
                <Languages className="size-4" />
              </Button>
            </div>
          </div>

          {/* Loading skeleton */}
          {loading && !initialLoadDone && (
            <div className="flex flex-col gap-2">
              {[0, 1, 2].map((i) => (
                <ItemCard key={i} className="animate-pulse p-3">
                  <div className="mb-2 h-3 w-3/4 rounded bg-muted" />
                  <div className="h-3 w-2/3 rounded bg-muted" />
                </ItemCard>
              ))}
            </div>
          )}

          {/* Empty state */}
          {initialLoadDone && !loading && isEmpty && (
            <div className="py-12 text-center text-muted-foreground">
              <p className="text-sm mb-1">
                {debouncedSearch ? t("No entries match your search.") : t("No entries yet.")}
              </p>
              {debouncedSearch && (
                <button
                  onClick={() => {
                    setSearchText("");
                    setDebouncedSearch("");
                  }}
                  className="text-xs text-primary hover:text-primary/80"
                >
                  Clear search
                </button>
              )}
            </div>
          )}

          {/* Entries */}
          {!isEmpty && (
            <div className="flex flex-col gap-1.5">
              {entries.map((entry) => (
                <TMGroupedEntry
                  key={entry.id}
                  entry={withHint(entry, viewMode === "bilingual" ? bilingualSrc : null)}
                  selected={selected.has(entry.id)}
                  onToggleSelect={() => toggleSelect(entry.id)}
                  onEditVariant={(locale, runs) => void handleEditVariant(entry, locale, runs)}
                  onDelete={() => void handleDelete(entry.id)}
                  visibleLocales={viewMode === "bilingual" ? bilingualVisible : displayLocales}
                />
              ))}
            </div>
          )}

          <Pagination
            page={page}
            pageSize={PAGE_SIZE}
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
      <Dialog open={showAddForm} onOpenChange={setShowAddForm}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>Add TM Entry</DialogTitle>
          </DialogHeader>
          <div className="flex flex-col gap-3">
            <div>
              <Label className="text-[12px]">Source</Label>
              <Input
                value={addSource}
                onChange={(e) => setAddSource(e.target.value)}
                placeholder="Source text"
                autoFocus
                className="mt-1"
              />
            </div>
            <div>
              <Label className="text-[12px]">Target</Label>
              <Input
                value={addTarget}
                onChange={(e) => setAddTarget(e.target.value)}
                placeholder="Target text"
                className="mt-1"
              />
            </div>
            <div className="flex gap-3">
              <div className="flex-1">
                <Label className="text-[12px]">Source locale</Label>
                {mergedLocales.length > 0 ? (
                  <LocaleSelect
                    value={addSrcLocale}
                    onChange={setAddSrcLocale}
                    locales={mergedLocales}
                    placeholder="Select source..."
                  />
                ) : (
                  <Input
                    value={addSrcLocale}
                    onChange={(e) => setAddSrcLocale(e.target.value)}
                    className="mt-1"
                  />
                )}
              </div>
              <div className="flex-1">
                <Label className="text-[12px]">Target locale</Label>
                {mergedLocales.length > 0 ? (
                  <LocaleSelect
                    value={addTgtLocale}
                    onChange={setAddTgtLocale}
                    locales={mergedLocales}
                    placeholder="Select target..."
                  />
                ) : (
                  <Input
                    value={addTgtLocale}
                    onChange={(e) => setAddTgtLocale(e.target.value)}
                    className="mt-1"
                  />
                )}
              </div>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setShowAddForm(false)}>
              Cancel
            </Button>
            <Button
              onClick={() => void handleAdd()}
              disabled={!addSource.trim() || !addTarget.trim()}
            >
              Add
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}

/**
 * Returns a TMEntryDTO where hint_src_lang is overridden for display
 * purposes when the caller (bilingual view) has picked a specific source.
 * When `override` is null or not present as a variant, the original entry
 * is returned untouched.
 */
function withHint(entry: TMEntryDTO, override: string | null): TMEntryDTO {
  if (!override) return entry;
  const variant: VariantDTO | undefined = entry.variants[override];
  if (!variant) return entry;
  return { ...entry, hint_src_lang: override };
}
