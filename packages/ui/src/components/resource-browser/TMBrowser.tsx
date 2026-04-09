import { useState, useEffect, useCallback, useRef, useMemo } from "react";
import type { TMAdapter } from "./adapters";
import type { TMEntryDTO, TMGroupedResult, TMFacets, TMSearchFilter, EntityAnnotationDTO, EntityPatternRequest } from "./types";
import type { SpanInfo } from "../../types/span";
import { CodedTextDisplay } from "./CodedTextDisplay";
import { InlineCodeEditor } from "../editor/InlineCodeEditor";
import { LocalePill } from "./LocalePill";
import { BulkActionBar } from "./BulkActionBar";
import { Pagination } from "./Pagination";
import { TMSearchBar } from "./TMSearchBar";
import { TMFacetSidebar, EMPTY_FACETS, type FacetSelection } from "./TMFacetSidebar";
import { TMGroupedEntry } from "./TMGroupedEntry";
import { EntityAnnotationDialog } from "./EntityAnnotationDialog";
import { relativeTime } from "./utils";
import { LocaleSelect, resolveLocaleName, type LocaleInfo } from "../ui/locale-select";
import { ItemCard } from "../ui/item-card";
import { ConfirmDeleteButton } from "../ui/confirm-delete-button";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from "../ui/dialog";
import { Button } from "../ui/button";
import { Input } from "../ui/input";
import { Label } from "../ui/label";
import { Badge } from "../ui/badge";
import { Checkbox } from "../ui/checkbox";
import { List, Languages, X } from "lucide-react";
import { cn } from "../../lib/utils";

type ViewMode = "bilingual" | "multilang";

interface TMBrowserProps {
  adapter: TMAdapter;
  sourceLocale?: string;
  targetLocales?: string[];
  /** Locale list for the add-entry form's locale selectors. If omitted, plain text inputs are used. */
  locales?: LocaleInfo[];
  onError?: (message: string, details?: unknown) => void;
}

const PAGE_SIZE = 50;

export function TMBrowser({
  adapter,
  sourceLocale: propSourceLocale = "",
  targetLocales: propTargetLocales = [],
  locales,
  onError,
}: TMBrowserProps) {
  // --- View mode ---
  const [viewMode, setViewMode] = useState<ViewMode>("bilingual");

  // --- Bilingual state ---
  const [entries, setEntries] = useState<TMEntryDTO[]>([]);

  // --- Multi-language state ---
  const [groups, setGroups] = useState<TMGroupedResult[]>([]);

  // --- Shared state ---
  const [totalCount, setTotalCount] = useState(0);
  const [searchText, setSearchText] = useState("");
  const [debouncedSearch, setDebouncedSearch] = useState("");
  const [page, setPage] = useState(0);
  const [loading, setLoading] = useState(true);
  const [initialLoadDone, setInitialLoadDone] = useState(false);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [editingId, setEditingId] = useState<string | null>(null);
  const [showAnnotateDialog, setShowAnnotateDialog] = useState(false);
  const [confirmBulkDelete, setConfirmBulkDelete] = useState(false);

  // --- Facets ---
  const [facets, setFacets] = useState<TMFacets | null>(null);
  const [facetSelection, setFacetSelection] = useState<FacetSelection>(EMPTY_FACETS);

  // --- Display locales for multi-language view (independent of filter) ---
  // undefined = show all; array (even empty) = show only those (empty = none shown).
  const [displayLocales, setDisplayLocales] = useState<string[] | undefined>(undefined);

  // --- Marked entities from search bar for entity-value filtering ---
  const [markedEntities, setMarkedEntities] = useState<EntityAnnotationDTO[]>([]);

  // --- Add entry form ---
  const [showAddForm, setShowAddForm] = useState(false);
  const [addSource, setAddSource] = useState("");
  const [addTarget, setAddTarget] = useState("");
  const [addSrcLocale, setAddSrcLocale] = useState(propSourceLocale);
  const [addTgtLocale, setAddTgtLocale] = useState(propTargetLocales[0] ?? "");

  // Merge locales prop with locales found in data.
  const mergedLocales = useMemo(() => {
    const known = new Map((locales ?? []).map((l) => [l.code, l]));
    for (const e of entries) {
      if (e.source_locale && !known.has(e.source_locale)) {
        known.set(e.source_locale, { code: e.source_locale, displayName: resolveLocaleName(e.source_locale) });
      }
      if (e.target_locale && !known.has(e.target_locale)) {
        known.set(e.target_locale, { code: e.target_locale, displayName: resolveLocaleName(e.target_locale) });
      }
    }
    return [...known.values()];
  }, [locales, entries]);

  // Available target locales (from facets or data).
  const availableTargetLocales = useMemo(() => {
    if (facets) {
      return [...new Set(facets.locale_pairs.map((lp) => lp.target_locale))];
    }
    return [...new Set(entries.map((e) => e.target_locale).filter(Boolean))];
  }, [facets, entries]);

  const toggleDisplayLocale = useCallback(
    (locale: string) => {
      setDisplayLocales((prev) => {
        // If undefined (showing all), clicking one locale switches to showing only the others.
        if (prev === undefined) {
          return availableTargetLocales.filter((l) => l !== locale);
        }
        return prev.includes(locale) ? prev.filter((l) => l !== locale) : [...prev, locale];
      });
    },
    [availableTargetLocales],
  );

  // Filter tokens (e.g. language:fr-FR, project:my-app) in the search bar.
  const [filterTokens, setFilterTokens] = useState<Array<{ key: string; value: string }>>([]);

  // Token language filter matches either source or target locale.
  const tokenLocale = useMemo(
    () => filterTokens.find((t) => t.key === "language")?.value ?? "",
    [filterTokens],
  );

  // Effective locales: facet selection > props. When a language token is
  // active, clear both locale params — the language filter is applied as
  // an OR filter via searchFilter.locale below.
  const effectiveSourceLocale = tokenLocale ? "" : propSourceLocale;
  const effectiveTargetLocale = tokenLocale
    ? ""
    : facetSelection.targetLocales.length === 1
      ? facetSelection.targetLocales[0]
      : facetSelection.targetLocales.length === 0
        ? propTargetLocales[0] ?? ""
        : "";

  // Search is submitted explicitly (Enter or icon click), not debounced.
  const handleSearchSubmit = useCallback((val: string) => {
    setSearchText(val);
    setDebouncedSearch(val);
    setPage(0);
  }, []);

  // Build search filter from facet selection + filter tokens + marked entities.
  const searchFilter = useMemo((): TMSearchFilter => {
    const filter: TMSearchFilter = {};
    const tokenProject = filterTokens.find((t) => t.key === "project")?.value;
    if (tokenProject) filter.project_id = tokenProject;
    else if (facetSelection.projects.length === 1) filter.project_id = facetSelection.projects[0];
    if (tokenLocale) filter.locale = tokenLocale;
    if (facetSelection.entityTypes.length > 0) filter.entity_types = facetSelection.entityTypes;
    if (facetSelection.codeFilter === "has_codes") filter.has_codes = true;
    if (facetSelection.codeFilter === "no_codes") filter.has_codes = false;
    if (markedEntities.length > 0) {
      filter.entity_values = markedEntities.map((e) => ({ value: e.text, type: e.type }));
    }
    return filter;
  }, [facetSelection, filterTokens, tokenLocale, markedEntities]);

  // Refs for stable callbacks.
  const adapterRef = useRef(adapter);
  const sourceLocaleRef = useRef(effectiveSourceLocale);
  const targetLocaleRef = useRef(effectiveTargetLocale);
  const filterRef = useRef(searchFilter);
  adapterRef.current = adapter;
  sourceLocaleRef.current = effectiveSourceLocale;
  targetLocaleRef.current = effectiveTargetLocale;
  filterRef.current = searchFilter;

  // --- Fetch logic ---
  const fetchEntries = useCallback(
    async (q: string, p: number) => {
      setLoading(true);
      try {
        const a = adapterRef.current;
        const f = filterRef.current;
        const hasFilter = Object.keys(f).length > 0;

        if (viewMode === "multilang") {
          const result = hasFilter && a.searchGroupedFiltered
            ? await a.searchGroupedFiltered(q, sourceLocaleRef.current, f, p * PAGE_SIZE, PAGE_SIZE)
            : a.searchGrouped
              ? await a.searchGrouped(q, sourceLocaleRef.current, p * PAGE_SIZE, PAGE_SIZE)
              : { groups: [], total_count: 0 };
          setGroups(result.groups ?? []);
          setEntries([]);
          setTotalCount(result.total_count);
        } else {
          const result = hasFilter && a.searchFiltered
            ? await a.searchFiltered(q, sourceLocaleRef.current, targetLocaleRef.current, f, p * PAGE_SIZE, PAGE_SIZE)
            : await a.search(q, sourceLocaleRef.current, targetLocaleRef.current, p * PAGE_SIZE, PAGE_SIZE);
          setEntries(result.entries ?? []);
          setGroups([]);
          setTotalCount(result.total_count);
        }
      } finally {
        setLoading(false);
        setInitialLoadDone(true);
      }
    },
    [viewMode],
  );

  // Fetch facets — scoped to current search/filter when the adapter supports it.
  const fetchFacets = useCallback(async () => {
    const a = adapterRef.current;
    try {
      if (a.getFacetsFiltered) {
        const data = await a.getFacetsFiltered(
          debouncedSearch,
          sourceLocaleRef.current,
          targetLocaleRef.current,
          filterRef.current,
        );
        setFacets(data);
      } else if (a.getFacets) {
        const data = await a.getFacets();
        setFacets(data);
      }
    } catch {
      // Facets are non-critical.
    }
  }, [debouncedSearch]);

  useEffect(() => {
    void fetchEntries(debouncedSearch, page);
  }, [fetchEntries, debouncedSearch, page, effectiveSourceLocale, effectiveTargetLocale, viewMode, searchFilter]);

  // Re-fetch facets whenever the search or filter context changes.
  useEffect(() => {
    void fetchFacets();
  }, [fetchFacets, debouncedSearch, effectiveSourceLocale, effectiveTargetLocale, searchFilter]);

  // Reset page when view mode, facet selection, or filter tokens change.
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

  const toggleSelectGroup = useCallback((group: TMGroupedResult) => {
    setSelected((prev) => {
      const next = new Set(prev);
      const ids = group.targets.map((t) => t.id);
      const allSelected = ids.every((id) => next.has(id));
      if (allSelected) {
        ids.forEach((id) => next.delete(id));
      } else {
        ids.forEach((id) => next.add(id));
      }
      return next;
    });
  }, []);

  const deselectAll = useCallback(() => {
    setSelected(new Set());
    setConfirmBulkDelete(false);
  }, []);

  // IDs currently visible in the results (bilingual = entry IDs; multilang = all target IDs).
  const visibleIds = useMemo(() => {
    if (viewMode === "multilang") {
      return groups.flatMap((g) => g.targets.map((t) => t.id));
    }
    return entries.map((e) => e.id);
  }, [viewMode, entries, groups]);

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

  // --- CRUD handlers ---
  const handleEdit = useCallback((entry: TMEntryDTO) => {
    setEditingId(entry.id);
  }, []);

  const handleSaveCodedEdit = useCallback(
    async (entry: TMEntryDTO, codedText: string, spans: SpanInfo[]) => {
      try {
        const plainText = codedText.replace(/[\uE001\uE002\uE003]/g, "");
        await adapter.updateEntry({
          entry_id: entry.id,
          source: entry.source_text,
          target: plainText,
          target_coded: codedText,
          target_spans: spans,
          source_locale: entry.source_locale,
          target_locale: entry.target_locale,
          project_id: entry.project_id,
        });
        setEditingId(null);
        void fetchEntries(debouncedSearch, page);
        void fetchFacets();
      } catch (err) {
        onError?.("Failed to save TM entry", err);
      }
    },
    [adapter, fetchEntries, fetchFacets, debouncedSearch, page, onError],
  );

  const handleSaveGroupedTarget = useCallback(
    async (targetId: string, codedText: string, spans: SpanInfo[]) => {
      try {
        const entry = await adapter.getEntry(targetId);
        if (!entry) return;
        const plainText = codedText.replace(/[\uE001\uE002\uE003]/g, "");
        await adapter.updateEntry({
          entry_id: targetId,
          source: entry.source_text,
          target: plainText,
          target_coded: codedText,
          target_spans: spans,
          source_locale: entry.source_locale,
          target_locale: entry.target_locale,
          project_id: entry.project_id,
        });
        void fetchEntries(debouncedSearch, page);
      } catch (err) {
        onError?.("Failed to save TM entry", err);
      }
    },
    [adapter, fetchEntries, debouncedSearch, page, onError],
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
  }, [adapter, selected, confirmBulkDelete, fetchEntries, fetchFacets, debouncedSearch, page, onError]);

  const handleAdd = useCallback(async () => {
    if (!addSource.trim() || !addTarget.trim()) return;
    try {
      await adapter.addEntry({
        source: addSource,
        target: addTarget,
        source_locale: addSrcLocale,
        target_locale: addTgtLocale,
      });
      setAddSource("");
      setAddTarget("");
      setShowAddForm(false);
      void fetchEntries(debouncedSearch, page);
      void fetchFacets();
    } catch (err) {
      onError?.("Failed to add TM entry", err);
    }
  }, [adapter, addSource, addTarget, addSrcLocale, addTgtLocale, fetchEntries, fetchFacets, debouncedSearch, page, onError]);

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

  const isEmpty = viewMode === "multilang" ? groups.length === 0 : entries.length === 0;

  // Build filter fields from facet data for the search bar's filter dropdown.
  const searchBarFilterFields = useMemo(() => {
    if (!facets) return [];
    const fields: Array<{ key: string; label: string; values?: Array<{ value: string; label: string }> }> = [];
    // Include both source and target locales so users can filter by any
    // language appearing in the TM (matches either column in the backend).
    const allLocales = new Set<string>();
    for (const lp of facets.locale_pairs) {
      if (lp.source_locale) allLocales.add(lp.source_locale);
      if (lp.target_locale) allLocales.add(lp.target_locale);
    }
    if (allLocales.size > 0) {
      fields.push({
        key: "language",
        label: "Language",
        values: [...allLocales].sort().map((l) => ({ value: l, label: l })),
      });
    }
    if (facets.projects.length > 0) {
      fields.push({
        key: "project",
        label: "Project",
        values: facets.projects.map((p) => ({
          value: p.project_id,
          label: p.project_id || "No project",
        })),
      });
    }
    return fields;
  }, [facets]);

  return (
    <div data-testid="tm-browser">
      {/* Google-style search bar — centered, full width */}
      <div className="mb-4">
        <TMSearchBar
          value={searchText}
          onChange={handleSearchSubmit}
          filters={filterTokens}
          onFiltersChange={setFilterTokens}
          filterFields={searchBarFilterFields}
          onLookup={adapter.lookup}
          onEntitiesChange={setMarkedEntities}
          sourceLocale={effectiveSourceLocale}
          targetLocale={effectiveTargetLocale}
        />
      </div>

      {/* Filters (left) + Results (right) */}
      <div className="flex gap-4">
        {/* Left: facet filters */}
        {adapter.getFacets && (
          <div className="w-56 shrink-0">
            <TMFacetSidebar
              facets={facets}
              selection={facetSelection}
              onSelectionChange={setFacetSelection}
            />
          </div>
        )}

        {/* Right: results */}
        <div className="flex-1 min-w-0">
        {/* Toolbar row: view toggle · count · Add Entry */}
        <div className="flex items-center gap-2 mb-2">
          {adapter.searchGrouped && (
            <div className="flex rounded-md border border-input">
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
          )}
          <div className="text-[12px] text-muted-foreground flex items-center gap-2">
            <span>
              {totalCount} {totalCount === 1 ? (viewMode === "multilang" ? "source" : "entry") : (viewMode === "multilang" ? "sources" : "entries")}
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

        {/* Selection + Show-locales row — aligned with the card checkboxes inside ItemCards (pl-3) */}
        {!isEmpty && (
          <div className="flex items-center gap-2 mb-2 pl-3">
            <Checkbox
              checked={allVisibleSelected ? true : someVisibleSelected ? "indeterminate" : false}
              onCheckedChange={toggleSelectAll}
              aria-label="Select all visible entries"
              title={allVisibleSelected ? "Deselect all" : "Select all on this page"}
            />
            {viewMode === "multilang" && availableTargetLocales.length > 1 && (
              <>
                <span className="text-[11px] text-muted-foreground ml-3">Show:</span>
                {availableTargetLocales.map((locale) => {
                  const active = displayLocales === undefined || displayLocales.includes(locale);
                  return (
                    <button
                      key={locale}
                      onClick={() => toggleDisplayLocale(locale)}
                      className={cn("transition-opacity", !active && "opacity-30")}
                    >
                      <LocalePill locale={locale} />
                    </button>
                  );
                })}
                <div className="flex items-center gap-1 ml-1 text-[10px]">
                  <button
                    onClick={() => setDisplayLocales(undefined)}
                    className="text-muted-foreground hover:text-foreground px-1 hover:underline"
                    title="Show all languages"
                  >
                    All
                  </button>
                  <span className="text-muted-foreground/50">·</span>
                  <button
                    onClick={() => setDisplayLocales([])}
                    className="text-muted-foreground hover:text-foreground px-1 hover:underline"
                    title="Hide all languages"
                  >
                    None
                  </button>
                </div>
              </>
            )}
          </div>
        )}

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
              {debouncedSearch ? "No entries match your search." : "No entries yet."}
            </p>
            {debouncedSearch && (
              <button
                onClick={() => { setSearchText(""); setDebouncedSearch(""); }}
                className="text-xs text-primary hover:text-primary/80"
              >
                Clear search
              </button>
            )}
          </div>
        )}

        {/* Multi-language view */}
        {viewMode === "multilang" && groups.length > 0 && (
          <div className="flex flex-col gap-1.5">
            {groups.map((group, idx) => (
              <TMGroupedEntry
                key={`${group.source_text}-${idx}`}
                group={group}
                selected={group.targets.every((t) => selected.has(t.id))}
                onToggleSelect={() => toggleSelectGroup(group)}
                onEditTarget={handleSaveGroupedTarget}
                onDeleteTarget={(id) => void handleDelete(id)}
                visibleLocales={displayLocales}
              />
            ))}
          </div>
        )}

        {/* Bilingual view */}
        {viewMode === "bilingual" && entries.length > 0 && (
          <div className="flex flex-col gap-1.5">
            {entries.map((entry: TMEntryDTO) => (
              <ItemCard
                key={entry.id}
                selected={selected.has(entry.id)}
                className="p-3"
                data-testid={`tm-entry-${entry.id}`}
              >
                <div className="flex items-start gap-2">
                  <Checkbox
                    checked={selected.has(entry.id)}
                    onCheckedChange={() => toggleSelect(entry.id)}
                    className="mt-1 shrink-0"
                    aria-label={`Select entry ${entry.source_text}`}
                  />
                  <div className="flex-1 min-w-0">
                    {/* Source */}
                    <div className="flex items-start gap-2 mb-0.5">
                      <LocalePill locale={entry.source_locale} />
                      <CodedTextDisplay
                        text={entry.source_text}
                        codedText={entry.source_coded}
                        spans={entry.source_spans}
                        className="text-[14px] font-medium text-foreground flex-1"
                      />
                    </div>
                    {/* Target */}
                    <div className="flex items-start gap-2">
                      <LocalePill locale={entry.target_locale} />
                      {editingId === entry.id ? (
                        <div className="flex-1">
                          <InlineCodeEditor
                            initialCodedText={entry.target_coded || entry.target_text}
                            initialSpans={entry.target_spans || []}
                            sourceSpans={entry.source_spans || []}
                            onSave={(codedText, spans) => void handleSaveCodedEdit(entry, codedText, spans)}
                            onCancel={() => setEditingId(null)}
                            compact
                          />
                        </div>
                      ) : (
                        <CodedTextDisplay
                          text={entry.target_text}
                          codedText={entry.target_coded}
                          spans={entry.target_spans}
                          className="text-[13px] text-muted-foreground flex-1"
                        />
                      )}
                    </div>
                    {/* Footer */}
                    <div className="flex items-center gap-2 mt-1.5 text-[10px] text-muted-foreground">
                      {entry.project_id ? (
                        <Badge variant="secondary" className="text-[10px] h-4 bg-blue-500/10 text-blue-600 dark:text-blue-400">Project</Badge>
                      ) : (
                        <Badge variant="secondary" className="text-[10px] h-4">User</Badge>
                      )}
                      <span>{relativeTime(entry.updated_at)}</span>
                      {editingId !== entry.id && (
                        <div className="ml-auto flex gap-1 opacity-0 transition-opacity group-hover:opacity-100">
                          <Button variant="ghost" size="sm" className="h-5 px-1 text-[10px] text-muted-foreground" onClick={() => handleEdit(entry)}>
                            Edit
                          </Button>
                          <ConfirmDeleteButton onDelete={() => void handleDelete(entry.id)} mode="inline" />
                        </div>
                      )}
                    </div>
                  </div>
                </div>
              </ItemCard>
            ))}
          </div>
        )}

        <Pagination page={page} pageSize={PAGE_SIZE} totalCount={totalCount} onPageChange={setPage} />
      </div>
      </div>

      {/* Bulk action bar */}
      <BulkActionBar
        selectedCount={selected.size}
        onDelete={handleBulkDelete}
        confirmDelete={confirmBulkDelete}
        onAnnotateEntities={adapter.annotateEntities ? () => setShowAnnotateDialog(true) : undefined}
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
              <Input value={addSource} onChange={(e) => setAddSource(e.target.value)} placeholder="Source text" autoFocus className="mt-1" />
            </div>
            <div>
              <Label className="text-[12px]">Target</Label>
              <Input value={addTarget} onChange={(e) => setAddTarget(e.target.value)} placeholder="Target text" className="mt-1" />
            </div>
            <div className="flex gap-3">
              <div className="flex-1">
                <Label className="text-[12px]">Source locale</Label>
                {mergedLocales.length > 0 ? (
                  <LocaleSelect value={addSrcLocale} onChange={setAddSrcLocale} locales={mergedLocales} placeholder="Select source..." />
                ) : (
                  <Input value={addSrcLocale} onChange={(e) => setAddSrcLocale(e.target.value)} className="mt-1" />
                )}
              </div>
              <div className="flex-1">
                <Label className="text-[12px]">Target locale</Label>
                {mergedLocales.length > 0 ? (
                  <LocaleSelect value={addTgtLocale} onChange={setAddTgtLocale} locales={mergedLocales} placeholder="Select target..." />
                ) : (
                  <Input value={addTgtLocale} onChange={(e) => setAddTgtLocale(e.target.value)} className="mt-1" />
                )}
              </div>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setShowAddForm(false)}>Cancel</Button>
            <Button onClick={() => void handleAdd()} disabled={!addSource.trim() || !addTarget.trim()}>Add</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
