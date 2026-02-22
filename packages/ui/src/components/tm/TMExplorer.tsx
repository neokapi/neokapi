import { useState, useEffect, useCallback, useRef, useMemo } from "react";
import { useTMApi } from "../../hooks/useTMApi";
import { useLocales } from "../../hooks/useLocales";
import { useSetBreadcrumb } from "../../context/BreadcrumbContext";
import type { TMEntryInfo } from "../../types/api";
import { Button } from "../ui/button";
import { Input } from "../ui/input";
import { Badge } from "../ui/badge";
import { CardContent, GlassCard } from "../ui/card";
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
    <Button variant="outline" size="sm" onClick={onBack} data-testid="tm-back-btn">
      <ArrowLeft className="w-3.5 h-3.5 mr-1" /> Back
    </Button>
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

  const totalPages = Math.max(1, Math.ceil(totalCount / PAGE_SIZE));
  const allLocales = [sourceLocale, ...targetLocales];

  return (
    <div data-testid="tm-explorer">
      {/* Header */}
      <div className="flex items-center gap-3 mb-6">
        <h2 className="flex-1 text-xl font-semibold">Translation Memory</h2>
        <Badge variant="secondary" data-testid="tm-count-badge">
          {totalCount} {totalCount === 1 ? "entry" : "entries"}
        </Badge>
        <Button onClick={() => setShowAddForm(true)} data-testid="tm-add-entry-btn">Add Entry</Button>
      </div>

      {/* Add entry form */}
      {showAddForm && (
        <GlassCard intensity="subtle" className="mb-4" data-testid="tm-add-form"><CardContent className="p-4">
          <div className="flex gap-2 flex-wrap">
            <Input type="text" placeholder="Source text" value={addSource} onChange={(e) => setAddSource(e.target.value)} className="flex-1" data-testid="tm-add-source-input" />
            <Input type="text" placeholder="Target text" value={addTarget} onChange={(e) => setAddTarget(e.target.value)} className="flex-1" data-testid="tm-add-target-input" />
            <select value={addSourceLocale} onChange={(e) => setAddSourceLocale(e.target.value)} className="px-3 py-2 border border-input rounded-md bg-transparent text-foreground text-sm" data-testid="tm-add-source-locale">
              {allLocales.map((l) => <option key={l} value={l}>{getDisplayName(l)} ({l})</option>)}
            </select>
            <select value={addTargetLocale} onChange={(e) => setAddTargetLocale(e.target.value)} className="px-3 py-2 border border-input rounded-md bg-transparent text-foreground text-sm" data-testid="tm-add-target-locale">
              {allLocales.map((l) => <option key={l} value={l}>{getDisplayName(l)} ({l})</option>)}
            </select>
            <Button onClick={handleAdd} data-testid="tm-add-submit">Add</Button>
            <Button variant="outline" onClick={() => setShowAddForm(false)} data-testid="tm-add-cancel">Cancel</Button>
          </div>
        </CardContent></GlassCard>
      )}

      {/* Search and filters */}
      <div className="flex gap-2 mb-4">
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
        <GlassCard intensity="subtle" data-testid="tm-empty-state">
          <CardContent className="p-12 text-center text-muted-foreground">
            {query || sourceLocaleFilter || targetLocaleFilter
              ? "No entries match your search."
              : "No translation memory entries yet. Add entries to build your TM."}
          </CardContent>
        </GlassCard>
      ) : (
        <GlassCard intensity="subtle" className="overflow-hidden">
          <table className="w-full border-collapse">
            <thead>
              <tr>
                <th className="px-4 py-2.5 text-left text-xs font-semibold text-muted-foreground border-b border-border uppercase tracking-wider">Source</th>
                <th className="px-4 py-2.5 text-left text-xs font-semibold text-muted-foreground border-b border-border uppercase tracking-wider">Target</th>
                <th className="px-4 py-2.5 text-left text-xs font-semibold text-muted-foreground border-b border-border uppercase tracking-wider">Source Locale</th>
                <th className="px-4 py-2.5 text-left text-xs font-semibold text-muted-foreground border-b border-border uppercase tracking-wider">Target Locale</th>
                <th className="px-4 py-2.5 text-left text-xs font-semibold text-muted-foreground border-b border-border uppercase tracking-wider">Updated</th>
                <th className="px-4 py-2.5 text-xs font-semibold text-muted-foreground border-b border-border w-[120px]">Actions</th>
              </tr>
            </thead>
            <tbody>
              {entries.map((entry) => (
                <tr key={entry.id} className="transition-colors hover:bg-accent/50" data-testid={`tm-entry-${entry.id}`}>
                  <td className="px-4 py-2.5 text-sm border-b border-border" data-testid={`tm-source-${entry.id}`}>{entry.source}</td>
                  <td className="px-4 py-2.5 text-sm border-b border-border" data-testid={`tm-target-${entry.id}`}>
                    {editingId === entry.id ? (
                      <div className="flex gap-1">
                        <Input type="text" value={editTarget} onChange={(e) => setEditTarget(e.target.value)} className="flex-1 h-8" data-testid={`tm-edit-input-${entry.id}`} />
                        <Button size="sm" onClick={() => handleSaveEdit(entry)} data-testid={`tm-save-btn-${entry.id}`}>Save</Button>
                        <Button size="sm" variant="outline" onClick={handleCancelEdit} data-testid={`tm-cancel-btn-${entry.id}`}>Cancel</Button>
                      </div>
                    ) : entry.target}
                  </td>
                  <td className="px-4 py-2.5 text-sm border-b border-border">{getDisplayName(entry.source_locale)}</td>
                  <td className="px-4 py-2.5 text-sm border-b border-border">{getDisplayName(entry.target_locale)}</td>
                  <td className="px-4 py-2.5 text-sm border-b border-border">{new Date(entry.updated_at).toLocaleDateString()}</td>
                  <td className="px-4 py-2.5 text-sm border-b border-border">
                    {editingId !== entry.id && (
                      <div className="flex gap-1">
                        <Button size="sm" variant="outline" onClick={() => handleEdit(entry)} data-testid={`tm-edit-btn-${entry.id}`}>Edit</Button>
                        <Button size="sm" variant="destructive" onClick={() => handleDelete(entry.id)} data-testid={`tm-delete-btn-${entry.id}`}>Delete</Button>
                      </div>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>

          {totalPages > 1 && (
            <div className="flex items-center justify-center gap-4 py-3 text-[13px] text-muted-foreground border-t border-border" data-testid="tm-pagination">
              <Button size="sm" variant="outline" onClick={() => setPage((p) => Math.max(0, p - 1))} disabled={page === 0} data-testid="tm-prev-page">Previous</Button>
              <span data-testid="tm-page-info">Page {page + 1} of {totalPages}</span>
              <Button size="sm" variant="outline" onClick={() => setPage((p) => Math.min(totalPages - 1, p + 1))} disabled={page >= totalPages - 1} data-testid="tm-next-page">Next</Button>
            </div>
          )}
        </GlassCard>
      )}
    </div>
  );
}
