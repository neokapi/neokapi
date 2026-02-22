import { useState, useEffect, useCallback, useRef, useMemo } from "react";
import { useTermsApi } from "../../hooks/useTermsApi";
import { useLocales } from "../../hooks/useLocales";
import { useSetBreadcrumb } from "../../context/BreadcrumbContext";
import type { ConceptInfo, TermInfo } from "../../types/api";
import { Button } from "../ui/button";
import { Input } from "../ui/input";
import { Badge } from "../ui/badge";
import { CardContent, GlassCard } from "../ui/card";
import { ArrowLeft } from "../icons";

interface TermExplorerProps {
  sourceLocale: string;
  targetLocales: string[];
  projectName?: string;
  onBack: () => void;
}

const PAGE_SIZE = 50;
const STATUS_OPTIONS = ["preferred", "approved", "admitted", "proposed", "deprecated", "forbidden"];

export function TermExplorer({ sourceLocale, targetLocales, projectName, onBack }: TermExplorerProps) {
  const { getDisplayName } = useLocales();

  const breadcrumbNode = useMemo(() => (
    <Button variant="outline" size="sm" onClick={onBack} data-testid="term-back-btn">
      <ArrowLeft className="w-3.5 h-3.5 mr-1" /> Back
    </Button>
  ), [onBack]);
  useSetBreadcrumb(breadcrumbNode);
  const {
    getTerms, addConcept, updateConcept, deleteConcept,
    importTermsCSV, importTermsJSON, exportTermsJSON,
  } = useTermsApi();
  const [concepts, setConcepts] = useState<ConceptInfo[]>([]);
  const [totalCount, setTotalCount] = useState(0);
  const [query, setQuery] = useState("");
  const [sourceLocaleFilter, setSourceLocaleFilter] = useState("");
  const [targetLocaleFilter, setTargetLocaleFilter] = useState("");
  const [page, setPage] = useState(0);
  const [showAddForm, setShowAddForm] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [editConcept, setEditConcept] = useState<ConceptInfo | null>(null);
  const [deleteConfirmId, setDeleteConfirmId] = useState<string | null>(null);
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const [newDomain, setNewDomain] = useState("");
  const [newDefinition, setNewDefinition] = useState("");
  const [newTerms, setNewTerms] = useState<TermInfo[]>([
    { text: "", locale: sourceLocale, status: "preferred" },
    { text: "", locale: targetLocales[0] || "", status: "preferred" },
  ]);

  const fetchConcepts = useCallback(
    async (q: string, srcLocale: string, tgtLocale: string, p: number) => {
      try {
        const result = await getTerms(q, srcLocale, tgtLocale, p * PAGE_SIZE, PAGE_SIZE);
        setConcepts(result.concepts || []);
        setTotalCount(result.total_count);
      } catch (e) {
        console.error("Failed to fetch terms:", e);
      }
    },
    [getTerms],
  );

  useEffect(() => {
    fetchConcepts(query, sourceLocaleFilter, targetLocaleFilter, page);
  }, [fetchConcepts, query, sourceLocaleFilter, targetLocaleFilter, page]);

  const handleQueryChange = useCallback((value: string) => {
    if (debounceRef.current) clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(() => { setQuery(value); setPage(0); }, 300);
  }, []);

  const handleAdd = useCallback(async () => {
    const validTerms = newTerms.filter(t => t.text.trim() !== "");
    if (validTerms.length === 0) return;
    try {
      await addConcept({
        project_id: "",
        domain: newDomain,
        definition: newDefinition,
        terms: validTerms,
      });
      setNewDomain("");
      setNewDefinition("");
      setNewTerms([
        { text: "", locale: sourceLocale, status: "preferred" },
        { text: "", locale: targetLocales[0] || "", status: "preferred" },
      ]);
      setShowAddForm(false);
      fetchConcepts(query, sourceLocaleFilter, targetLocaleFilter, page);
    } catch (e) {
      console.error("Failed to add concept:", e);
    }
  }, [addConcept, newDomain, newDefinition, newTerms, sourceLocale, targetLocales, fetchConcepts, query, sourceLocaleFilter, targetLocaleFilter, page]);

  const handleEdit = useCallback((concept: ConceptInfo) => {
    setEditingId(concept.id);
    setEditConcept({ ...concept, terms: [...concept.terms] });
  }, []);

  const handleSaveEdit = useCallback(async () => {
    if (!editConcept) return;
    try {
      await updateConcept({
        project_id: "",
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
  }, [updateConcept, editConcept, fetchConcepts, query, sourceLocaleFilter, targetLocaleFilter, page]);

  const handleDelete = useCallback(async (conceptId: string) => {
    try {
      await deleteConcept(conceptId);
      setDeleteConfirmId(null);
      fetchConcepts(query, sourceLocaleFilter, targetLocaleFilter, page);
    } catch (e) {
      console.error("Failed to delete concept:", e);
    }
  }, [deleteConcept, fetchConcepts, query, sourceLocaleFilter, targetLocaleFilter, page]);

  const handleImportCSV = useCallback(async () => {
    const input = document.createElement("input");
    input.type = "file";
    input.accept = ".csv,.tsv";
    input.onchange = async () => {
      const file = input.files?.[0];
      if (!file) return;
      const content = await file.text();
      try {
        const count = await importTermsCSV(content, sourceLocale, targetLocales[0] || "", "", true);
        alert(`Imported ${count} concepts`);
        fetchConcepts(query, sourceLocaleFilter, targetLocaleFilter, page);
      } catch (e) {
        console.error("CSV import failed:", e);
      }
    };
    input.click();
  }, [importTermsCSV, sourceLocale, targetLocales, fetchConcepts, query, sourceLocaleFilter, targetLocaleFilter, page]);

  const handleImportJSON = useCallback(async () => {
    const input = document.createElement("input");
    input.type = "file";
    input.accept = ".json";
    input.onchange = async () => {
      const file = input.files?.[0];
      if (!file) return;
      const content = await file.text();
      try {
        const count = await importTermsJSON(content);
        alert(`Imported ${count} concepts`);
        fetchConcepts(query, sourceLocaleFilter, targetLocaleFilter, page);
      } catch (e) {
        console.error("JSON import failed:", e);
      }
    };
    input.click();
  }, [importTermsJSON, fetchConcepts, query, sourceLocaleFilter, targetLocaleFilter, page]);

  const handleExportJSON = useCallback(async () => {
    try {
      const json = await exportTermsJSON(projectName || "termbase");
      const blob = new Blob([json], { type: "application/json" });
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = `${projectName || "termbase"}-termbase.json`;
      a.click();
      URL.revokeObjectURL(url);
    } catch (e) {
      console.error("Export failed:", e);
    }
  }, [exportTermsJSON, projectName]);

  const statusBadge = (status: string) => {
    const variants: Record<string, "default" | "secondary" | "destructive" | "outline"> = {
      preferred: "default", approved: "default", admitted: "secondary",
      proposed: "outline", deprecated: "destructive", forbidden: "destructive",
    };
    return <Badge variant={variants[status] || "secondary"} className="text-[10px] px-1.5 py-0">{status}</Badge>;
  };

  const addTermRow = (terms: TermInfo[], setter: (t: TermInfo[]) => void) => setter([...terms, { text: "", locale: "", status: "approved" }]);
  const removeTermRow = (terms: TermInfo[], setter: (t: TermInfo[]) => void, idx: number) => setter(terms.filter((_, i) => i !== idx));
  const updateTermRow = (terms: TermInfo[], setter: (t: TermInfo[]) => void, idx: number, field: keyof TermInfo, value: string) => {
    const updated = [...terms];
    updated[idx] = { ...updated[idx], [field]: value };
    setter(updated);
  };

  const allLocales = [sourceLocale, ...targetLocales];
  const totalPages = Math.ceil(totalCount / PAGE_SIZE);

  return (
    <div data-testid="term-explorer">
      <div className="flex items-center gap-3 mb-6">
        <h2 className="flex-1 text-xl font-semibold">Terminology</h2>
        <Badge variant="secondary" data-testid="term-count-badge">{totalCount} concepts</Badge>
      </div>

      <div className="flex gap-2 mb-4 flex-wrap">
        <Input type="text" placeholder="Search terms..." defaultValue={query} onChange={(e) => handleQueryChange(e.target.value)} className="flex-1" data-testid="term-search-input" />
        <select value={sourceLocaleFilter} onChange={(e) => { setSourceLocaleFilter(e.target.value); setPage(0); }} className="px-3 py-2 border border-input rounded-md bg-transparent text-foreground text-sm" data-testid="term-source-locale-filter">
          <option value="">All source locales</option>
          {allLocales.map((l) => <option key={l} value={l}>{getDisplayName(l)} ({l})</option>)}
        </select>
        <select value={targetLocaleFilter} onChange={(e) => { setTargetLocaleFilter(e.target.value); setPage(0); }} className="px-3 py-2 border border-input rounded-md bg-transparent text-foreground text-sm" data-testid="term-target-locale-filter">
          <option value="">All target locales</option>
          {allLocales.map((l) => <option key={l} value={l}>{getDisplayName(l)} ({l})</option>)}
        </select>
        <div className="flex-1" />
        <Button variant="outline" size="sm" onClick={handleImportCSV} data-testid="term-import-csv-btn">Import CSV</Button>
        <Button variant="outline" size="sm" onClick={handleImportJSON} data-testid="term-import-json-btn">Import JSON</Button>
        <Button variant="outline" size="sm" onClick={handleExportJSON} data-testid="term-export-json-btn">Export JSON</Button>
        <Button size="sm" onClick={() => setShowAddForm(!showAddForm)} data-testid="term-add-btn">+ Add Concept</Button>
      </div>

      {showAddForm && (
        <GlassCard intensity="subtle" className="mb-4" data-testid="term-add-form"><CardContent className="p-4">
          <h4 className="font-semibold mb-2">New Concept</h4>
          <div className="flex gap-2 mb-2">
            <Input placeholder="Domain" value={newDomain} onChange={(e) => setNewDomain(e.target.value)} className="flex-1" data-testid="term-add-domain" />
            <Input placeholder="Definition" value={newDefinition} onChange={(e) => setNewDefinition(e.target.value)} className="flex-[2]" data-testid="term-add-definition" />
          </div>
          <div className="text-xs font-semibold mb-1">Terms:</div>
          {newTerms.map((term, idx) => (
            <div key={idx} className="flex gap-1.5 mb-1">
              <Input placeholder="Term text" value={term.text} onChange={(e) => updateTermRow(newTerms, setNewTerms, idx, "text", e.target.value)} className="flex-[2]" />
              <select value={term.locale} onChange={(e) => updateTermRow(newTerms, setNewTerms, idx, "locale", e.target.value)} className="flex-1 px-2 py-1 border border-input rounded-md bg-transparent text-foreground text-sm">
                {allLocales.map((l) => <option key={l} value={l}>{getDisplayName(l)} ({l})</option>)}
              </select>
              <select value={term.status} onChange={(e) => updateTermRow(newTerms, setNewTerms, idx, "status", e.target.value)} className="flex-1 px-2 py-1 border border-input rounded-md bg-transparent text-foreground text-sm">
                {STATUS_OPTIONS.map((s) => <option key={s} value={s}>{s}</option>)}
              </select>
              <Button variant="destructive" size="sm" onClick={() => removeTermRow(newTerms, setNewTerms, idx)}>x</Button>
            </div>
          ))}
          <div className="flex gap-2 mt-2">
            <Button variant="outline" size="sm" onClick={() => addTermRow(newTerms, setNewTerms)}>+ Term</Button>
            <div className="flex-1" />
            <Button variant="outline" size="sm" onClick={() => setShowAddForm(false)} data-testid="term-add-cancel">Cancel</Button>
            <Button size="sm" onClick={handleAdd} data-testid="term-add-submit">Save</Button>
          </div>
        </CardContent></GlassCard>
      )}

      <GlassCard intensity="subtle" className="overflow-hidden">
        <table className="w-full border-collapse text-[13px]">
          <thead>
            <tr>
              <th className="px-3 py-2 text-left text-xs font-semibold text-muted-foreground border-b border-border uppercase tracking-wider">Domain</th>
              <th className="px-3 py-2 text-left text-xs font-semibold text-muted-foreground border-b border-border uppercase tracking-wider">Terms</th>
              <th className="px-3 py-2 text-left text-xs font-semibold text-muted-foreground border-b border-border uppercase tracking-wider">Definition</th>
              <th className="px-3 py-2 text-left text-xs font-semibold text-muted-foreground border-b border-border uppercase tracking-wider w-[100px]">Actions</th>
            </tr>
          </thead>
          <tbody>
            {concepts.length === 0 && (
              <tr>
                <td colSpan={4} className="p-8 text-center">
                  {totalCount === 0 ? (
                    <div data-testid="term-empty-state">
                      <div className="text-[15px] font-semibold text-foreground mb-2">No concepts yet</div>
                      <div className="text-[13px] text-muted-foreground mb-4">
                        Add terms manually or import from a CSV or JSON termbase file.
                      </div>
                      <div className="flex gap-2 justify-center">
                        <Button size="sm" onClick={() => setShowAddForm(true)}>+ Add Concept</Button>
                        <Button variant="outline" size="sm" onClick={handleImportCSV}>Import CSV</Button>
                        <Button variant="outline" size="sm" onClick={handleImportJSON}>Import JSON</Button>
                      </div>
                    </div>
                  ) : (
                    <div className="text-[13px] text-muted-foreground">No results match your search.</div>
                  )}
                </td>
              </tr>
            )}
            {concepts.map((concept) => (
              <tr key={concept.id} className="border-b border-border transition-colors hover:bg-accent/50" data-testid={`term-concept-${concept.id}`}>
                {editingId === concept.id && editConcept ? (
                  <>
                    <td className="px-3 py-2 align-top"><Input value={editConcept.domain} onChange={(e) => setEditConcept({ ...editConcept, domain: e.target.value })} className="w-full" /></td>
                    <td className="px-3 py-2 align-top">
                      {editConcept.terms.map((term, idx) => (
                        <div key={idx} className="flex gap-1 mb-0.5">
                          <Input value={term.text} onChange={(e) => { const terms = [...editConcept.terms]; terms[idx] = { ...terms[idx], text: e.target.value }; setEditConcept({ ...editConcept, terms }); }} className="flex-[2]" />
                          <select value={term.locale} onChange={(e) => { const terms = [...editConcept.terms]; terms[idx] = { ...terms[idx], locale: e.target.value }; setEditConcept({ ...editConcept, terms }); }} className="w-[60px] px-1 py-1 border border-input rounded-md bg-transparent text-foreground text-xs">
                            {allLocales.map((l) => <option key={l} value={l}>{getDisplayName(l)} ({l})</option>)}
                          </select>
                          <select value={term.status} onChange={(e) => { const terms = [...editConcept.terms]; terms[idx] = { ...terms[idx], status: e.target.value }; setEditConcept({ ...editConcept, terms }); }} className="w-[80px] px-1 py-1 border border-input rounded-md bg-transparent text-foreground text-xs">
                            {STATUS_OPTIONS.map((s) => <option key={s} value={s}>{s}</option>)}
                          </select>
                        </div>
                      ))}
                      <Button variant="outline" size="sm" className="text-[11px] px-1.5 py-0 h-6" onClick={() => setEditConcept({ ...editConcept, terms: [...editConcept.terms, { text: "", locale: sourceLocale, status: "approved" }] })}>+ term</Button>
                    </td>
                    <td className="px-3 py-2 align-top"><Input value={editConcept.definition} onChange={(e) => setEditConcept({ ...editConcept, definition: e.target.value })} className="w-full" /></td>
                    <td className="px-3 py-2 align-top">
                      <div className="flex gap-1">
                        <Button size="sm" onClick={handleSaveEdit} data-testid={`term-save-btn-${concept.id}`}>Save</Button>
                        <Button variant="outline" size="sm" onClick={() => { setEditingId(null); setEditConcept(null); }}>Cancel</Button>
                      </div>
                    </td>
                  </>
                ) : (
                  <>
                    <td className="px-3 py-2 align-top"><span className="text-[11px] text-muted-foreground">{concept.domain || "-"}</span></td>
                    <td className="px-3 py-2 align-top">
                      {concept.terms.map((term, idx) => (
                        <div key={idx} className="mb-0.5">
                          <span className={term.status === "preferred" ? "font-semibold" : ""}>{term.text}</span>
                          <span className="text-[11px] text-muted-foreground ml-1">[{getDisplayName(term.locale)}]</span>
                          {" "}{statusBadge(term.status)}
                          {term.note && <span className="text-[11px] text-muted-foreground ml-1">({term.note})</span>}
                        </div>
                      ))}
                    </td>
                    <td className="px-3 py-2 align-top"><span className="text-xs">{concept.definition || "-"}</span></td>
                    <td className="px-3 py-2 align-top">
                      <div className="flex gap-1">
                        <Button variant="outline" size="sm" onClick={() => handleEdit(concept)} data-testid={`term-edit-btn-${concept.id}`}>Edit</Button>
                        {deleteConfirmId === concept.id ? (
                          <span className="inline-flex gap-1 ml-1">
                            <Button variant="destructive" size="sm" onClick={() => handleDelete(concept.id)} data-testid={`term-confirm-delete-${concept.id}`}>Confirm</Button>
                            <Button variant="outline" size="sm" onClick={() => setDeleteConfirmId(null)}>Cancel</Button>
                          </span>
                        ) : (
                          <Button variant="outline" size="sm" className="text-destructive border-destructive hover:bg-destructive/10" onClick={() => setDeleteConfirmId(concept.id)} data-testid={`term-delete-btn-${concept.id}`}>Delete</Button>
                        )}
                      </div>
                    </td>
                  </>
                )}
              </tr>
            ))}
          </tbody>
        </table>

        {totalPages > 1 && (
          <div className="flex items-center justify-center gap-4 py-3 text-[13px] text-muted-foreground border-t border-border" data-testid="term-pagination">
          <Button size="sm" variant="outline" onClick={() => setPage(Math.max(0, page - 1))} disabled={page === 0} data-testid="term-prev-page">Previous</Button>
          <span data-testid="term-page-info">Page {page + 1} of {totalPages}</span>
          <Button size="sm" variant="outline" onClick={() => setPage(Math.min(totalPages - 1, page + 1))} disabled={page >= totalPages - 1} data-testid="term-next-page">Next</Button>
          </div>
        )}
      </GlassCard>
    </div>
  );
}
