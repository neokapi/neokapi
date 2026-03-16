import { useState, useEffect, useCallback, useRef, useMemo } from "react";
import { useTermsApi } from "../../hooks/useTermsApi";
import { useLocales } from "../../hooks/useLocales";
import { useSetBreadcrumb } from "../../context/BreadcrumbContext";
import type { ConceptInfo, TermInfo } from "../../types/api";
import type { FilterToken, FilterField, FilterPreset } from "../FilterBar";
import { FilterBar } from "../FilterBar";
import { Button } from "../ui/button";
import { Input } from "../ui/input";
import { Label } from "../ui/label";
import { Badge } from "../ui/badge";
import { Card } from "../ui/card";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from "../ui/dialog";
import { Alert, AlertDescription } from "../ui/alert";
import { LocaleSelect } from "../LocaleSelect";
import { ArrowLeft } from "../icons";
import { cn } from "../../lib/utils";

interface TermExplorerProps {
  sourceLocale: string;
  targetLocales: string[];
  projectId?: string;
  projectName?: string;
  /** Projects in the workspace — used for project filter suggestions */
  projects?: { id: string; name: string }[];
  onBack: () => void;
}

const PAGE_SIZE = 50;
const STATUS_OPTIONS = ["preferred", "approved", "admitted", "proposed", "deprecated", "forbidden"];

export function TermExplorer({
  sourceLocale,
  targetLocales,
  projectId,
  projectName,
  projects,
  onBack,
}: TermExplorerProps) {
  const { getDisplayName } = useLocales();

  const breadcrumbNode = useMemo(
    () => (
      <button
        onClick={onBack}
        data-testid="term-back-btn"
        className="flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground transition-colors cursor-pointer bg-transparent border-none p-0"
      >
        <ArrowLeft className="w-3.5 h-3.5" /> Back
      </button>
    ),
    [onBack],
  );
  useSetBreadcrumb(breadcrumbNode);
  const {
    getTerms,
    addConcept,
    updateConcept,
    deleteConcept,
    importTermsCSV,
    importTermsJSON,
    exportTermsJSON,
  } = useTermsApi();
  const [concepts, setConcepts] = useState<ConceptInfo[]>([]);
  const [totalCount, setTotalCount] = useState(0);
  const [filters, setFilters] = useState<FilterToken[]>([]);
  const [searchText, setSearchText] = useState("");
  const [page, setPage] = useState(0);

  // Derive filter values from tokens
  const query = searchText;
  const sourceLocaleFilter = filters.find((f) => f.key === "source")?.value ?? "";
  const targetLocaleFilter = filters.find((f) => f.key === "target")?.value ?? "";

  const allLocales = useMemo(() => {
    const set = new Set([sourceLocale, ...targetLocales].filter(Boolean));
    return [...set];
  }, [sourceLocale, targetLocales]);

  const filterFields = useMemo<FilterField[]>(() => {
    const fields: FilterField[] = [
      {
        key: "source",
        label: "Source locale",
        hint: "filter by source language",
        values: allLocales.map((l) => ({ value: l, label: getDisplayName(l) })),
      },
      {
        key: "target",
        label: "Target locale",
        hint: "filter by target language",
        values: allLocales.map((l) => ({ value: l, label: getDisplayName(l) })),
      },
      {
        key: "status",
        label: "Status",
        hint: "filter by term status",
        values: STATUS_OPTIONS.map((s) => ({ value: s, label: s })),
      },
    ];
    if (projects && projects.length > 0) {
      fields.unshift({
        key: "project",
        label: "Project",
        hint: "filter by project",
        values: projects.map((p) => ({ value: p.id, label: p.name })),
      });
    }
    return fields;
  }, [allLocales, getDisplayName, projects]);

  const filterPresets = useMemo<FilterPreset[]>(() => {
    const presets: FilterPreset[] = [];
    if (sourceLocale) {
      presets.push({
        label: "Source: " + getDisplayName(sourceLocale),
        filters: [{ key: "source", value: sourceLocale }],
      });
    }
    for (const t of targetLocales.slice(0, 3)) {
      presets.push({
        label: "Target: " + getDisplayName(t),
        filters: [{ key: "target", value: t }],
      });
    }
    presets.push({ label: "Deprecated terms", filters: [{ key: "status", value: "deprecated" }] });
    return presets;
  }, [sourceLocale, targetLocales, getDisplayName]);

  const handleFiltersChange = useCallback((newFilters: FilterToken[]) => {
    setFilters(newFilters);
    setPage(0);
  }, []);

  const handleSearchChange = useCallback((newSearch: string) => {
    setSearchText(newSearch);
    setPage(0);
  }, []);
  const [showAddForm, setShowAddForm] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [editConcept, setEditConcept] = useState<ConceptInfo | null>(null);
  const [deleteConfirmId, setDeleteConfirmId] = useState<string | null>(null);
  const [successMessage, setSuccessMessage] = useState<string | null>(null);

  const [newProjectScoped, setNewProjectScoped] = useState(false);
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
    void fetchConcepts(query, sourceLocaleFilter, targetLocaleFilter, page);
  }, [fetchConcepts, query, sourceLocaleFilter, targetLocaleFilter, page]);

  // (query/filter changes handled by FilterBar callbacks above)

  const handleAdd = useCallback(async () => {
    const validTerms = newTerms.filter((t) => t.text.trim() !== "");
    if (validTerms.length === 0) return;
    try {
      await addConcept({
        project_id: newProjectScoped && projectId ? projectId : "",
        domain: newDomain,
        definition: newDefinition,
        terms: validTerms,
      });
      setNewProjectScoped(false);
      setNewDomain("");
      setNewDefinition("");
      setNewTerms([
        { text: "", locale: sourceLocale, status: "preferred" },
        { text: "", locale: targetLocales[0] || "", status: "preferred" },
      ]);
      setShowAddForm(false);
      void fetchConcepts(query, sourceLocaleFilter, targetLocaleFilter, page);
    } catch (e) {
      console.error("Failed to add concept:", e);
    }
  }, [
    addConcept,
    newProjectScoped,
    projectId,
    newDomain,
    newDefinition,
    newTerms,
    sourceLocale,
    targetLocales,
    fetchConcepts,
    query,
    sourceLocaleFilter,
    targetLocaleFilter,
    page,
  ]);

  const handleAddDialogChange = useCallback(
    (open: boolean) => {
      if (!open) {
        setNewProjectScoped(false);
        setNewDomain("");
        setNewDefinition("");
        setNewTerms([
          { text: "", locale: sourceLocale, status: "preferred" },
          { text: "", locale: targetLocales[0] || "", status: "preferred" },
        ]);
      }
      setShowAddForm(open);
    },
    [sourceLocale, targetLocales],
  );

  const handleEdit = useCallback((concept: ConceptInfo) => {
    setEditingId(concept.id);
    setEditConcept({ ...concept, terms: [...concept.terms] });
  }, []);

  const handleSaveEdit = useCallback(async () => {
    if (!editConcept) return;
    try {
      await updateConcept({
        project_id: editConcept.project_id ?? "",
        concept_id: editConcept.id,
        domain: editConcept.domain,
        definition: editConcept.definition,
        terms: editConcept.terms,
      });
      setEditingId(null);
      setEditConcept(null);
      void fetchConcepts(query, sourceLocaleFilter, targetLocaleFilter, page);
    } catch (e) {
      console.error("Failed to update concept:", e);
    }
  }, [
    updateConcept,
    editConcept,
    fetchConcepts,
    query,
    sourceLocaleFilter,
    targetLocaleFilter,
    page,
  ]);

  const handleDelete = useCallback(
    async (conceptId: string) => {
      try {
        await deleteConcept(conceptId);
        setDeleteConfirmId(null);
        void fetchConcepts(query, sourceLocaleFilter, targetLocaleFilter, page);
      } catch (e) {
        console.error("Failed to delete concept:", e);
      }
    },
    [deleteConcept, fetchConcepts, query, sourceLocaleFilter, targetLocaleFilter, page],
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
        const count = await importTermsCSV(content, sourceLocale, targetLocales[0] || "", "", true);
        setSuccessMessage(`Imported ${count} concepts`);
        void fetchConcepts(query, sourceLocaleFilter, targetLocaleFilter, page);
      } catch (e) {
        console.error("CSV import failed:", e);
      }
    };
    input.click();
  }, [
    importTermsCSV,
    sourceLocale,
    targetLocales,
    fetchConcepts,
    query,
    sourceLocaleFilter,
    targetLocaleFilter,
    page,
  ]);

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
        setSuccessMessage(`Imported ${count} concepts`);
        void fetchConcepts(query, sourceLocaleFilter, targetLocaleFilter, page);
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
      preferred: "default",
      approved: "default",
      admitted: "secondary",
      proposed: "outline",
      deprecated: "destructive",
      forbidden: "destructive",
    };
    return (
      <Badge variant={variants[status] || "secondary"} className="text-[10px] px-1.5 py-0">
        {status}
      </Badge>
    );
  };

  const addTermRow = (terms: TermInfo[], setter: (t: TermInfo[]) => void) =>
    setter([...terms, { text: "", locale: "", status: "approved" }]);
  const removeTermRow = (terms: TermInfo[], setter: (t: TermInfo[]) => void, idx: number) =>
    setter(terms.filter((_, i) => i !== idx));
  const updateTermRow = (
    terms: TermInfo[],
    setter: (t: TermInfo[]) => void,
    idx: number,
    field: keyof TermInfo,
    value: string,
  ) => {
    const updated = [...terms];
    updated[idx] = { ...updated[idx], [field]: value };
    setter(updated);
  };

  const totalPages = Math.ceil(totalCount / PAGE_SIZE);

  return (
    <div data-testid="term-explorer">
      <Card className="p-6">
        {/* Header */}
        <div className="flex items-center justify-between mb-6">
          <div>
            <h2 className="text-xl font-semibold">Terminology</h2>
            <p className="text-[13px] text-muted-foreground mt-1" data-testid="term-count-badge">
              {totalCount} {totalCount === 1 ? "concept" : "concepts"}
            </p>
          </div>
          <div className="flex gap-2">
            <Button
              variant="ghost"
              size="sm"
              onClick={handleImportCSV}
              data-testid="term-import-csv-btn"
            >
              Import CSV
            </Button>
            <Button
              variant="ghost"
              size="sm"
              onClick={handleImportJSON}
              data-testid="term-import-json-btn"
            >
              Import JSON
            </Button>
            <Button
              variant="ghost"
              size="sm"
              onClick={handleExportJSON}
              data-testid="term-export-json-btn"
            >
              Export JSON
            </Button>
            <Button size="sm" onClick={() => setShowAddForm(true)} data-testid="term-add-btn">
              + Add Concept
            </Button>
          </div>
        </div>

        {/* Success message */}
        {successMessage && (
          <Alert className="mb-6 border-green-200 text-green-800 dark:border-green-800 dark:text-green-400">
            <AlertDescription>{successMessage}</AlertDescription>
          </Alert>
        )}

        {/* Search and filters */}
        <div className="mb-6" data-testid="term-search-input">
          <FilterBar
            filters={filters}
            onFiltersChange={handleFiltersChange}
            search={searchText}
            onSearchChange={handleSearchChange}
            fields={filterFields}
            presets={filterPresets}
            placeholder="Search terminology..."
          />
        </div>

        {/* Table */}
        {concepts.length === 0 ? (
          <div className="py-12 text-center" data-testid="term-empty-state">
            {totalCount === 0 ? (
              <>
                <div className="text-[15px] font-semibold text-foreground mb-2">
                  No concepts yet
                </div>
                <div className="text-[13px] text-muted-foreground mb-4">
                  Add terms manually or import from a CSV or JSON termbase file.
                </div>
                <div className="flex gap-2 justify-center">
                  <Button size="sm" onClick={() => setShowAddForm(true)}>
                    + Add Concept
                  </Button>
                  <Button variant="ghost" size="sm" onClick={handleImportCSV}>
                    Import CSV
                  </Button>
                  <Button variant="ghost" size="sm" onClick={handleImportJSON}>
                    Import JSON
                  </Button>
                </div>
              </>
            ) : (
              <>
                <p className="text-[13px] text-muted-foreground mb-2">
                  No results match your search.
                </p>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => {
                    setFilters([]);
                    setSearchText("");
                  }}
                >
                  Clear filters
                </Button>
              </>
            )}
          </div>
        ) : (
          <>
            <div className="overflow-x-auto">
              <table className="w-full border-collapse text-[13px]">
                <thead>
                  <tr className="border-b border-border">
                    <th className="px-3 py-2.5 text-left text-sm font-medium text-muted-foreground">
                      Domain
                    </th>
                    <th className="px-3 py-2.5 text-left text-sm font-medium text-muted-foreground">
                      Terms
                    </th>
                    <th className="px-3 py-2.5 text-left text-sm font-medium text-muted-foreground">
                      Definition
                    </th>
                    <th className="px-3 py-2.5 text-sm font-medium text-muted-foreground w-[100px]">
                      Actions
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {concepts.map((concept) => (
                    <tr
                      key={concept.id}
                      className="border-b border-border/50 transition-colors hover:bg-accent/50"
                      data-testid={`term-concept-${concept.id}`}
                    >
                      {editingId === concept.id && editConcept ? (
                        <>
                          <td className="px-3 py-2 align-top">
                            <Input
                              value={editConcept.domain}
                              onChange={(e) =>
                                setEditConcept({ ...editConcept, domain: e.target.value })
                              }
                              className="w-full"
                            />
                          </td>
                          <td className="px-3 py-2 align-top">
                            {editConcept.terms.map((term, idx) => (
                              <div key={idx} className="flex gap-1 mb-0.5">
                                <Input
                                  value={term.text}
                                  onChange={(e) => {
                                    const terms = [...editConcept.terms];
                                    terms[idx] = { ...terms[idx], text: e.target.value };
                                    setEditConcept({ ...editConcept, terms });
                                  }}
                                  className="flex-[2]"
                                />
                                <select
                                  value={term.locale}
                                  onChange={(e) => {
                                    const terms = [...editConcept.terms];
                                    terms[idx] = { ...terms[idx], locale: e.target.value };
                                    setEditConcept({ ...editConcept, terms });
                                  }}
                                  className="w-[60px] px-1 py-1 border border-input rounded-md bg-transparent text-foreground text-xs"
                                >
                                  {allLocales.map((l) => (
                                    <option key={l} value={l}>
                                      {getDisplayName(l)} ({l})
                                    </option>
                                  ))}
                                </select>
                                <select
                                  value={term.status}
                                  onChange={(e) => {
                                    const terms = [...editConcept.terms];
                                    terms[idx] = { ...terms[idx], status: e.target.value };
                                    setEditConcept({ ...editConcept, terms });
                                  }}
                                  className="w-[80px] px-1 py-1 border border-input rounded-md bg-transparent text-foreground text-xs"
                                >
                                  {STATUS_OPTIONS.map((s) => (
                                    <option key={s} value={s}>
                                      {s}
                                    </option>
                                  ))}
                                </select>
                              </div>
                            ))}
                            <Button
                              variant="outline"
                              size="sm"
                              className="text-[11px] px-1.5 py-0 h-6"
                              onClick={() =>
                                setEditConcept({
                                  ...editConcept,
                                  terms: [
                                    ...editConcept.terms,
                                    { text: "", locale: sourceLocale, status: "approved" },
                                  ],
                                })
                              }
                            >
                              + term
                            </Button>
                          </td>
                          <td className="px-3 py-2 align-top">
                            <Input
                              value={editConcept.definition}
                              onChange={(e) =>
                                setEditConcept({ ...editConcept, definition: e.target.value })
                              }
                              className="w-full"
                            />
                          </td>
                          <td className="px-3 py-2 align-top">
                            <div className="flex gap-1">
                              <Button
                                size="sm"
                                onClick={handleSaveEdit}
                                data-testid={`term-save-btn-${concept.id}`}
                              >
                                Save
                              </Button>
                              <Button
                                variant="outline"
                                size="sm"
                                onClick={() => {
                                  setEditingId(null);
                                  setEditConcept(null);
                                }}
                              >
                                Cancel
                              </Button>
                            </div>
                          </td>
                        </>
                      ) : (
                        <>
                          <td className="px-3 py-2 align-top">
                            <span className="text-[11px] text-muted-foreground">
                              {concept.domain || "-"}
                            </span>
                            {concept.project_id ? (
                              <span className="block text-[10px] mt-0.5 px-1.5 py-px rounded bg-blue-500/10 text-blue-600 dark:text-blue-400 w-fit">
                                Project
                              </span>
                            ) : (
                              <span className="block text-[10px] mt-0.5 px-1.5 py-px rounded bg-muted text-muted-foreground w-fit">
                                Workspace
                              </span>
                            )}
                          </td>
                          <td className="px-3 py-2 align-top">
                            {concept.terms.map((term, idx) => (
                              <div key={idx} className="mb-0.5">
                                <span
                                  className={term.status === "preferred" ? "font-semibold" : ""}
                                >
                                  {term.text}
                                </span>
                                <span className="text-[11px] text-muted-foreground ml-1">
                                  [{getDisplayName(term.locale)}]
                                </span>{" "}
                                {statusBadge(term.status)}
                                {term.note && (
                                  <span className="text-[11px] text-muted-foreground ml-1">
                                    ({term.note})
                                  </span>
                                )}
                              </div>
                            ))}
                          </td>
                          <td className="px-3 py-2 align-top">
                            <span className="text-xs">{concept.definition || "-"}</span>
                          </td>
                          <td className="px-3 py-2 align-top">
                            <div className="flex gap-1">
                              <Button
                                variant="ghost"
                                size="sm"
                                onClick={() => handleEdit(concept)}
                                data-testid={`term-edit-btn-${concept.id}`}
                              >
                                Edit
                              </Button>
                              {deleteConfirmId === concept.id ? (
                                <span className="inline-flex gap-1 ml-1">
                                  <Button
                                    variant="destructive"
                                    size="sm"
                                    onClick={() => handleDelete(concept.id)}
                                    data-testid={`term-confirm-delete-${concept.id}`}
                                  >
                                    Confirm
                                  </Button>
                                  <Button
                                    variant="outline"
                                    size="sm"
                                    onClick={() => setDeleteConfirmId(null)}
                                  >
                                    Cancel
                                  </Button>
                                </span>
                              ) : (
                                <Button
                                  variant="ghost"
                                  size="sm"
                                  className="text-destructive hover:text-destructive"
                                  onClick={() => setDeleteConfirmId(concept.id)}
                                  data-testid={`term-delete-btn-${concept.id}`}
                                >
                                  Delete
                                </Button>
                              )}
                            </div>
                          </td>
                        </>
                      )}
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>

            {totalPages > 1 && (
              <div
                className="flex items-center justify-between mt-6 pt-6 border-t border-border"
                data-testid="term-pagination"
              >
                <span className="text-sm text-muted-foreground">
                  Showing {page * PAGE_SIZE + 1} to {Math.min((page + 1) * PAGE_SIZE, totalCount)}{" "}
                  of {totalCount}
                </span>
                <div className="flex items-center gap-2">
                  <Button
                    size="sm"
                    variant="ghost"
                    onClick={() => setPage(Math.max(0, page - 1))}
                    disabled={page === 0}
                    data-testid="term-prev-page"
                  >
                    Previous
                  </Button>
                  <span className="text-sm text-muted-foreground" data-testid="term-page-info">
                    Page {page + 1} of {totalPages}
                  </span>
                  <Button
                    size="sm"
                    variant="ghost"
                    onClick={() => setPage(Math.min(totalPages - 1, page + 1))}
                    disabled={page >= totalPages - 1}
                    data-testid="term-next-page"
                  >
                    Next
                  </Button>
                </div>
              </div>
            )}
          </>
        )}
      </Card>

      <Dialog open={showAddForm} onOpenChange={handleAddDialogChange}>
        <DialogContent
          className="sm:max-w-[640px]"
          data-testid="term-add-form"
          onInteractOutside={(e: Event) => e.preventDefault()}
        >
          <DialogHeader>
            <DialogTitle>New Concept</DialogTitle>
          </DialogHeader>
          <div className="flex flex-col gap-4 py-2">
            {projectId && (
              <div className="flex gap-2 items-center">
                <Label className="text-muted-foreground">Scope</Label>
                <div className="flex gap-2">
                  <button
                    type="button"
                    className={cn(
                      "text-xs px-2.5 py-1 rounded border transition-colors",
                      !newProjectScoped
                        ? "bg-primary text-primary-foreground border-primary"
                        : "border-input hover:bg-accent",
                    )}
                    onClick={() => setNewProjectScoped(false)}
                  >
                    Workspace
                  </button>
                  <button
                    type="button"
                    className={cn(
                      "text-xs px-2.5 py-1 rounded border transition-colors",
                      newProjectScoped
                        ? "bg-primary text-primary-foreground border-primary"
                        : "border-input hover:bg-accent",
                    )}
                    onClick={() => setNewProjectScoped(true)}
                  >
                    Project
                  </button>
                </div>
              </div>
            )}
            <div className="flex gap-3">
              <div className="flex-1">
                <Label className="text-muted-foreground">Domain</Label>
                <Input
                  placeholder="e.g. Legal, Medical"
                  value={newDomain}
                  onChange={(e) => setNewDomain(e.target.value)}
                  className="mt-1"
                  data-testid="term-add-domain"
                  autoFocus
                />
              </div>
              <div className="flex-[2]">
                <Label className="text-muted-foreground">Definition</Label>
                <Input
                  placeholder="Concept definition"
                  value={newDefinition}
                  onChange={(e) => setNewDefinition(e.target.value)}
                  className="mt-1"
                  data-testid="term-add-definition"
                />
              </div>
            </div>
            <div>
              <Label className="text-muted-foreground mb-2 block">Terms</Label>
              <div className="flex flex-col gap-1.5">
                {newTerms.map((term, idx) => (
                  <div key={idx} className="flex gap-1.5 items-start">
                    <Input
                      placeholder="Term text"
                      value={term.text}
                      onChange={(e) =>
                        updateTermRow(newTerms, setNewTerms, idx, "text", e.target.value)
                      }
                      className="flex-[2]"
                    />
                    <LocaleSelect
                      value={term.locale}
                      onChange={(v) => updateTermRow(newTerms, setNewTerms, idx, "locale", v)}
                      codes={allLocales}
                      className="flex-1"
                    />
                    <select
                      value={term.status}
                      onChange={(e) =>
                        updateTermRow(newTerms, setNewTerms, idx, "status", e.target.value)
                      }
                      className="flex-1 px-2 py-1.5 border border-input rounded-md bg-transparent text-foreground text-sm"
                    >
                      {STATUS_OPTIONS.map((s) => (
                        <option key={s} value={s}>
                          {s}
                        </option>
                      ))}
                    </select>
                    <Button
                      variant="ghost"
                      size="sm"
                      className="text-destructive hover:text-destructive h-9 w-9 p-0"
                      onClick={() => removeTermRow(newTerms, setNewTerms, idx)}
                    >
                      x
                    </Button>
                  </div>
                ))}
              </div>
              <Button
                variant="outline"
                size="sm"
                className="mt-2"
                onClick={() => addTermRow(newTerms, setNewTerms)}
              >
                + Term
              </Button>
            </div>
          </div>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => handleAddDialogChange(false)}
              data-testid="term-add-cancel"
            >
              Cancel
            </Button>
            <Button
              onClick={handleAdd}
              disabled={newTerms.every((t) => !t.text.trim())}
              data-testid="term-add-submit"
            >
              Save
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
