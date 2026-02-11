import { useState, useEffect, useCallback, useRef } from "react";
import { useTMApi } from "../../hooks/useTMApi";
import { useLocales } from "../../hooks/useLocales";
import type { TMEntryInfo } from "../../types/api";

interface TMExplorerProps {
  sourceLocale: string;
  targetLocales: string[];
  onBack: () => void;
}

const PAGE_SIZE = 50;

export function TMExplorer({ sourceLocale, targetLocales, onBack }: TMExplorerProps) {
  const { getDisplayName } = useLocales();
  const tmApi = useTMApi();
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
        const result = await tmApi.getTMEntries(q, srcLocale, tgtLocale, p * PAGE_SIZE, PAGE_SIZE);
        setEntries(result.entries || []);
        setTotalCount(result.total_count);
      } catch (e) {
        console.error("Failed to fetch TM entries:", e);
      }
    },
    [tmApi],
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
        await tmApi.updateTMEntry({
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
    [tmApi, editTarget, fetchEntries, query, sourceLocaleFilter, targetLocaleFilter, page],
  );

  const handleCancelEdit = useCallback(() => {
    setEditingId(null);
  }, []);

  const handleDelete = useCallback(
    async (entryId: string) => {
      try {
        await tmApi.deleteTMEntry(entryId);
        fetchEntries(query, sourceLocaleFilter, targetLocaleFilter, page);
      } catch (e) {
        console.error("Failed to delete TM entry:", e);
      }
    },
    [tmApi, fetchEntries, query, sourceLocaleFilter, targetLocaleFilter, page],
  );

  const handleAdd = useCallback(async () => {
    if (!addSource.trim() || !addTarget.trim()) return;
    try {
      await tmApi.addTMEntry(addSource, addTarget, addSourceLocale, addTargetLocale);
      setAddSource("");
      setAddTarget("");
      setShowAddForm(false);
      fetchEntries(query, sourceLocaleFilter, targetLocaleFilter, page);
    } catch (e) {
      console.error("Failed to add TM entry:", e);
    }
  }, [tmApi, addSource, addTarget, addSourceLocale, addTargetLocale, fetchEntries, query, sourceLocaleFilter, targetLocaleFilter, page]);

  const totalPages = Math.max(1, Math.ceil(totalCount / PAGE_SIZE));
  const allLocales = [sourceLocale, ...targetLocales];

  return (
    <div data-testid="tm-explorer">
      {/* Header */}
      <div style={{ display: "flex", alignItems: "center", gap: 12, marginBottom: 24 }}>
        <button onClick={onBack} style={backBtnStyle} data-testid="tm-back-btn">&#8592; Back</button>
        <h2 style={{ margin: 0, flex: 1 }}>Translation Memory</h2>
        <span style={badgeStyle} data-testid="tm-count-badge">
          {totalCount} {totalCount === 1 ? "entry" : "entries"}
        </span>
        <button onClick={() => setShowAddForm(true)} style={addBtnStyle} data-testid="tm-add-entry-btn">Add Entry</button>
      </div>

      {/* Add entry form */}
      {showAddForm && (
        <div style={addFormStyle} data-testid="tm-add-form">
          <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
            <input type="text" placeholder="Source text" value={addSource} onChange={(e) => setAddSource(e.target.value)} style={{ ...inputStyle, flex: 1 }} data-testid="tm-add-source-input" />
            <input type="text" placeholder="Target text" value={addTarget} onChange={(e) => setAddTarget(e.target.value)} style={{ ...inputStyle, flex: 1 }} data-testid="tm-add-target-input" />
            <select value={addSourceLocale} onChange={(e) => setAddSourceLocale(e.target.value)} style={selectStyle} data-testid="tm-add-source-locale">
              {allLocales.map((l) => <option key={l} value={l}>{getDisplayName(l)} ({l})</option>)}
            </select>
            <select value={addTargetLocale} onChange={(e) => setAddTargetLocale(e.target.value)} style={selectStyle} data-testid="tm-add-target-locale">
              {allLocales.map((l) => <option key={l} value={l}>{getDisplayName(l)} ({l})</option>)}
            </select>
            <button onClick={handleAdd} style={saveBtnStyle} data-testid="tm-add-submit">Add</button>
            <button onClick={() => setShowAddForm(false)} style={cancelBtnStyle} data-testid="tm-add-cancel">Cancel</button>
          </div>
        </div>
      )}

      {/* Search and filters */}
      <div style={{ display: "flex", gap: 8, marginBottom: 16 }}>
        <input type="text" placeholder="Search entries..." defaultValue={query} onChange={(e) => handleQueryChange(e.target.value)} style={{ ...inputStyle, flex: 1 }} data-testid="tm-search-input" />
        <select value={sourceLocaleFilter} onChange={(e) => { setSourceLocaleFilter(e.target.value); setPage(0); }} style={selectStyle} data-testid="tm-source-locale-filter">
          <option value="">All source locales</option>
          {allLocales.map((l) => <option key={l} value={l}>{getDisplayName(l)} ({l})</option>)}
        </select>
        <select value={targetLocaleFilter} onChange={(e) => { setTargetLocaleFilter(e.target.value); setPage(0); }} style={selectStyle} data-testid="tm-target-locale-filter">
          <option value="">All target locales</option>
          {allLocales.map((l) => <option key={l} value={l}>{getDisplayName(l)} ({l})</option>)}
        </select>
      </div>

      {/* Table */}
      {entries.length === 0 ? (
        <div style={emptyStyle} data-testid="tm-empty-state">
          {query || sourceLocaleFilter || targetLocaleFilter
            ? "No entries match your search."
            : "No translation memory entries yet. Add entries to build your TM."}
        </div>
      ) : (
        <div>
          <table style={tableStyle}>
            <thead>
              <tr>
                <th style={thStyle}>Source</th>
                <th style={thStyle}>Target</th>
                <th style={thStyle}>Source Locale</th>
                <th style={thStyle}>Target Locale</th>
                <th style={thStyle}>Updated</th>
                <th style={{ ...thStyle, width: 120 }}>Actions</th>
              </tr>
            </thead>
            <tbody>
              {entries.map((entry) => (
                <tr key={entry.id} style={rowStyle} data-testid={`tm-entry-${entry.id}`}>
                  <td style={tdStyle}>{entry.source}</td>
                  <td style={tdStyle}>
                    {editingId === entry.id ? (
                      <div style={{ display: "flex", gap: 4 }}>
                        <input type="text" value={editTarget} onChange={(e) => setEditTarget(e.target.value)} style={{ ...inputStyle, flex: 1 }} data-testid={`tm-edit-input-${entry.id}`} />
                        <button onClick={() => handleSaveEdit(entry)} style={saveBtnSmallStyle} data-testid={`tm-save-btn-${entry.id}`}>Save</button>
                        <button onClick={handleCancelEdit} style={cancelBtnSmallStyle} data-testid={`tm-cancel-btn-${entry.id}`}>Cancel</button>
                      </div>
                    ) : entry.target}
                  </td>
                  <td style={tdStyle}>{getDisplayName(entry.source_locale)}</td>
                  <td style={tdStyle}>{getDisplayName(entry.target_locale)}</td>
                  <td style={tdStyle}>{new Date(entry.updated_at).toLocaleDateString()}</td>
                  <td style={tdStyle}>
                    {editingId !== entry.id && (
                      <div style={{ display: "flex", gap: 4 }}>
                        <button onClick={() => handleEdit(entry)} style={actionBtnStyle} data-testid={`tm-edit-btn-${entry.id}`}>Edit</button>
                        <button onClick={() => handleDelete(entry.id)} style={deleteBtnStyle} data-testid={`tm-delete-btn-${entry.id}`}>Delete</button>
                      </div>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>

          {totalPages > 1 && (
            <div style={paginationStyle} data-testid="tm-pagination">
              <button onClick={() => setPage((p) => Math.max(0, p - 1))} disabled={page === 0} style={pageBtnStyle} data-testid="tm-prev-page">Previous</button>
              <span data-testid="tm-page-info">Page {page + 1} of {totalPages}</span>
              <button onClick={() => setPage((p) => Math.min(totalPages - 1, p + 1))} disabled={page >= totalPages - 1} style={pageBtnStyle} data-testid="tm-next-page">Next</button>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

const backBtnStyle: React.CSSProperties = { padding: "6px 12px", backgroundColor: "var(--bg-tertiary)", color: "var(--text-primary)", border: "1px solid var(--border)", borderRadius: 6, fontSize: 13, cursor: "pointer" };
const badgeStyle: React.CSSProperties = { padding: "4px 10px", backgroundColor: "var(--bg-tertiary)", borderRadius: 12, fontSize: 12, color: "var(--text-secondary)" };
const addBtnStyle: React.CSSProperties = { padding: "8px 16px", backgroundColor: "var(--accent)", color: "#fff", border: "none", borderRadius: 6, fontSize: 13, cursor: "pointer", fontWeight: 600 };
const inputStyle: React.CSSProperties = { padding: "8px 12px", backgroundColor: "var(--bg-tertiary)", color: "var(--text-primary)", border: "1px solid var(--border)", borderRadius: 6, fontSize: 13, outline: "none" };
const selectStyle: React.CSSProperties = { padding: "8px 12px", backgroundColor: "var(--bg-tertiary)", color: "var(--text-primary)", border: "1px solid var(--border)", borderRadius: 6, fontSize: 13, outline: "none" };
const addFormStyle: React.CSSProperties = { marginBottom: 16, padding: 16, backgroundColor: "var(--bg-secondary)", border: "1px solid var(--border)", borderRadius: 8 };
const saveBtnStyle: React.CSSProperties = { padding: "8px 16px", backgroundColor: "var(--accent)", color: "#fff", border: "none", borderRadius: 6, fontSize: 13, cursor: "pointer", fontWeight: 600 };
const cancelBtnStyle: React.CSSProperties = { padding: "8px 16px", backgroundColor: "var(--bg-tertiary)", color: "var(--text-primary)", border: "1px solid var(--border)", borderRadius: 6, fontSize: 13, cursor: "pointer" };
const tableStyle: React.CSSProperties = { width: "100%", borderCollapse: "collapse", backgroundColor: "var(--bg-secondary)", borderRadius: 8, overflow: "hidden" };
const thStyle: React.CSSProperties = { padding: "10px 16px", textAlign: "left", fontSize: 12, fontWeight: 600, color: "var(--text-secondary)", borderBottom: "1px solid var(--border)", textTransform: "uppercase", letterSpacing: 0.5 };
const tdStyle: React.CSSProperties = { padding: "10px 16px", fontSize: 14, borderBottom: "1px solid var(--border)" };
const rowStyle: React.CSSProperties = { transition: "background-color 0.1s ease" };
const actionBtnStyle: React.CSSProperties = { padding: "4px 8px", backgroundColor: "var(--bg-tertiary)", color: "var(--text-primary)", border: "1px solid var(--border)", borderRadius: 4, fontSize: 12, cursor: "pointer" };
const deleteBtnStyle: React.CSSProperties = { padding: "4px 8px", backgroundColor: "transparent", color: "#ef4444", border: "1px solid #ef4444", borderRadius: 4, fontSize: 12, cursor: "pointer" };
const saveBtnSmallStyle: React.CSSProperties = { padding: "4px 8px", backgroundColor: "var(--accent)", color: "#fff", border: "none", borderRadius: 4, fontSize: 12, cursor: "pointer" };
const cancelBtnSmallStyle: React.CSSProperties = { padding: "4px 8px", backgroundColor: "var(--bg-tertiary)", color: "var(--text-primary)", border: "1px solid var(--border)", borderRadius: 4, fontSize: 12, cursor: "pointer" };
const emptyStyle: React.CSSProperties = { padding: 48, textAlign: "center", color: "var(--text-secondary)", backgroundColor: "var(--bg-secondary)", borderRadius: 8, border: "1px solid var(--border)" };
const paginationStyle: React.CSSProperties = { display: "flex", alignItems: "center", justifyContent: "center", gap: 16, marginTop: 16, fontSize: 13, color: "var(--text-secondary)" };
const pageBtnStyle: React.CSSProperties = { padding: "6px 12px", backgroundColor: "var(--bg-tertiary)", color: "var(--text-primary)", border: "1px solid var(--border)", borderRadius: 6, fontSize: 13, cursor: "pointer" };
