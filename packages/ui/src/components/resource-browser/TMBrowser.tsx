import { useState, useEffect, useCallback, useRef, useMemo } from "react";
import type { TMAdapter } from "./adapters";
import type { TMEntryDTO, TMGroupedResult, TMFacets, EntityPatternRequest } from "./types";
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
import { List, Languages } from "lucide-react";
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

  // Effective locales from facet selection.
  const effectiveSourceLocale = propSourceLocale;
  const effectiveTargetLocale =
    facetSelection.targetLocales.length === 1
      ? facetSelection.targetLocales[0]
      : propTargetLocales[0] ?? "";

  // Debounce search.
  const debounceRef = useRef<ReturnType<typeof setTimeout>>(undefined);
  const handleSearchChange = useCallback((val: string) => {
    setSearchText(val);
    clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(() => {
      setDebouncedSearch(val);
      setPage(0);
    }, 200);
  }, []);

  // Refs for stable callbacks.
  const adapterRef = useRef(adapter);
  const sourceLocaleRef = useRef(effectiveSourceLocale);
  const targetLocaleRef = useRef(effectiveTargetLocale);
  adapterRef.current = adapter;
  sourceLocaleRef.current = effectiveSourceLocale;
  targetLocaleRef.current = effectiveTargetLocale;

  // --- Fetch logic ---
  const fetchEntries = useCallback(
    async (q: string, p: number) => {
      setLoading(true);
      try {
        if (viewMode === "multilang" && adapterRef.current.searchGrouped) {
          const result = await adapterRef.current.searchGrouped(
            q,
            sourceLocaleRef.current,
            p * PAGE_SIZE,
            PAGE_SIZE,
          );
          setGroups(result.groups ?? []);
          setEntries([]);
          setTotalCount(result.total_count);
        } else {
          const result = await adapterRef.current.search(
            q,
            sourceLocaleRef.current,
            targetLocaleRef.current,
            p * PAGE_SIZE,
            PAGE_SIZE,
          );
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

  // Fetch facets.
  const fetchFacets = useCallback(async () => {
    if (adapterRef.current.getFacets) {
      try {
        const data = await adapterRef.current.getFacets();
        setFacets(data);
      } catch {
        // Facets are non-critical.
      }
    }
  }, []);

  useEffect(() => {
    void fetchEntries(debouncedSearch, page);
  }, [fetchEntries, debouncedSearch, page, effectiveSourceLocale, effectiveTargetLocale, viewMode]);

  useEffect(() => {
    void fetchFacets();
  }, [fetchFacets]);

  // Reset page when view mode changes.
  useEffect(() => {
    setPage(0);
    setSelected(new Set());
  }, [viewMode]);

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

  return (
    <div className="flex gap-4" data-testid="tm-browser">
      {/* Main column */}
      <div className="flex-1 min-w-0">
        {/* Search bar + actions */}
        <div className="mb-4">
          <TMSearchBar
            value={searchText}
            onChange={handleSearchChange}
            onLookup={adapter.lookup}
            sourceLocale={effectiveSourceLocale}
            targetLocale={effectiveTargetLocale}
            actions={
              <div className="flex items-center gap-1">
                {/* View mode toggle */}
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
                <Button size="sm" onClick={() => setShowAddForm(true)} className="whitespace-nowrap">
                  Add Entry
                </Button>
              </div>
            }
          />
        </div>

        {/* Count + loading */}
        <div className="text-[12px] text-muted-foreground mb-3 flex items-center gap-2">
          <span>
            {totalCount} {totalCount === 1 ? (viewMode === "multilang" ? "source" : "entry") : (viewMode === "multilang" ? "sources" : "entries")}
            {debouncedSearch && " matching"}
          </span>
          {loading && initialLoadDone && (
            <span className="inline-block w-3 h-3 border-2 border-current border-t-transparent rounded-full animate-spin opacity-50" />
          )}
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
                      <CodedTextDisplay
                        text={entry.source_text}
                        codedText={entry.source_coded}
                        spans={entry.source_spans}
                        className="text-[13px] text-foreground flex-1"
                      />
                      <LocalePill locale={entry.source_locale} />
                    </div>
                    {/* Target */}
                    <div className="flex items-start gap-2">
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
                      <LocalePill locale={entry.target_locale} />
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

      {/* Facet sidebar (right) */}
      {adapter.getFacets && (
        <div className="w-56 shrink-0 border-l border-border pl-4">
          <TMFacetSidebar
            facets={facets}
            selection={facetSelection}
            onSelectionChange={setFacetSelection}
          />
        </div>
      )}

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
