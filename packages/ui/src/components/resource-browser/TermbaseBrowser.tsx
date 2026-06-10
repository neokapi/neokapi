import { useState, useEffect, useCallback, useRef, useMemo } from "react";
import { t } from "@neokapi/kapi-react/runtime";
import type { TermbaseAdapter } from "./adapters";
import type { ConceptDTO, TermDTO } from "./types";
import { BulkActionBar } from "./BulkActionBar";
import { ConceptCard } from "./ConceptCard";
import { Pagination } from "./Pagination";
import { FilterBar, type FilterToken, type FilterField, type FilterPreset } from "../ui/filter-bar";
import { LocaleSelect, resolveLocaleName, type LocaleInfo } from "../ui/locale-select";

interface TermbaseBrowserProps {
  adapter: TermbaseAdapter;
  sourceLocale?: string;
  targetLocales?: string[];
  projectId?: string;
  /** Filter fields for the integrated FilterBar. If omitted, a plain search input is shown. */
  filterFields?: FilterField[];
  /** Quick-access filter presets. */
  filterPresets?: FilterPreset[];
  /** Locale list for the add-concept form's locale selectors. If omitted, plain text inputs are used. */
  locales?: LocaleInfo[];
  onError?: (message: string, details?: unknown) => void;
}

const PAGE_SIZE = 50;
const STATUS_OPTIONS = ["preferred", "approved", "admitted", "proposed", "deprecated", "forbidden"];

export function TermbaseBrowser({
  adapter,
  sourceLocale: propSourceLocale = "",
  targetLocales: propTargetLocales = [],
  projectId,
  filterFields,
  filterPresets,
  locales,
  onError,
}: TermbaseBrowserProps) {
  const [concepts, setConcepts] = useState<ConceptDTO[]>([]);
  const [totalCount, setTotalCount] = useState(0);
  const [searchText, setSearchText] = useState("");
  const [debouncedSearch, setDebouncedSearch] = useState("");
  const [filterTokens, setFilterTokens] = useState<FilterToken[]>([]);
  const [page, setPage] = useState(0);
  const [loading, setLoading] = useState(true);
  const [initialLoadDone, setInitialLoadDone] = useState(false);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [editingId, setEditingId] = useState<string | null>(null);
  const [editConcept, setEditConcept] = useState<ConceptDTO | null>(null);
  const [deleteConfirmId, setDeleteConfirmId] = useState<string | null>(null);
  const [showAddForm, setShowAddForm] = useState(false);

  // Add form state
  const [newDomain, setNewDomain] = useState("");
  const [newDefinition, setNewDefinition] = useState("");
  const [newTerms, setNewTerms] = useState<TermDTO[]>([
    { text: "", locale: propSourceLocale, status: "preferred" },
    { text: "", locale: propTargetLocales[0] ?? "", status: "preferred" },
  ]);

  // Merge locales prop with locales found in data so unknown codes are selectable.
  const mergedLocales = useMemo(() => {
    const known = new Map((locales ?? []).map((l) => [l.code, l]));
    for (const c of concepts) {
      for (const t of c.terms ?? []) {
        if (t.locale && !known.has(t.locale)) {
          known.set(t.locale, { code: t.locale, displayName: resolveLocaleName(t.locale) });
        }
      }
    }
    return [...known.values()];
  }, [locales, concepts]);

  const debounceRef = useRef<ReturnType<typeof setTimeout>>(undefined);

  // Derive effective locales from filter tokens, falling back to props.
  const effectiveSourceLocale =
    filterTokens.find((t) => t.key === "locale" || t.key === "source")?.value ?? propSourceLocale;
  const effectiveTargetLocale =
    filterTokens.find((t) => t.key === "target")?.value ?? propTargetLocales[0] ?? "";

  // Handle search from FilterBar (Enter-driven).
  const handleFilterSearchChange = useCallback((val: string) => {
    setSearchText(val);
    setDebouncedSearch(val);
    setPage(0);
  }, []);

  // Use refs to avoid re-creating fetchConcepts when props change identity.
  const adapterRef = useRef(adapter);
  const sourceLocaleRef = useRef(effectiveSourceLocale);
  const targetLocaleRef = useRef(effectiveTargetLocale);
  adapterRef.current = adapter;
  sourceLocaleRef.current = effectiveSourceLocale;
  targetLocaleRef.current = effectiveTargetLocale;

  const fetchConcepts = useCallback(
    async (q: string, p: number) => {
      setLoading(true);
      try {
        const result = await adapterRef.current.search(
          q,
          sourceLocaleRef.current,
          targetLocaleRef.current,
          p * PAGE_SIZE,
          PAGE_SIZE,
        );
        setConcepts(result.concepts ?? []);
        setTotalCount(result.total_count);
      } finally {
        setLoading(false);
        setInitialLoadDone(true);
      }
    },
    [], // stable — reads from refs
  );

  useEffect(() => {
    void fetchConcepts(debouncedSearch, page);
  }, [fetchConcepts, debouncedSearch, page, effectiveSourceLocale, effectiveTargetLocale]);

  const handleSearch = useCallback((e: React.ChangeEvent<HTMLInputElement>) => {
    const val = e.target.value;
    setSearchText(val);
    clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(() => {
      setDebouncedSearch(val);
      setPage(0);
    }, 200);
  }, []);

  const toggleSelect = useCallback((id: string) => {
    setSelected((prev: Set<string>) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }, []);

  const deselectAll = useCallback(() => setSelected(new Set()), []);

  const handleEdit = useCallback((c: ConceptDTO) => {
    setEditingId(c.id);
    setEditConcept({ ...c, terms: [...c.terms] });
  }, []);

  const handleSaveEdit = useCallback(async () => {
    if (!editConcept) return;
    try {
      await adapter.updateConcept({
        concept_id: editConcept.id,
        project_id: editConcept.project_id,
        domain: editConcept.domain,
        definition: editConcept.definition,
        terms: editConcept.terms,
      });
      setEditingId(null);
      setEditConcept(null);
      void fetchConcepts(debouncedSearch, page);
    } catch (err) {
      onError?.("Failed to save concept", err);
    }
  }, [adapter, editConcept, fetchConcepts, debouncedSearch, page, onError]);

  const handleDelete = useCallback(
    async (id: string) => {
      try {
        await adapter.deleteConcept(id);
        setDeleteConfirmId(null);
        setSelected((prev: Set<string>) => {
          const next = new Set(prev);
          next.delete(id);
          return next;
        });
        void fetchConcepts(debouncedSearch, page);
      } catch (err) {
        onError?.("Failed to delete concept", err);
      }
    },
    [adapter, fetchConcepts, debouncedSearch, page, onError],
  );

  const handleBulkDelete = useCallback(async () => {
    try {
      await adapter.deleteConcepts([...selected]);
      setSelected(new Set());
      void fetchConcepts(debouncedSearch, page);
    } catch (err) {
      onError?.("Failed to delete concepts", err);
    }
  }, [adapter, selected, fetchConcepts, debouncedSearch, page, onError]);

  const handleAdd = useCallback(async () => {
    const validTerms = newTerms.filter((t: TermDTO) => t.text.trim());
    if (validTerms.length === 0) return;
    try {
      await adapter.addConcept({
        project_id: projectId,
        domain: newDomain,
        definition: newDefinition,
        terms: validTerms,
      });
      setNewDomain("");
      setNewDefinition("");
      setNewTerms([
        { text: "", locale: propSourceLocale, status: "preferred" },
        { text: "", locale: propTargetLocales[0] ?? "", status: "preferred" },
      ]);
      setShowAddForm(false);
      void fetchConcepts(debouncedSearch, page);
    } catch (err) {
      onError?.("Failed to add concept", err);
    }
  }, [
    adapter,
    projectId,
    newDomain,
    newDefinition,
    newTerms,
    propSourceLocale,
    propTargetLocales,
    fetchConcepts,
    debouncedSearch,
    page,
    onError,
  ]);

  return (
    <div data-testid="termbase-browser">
      {/* Search + Actions */}
      <div className="flex items-center gap-2 mb-4">
        {filterFields ? (
          <div className="flex-1">
            <FilterBar
              filters={filterTokens}
              onFiltersChange={(tokens) => {
                setFilterTokens(tokens);
                setPage(0);
              }}
              search={searchText}
              onSearchChange={handleFilterSearchChange}
              fields={filterFields}
              presets={filterPresets}
              placeholder="Search terminology..."
            />
          </div>
        ) : (
          <div className="relative flex-1">
            <input
              type="text"
              value={searchText}
              onChange={handleSearch}
              placeholder="Search terminology..."
              className="w-full rounded-md border border-input bg-transparent pl-8 pr-3 py-2 text-sm outline-none focus:ring-1 focus:ring-ring"
            />
            <svg
              className="absolute left-2.5 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-muted-foreground"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth={2}
                d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"
              />
            </svg>
          </div>
        )}
        <button
          onClick={() => setShowAddForm(true)}
          className="rounded-md bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground hover:bg-primary/90 whitespace-nowrap"
        >
          Add Concept
        </button>
      </div>

      {/* Count + inline loading indicator for subsequent fetches */}
      <div className="text-[12px] text-muted-foreground mb-3 flex items-center gap-2">
        <span>
          {totalCount} {totalCount === 1 ? "concept" : "concepts"}
        </span>
        {loading && initialLoadDone && (
          <span className="inline-block w-3 h-3 border-2 border-current border-t-transparent rounded-full animate-spin opacity-50" />
        )}
      </div>

      {/* Loading — only on initial load */}
      {loading && !initialLoadDone && (
        <div className="grid grid-cols-1 md:grid-cols-2 gap-2">
          {[0, 1, 2, 3].map((i) => (
            <div key={i} className="rounded-lg border border-border p-4 animate-pulse">
              <div className="h-3 bg-muted rounded w-1/4 mb-3" />
              <div className="h-2.5 bg-muted rounded w-3/4 mb-2" />
              <div className="h-2.5 bg-muted rounded w-1/2" />
            </div>
          ))}
        </div>
      )}

      {/* Empty — only after initial load */}
      {initialLoadDone && !loading && concepts.length === 0 && (
        <div className="py-12 text-center">
          <p className="text-sm text-muted-foreground mb-1">
            {searchText ? t("No concepts match your search.") : t("No concepts yet.")}
          </p>
          {searchText ? (
            <button
              onClick={() => setSearchText("")}
              className="text-xs text-primary hover:text-primary/80"
            >
              Clear search
            </button>
          ) : (
            <button
              onClick={() => setShowAddForm(true)}
              className="text-xs text-primary hover:text-primary/80"
            >
              Add your first concept
            </button>
          )}
        </div>
      )}

      {/* Concept cards */}
      {concepts.length > 0 && (
        <div className="grid grid-cols-1 md:grid-cols-2 gap-2">
          {concepts.map((concept: ConceptDTO) =>
            editingId === concept.id && editConcept ? (
              <div
                key={concept.id}
                className="rounded-lg border border-primary/40 bg-primary/5 p-4"
                data-testid={`concept-${concept.id}`}
              >
                {/* Edit mode */}
                <div className="flex flex-col gap-2">
                  <input
                    type="text"
                    value={editConcept.domain}
                    onChange={(e) => setEditConcept({ ...editConcept, domain: e.target.value })}
                    placeholder="Domain"
                    className="rounded border border-input bg-transparent px-2 py-1 text-[12px] outline-none focus:ring-1 focus:ring-ring"
                  />
                  <input
                    type="text"
                    value={editConcept.definition}
                    onChange={(e) => setEditConcept({ ...editConcept, definition: e.target.value })}
                    placeholder="Definition"
                    className="rounded border border-input bg-transparent px-2 py-1 text-[12px] outline-none focus:ring-1 focus:ring-ring"
                  />
                  {editConcept.terms.map((term: TermDTO, idx: number) => (
                    <div key={idx} className="flex gap-1">
                      <input
                        type="text"
                        value={term.text}
                        onChange={(e: React.ChangeEvent<HTMLInputElement>) => {
                          const terms = [...editConcept.terms];
                          terms[idx] = { ...terms[idx], text: e.target.value };
                          setEditConcept({ ...editConcept, terms });
                        }}
                        className="flex-1 rounded border border-input bg-transparent px-2 py-1 text-[12px] outline-none"
                      />
                      {mergedLocales.length > 0 ? (
                        <div>
                          <LocaleSelect
                            value={term.locale}
                            onChange={(v) => {
                              const terms = [...editConcept.terms];
                              terms[idx] = { ...terms[idx], locale: v };
                              setEditConcept({ ...editConcept, terms });
                            }}
                            locales={mergedLocales}
                            placeholder="Locale"
                            compact
                          />
                        </div>
                      ) : (
                        <input
                          type="text"
                          value={term.locale}
                          onChange={(e) => {
                            const terms = [...editConcept.terms];
                            terms[idx] = { ...terms[idx], locale: e.target.value };
                            setEditConcept({ ...editConcept, terms });
                          }}
                          className="w-16 rounded border border-input bg-transparent px-2 py-1 text-[12px] outline-none"
                        />
                      )}
                      <select
                        value={term.status}
                        onChange={(e) => {
                          const terms = [...editConcept.terms];
                          terms[idx] = { ...terms[idx], status: e.target.value };
                          setEditConcept({ ...editConcept, terms });
                        }}
                        className="rounded border border-input bg-transparent px-1 py-1 text-[11px] outline-none"
                      >
                        {STATUS_OPTIONS.map((s) => (
                          <option key={s} value={s}>
                            {s}
                          </option>
                        ))}
                      </select>
                    </div>
                  ))}
                  <div className="flex gap-1 mt-1">
                    <button
                      onClick={() => void handleSaveEdit()}
                      className="text-[11px] text-primary"
                    >
                      Save
                    </button>
                    <button
                      onClick={() => {
                        setEditingId(null);
                        setEditConcept(null);
                      }}
                      className="text-[11px] text-muted-foreground"
                    >
                      Cancel
                    </button>
                  </div>
                </div>
              </div>
            ) : (
              /* Display mode */
              <ConceptCard
                key={concept.id}
                concept={concept}
                referenceLocale={effectiveSourceLocale || propSourceLocale || undefined}
                selected={selected.has(concept.id)}
                onToggleSelect={() => toggleSelect(concept.id)}
                onEdit={() => handleEdit(concept)}
                onDelete={() => setDeleteConfirmId(concept.id)}
                deleteConfirm={deleteConfirmId === concept.id}
                onDeleteConfirm={() => void handleDelete(concept.id)}
                onDeleteCancel={() => setDeleteConfirmId(null)}
              />
            ),
          )}
        </div>
      )}

      <Pagination page={page} pageSize={PAGE_SIZE} totalCount={totalCount} onPageChange={setPage} />

      <BulkActionBar
        selectedCount={selected.size}
        onDelete={handleBulkDelete}
        onDeselectAll={deselectAll}
      />

      {/* Add concept dialog */}
      {showAddForm && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40">
          <div className="w-full max-w-lg rounded-xl border border-border bg-card p-6 shadow-lg">
            <h3 className="text-base font-semibold mb-4">New Concept</h3>
            <div className="flex flex-col gap-3">
              <div className="flex gap-3">
                <div className="flex-1">
                  <label className="text-[12px] text-muted-foreground block mb-1">Domain</label>
                  <input
                    type="text"
                    value={newDomain}
                    onChange={(e) => setNewDomain(e.target.value)}
                    placeholder="e.g. Legal, Medical"
                    className="w-full rounded-md border border-input bg-transparent px-3 py-2 text-sm outline-none focus:ring-1 focus:ring-ring"
                    autoFocus
                  />
                </div>
                <div className="flex-[2]">
                  <label className="text-[12px] text-muted-foreground block mb-1">Definition</label>
                  <input
                    type="text"
                    value={newDefinition}
                    onChange={(e) => setNewDefinition(e.target.value)}
                    placeholder="Concept definition"
                    className="w-full rounded-md border border-input bg-transparent px-3 py-2 text-sm outline-none focus:ring-1 focus:ring-ring"
                  />
                </div>
              </div>
              <div>
                <label className="text-[12px] text-muted-foreground block mb-1">Terms</label>
                {newTerms.map((term: TermDTO, idx: number) => (
                  <div key={idx} className="flex gap-1.5 mb-1">
                    <input
                      type="text"
                      value={term.text}
                      onChange={(e: React.ChangeEvent<HTMLInputElement>) => {
                        const t = [...newTerms];
                        t[idx] = { ...t[idx], text: e.target.value };
                        setNewTerms(t);
                      }}
                      placeholder="Term"
                      className="flex-[2] rounded border border-input bg-transparent px-2 py-1.5 text-sm outline-none"
                    />
                    {mergedLocales.length > 0 ? (
                      <div>
                        <LocaleSelect
                          value={term.locale}
                          onChange={(v) => {
                            const t = [...newTerms];
                            t[idx] = { ...t[idx], locale: v };
                            setNewTerms(t);
                          }}
                          compact
                          locales={mergedLocales}
                          placeholder="Locale"
                        />
                      </div>
                    ) : (
                      <input
                        type="text"
                        value={term.locale}
                        onChange={(e) => {
                          const t = [...newTerms];
                          t[idx] = { ...t[idx], locale: e.target.value };
                          setNewTerms(t);
                        }}
                        placeholder="Locale"
                        className="w-20 rounded border border-input bg-transparent px-2 py-1.5 text-sm outline-none"
                      />
                    )}
                    <select
                      value={term.status}
                      onChange={(e) => {
                        const t = [...newTerms];
                        t[idx] = { ...t[idx], status: e.target.value };
                        setNewTerms(t);
                      }}
                      className="rounded border border-input bg-transparent px-1 py-1.5 text-xs outline-none"
                    >
                      {STATUS_OPTIONS.map((s) => (
                        <option key={s} value={s}>
                          {s}
                        </option>
                      ))}
                    </select>
                    {newTerms.length > 1 && (
                      <button
                        onClick={() =>
                          setNewTerms(newTerms.filter((_: TermDTO, i: number) => i !== idx))
                        }
                        className="text-xs text-muted-foreground hover:text-destructive px-1"
                      >
                        x
                      </button>
                    )}
                  </div>
                ))}
                <button
                  onClick={() =>
                    setNewTerms([...newTerms, { text: "", locale: "", status: "approved" }])
                  }
                  className="text-xs text-primary hover:text-primary/80 mt-1"
                >
                  + Add term
                </button>
              </div>
            </div>
            <div className="flex gap-2 mt-4 pt-3 border-t border-border">
              <button
                onClick={() => void handleAdd()}
                disabled={newTerms.every((t: TermDTO) => !t.text.trim())}
                className="rounded-md bg-primary px-4 py-1.5 text-xs font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
              >
                Save
              </button>
              <button
                onClick={() => setShowAddForm(false)}
                className="rounded-md border border-border px-4 py-1.5 text-xs hover:bg-accent transition-colors"
              >
                Cancel
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
