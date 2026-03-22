import { useState, useEffect, useCallback, useMemo } from "react";
import { useTermsApi } from "../../hooks/useTermsApi";
import { useGraphApi } from "../../hooks/useGraphApi";
import { useLocales } from "../../hooks/useLocales";
import { useSetBreadcrumb } from "../../context/BreadcrumbContext";
import type { ConceptInfo, TermInfo, ConceptHierarchyNode, GraphEdge, GraphNode } from "../../types/api";
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
import { ConceptGraph } from "./ConceptGraph";
import { ConceptDetailPanel } from "./ConceptDetailPanel";
import { cn } from "../../lib/utils";

interface ConceptExplorerProps {
  sourceLocale: string;
  targetLocales: string[];
  projectId?: string;
  projectName?: string;
  projects?: { id: string; name: string }[];
  onBack: () => void;
}

type ViewMode = "list" | "graph";

const PAGE_SIZE = 50;
const STATUS_OPTIONS = ["preferred", "approved", "admitted", "proposed", "deprecated", "forbidden"];

export function ConceptExplorer({
  sourceLocale,
  targetLocales,
  projectId,
  projectName,
  projects,
  onBack,
}: ConceptExplorerProps) {
  const { getDisplayName } = useLocales();

  const breadcrumbNode = useMemo(
    () => (
      <button
        onClick={onBack}
        data-testid="concept-back-btn"
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

  const { getConceptHierarchy, getGraphEdges } = useGraphApi();

  // View state
  const [viewMode, setViewMode] = useState<ViewMode>("list");
  const [concepts, setConcepts] = useState<ConceptInfo[]>([]);
  const [totalCount, setTotalCount] = useState(0);
  const [filters, setFilters] = useState<FilterToken[]>([]);
  const [searchText, setSearchText] = useState("");
  const [page, setPage] = useState(0);
  const [selectedConcept, setSelectedConcept] = useState<ConceptInfo | null>(null);

  // Graph state
  const [hierarchy, setHierarchy] = useState<ConceptHierarchyNode[]>([]);
  const [graphEdges, setGraphEdges] = useState<GraphEdge[]>([]);
  const [detailEdges, setDetailEdges] = useState<GraphEdge[]>([]);
  const [detailNeighbors, setDetailNeighbors] = useState<GraphNode[]>([]);

  // Form state
  const [showAddForm, setShowAddForm] = useState(false);
  const [editingConcept, setEditingConcept] = useState<ConceptInfo | null>(null);
  const [deleteConfirmId, setDeleteConfirmId] = useState<string | null>(null);
  const [successMessage, setSuccessMessage] = useState<string | null>(null);

  const [newProjectScoped, setNewProjectScoped] = useState(false);
  const [newDomain, setNewDomain] = useState("");
  const [newDefinition, setNewDefinition] = useState("");
  const [newTerms, setNewTerms] = useState<TermInfo[]>([
    { text: "", locale: sourceLocale, status: "preferred" },
    { text: "", locale: targetLocales[0] || "", status: "preferred" },
  ]);

  // Derived filter values
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

  // ── Data fetching ──────────────────────────────────────────────────────

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

  // Load graph data when switching to graph view
  useEffect(() => {
    if (viewMode !== "graph") return;
    void (async () => {
      try {
        const h = await getConceptHierarchy();
        setHierarchy(h || []);
      } catch {
        // Graph store may not be configured
        setHierarchy([]);
      }
    })();
  }, [viewMode, getConceptHierarchy]);

  // Load edges for all visible concepts in graph mode
  useEffect(() => {
    if (viewMode !== "graph" || concepts.length === 0) return;
    void (async () => {
      const allEdges: GraphEdge[] = [];
      const seen = new Set<string>();
      for (const c of concepts.slice(0, 30)) {
        try {
          const edges = await getGraphEdges(c.id);
          for (const e of edges) {
            if (!seen.has(e.id)) {
              seen.add(e.id);
              allEdges.push(e);
            }
          }
        } catch {
          // skip
        }
      }
      setGraphEdges(allEdges);
    })();
  }, [viewMode, concepts, getGraphEdges]);

  // Load edges for selected concept (detail panel)
  useEffect(() => {
    if (!selectedConcept) {
      setDetailEdges([]);
      setDetailNeighbors([]);
      return;
    }
    void (async () => {
      try {
        const edges = await getGraphEdges(selectedConcept.id);
        setDetailEdges(edges || []);
      } catch {
        setDetailEdges([]);
      }
    })();
  }, [selectedConcept, getGraphEdges]);

  // ── CRUD handlers ──────────────────────────────────────────────────────

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

  const handleSaveEdit = useCallback(async () => {
    if (!editingConcept) return;
    try {
      await updateConcept({
        project_id: editingConcept.project_id ?? "",
        concept_id: editingConcept.id,
        domain: editingConcept.domain,
        definition: editingConcept.definition,
        terms: editingConcept.terms,
      });
      setEditingConcept(null);
      // If selected concept was being edited, update it
      if (selectedConcept?.id === editingConcept.id) {
        setSelectedConcept(editingConcept);
      }
      void fetchConcepts(query, sourceLocaleFilter, targetLocaleFilter, page);
    } catch (e) {
      console.error("Failed to update concept:", e);
    }
  }, [updateConcept, editingConcept, selectedConcept, fetchConcepts, query, sourceLocaleFilter, targetLocaleFilter, page]);

  const handleDelete = useCallback(
    async (conceptId: string) => {
      try {
        await deleteConcept(conceptId);
        setDeleteConfirmId(null);
        if (selectedConcept?.id === conceptId) setSelectedConcept(null);
        void fetchConcepts(query, sourceLocaleFilter, targetLocaleFilter, page);
      } catch (e) {
        console.error("Failed to delete concept:", e);
      }
    },
    [deleteConcept, selectedConcept, fetchConcepts, query, sourceLocaleFilter, targetLocaleFilter, page],
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

  // ── Concept selection & navigation ─────────────────────────────────────

  const handleSelectConcept = useCallback((concept: ConceptInfo) => {
    setSelectedConcept(concept);
  }, []);

  const handleNavigateConcept = useCallback(
    (conceptId: string) => {
      const concept = concepts.find((c) => c.id === conceptId);
      if (concept) {
        setSelectedConcept(concept);
      }
    },
    [concepts],
  );

  const handleEditFromPanel = useCallback((concept: ConceptInfo) => {
    setEditingConcept({ ...concept, terms: [...concept.terms] });
  }, []);

  // ── Rendering helpers ──────────────────────────────────────────────────

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
    <div data-testid="concept-explorer" className="flex h-full">
      {/* Main content area */}
      <div className="flex-1 flex flex-col min-w-0">
        <Card className="flex-1 flex flex-col p-0 overflow-hidden">
          {/* Header */}
          <div className="flex items-center justify-between p-6 pb-0">
            <div>
              <h2 className="text-xl font-semibold">Concepts</h2>
              <p className="text-[13px] text-muted-foreground mt-1" data-testid="concept-count-badge">
                {totalCount} {totalCount === 1 ? "concept" : "concepts"}
              </p>
            </div>
            <div className="flex gap-2 items-center">
              {/* View mode toggle */}
              <div className="flex border border-border rounded-md overflow-hidden">
                <button
                  onClick={() => setViewMode("list")}
                  className={cn(
                    "px-3 py-1.5 text-xs font-medium transition-colors",
                    viewMode === "list"
                      ? "bg-primary text-primary-foreground"
                      : "hover:bg-accent text-muted-foreground",
                  )}
                >
                  List
                </button>
                <button
                  onClick={() => setViewMode("graph")}
                  className={cn(
                    "px-3 py-1.5 text-xs font-medium transition-colors",
                    viewMode === "graph"
                      ? "bg-primary text-primary-foreground"
                      : "hover:bg-accent text-muted-foreground",
                  )}
                >
                  Graph
                </button>
              </div>
              <Button variant="ghost" size="sm" onClick={handleImportCSV}>
                Import CSV
              </Button>
              <Button variant="ghost" size="sm" onClick={handleImportJSON}>
                Import JSON
              </Button>
              <Button variant="ghost" size="sm" onClick={handleExportJSON}>
                Export JSON
              </Button>
              <Button size="sm" onClick={() => setShowAddForm(true)}>
                + Add Concept
              </Button>
            </div>
          </div>

          {/* Success message */}
          {successMessage && (
            <div className="px-6 pt-4">
              <Alert className="border-green-200 text-green-800 dark:border-green-800 dark:text-green-400">
                <AlertDescription>{successMessage}</AlertDescription>
              </Alert>
            </div>
          )}

          {/* Search and filters */}
          <div className="px-6 pt-4 pb-2" data-testid="concept-search-input">
            <FilterBar
              filters={filters}
              onFiltersChange={handleFiltersChange}
              search={searchText}
              onSearchChange={handleSearchChange}
              fields={filterFields}
              presets={filterPresets}
              placeholder="Search concepts..."
            />
          </div>

          {/* Content area */}
          {concepts.length === 0 ? (
            <div className="flex-1 flex items-center justify-center">
              <div className="text-center">
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
            </div>
          ) : viewMode === "list" ? (
            /* ── List view ───────────────────────────────────────────── */
            <div className="flex-1 overflow-auto px-6 pb-6">
              <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-3 mt-2">
                {concepts.map((concept) => {
                  const preferred =
                    concept.terms.find((t) => t.status === "preferred") ?? concept.terms[0];
                  const isSelected = selectedConcept?.id === concept.id;

                  return (
                    <button
                      key={concept.id}
                      onClick={() => handleSelectConcept(concept)}
                      data-testid={`concept-card-${concept.id}`}
                      className={cn(
                        "text-left rounded-lg border bg-card p-4 transition-all hover:shadow-md hover:border-primary/40 cursor-pointer",
                        isSelected && "ring-2 ring-primary border-primary shadow-md",
                      )}
                    >
                      {/* Domain */}
                      {concept.domain && (
                        <span className="text-[10px] uppercase tracking-wider text-muted-foreground font-medium">
                          {concept.domain}
                        </span>
                      )}

                      {/* Preferred term */}
                      <div className="text-sm font-semibold mt-0.5 leading-tight">
                        {preferred?.text ?? "Untitled"}
                      </div>

                      {/* Definition preview */}
                      {concept.definition && (
                        <p className="text-[12px] text-muted-foreground mt-1 line-clamp-2 leading-snug">
                          {concept.definition}
                        </p>
                      )}

                      {/* Terms preview */}
                      <div className="mt-2 flex flex-wrap gap-1">
                        {concept.terms.slice(0, 4).map((term, i) => (
                          <span key={i} className="inline-flex items-center gap-0.5">
                            <span
                              className={cn(
                                "text-[11px]",
                                term.status === "preferred" && "font-semibold",
                              )}
                            >
                              {term.text}
                            </span>
                            <span className="text-[10px] text-muted-foreground">
                              [{getDisplayName(term.locale)}]
                            </span>
                            {statusBadge(term.status)}
                          </span>
                        ))}
                        {concept.terms.length > 4 && (
                          <span className="text-[10px] text-muted-foreground">
                            +{concept.terms.length - 4} more
                          </span>
                        )}
                      </div>

                      {/* Scope & date */}
                      <div className="flex items-center gap-2 mt-2">
                        <Badge
                          variant={concept.project_id ? "default" : "secondary"}
                          className="text-[9px] px-1 py-0 h-3.5"
                        >
                          {concept.project_id ? "Project" : "Workspace"}
                        </Badge>
                        <span className="text-[10px] text-muted-foreground">
                          {new Date(concept.updated_at || concept.created_at).toLocaleDateString()}
                        </span>
                      </div>
                    </button>
                  );
                })}
              </div>

              {/* Pagination */}
              {totalPages > 1 && (
                <div className="flex items-center justify-between mt-6 pt-6 border-t border-border">
                  <span className="text-sm text-muted-foreground">
                    Showing {page * PAGE_SIZE + 1} to{" "}
                    {Math.min((page + 1) * PAGE_SIZE, totalCount)} of {totalCount}
                  </span>
                  <div className="flex items-center gap-2">
                    <Button
                      size="sm"
                      variant="ghost"
                      onClick={() => setPage(Math.max(0, page - 1))}
                      disabled={page === 0}
                    >
                      Previous
                    </Button>
                    <span className="text-sm text-muted-foreground">
                      Page {page + 1} of {totalPages}
                    </span>
                    <Button
                      size="sm"
                      variant="ghost"
                      onClick={() => setPage(Math.min(totalPages - 1, page + 1))}
                      disabled={page >= totalPages - 1}
                    >
                      Next
                    </Button>
                  </div>
                </div>
              )}
            </div>
          ) : (
            /* ── Graph view ──────────────────────────────────────────── */
            <div className="flex-1 min-h-0">
              <ConceptGraph
                concepts={concepts}
                hierarchy={hierarchy}
                graphEdges={graphEdges}
                selectedId={selectedConcept?.id ?? null}
                onSelectConcept={handleSelectConcept}
                onNavigateConcept={handleNavigateConcept}
              />
            </div>
          )}
        </Card>
      </div>

      {/* Detail panel (slides in from right) */}
      {selectedConcept && (
        <ConceptDetailPanel
          concept={selectedConcept}
          edges={detailEdges}
          neighbors={detailNeighbors}
          allConcepts={concepts}
          onClose={() => setSelectedConcept(null)}
          onEdit={handleEditFromPanel}
          onDelete={handleDelete}
          onNavigate={handleNavigateConcept}
        />
      )}

      {/* ── Add Concept Dialog ─────────────────────────────────────── */}
      <Dialog open={showAddForm} onOpenChange={handleAddDialogChange}>
        <DialogContent
          className="sm:max-w-[640px]"
          data-testid="concept-add-form"
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
            <Button variant="outline" onClick={() => handleAddDialogChange(false)}>
              Cancel
            </Button>
            <Button
              onClick={handleAdd}
              disabled={newTerms.every((t) => !t.text.trim())}
            >
              Save
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* ── Edit Concept Dialog ────────────────────────────────────── */}
      <Dialog open={!!editingConcept} onOpenChange={(open) => !open && setEditingConcept(null)}>
        <DialogContent
          className="sm:max-w-[640px]"
          onInteractOutside={(e: Event) => e.preventDefault()}
        >
          <DialogHeader>
            <DialogTitle>Edit Concept</DialogTitle>
          </DialogHeader>
          {editingConcept && (
            <div className="flex flex-col gap-4 py-2">
              <div className="flex gap-3">
                <div className="flex-1">
                  <Label className="text-muted-foreground">Domain</Label>
                  <Input
                    value={editingConcept.domain}
                    onChange={(e) =>
                      setEditingConcept({ ...editingConcept, domain: e.target.value })
                    }
                    className="mt-1"
                  />
                </div>
                <div className="flex-[2]">
                  <Label className="text-muted-foreground">Definition</Label>
                  <Input
                    value={editingConcept.definition}
                    onChange={(e) =>
                      setEditingConcept({ ...editingConcept, definition: e.target.value })
                    }
                    className="mt-1"
                  />
                </div>
              </div>
              <div>
                <Label className="text-muted-foreground mb-2 block">Terms</Label>
                <div className="flex flex-col gap-1.5">
                  {editingConcept.terms.map((term, idx) => (
                    <div key={idx} className="flex gap-1.5 items-start">
                      <Input
                        value={term.text}
                        onChange={(e) => {
                          const terms = [...editingConcept.terms];
                          terms[idx] = { ...terms[idx], text: e.target.value };
                          setEditingConcept({ ...editingConcept, terms });
                        }}
                        className="flex-[2]"
                      />
                      <select
                        value={term.locale}
                        onChange={(e) => {
                          const terms = [...editingConcept.terms];
                          terms[idx] = { ...terms[idx], locale: e.target.value };
                          setEditingConcept({ ...editingConcept, terms });
                        }}
                        className="w-[80px] px-1 py-1.5 border border-input rounded-md bg-transparent text-foreground text-xs"
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
                          const terms = [...editingConcept.terms];
                          terms[idx] = { ...terms[idx], status: e.target.value };
                          setEditingConcept({ ...editingConcept, terms });
                        }}
                        className="w-[100px] px-1 py-1.5 border border-input rounded-md bg-transparent text-foreground text-xs"
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
                        onClick={() => {
                          setEditingConcept({
                            ...editingConcept,
                            terms: editingConcept.terms.filter((_, i) => i !== idx),
                          });
                        }}
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
                  onClick={() =>
                    setEditingConcept({
                      ...editingConcept,
                      terms: [
                        ...editingConcept.terms,
                        { text: "", locale: sourceLocale, status: "approved" },
                      ],
                    })
                  }
                >
                  + Term
                </Button>
              </div>
            </div>
          )}
          <DialogFooter>
            <Button variant="outline" onClick={() => setEditingConcept(null)}>
              Cancel
            </Button>
            <Button onClick={handleSaveEdit}>Save</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
