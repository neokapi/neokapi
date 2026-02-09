import { useState, useEffect, useCallback, useRef } from "react";
import { useTermsApi } from "../hooks/useApi";
import type { ProjectInfo, ConceptInfo, TermInfo } from "../types/api";

interface TermExplorerProps {
  project: ProjectInfo;
  onBack: () => void;
}

const PAGE_SIZE = 50;

const STATUS_OPTIONS = ["preferred", "approved", "admitted", "proposed", "deprecated", "forbidden"];

export function TermExplorer({ project, onBack }: TermExplorerProps) {
  const termsApi = useTermsApi();
  const [concepts, setConcepts] = useState<ConceptInfo[]>([]);
  const [totalCount, setTotalCount] = useState(0);
  const [query, setQuery] = useState("");
  const [sourceLocaleFilter, setSourceLocaleFilter] = useState("");
  const [targetLocaleFilter, setTargetLocaleFilter] = useState("");
  const [page, setPage] = useState(0);
  const [showAddForm, setShowAddForm] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [editConcept, setEditConcept] = useState<ConceptInfo | null>(null);
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // New concept form state
  const [newDomain, setNewDomain] = useState("");
  const [newDefinition, setNewDefinition] = useState("");
  const [newTerms, setNewTerms] = useState<TermInfo[]>([
    { text: "", locale: project.source_locale, status: "preferred" },
    { text: "", locale: project.target_locales[0] || "", status: "preferred" },
  ]);

  const fetchConcepts = useCallback(
    async (q: string, srcLocale: string, tgtLocale: string, p: number) => {
      try {
        const result = await termsApi.getTerms(
          project.id, q, srcLocale, tgtLocale,
          p * PAGE_SIZE, PAGE_SIZE,
        );
        setConcepts(result.concepts || []);
        setTotalCount(result.total_count);
      } catch (e) {
        console.error("Failed to fetch terms:", e);
      }
    },
    [project.id, termsApi],
  );

  useEffect(() => {
    fetchConcepts(query, sourceLocaleFilter, targetLocaleFilter, page);
  }, [fetchConcepts, query, sourceLocaleFilter, targetLocaleFilter, page]);

  const handleQueryChange = useCallback((value: string) => {
    if (debounceRef.current) clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(() => {
      setQuery(value);
      setPage(0);
    }, 300);
  }, []);

  const handleAdd = useCallback(async () => {
    const validTerms = newTerms.filter(t => t.text.trim() !== "");
    if (validTerms.length === 0) return;
    try {
      await termsApi.addConcept({
        project_id: project.id,
        domain: newDomain,
        definition: newDefinition,
        terms: validTerms,
      });
      setNewDomain("");
      setNewDefinition("");
      setNewTerms([
        { text: "", locale: project.source_locale, status: "preferred" },
        { text: "", locale: project.target_locales[0] || "", status: "preferred" },
      ]);
      setShowAddForm(false);
      fetchConcepts(query, sourceLocaleFilter, targetLocaleFilter, page);
    } catch (e) {
      console.error("Failed to add concept:", e);
    }
  }, [project, termsApi, newDomain, newDefinition, newTerms, fetchConcepts, query, sourceLocaleFilter, targetLocaleFilter, page]);

  const handleEdit = useCallback((concept: ConceptInfo) => {
    setEditingId(concept.id);
    setEditConcept({ ...concept, terms: [...concept.terms] });
  }, []);

  const handleSaveEdit = useCallback(async () => {
    if (!editConcept) return;
    try {
      await termsApi.updateConcept({
        project_id: project.id,
        concept_id: editConcept.id,
        domain: editConcept.domain,
        definition: editConcept.definition,
        terms: editConcept.terms,
      });
      setEditingId(null);
      setEditConcept(null);
      fetchConcepts(query, sourceLocaleFilter, targetLocaleFilter, page);
    } catch (e) {
      console.error("Failed to update concept:", e);
    }
  }, [project.id, termsApi, editConcept, fetchConcepts, query, sourceLocaleFilter, targetLocaleFilter, page]);

  const handleDelete = useCallback(
    async (conceptId: string) => {
      try {
        await termsApi.deleteConcept(project.id, conceptId);
        fetchConcepts(query, sourceLocaleFilter, targetLocaleFilter, page);
      } catch (e) {
        console.error("Failed to delete concept:", e);
      }
    },
    [project.id, termsApi, fetchConcepts, query, sourceLocaleFilter, targetLocaleFilter, page],
  );

  const handleImportCSV = useCallback(async () => {
    const input = document.createElement("input");
    input.type = "file";
    input.accept = ".csv,.tsv";
    input.onchange = async () => {
      const file = input.files?.[0];
      if (!file) return;
      const content = await file.text();
      try {
        const count = await termsApi.importTermsCSV(
          project.id, content,
          project.source_locale,
          project.target_locales[0] || "",
          "", true,
        );
        alert(`Imported ${count} concepts`);
        fetchConcepts(query, sourceLocaleFilter, targetLocaleFilter, page);
      } catch (e) {
        console.error("CSV import failed:", e);
      }
    };
    input.click();
  }, [project, termsApi, fetchConcepts, query, sourceLocaleFilter, targetLocaleFilter, page]);

  const handleExportJSON = useCallback(async () => {
    try {
      const json = await termsApi.exportTermsJSON(project.id, project.name);
      const blob = new Blob([json], { type: "application/json" });
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = `${project.name}-termbase.json`;
      a.click();
      URL.revokeObjectURL(url);
    } catch (e) {
      console.error("Export failed:", e);
    }
  }, [project, termsApi]);

  const statusBadge = (status: string) => {
    const colors: Record<string, string> = {
      preferred: "#22c55e",
      approved: "#3b82f6",
      admitted: "#a78bfa",
      proposed: "#fbbf24",
      deprecated: "#f87171",
      forbidden: "#ef4444",
    };
    return (
      <span style={{
        display: "inline-block",
        padding: "1px 6px",
        borderRadius: 3,
        fontSize: 10,
        fontWeight: 600,
        color: "#fff",
        background: colors[status] || "#6b7280",
      }}>
        {status}
      </span>
    );
  };

  const addTermRow = (terms: TermInfo[], setter: (t: TermInfo[]) => void) => {
    setter([...terms, { text: "", locale: "", status: "approved" }]);
  };

  const removeTermRow = (terms: TermInfo[], setter: (t: TermInfo[]) => void, idx: number) => {
    setter(terms.filter((_, i) => i !== idx));
  };

  const updateTermRow = (terms: TermInfo[], setter: (t: TermInfo[]) => void, idx: number, field: keyof TermInfo, value: string) => {
    const updated = [...terms];
    updated[idx] = { ...updated[idx], [field]: value };
    setter(updated);
  };

  const allLocales = [project.source_locale, ...project.target_locales];
  const totalPages = Math.ceil(totalCount / PAGE_SIZE);

  return (
    <div>
      <div style={{ display: "flex", alignItems: "center", gap: 12, marginBottom: 20 }}>
        <button onClick={onBack} style={backBtnStyle}>&#8592; Project</button>
        <h2 style={{ margin: 0, flex: 1 }}>Terminology</h2>
        <span style={{ fontSize: 13, color: "var(--text-secondary)" }}>{totalCount} concepts</span>
      </div>

      {/* Toolbar */}
      <div style={{ display: "flex", gap: 8, marginBottom: 16, flexWrap: "wrap" }}>
        <input
          type="text"
          placeholder="Search terms..."
          defaultValue={query}
          onChange={(e) => handleQueryChange(e.target.value)}
          style={inputStyle}
        />
        <select
          value={sourceLocaleFilter}
          onChange={(e) => { setSourceLocaleFilter(e.target.value); setPage(0); }}
          style={selectStyle}
        >
          <option value="">All source locales</option>
          {allLocales.map((l) => <option key={l} value={l}>{l}</option>)}
        </select>
        <select
          value={targetLocaleFilter}
          onChange={(e) => { setTargetLocaleFilter(e.target.value); setPage(0); }}
          style={selectStyle}
        >
          <option value="">All target locales</option>
          {allLocales.map((l) => <option key={l} value={l}>{l}</option>)}
        </select>
        <div style={{ flex: 1 }} />
        <button onClick={handleImportCSV} style={toolBtnStyle}>Import CSV</button>
        <button onClick={handleExportJSON} style={toolBtnStyle}>Export JSON</button>
        <button onClick={() => setShowAddForm(!showAddForm)} style={addBtnStyle}>
          + Add Concept
        </button>
      </div>

      {/* Add concept form */}
      {showAddForm && (
        <div style={formStyle}>
          <h4 style={{ margin: "0 0 8px" }}>New Concept</h4>
          <div style={{ display: "flex", gap: 8, marginBottom: 8 }}>
            <input
              placeholder="Domain"
              value={newDomain}
              onChange={(e) => setNewDomain(e.target.value)}
              style={{ ...inputStyle, flex: 1 }}
            />
            <input
              placeholder="Definition"
              value={newDefinition}
              onChange={(e) => setNewDefinition(e.target.value)}
              style={{ ...inputStyle, flex: 2 }}
            />
          </div>
          <div style={{ fontSize: 12, fontWeight: 600, marginBottom: 4 }}>Terms:</div>
          {newTerms.map((term, idx) => (
            <div key={idx} style={{ display: "flex", gap: 6, marginBottom: 4 }}>
              <input
                placeholder="Term text"
                value={term.text}
                onChange={(e) => updateTermRow(newTerms, setNewTerms, idx, "text", e.target.value)}
                style={{ ...inputStyle, flex: 2 }}
              />
              <select
                value={term.locale}
                onChange={(e) => updateTermRow(newTerms, setNewTerms, idx, "locale", e.target.value)}
                style={{ ...selectStyle, flex: 1 }}
              >
                {allLocales.map((l) => <option key={l} value={l}>{l}</option>)}
              </select>
              <select
                value={term.status}
                onChange={(e) => updateTermRow(newTerms, setNewTerms, idx, "status", e.target.value)}
                style={{ ...selectStyle, flex: 1 }}
              >
                {STATUS_OPTIONS.map((s) => <option key={s} value={s}>{s}</option>)}
              </select>
              <button onClick={() => removeTermRow(newTerms, setNewTerms, idx)} style={delBtnStyle}>x</button>
            </div>
          ))}
          <div style={{ display: "flex", gap: 8, marginTop: 8 }}>
            <button onClick={() => addTermRow(newTerms, setNewTerms)} style={toolBtnStyle}>+ Term</button>
            <div style={{ flex: 1 }} />
            <button onClick={() => setShowAddForm(false)} style={toolBtnStyle}>Cancel</button>
            <button onClick={handleAdd} style={addBtnStyle}>Save</button>
          </div>
        </div>
      )}

      {/* Concepts table */}
      <div style={{ border: "1px solid var(--border)", borderRadius: 6, overflow: "hidden" }}>
        <table style={{ width: "100%", borderCollapse: "collapse", fontSize: 13 }}>
          <thead>
            <tr style={{ background: "var(--bg-secondary)", borderBottom: "1px solid var(--border)" }}>
              <th style={thStyle}>Domain</th>
              <th style={thStyle}>Terms</th>
              <th style={thStyle}>Definition</th>
              <th style={{ ...thStyle, width: 100 }}>Actions</th>
            </tr>
          </thead>
          <tbody>
            {concepts.length === 0 && (
              <tr>
                <td colSpan={4} style={{ padding: 24, textAlign: "center", color: "var(--text-secondary)" }}>
                  {totalCount === 0 ? "No concepts yet. Add terms or import a termbase." : "No results found."}
                </td>
              </tr>
            )}
            {concepts.map((concept) => (
              <tr key={concept.id} style={{ borderBottom: "1px solid var(--border)" }}>
                {editingId === concept.id && editConcept ? (
                  <>
                    <td style={tdStyle}>
                      <input
                        value={editConcept.domain}
                        onChange={(e) => setEditConcept({ ...editConcept, domain: e.target.value })}
                        style={{ ...inputStyle, width: "100%" }}
                      />
                    </td>
                    <td style={tdStyle}>
                      {editConcept.terms.map((term, idx) => (
                        <div key={idx} style={{ display: "flex", gap: 4, marginBottom: 2 }}>
                          <input
                            value={term.text}
                            onChange={(e) => {
                              const terms = [...editConcept.terms];
                              terms[idx] = { ...terms[idx], text: e.target.value };
                              setEditConcept({ ...editConcept, terms });
                            }}
                            style={{ ...inputStyle, flex: 2 }}
                          />
                          <select
                            value={term.locale}
                            onChange={(e) => {
                              const terms = [...editConcept.terms];
                              terms[idx] = { ...terms[idx], locale: e.target.value };
                              setEditConcept({ ...editConcept, terms });
                            }}
                            style={{ ...selectStyle, width: 60 }}
                          >
                            {allLocales.map((l) => <option key={l} value={l}>{l}</option>)}
                          </select>
                          <select
                            value={term.status}
                            onChange={(e) => {
                              const terms = [...editConcept.terms];
                              terms[idx] = { ...terms[idx], status: e.target.value };
                              setEditConcept({ ...editConcept, terms });
                            }}
                            style={{ ...selectStyle, width: 80 }}
                          >
                            {STATUS_OPTIONS.map((s) => <option key={s} value={s}>{s}</option>)}
                          </select>
                        </div>
                      ))}
                      <button onClick={() => setEditConcept({
                        ...editConcept,
                        terms: [...editConcept.terms, { text: "", locale: project.source_locale, status: "approved" }],
                      })} style={{ ...toolBtnStyle, fontSize: 11, padding: "1px 6px" }}>+ term</button>
                    </td>
                    <td style={tdStyle}>
                      <input
                        value={editConcept.definition}
                        onChange={(e) => setEditConcept({ ...editConcept, definition: e.target.value })}
                        style={{ ...inputStyle, width: "100%" }}
                      />
                    </td>
                    <td style={tdStyle}>
                      <button onClick={handleSaveEdit} style={saveBtnStyle}>Save</button>
                      <button onClick={() => { setEditingId(null); setEditConcept(null); }} style={toolBtnStyle}>Cancel</button>
                    </td>
                  </>
                ) : (
                  <>
                    <td style={tdStyle}>
                      <span style={{ fontSize: 11, color: "var(--text-secondary)" }}>{concept.domain || "-"}</span>
                    </td>
                    <td style={tdStyle}>
                      {concept.terms.map((term, idx) => (
                        <div key={idx} style={{ marginBottom: 2 }}>
                          <span style={{ fontWeight: term.status === "preferred" ? 600 : 400 }}>{term.text}</span>
                          <span style={{ fontSize: 11, color: "var(--text-secondary)", marginLeft: 4 }}>[{term.locale}]</span>
                          {" "}
                          {statusBadge(term.status)}
                          {term.note && <span style={{ fontSize: 11, color: "var(--text-secondary)", marginLeft: 4 }}>({term.note})</span>}
                        </div>
                      ))}
                    </td>
                    <td style={tdStyle}>
                      <span style={{ fontSize: 12 }}>{concept.definition || "-"}</span>
                    </td>
                    <td style={tdStyle}>
                      <button onClick={() => handleEdit(concept)} style={toolBtnStyle}>Edit</button>
                      <button onClick={() => handleDelete(concept.id)} style={delBtnStyle}>Delete</button>
                    </td>
                  </>
                )}
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {/* Pagination */}
      {totalPages > 1 && (
        <div style={{ display: "flex", justifyContent: "center", gap: 8, marginTop: 12 }}>
          <button onClick={() => setPage(Math.max(0, page - 1))} disabled={page === 0} style={toolBtnStyle}>Prev</button>
          <span style={{ fontSize: 13, lineHeight: "28px" }}>Page {page + 1} of {totalPages}</span>
          <button onClick={() => setPage(Math.min(totalPages - 1, page + 1))} disabled={page >= totalPages - 1} style={toolBtnStyle}>Next</button>
        </div>
      )}
    </div>
  );
}

// --- Styles ---

const backBtnStyle: React.CSSProperties = {
  background: "none",
  border: "none",
  color: "var(--accent)",
  cursor: "pointer",
  fontSize: 14,
  padding: "4px 8px",
};

const inputStyle: React.CSSProperties = {
  padding: "5px 8px",
  border: "1px solid var(--border)",
  borderRadius: 4,
  fontSize: 13,
  background: "var(--bg-primary)",
  color: "var(--text-primary)",
};

const selectStyle: React.CSSProperties = {
  padding: "5px 8px",
  border: "1px solid var(--border)",
  borderRadius: 4,
  fontSize: 13,
  background: "var(--bg-primary)",
  color: "var(--text-primary)",
};

const toolBtnStyle: React.CSSProperties = {
  padding: "4px 10px",
  border: "1px solid var(--border)",
  borderRadius: 4,
  fontSize: 12,
  background: "var(--bg-secondary)",
  color: "var(--text-primary)",
  cursor: "pointer",
};

const addBtnStyle: React.CSSProperties = {
  padding: "4px 12px",
  border: "none",
  borderRadius: 4,
  fontSize: 12,
  fontWeight: 600,
  background: "var(--accent)",
  color: "#fff",
  cursor: "pointer",
};

const saveBtnStyle: React.CSSProperties = {
  padding: "4px 10px",
  border: "none",
  borderRadius: 4,
  fontSize: 12,
  background: "#22c55e",
  color: "#fff",
  cursor: "pointer",
  marginRight: 4,
};

const delBtnStyle: React.CSSProperties = {
  padding: "4px 8px",
  border: "1px solid #f87171",
  borderRadius: 4,
  fontSize: 12,
  background: "transparent",
  color: "#f87171",
  cursor: "pointer",
};

const formStyle: React.CSSProperties = {
  padding: 16,
  border: "1px solid var(--border)",
  borderRadius: 6,
  marginBottom: 16,
  background: "var(--bg-secondary)",
};

const thStyle: React.CSSProperties = {
  textAlign: "left",
  padding: "8px 12px",
  fontSize: 11,
  fontWeight: 600,
  textTransform: "uppercase",
  color: "var(--text-secondary)",
};

const tdStyle: React.CSSProperties = {
  padding: "8px 12px",
  verticalAlign: "top",
};
