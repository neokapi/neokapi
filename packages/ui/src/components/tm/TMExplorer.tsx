import { useState, useEffect, useCallback, useRef, useMemo } from "react";
import { useTMApi } from "../../hooks/useTMApi";
import { useLocales } from "../../hooks/useLocales";
import { useSetBreadcrumb } from "../../context/BreadcrumbContext";
import type { TMEntryInfo } from "../../types/api";
import { Button } from "../ui/button";
import { Input } from "../ui/input";
import { Label } from "../ui/label";
import { GlassCard } from "../ui/card";
import {
  Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter,
} from "../ui/dialog";
import { LocaleSelect } from "../LocaleSelect";
import { ArrowLeft } from "../icons";

interface TMExplorerProps {
  sourceLocale: string;
  targetLocales: string[];
  onBack: () => void;
}

const PAGE_SIZE = 50;

export function TMExplorer({ sourceLocale, targetLocales, onBack }: TMExplorerProps) {
  const { getDisplayName } = useLocales();

  const breadcrumbNode = useMemo(() => (
    <button onClick={onBack} data-testid="tm-back-btn" className="flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground transition-colors cursor-pointer bg-transparent border-none p-0">
      <ArrowLeft className="w-3.5 h-3.5" /> Back
    </button>
  ), [onBack]);
  useSetBreadcrumb(breadcrumbNode);
  const { getTMEntries, addTMEntry, updateTMEntry, deleteTMEntry } = useTMApi();
  const [entries, setEntries] = useState<TMEntryInfo[]>([]);
  const [totalCount, setTotalCount] = useState(0);
  const [query, setQuery] = useState("");
  const [sourceLocaleFilter, setSourceLocaleFilter] = useState("");
  const [targetLocaleFilter, setTargetLocaleFilter] = useState("");
  const [page, setPage] = useState(0);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [editTarget, setEditTarget] = useState("");
  const [showAddForm, setShowAddForm] = useState(false);
  const [addSource, setAddSource] = useState("");
  const [addTarget, setAddTarget] = useState("");
  const [addSourceLocale, setAddSourceLocale] = useState(sourceLocale);
  const [addTargetLocale, setAddTargetLocale] = useState(targetLocales[0] || "");
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const fetchEntries = useCallback(
    async (q: string, srcLocale: string, tgtLocale: string, p: number) => {
      try {
        const result = await getTMEntries(q, srcLocale, tgtLocale, p * PAGE_SIZE, PAGE_SIZE);
        setEntries(result.entries || []);
        setTotalCount(result.total_count);
      } catch (e) {
        console.error("Failed to fetch TM entries:", e);
      }
    },
    [getTMEntries],
  );

  useEffect(() => {
    fetchEntries(query, sourceLocaleFilter, targetLocaleFilter, page);
  }, [fetchEntries, query, sourceLocaleFilter, targetLocaleFilter, page]);

  const handleQueryChange = useCallback((value: string) => {
    if (debounceRef.current) clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(() => {
      setQuery(value);
      setPage(0);
    }, 300);
  }, []);

  const handleEdit = useCallback((entry: TMEntryInfo) => {
    setEditingId(entry.id);
    setEditTarget(entry.target);
  }, []);

  const handleSaveEdit = useCallback(
    async (entry: TMEntryInfo) => {
      try {
        await updateTMEntry({
          project_id: "",
          entry_id: entry.id,
          source: entry.source,
          target: editTarget,
          source_locale: entry.source_locale,
          target_locale: entry.target_locale,
        });
        setEditingId(null);
        fetchEntries(query, sourceLocaleFilter, targetLocaleFilter, page);
      } catch (e) {
        console.error("Failed to update TM entry:", e);
      }
    },
    [updateTMEntry, editTarget, fetchEntries, query, sourceLocaleFilter, targetLocaleFilter, page],
  );

  const handleCancelEdit = useCallback(() => {
    setEditingId(null);
  }, []);

  const handleDelete = useCallback(
    async (entryId: string) => {
      try {
        await deleteTMEntry(entryId);
        fetchEntries(query, sourceLocaleFilter, targetLocaleFilter, page);
      } catch (e) {
        console.error("Failed to delete TM entry:", e);
      }
    },
    [deleteTMEntry, fetchEntries, query, sourceLocaleFilter, targetLocaleFilter, page],
  );

  const handleAdd = useCallback(async () => {
    if (!addSource.trim() || !addTarget.trim()) return;
    try {
      await addTMEntry(addSource, addTarget, addSourceLocale, addTargetLocale);
      setAddSource("");
      setAddTarget("");
      setShowAddForm(false);
      fetchEntries(query, sourceLocaleFilter, targetLocaleFilter, page);
    } catch (e) {
      console.error("Failed to add TM entry:", e);
    }
  }, [addTMEntry, addSource, addTarget, addSourceLocale, addTargetLocale, fetchEntries, query, sourceLocaleFilter, targetLocaleFilter, page]);

  const handleAddDialogChange = useCallback((open: boolean) => {
    if (!open) {
      setAddSource("");
      setAddTarget("");
      setAddSourceLocale(sourceLocale);
      setAddTargetLocale(targetLocales[0] || "");
    }
    setShowAddForm(open);
  }, [sourceLocale, targetLocales]);

  const totalPages = Math.max(1, Math.ceil(totalCount / PAGE_SIZE));
  const allLocales = [sourceLocale, ...targetLocales];

  return (
    <div data-testid="tm-explorer">
      <GlassCard intensity="subtle" className="p-6">
        {/* Header */}
        <div className="flex items-center justify-between mb-6">
          <div>
            <h2 className="text-xl font-semibold">Translation Memory</h2>
            <p className="text-[13px] text-muted-foreground mt-1" data-testid="tm-count-badge">
              {totalCount} {totalCount === 1 ? "entry" : "entries"}
            </p>
          </div>
          <Button onClick={() => setShowAddForm(true)} data-testid="tm-add-entry-btn">Add Entry</Button>
        </div>

        {/* Search and filters */}
        <div className="flex gap-2 mb-6">
          <Input type="text" placeholder="Search entries..." defaultValue={query} onChange={(e) => handleQueryChange(e.target.value)} className="flex-1" data-testid="tm-search-input" />
          <select value={sourceLocaleFilter} onChange={(e) => { setSourceLocaleFilter(e.target.value); setPage(0); }} className="px-3 py-2 border border-input rounded-md bg-transparent text-foreground text-sm" data-testid="tm-source-locale-filter">
            <option value="">All source locales</option>
            {allLocales.map((l) => <option key={l} value={l}>{getDisplayName(l)} ({l})</option>)}
          </select>
          <select value={targetLocaleFilter} onChange={(e) => { setTargetLocaleFilter(e.target.value); setPage(0); }} className="px-3 py-2 border border-input rounded-md bg-transparent text-foreground text-sm" data-testid="tm-target-locale-filter">
            <option value="">All target locales</option>
            {allLocales.map((l) => <option key={l} value={l}>{getDisplayName(l)} ({l})</option>)}
          </select>
        </div>

        {/* Table */}
        {entries.length === 0 ? (
          <div className="py-12 text-center text-muted-foreground" data-testid="tm-empty-state">
            <p className="mb-2">{query || sourceLocaleFilter || targetLocaleFilter
              ? "No entries match your search."
              : "No translation memory entries yet. Add entries to build your TM."}</p>
            {(query || sourceLocaleFilter || targetLocaleFilter) && (
              <Button variant="ghost" size="sm" onClick={() => { setQuery(""); setSourceLocaleFilter(""); setTargetLocaleFilter(""); }}>Clear filters</Button>
            )}
          </div>
        ) : (
          <>
            <div className="overflow-x-auto">
              <table className="w-full border-collapse">
                <thead>
                  <tr className="border-b border-border">
                    <th className="px-4 py-2.5 text-left text-sm font-medium text-muted-foreground">Source</th>
                    <th className="px-4 py-2.5 text-left text-sm font-medium text-muted-foreground">Target</th>
                    <th className="px-4 py-2.5 text-left text-sm font-medium text-muted-foreground">Source Locale</th>
                    <th className="px-4 py-2.5 text-left text-sm font-medium text-muted-foreground">Target Locale</th>
                    <th className="px-4 py-2.5 text-left text-sm font-medium text-muted-foreground">Updated</th>
                    <th className="px-4 py-2.5 text-sm font-medium text-muted-foreground w-[120px]">Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {entries.map((entry) => (
                    <tr key={entry.id} className="border-b border-border/50 transition-colors hover:bg-accent/50" data-testid={`tm-entry-${entry.id}`}>
                      <td className="px-4 py-2.5 text-sm font-medium" data-testid={`tm-source-${entry.id}`}>{entry.source}</td>
                      <td className="px-4 py-2.5 text-sm text-muted-foreground" data-testid={`tm-target-${entry.id}`}>
                        {editingId === entry.id ? (
                          <div className="flex gap-1">
                            <Input type="text" value={editTarget} onChange={(e) => setEditTarget(e.target.value)} className="flex-1 h-8" data-testid={`tm-edit-input-${entry.id}`} />
                            <Button size="sm" onClick={() => handleSaveEdit(entry)} data-testid={`tm-save-btn-${entry.id}`}>Save</Button>
                            <Button size="sm" variant="outline" onClick={handleCancelEdit} data-testid={`tm-cancel-btn-${entry.id}`}>Cancel</Button>
                          </div>
                        ) : entry.target}
                      </td>
                      <td className="px-4 py-2.5 text-sm text-muted-foreground">{getDisplayName(entry.source_locale)}</td>
                      <td className="px-4 py-2.5 text-sm text-muted-foreground">{getDisplayName(entry.target_locale)}</td>
                      <td className="px-4 py-2.5 text-sm text-muted-foreground">{new Date(entry.updated_at).toLocaleDateString()}</td>
                      <td className="px-4 py-2.5 text-sm">
                        {editingId !== entry.id && (
                          <div className="flex gap-1">
                            <Button size="sm" variant="ghost" onClick={() => handleEdit(entry)} data-testid={`tm-edit-btn-${entry.id}`}>Edit</Button>
                            <Button size="sm" variant="ghost" className="text-destructive hover:text-destructive" onClick={() => handleDelete(entry.id)} data-testid={`tm-delete-btn-${entry.id}`}>Delete</Button>
                          </div>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>

            {totalPages > 1 && (
              <div className="flex items-center justify-between mt-6 pt-6 border-t border-border" data-testid="tm-pagination">
                <span className="text-sm text-muted-foreground">
                  Showing {page * PAGE_SIZE + 1} to {Math.min((page + 1) * PAGE_SIZE, totalCount)} of {totalCount}
                </span>
                <div className="flex items-center gap-2">
                  <Button size="sm" variant="ghost" onClick={() => setPage((p) => Math.max(0, p - 1))} disabled={page === 0} data-testid="tm-prev-page">Previous</Button>
                  <span className="text-sm text-muted-foreground" data-testid="tm-page-info">Page {page + 1} of {totalPages}</span>
                  <Button size="sm" variant="ghost" onClick={() => setPage((p) => Math.min(totalPages - 1, p + 1))} disabled={page >= totalPages - 1} data-testid="tm-next-page">Next</Button>
                </div>
              </div>
            )}
          </>
        )}
      </GlassCard>

      <Dialog open={showAddForm} onOpenChange={handleAddDialogChange}>
        <DialogContent size="md" data-testid="tm-add-form" onInteractOutside={(e: Event) => e.preventDefault()}>
          <DialogHeader>
            <DialogTitle>Add TM Entry</DialogTitle>
          </DialogHeader>
          <div className="flex flex-col gap-4 py-2">
            <div>
              <Label className="text-muted-foreground">Source Text</Label>
              <Input type="text" placeholder="Source text" value={addSource} onChange={(e) => setAddSource(e.target.value)} className="mt-1" data-testid="tm-add-source-input" autoFocus />
            </div>
            <div>
              <Label className="text-muted-foreground">Target Text</Label>
              <Input type="text" placeholder="Target text" value={addTarget} onChange={(e) => setAddTarget(e.target.value)} className="mt-1" data-testid="tm-add-target-input" />
            </div>
            <div className="flex gap-3">
              <div className="flex flex-col gap-1 flex-1">
                <Label className="text-muted-foreground">Source Locale</Label>
                <LocaleSelect value={addSourceLocale} onChange={setAddSourceLocale} codes={allLocales} data-testid="tm-add-source-locale" />
              </div>
              <div className="flex flex-col gap-1 flex-1">
                <Label className="text-muted-foreground">Target Locale</Label>
                <LocaleSelect value={addTargetLocale} onChange={setAddTargetLocale} codes={allLocales} data-testid="tm-add-target-locale" />
              </div>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => handleAddDialogChange(false)} data-testid="tm-add-cancel">Cancel</Button>
            <Button onClick={handleAdd} disabled={!addSource.trim() || !addTarget.trim()} data-testid="tm-add-submit">Add</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
