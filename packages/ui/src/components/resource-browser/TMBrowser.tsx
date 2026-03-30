import { useState, useEffect, useCallback, useRef } from "react";
import type { TMAdapter } from "./adapters";
import type { TMEntryDTO, EntityPatternRequest } from "./types";
import { CodedTextDisplay } from "./CodedTextDisplay";
import { LocalePill } from "./LocalePill";
import { BulkActionBar } from "./BulkActionBar";
import { Pagination } from "./Pagination";
import { TMLookupPanel } from "./TMLookupPanel";
import { EntityAnnotationDialog } from "./EntityAnnotationDialog";
import { relativeTime } from "./utils";

interface TMBrowserProps {
  adapter: TMAdapter;
  sourceLocale?: string;
  targetLocales?: string[];
  showLookup?: boolean;
}

const PAGE_SIZE = 50;

export function TMBrowser({
  adapter,
  sourceLocale = "",
  targetLocales = [],
  showLookup = false,
}: TMBrowserProps) {
  const [entries, setEntries] = useState<TMEntryDTO[]>([]);
  const [totalCount, setTotalCount] = useState(0);
  const [searchText, setSearchText] = useState("");
  const [debouncedSearch, setDebouncedSearch] = useState("");
  const [page, setPage] = useState(0);
  const [loading, setLoading] = useState(true);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [editingId, setEditingId] = useState<string | null>(null);
  const [editTarget, setEditTarget] = useState("");
  const [showAnnotateDialog, setShowAnnotateDialog] = useState(false);
  const [confirmBulkDelete, setConfirmBulkDelete] = useState(false);

  // Add entry form
  const [showAddForm, setShowAddForm] = useState(false);
  const [addSource, setAddSource] = useState("");
  const [addTarget, setAddTarget] = useState("");
  const [addSrcLocale, setAddSrcLocale] = useState(sourceLocale);
  const [addTgtLocale, setAddTgtLocale] = useState(targetLocales[0] ?? "");

  // Debounce search input
  const debounceRef = useRef<ReturnType<typeof setTimeout>>();
  const handleSearch = useCallback((e: React.ChangeEvent<HTMLInputElement>) => {
    const val = e.target.value;
    setSearchText(val);
    clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(() => {
      setDebouncedSearch(val);
      setPage(0);
    }, 200);
  }, []);

  const fetchEntries = useCallback(
    async (q: string, p: number) => {
      setLoading(true);
      try {
        const result = await adapter.search(q, sourceLocale, targetLocales[0] ?? "", p * PAGE_SIZE, PAGE_SIZE);
        setEntries(result.entries ?? []);
        setTotalCount(result.total_count);
      } finally {
        setLoading(false);
      }
    },
    [adapter, sourceLocale, targetLocales],
  );

  useEffect(() => {
    void fetchEntries(debouncedSearch, page);
  }, [fetchEntries, debouncedSearch, page]);

  const toggleSelect = useCallback((id: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }, []);

  const selectAll = useCallback(() => {
    setSelected(new Set(entries.map((e) => e.id)));
  }, [entries]);

  const deselectAll = useCallback(() => {
    setSelected(new Set());
    setConfirmBulkDelete(false);
  }, []);

  const handleEdit = useCallback((entry: TMEntryDTO) => {
    setEditingId(entry.id);
    setEditTarget(entry.target_text);
  }, []);

  const handleSaveEdit = useCallback(
    async (entry: TMEntryDTO) => {
      await adapter.updateEntry({
        entry_id: entry.id,
        source: entry.source_text,
        target: editTarget,
        source_locale: entry.source_locale,
        target_locale: entry.target_locale,
        project_id: entry.project_id,
      });
      setEditingId(null);
      void fetchEntries(debouncedSearch, page);
    },
    [adapter, editTarget, fetchEntries, debouncedSearch, page],
  );

  const handleDelete = useCallback(
    async (id: string) => {
      await adapter.deleteEntry(id);
      setSelected((prev) => {
        const next = new Set(prev);
        next.delete(id);
        return next;
      });
      void fetchEntries(debouncedSearch, page);
    },
    [adapter, fetchEntries, debouncedSearch, page],
  );

  const handleBulkDelete = useCallback(async () => {
    if (!confirmBulkDelete) {
      setConfirmBulkDelete(true);
      return;
    }
    const ids = [...selected];
    await adapter.deleteEntries(ids);
    setSelected(new Set());
    setConfirmBulkDelete(false);
    void fetchEntries(debouncedSearch, page);
  }, [adapter, selected, confirmBulkDelete, fetchEntries, debouncedSearch, page]);

  const handleAdd = useCallback(async () => {
    if (!addSource.trim() || !addTarget.trim()) return;
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
  }, [adapter, addSource, addTarget, addSrcLocale, addTgtLocale, fetchEntries, debouncedSearch, page]);

  const handleAnnotateEntities = useCallback(
    async (patterns: EntityPatternRequest[]) => {
      if (!adapter.annotateEntities) throw new Error("Adapter does not support entity annotation");
      const result = await adapter.annotateEntities({
        entry_ids: [...selected],
        patterns,
      });
      void fetchEntries(debouncedSearch, page);
      return result;
    },
    [adapter, selected, fetchEntries, debouncedSearch, page],
  );

  return (
    <div className="flex gap-4" data-testid="tm-browser">
      {/* Main column */}
      <div className="flex-1 min-w-0">
        {/* Search + Actions */}
        <div className="flex items-center gap-2 mb-4">
          <div className="relative flex-1">
            <input
              type="text"
              value={searchText}
              onChange={handleSearch}
              placeholder="Search translation memory..."
              className="w-full rounded-md border border-input bg-transparent pl-8 pr-3 py-2 text-sm outline-none focus:ring-1 focus:ring-ring"
            />
            <svg className="absolute left-2.5 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-muted-foreground" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><circle cx="11" cy="11" r="8"/><path d="m21 21-4.3-4.3"/></svg>
          </div>
          {selected.size > 0 && selected.size < entries.length && (
            <button onClick={selectAll} className="text-[11px] text-primary hover:text-primary/80">
              Select all
            </button>
          )}
          <button
            onClick={() => setShowAddForm(true)}
            className="rounded-md bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground hover:bg-primary/90 whitespace-nowrap"
          >
            Add Entry
          </button>
        </div>

        {/* Entry count */}
        <div className="text-[12px] text-muted-foreground mb-3">
          {totalCount} {totalCount === 1 ? "entry" : "entries"}
          {debouncedSearch && " matching"}
        </div>

        {/* Loading skeleton */}
        {loading && entries.length === 0 && (
          <div className="flex flex-col gap-2">
            {[0, 1, 2].map((i) => (
              <div key={i} className="rounded-md border border-border p-3 animate-pulse">
                <div className="h-3 bg-muted rounded w-3/4 mb-2" />
                <div className="h-3 bg-muted rounded w-2/3" />
              </div>
            ))}
          </div>
        )}

        {/* Empty state */}
        {!loading && entries.length === 0 && (
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

        {/* Entry list */}
        {entries.length > 0 && (
          <div className="flex flex-col gap-1.5">
            {entries.map((entry) => (
              <div
                key={entry.id}
                className={`group rounded-md border p-3 transition-colors ${
                  selected.has(entry.id)
                    ? "border-primary/40 bg-primary/5"
                    : "border-border hover:border-border/80"
                }`}
                data-testid={`tm-entry-${entry.id}`}
              >
                <div className="flex items-start gap-2">
                  <input
                    type="checkbox"
                    checked={selected.has(entry.id)}
                    onChange={() => toggleSelect(entry.id)}
                    className="mt-1 shrink-0 rounded"
                    aria-label={`Select entry ${entry.source_text}`}
                  />

                  <div className="flex-1 min-w-0">
                    {/* Source */}
                    <div className="flex items-start gap-2 mb-0.5">
                      <span className="text-[10px] text-muted-foreground w-5 shrink-0 pt-0.5 select-none">src</span>
                      <CodedTextDisplay
                        text={entry.source_text}
                        codedText={entry.source_coded}
                        spans={entry.source_spans}
                        className="text-[13px] text-foreground flex-1"
                      />
                      <LocalePill locale={entry.source_locale} />
                    </div>

                    {/* Target (or edit mode) */}
                    <div className="flex items-start gap-2">
                      <span className="text-[10px] text-muted-foreground w-5 shrink-0 pt-0.5 select-none">tgt</span>
                      {editingId === entry.id ? (
                        <div className="flex-1 flex gap-1">
                          <input
                            type="text"
                            value={editTarget}
                            onChange={(e) => setEditTarget(e.target.value)}
                            className="flex-1 rounded border border-input bg-transparent px-2 py-1 text-[13px] outline-none focus:ring-1 focus:ring-ring"
                            autoFocus
                            onKeyDown={(e) => {
                              if (e.key === "Enter") void handleSaveEdit(entry);
                              if (e.key === "Escape") setEditingId(null);
                            }}
                          />
                          <button
                            onClick={() => void handleSaveEdit(entry)}
                            className="text-[11px] text-primary hover:text-primary/80"
                          >
                            Save
                          </button>
                          <button
                            onClick={() => setEditingId(null)}
                            className="text-[11px] text-muted-foreground hover:text-foreground"
                          >
                            Cancel
                          </button>
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
                    <div className="flex items-center gap-2 mt-1.5 pl-7 text-[10px] text-muted-foreground">
                      {entry.project_id ? (
                        <span className="px-1.5 py-px rounded bg-blue-500/10 text-blue-600 dark:text-blue-400">
                          Project
                        </span>
                      ) : (
                        <span className="px-1.5 py-px rounded bg-muted">User</span>
                      )}
                      <span>{relativeTime(entry.updated_at)}</span>

                      {/* Actions — visible at reduced opacity, full on hover */}
                      {editingId !== entry.id && (
                        <div className="ml-auto flex gap-1 opacity-30 group-hover:opacity-100 transition-opacity">
                          <button
                            onClick={() => handleEdit(entry)}
                            className="text-[10px] text-muted-foreground hover:text-foreground transition-colors"
                          >
                            Edit
                          </button>
                          <button
                            onClick={() => void handleDelete(entry.id)}
                            className="text-[10px] text-destructive hover:text-destructive/80 transition-colors"
                          >
                            Delete
                          </button>
                        </div>
                      )}
                    </div>
                  </div>
                </div>
              </div>
            ))}
          </div>
        )}

        <Pagination page={page} pageSize={PAGE_SIZE} totalCount={totalCount} onPageChange={setPage} />
      </div>

      {/* Lookup panel (right side) */}
      {showLookup && adapter.lookup && (
        <div className="w-80 shrink-0 border-l border-border pl-4">
          <TMLookupPanel
            sourceLocale={sourceLocale}
            targetLocale={targetLocales[0] ?? ""}
            onLookup={adapter.lookup}
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
      {showAddForm && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40">
          <div className="w-full max-w-lg rounded-xl border border-border bg-card p-6 shadow-lg">
            <h3 className="text-base font-semibold mb-4">Add TM Entry</h3>
            <div className="flex flex-col gap-3">
              <div>
                <label className="text-[12px] text-muted-foreground block mb-1">Source</label>
                <input
                  type="text"
                  value={addSource}
                  onChange={(e) => setAddSource(e.target.value)}
                  placeholder="Source text"
                  className="w-full rounded-md border border-input bg-transparent px-3 py-2 text-sm outline-none focus:ring-1 focus:ring-ring"
                  autoFocus
                />
              </div>
              <div>
                <label className="text-[12px] text-muted-foreground block mb-1">Target</label>
                <input
                  type="text"
                  value={addTarget}
                  onChange={(e) => setAddTarget(e.target.value)}
                  placeholder="Target text"
                  className="w-full rounded-md border border-input bg-transparent px-3 py-2 text-sm outline-none focus:ring-1 focus:ring-ring"
                />
              </div>
              <div className="flex gap-3">
                <div className="flex-1">
                  <label className="text-[12px] text-muted-foreground block mb-1">Source locale</label>
                  <input type="text" value={addSrcLocale} onChange={(e) => setAddSrcLocale(e.target.value)} className="w-full rounded-md border border-input bg-transparent px-3 py-2 text-sm outline-none focus:ring-1 focus:ring-ring" />
                </div>
                <div className="flex-1">
                  <label className="text-[12px] text-muted-foreground block mb-1">Target locale</label>
                  <input type="text" value={addTgtLocale} onChange={(e) => setAddTgtLocale(e.target.value)} className="w-full rounded-md border border-input bg-transparent px-3 py-2 text-sm outline-none focus:ring-1 focus:ring-ring" />
                </div>
              </div>
            </div>
            <div className="flex gap-2 mt-4 pt-3 border-t border-border">
              <button onClick={() => void handleAdd()} disabled={!addSource.trim() || !addTarget.trim()} className="rounded-md bg-primary px-4 py-1.5 text-xs font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50">Add</button>
              <button onClick={() => setShowAddForm(false)} className="rounded-md border border-border px-4 py-1.5 text-xs hover:bg-accent transition-colors">Cancel</button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
