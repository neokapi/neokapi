import { useState, useEffect, useCallback } from "react";
import { Workflow, Plus, Play, Trash2, X, Save, Copy, Lock, Import } from "lucide-react";
import { api } from "../hooks/useApi";
import { useError } from "./ErrorBanner";
import { FlowPage } from "./FlowPage";
import type { FlowSpec, FlowInfo } from "../types/api";

interface FlowsPageProps {
  /** Project tab ID — if provided, flows are scoped to the project. */
  tabID?: string;
  /** Project flows map — used for project-mode flow data. */
  projectFlows?: Record<string, FlowSpec>;
  /** Called when a project flow is modified. */
  onFlowChange?: (name: string, spec: FlowSpec) => void;
  /** Called when a flow is deleted from the project. */
  onFlowDelete?: (name: string) => void;
}

interface FlowListItem {
  id: string;
  name: string;
  description: string;
  source: string; // "built-in" | "user" | "project"
  stepCount: number;
}

export function FlowsPage({
  tabID,
  projectFlows,
  onFlowChange,
  onFlowDelete,
}: FlowsPageProps) {
  const [flows, setFlows] = useState<FlowListItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [selectedSpec, setSelectedSpec] = useState<FlowSpec | null>(null);
  const [selectedSource, setSelectedSource] = useState<string>("user");
  const [showCreateDialog, setShowCreateDialog] = useState(false);
  const [showImportDialog, setShowImportDialog] = useState(false);
  const [importFlows, setImportFlows] = useState<FlowListItem[]>([]);
  const [newFlowName, setNewFlowName] = useState("");
  const [deleteConfirm, setDeleteConfirm] = useState<string | null>(null);

  const { showError } = useError();
  const isProjectMode = !!tabID;

  const refreshFlows = useCallback(async () => {
    setLoading(true);
    try {
      if (isProjectMode) {
        // Project mode: list project flows.
        const result = await api.listFlows(tabID);
        setFlows(
          (result ?? []).map((f) => ({
            id: f.name,
            name: f.name,
            description: f.description,
            source: "project",
            stepCount: f.step_count,
          })),
        );
      } else {
        // Ad-hoc mode: list built-in + user flows.
        const result = await api.listUserFlows();
        setFlows(
          (result ?? []).map((f) => ({
            id: f.id,
            name: f.name,
            description: f.description,
            source: f.source,
            stepCount: f.step_count,
          })),
        );
      }
    } catch (err) {
      showError("Failed to load flows", err);
    } finally {
      setLoading(false);
    }
  }, [tabID, isProjectMode, showError]);

  useEffect(() => {
    void refreshFlows();
  }, [refreshFlows]);

  const handleOpenFlow = useCallback(
    async (item: FlowListItem) => {
      try {
        if (isProjectMode) {
          const spec = await api.getFlow(tabID!, item.id);
          if (spec) {
            setSelectedId(item.id);
            setSelectedSpec(spec as FlowSpec);
            setSelectedSource("project");
          }
        } else {
          const detail = await api.getUserFlow(item.id);
          if (detail) {
            setSelectedId(item.id);
            setSelectedSpec({ description: detail.description, steps: detail.steps as FlowSpec["steps"] });
            setSelectedSource(detail.source);
          }
        }
      } catch (err) {
        showError("Failed to open flow", err);
      }
    },
    [tabID, isProjectMode, showError],
  );

  const handleFlowChange = useCallback(
    async (spec: FlowSpec) => {
      if (!selectedId) return;
      setSelectedSpec(spec);
      try {
        if (isProjectMode) {
          await api.saveFlow(tabID!, selectedId, spec);
          onFlowChange?.(selectedId, spec);
        } else if (selectedSource === "user") {
          await api.saveUserFlow({
            id: selectedId,
            name: selectedId,
            description: spec.description ?? "",
            steps: spec.steps,
          });
        }
        // Built-in flows are read-only — changes aren't saved.
      } catch (err) {
        showError("Failed to save flow", err);
      }
    },
    [selectedId, selectedSource, tabID, isProjectMode, onFlowChange, showError],
  );

  const handleSaveProject = useCallback(async () => {
    if (!tabID) return;
    try {
      await api.saveProject(tabID);
    } catch (err) {
      showError("Failed to save project", err);
    }
  }, [tabID, showError]);

  const handleCopyBuiltIn = useCallback(
    async (item: FlowListItem) => {
      const newName = `my-${item.id}`;
      try {
        if (isProjectMode) {
          // Copy to project flows.
          const detail = await api.getUserFlow(item.id);
          if (detail) {
            const spec: FlowSpec = { description: detail.description, steps: detail.steps as FlowSpec["steps"] };
            await api.saveFlow(tabID!, newName, spec);
            onFlowChange?.(newName, spec);
            setSelectedId(newName);
            setSelectedSpec(spec);
            setSelectedSource("project");
          }
        } else {
          const newID = await api.copyBuiltInFlow(item.id, newName);
          if (newID) {
            const detail = await api.getUserFlow(newID);
            if (detail) {
              setSelectedId(newID);
              setSelectedSpec({ description: detail.description, steps: detail.steps as FlowSpec["steps"] });
              setSelectedSource("user");
            }
          }
        }
        void refreshFlows();
      } catch (err) {
        showError("Failed to copy flow", err);
      }
    },
    [tabID, isProjectMode, onFlowChange, refreshFlows, showError],
  );

  const handleOpenImportDialog = useCallback(async () => {
    try {
      const all = await api.listUserFlows();
      // Show built-in + user flows (everything available outside this project).
      setImportFlows(
        (all ?? []).map((f) => ({
          id: f.id,
          name: f.name,
          description: f.description,
          source: f.source,
          stepCount: f.step_count,
        })),
      );
      setShowImportDialog(true);
    } catch (err) {
      showError("Failed to load available flows", err);
    }
  }, [showError]);

  const handleImportFlow = useCallback(
    async (item: FlowListItem) => {
      if (!tabID) return;
      try {
        const detail = await api.getUserFlow(item.id);
        if (detail) {
          const name = item.id;
          const spec: FlowSpec = { description: detail.description, steps: detail.steps as FlowSpec["steps"] };
          await api.saveFlow(tabID, name, spec);
          onFlowChange?.(name, spec);
          setShowImportDialog(false);
          setSelectedId(name);
          setSelectedSpec(spec);
          setSelectedSource("project");
          void refreshFlows();
        }
      } catch (err) {
        showError("Failed to import flow", err);
      }
    },
    [tabID, onFlowChange, refreshFlows, showError],
  );

  const handleCreateFlow = useCallback(async () => {
    const name = newFlowName.trim().replace(/\s+/g, "-").toLowerCase();
    if (!name) return;
    const spec: FlowSpec = { steps: [] };
    try {
      if (isProjectMode) {
        await api.saveFlow(tabID!, name, spec);
        onFlowChange?.(name, spec);
      } else {
        await api.saveUserFlow({ id: name, name, description: "", steps: [] });
      }
      setNewFlowName("");
      setShowCreateDialog(false);
      setSelectedId(name);
      setSelectedSpec(spec);
      setSelectedSource(isProjectMode ? "project" : "user");
      void refreshFlows();
    } catch (err) {
      showError("Failed to create flow", err);
    }
  }, [newFlowName, tabID, isProjectMode, onFlowChange, refreshFlows, showError]);

  const handleDeleteFlow = useCallback(
    async (item: FlowListItem) => {
      try {
        if (isProjectMode) {
          await api.deleteFlow(tabID!, item.id);
          onFlowDelete?.(item.id);
        } else {
          await api.deleteUserFlow(item.id);
        }
        setDeleteConfirm(null);
        if (selectedId === item.id) {
          setSelectedId(null);
          setSelectedSpec(null);
        }
        void refreshFlows();
      } catch (err) {
        showError("Failed to delete flow", err);
      }
    },
    [tabID, isProjectMode, onFlowDelete, selectedId, refreshFlows, showError],
  );

  const handleCloseEditor = useCallback(() => {
    setSelectedId(null);
    setSelectedSpec(null);
    void refreshFlows();
  }, [refreshFlows]);

  // Editor view.
  if (selectedId && selectedSpec) {
    const isReadOnly = selectedSource === "built-in";
    return (
      <div className="flex flex-col h-full">
        <div className="flex items-center gap-3 px-6 py-3 border-b border-border shrink-0">
          <button
            onClick={handleCloseEditor}
            className="p-1 rounded hover:bg-accent text-muted-foreground hover:text-foreground transition-colors"
            title="Back to flow list"
          >
            <X size={16} />
          </button>
          <Workflow size={16} className="text-muted-foreground" />
          <h1 className="text-sm font-semibold">{selectedId}</h1>
          {isReadOnly && (
            <span className="flex items-center gap-1 text-[10px] text-muted-foreground px-1.5 py-0.5 rounded bg-muted">
              <Lock size={9} /> Built-in (read-only)
            </span>
          )}
          <div className="ml-auto flex gap-2">
            {isReadOnly && (
              <button
                onClick={() => void handleCopyBuiltIn({ id: selectedId, name: selectedId, description: "", source: "built-in", stepCount: 0 })}
                className="flex items-center gap-1.5 rounded-md border border-border px-3 py-1 text-xs text-muted-foreground hover:text-foreground hover:bg-accent transition-colors"
              >
                <Copy size={12} />
                Copy to edit
              </button>
            )}
            {tabID && !isReadOnly && (
              <button
                onClick={() => void handleSaveProject()}
                className="flex items-center gap-1.5 rounded-md border border-border px-3 py-1 text-xs text-muted-foreground hover:text-foreground hover:bg-accent transition-colors"
              >
                <Save size={12} />
                Save
              </button>
            )}
          </div>
        </div>

        <div className="flex-1 overflow-hidden">
          <FlowPage
            flowName={selectedId}
            flow={selectedSpec}
            onChange={isReadOnly ? () => {} : handleFlowChange}
            onRun={undefined}
            readOnly={isReadOnly}
          />
        </div>
      </div>
    );
  }

  // Flow list view.
  return (
    <div className="p-6">
      <div className="mb-6 flex items-center justify-between">
        <h1 className="text-xl font-semibold">
          {isProjectMode ? "Project Flows" : "Flows"}
        </h1>
        <div className="flex gap-2">
          {isProjectMode && (
            <button
              onClick={() => void handleOpenImportDialog()}
              className="flex items-center gap-1.5 rounded-md border border-border px-3 py-1.5 text-xs text-muted-foreground hover:text-foreground hover:bg-accent transition-colors"
            >
              <Import size={12} />
              Import Flow
            </button>
          )}
          <button
            onClick={() => setShowCreateDialog(true)}
            className="flex items-center gap-1.5 rounded-md bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground hover:bg-primary/90"
          >
            <Plus size={12} />
            New Flow
          </button>
        </div>
      </div>

      {loading && (
        <div className="grid grid-cols-1 md:grid-cols-2 gap-2">
          {[0, 1, 2].map((i) => (
            <div key={i} className="rounded-lg border border-border p-4 animate-pulse">
              <div className="h-3.5 bg-muted rounded w-1/3 mb-2" />
              <div className="h-2.5 bg-muted rounded w-2/3" />
            </div>
          ))}
        </div>
      )}

      {!loading && flows.length > 0 && (
        <div className="grid grid-cols-1 md:grid-cols-2 gap-2">
          {flows.map((item) => (
            <div
              key={item.id}
              className="group rounded-lg border border-border bg-card p-4 transition-all hover:border-primary/30 hover:shadow-md cursor-pointer"
              onClick={() => void handleOpenFlow(item)}
            >
              <div className="flex items-start gap-3">
                <Workflow size={18} className="mt-0.5 text-muted-foreground group-hover:text-primary transition-colors shrink-0" />
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2">
                    <span className="text-sm font-semibold text-foreground group-hover:text-primary transition-colors truncate">
                      {item.name}
                    </span>
                    {item.source === "built-in" && (
                      <span className="text-[10px] px-1.5 py-px rounded bg-muted text-muted-foreground shrink-0">
                        built-in
                      </span>
                    )}
                  </div>
                  {item.description && (
                    <div className="text-[11px] text-muted-foreground truncate mt-0.5">
                      {item.description}
                    </div>
                  )}
                  <div className="flex items-center gap-3 mt-2 text-[11px] text-muted-foreground">
                    <span>{item.stepCount} step{item.stepCount !== 1 ? "s" : ""}</span>
                  </div>
                </div>

                <div
                  className="flex gap-1 opacity-0 group-hover:opacity-100 transition-opacity"
                  onClick={(e) => e.stopPropagation()}
                >
                  {item.source === "built-in" ? (
                    <button
                      onClick={() => void handleCopyBuiltIn(item)}
                      className="p-1.5 rounded hover:bg-accent text-muted-foreground hover:text-foreground transition-colors"
                      title="Copy to edit"
                    >
                      <Copy size={12} />
                    </button>
                  ) : (
                    <>
                      {deleteConfirm === item.id ? (
                        <div className="flex gap-1">
                          <button
                            onClick={() => void handleDeleteFlow(item)}
                            className="px-2 py-0.5 rounded text-[10px] bg-destructive text-destructive-foreground"
                          >
                            Delete
                          </button>
                          <button
                            onClick={() => setDeleteConfirm(null)}
                            className="px-2 py-0.5 rounded text-[10px] text-muted-foreground hover:text-foreground"
                          >
                            Cancel
                          </button>
                        </div>
                      ) : (
                        <button
                          onClick={() => setDeleteConfirm(item.id)}
                          className="p-1.5 rounded hover:bg-destructive/10 text-muted-foreground hover:text-destructive transition-colors"
                          title="Delete flow"
                        >
                          <Trash2 size={12} />
                        </button>
                      )}
                    </>
                  )}
                </div>
              </div>
            </div>
          ))}
        </div>
      )}

      {!loading && flows.length === 0 && (
        <div className="rounded-lg border border-dashed border-border p-8 text-center">
          <Workflow size={24} className="mx-auto mb-2 text-muted-foreground/50" />
          <p className="text-sm text-muted-foreground mb-3">
            {isProjectMode
              ? "No flows defined in this project yet."
              : "Create a flow or copy a built-in to get started."}
          </p>
          <button
            onClick={() => setShowCreateDialog(true)}
            className="rounded-md bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground hover:bg-primary/90"
          >
            Create Flow
          </button>
        </div>
      )}

      {showCreateDialog && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40">
          <div className="w-full max-w-sm rounded-xl border border-border bg-background p-6 shadow-lg">
            <h2 className="text-lg font-semibold mb-3">New Flow</h2>
            <label className="text-xs text-muted-foreground block mb-1">Flow Name</label>
            <input
              type="text"
              value={newFlowName}
              onChange={(e) => setNewFlowName(e.target.value)}
              onKeyDown={(e) => { if (e.key === "Enter") void handleCreateFlow(); }}
              placeholder="translate-and-qa"
              autoFocus
              className="w-full rounded-md border border-input bg-transparent px-3 py-2 text-sm outline-none focus:ring-1 focus:ring-ring mb-1"
            />
            <p className="text-[10px] text-muted-foreground mb-4">
              You can start from a template in the editor.
            </p>
            <div className="flex gap-2">
              <button
                onClick={() => void handleCreateFlow()}
                disabled={!newFlowName.trim()}
                className="rounded-md bg-primary px-4 py-2 text-xs font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
              >
                Create
              </button>
              <button
                onClick={() => { setShowCreateDialog(false); setNewFlowName(""); }}
                className="rounded-md border border-border px-4 py-2 text-xs hover:bg-accent transition-colors"
              >
                Cancel
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Import flow dialog (project mode) */}
      {showImportDialog && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40">
          <div className="w-full max-w-md rounded-xl border border-border bg-background p-6 shadow-lg">
            <div className="flex items-center justify-between mb-4">
              <h2 className="text-lg font-semibold">Import Flow</h2>
              <button
                onClick={() => setShowImportDialog(false)}
                className="p-1 rounded hover:bg-accent text-muted-foreground"
              >
                <X size={14} />
              </button>
            </div>
            <p className="text-xs text-muted-foreground mb-4">
              Copy a built-in or user flow into this project. The flow will be independent — changes won't affect the original.
            </p>
            {importFlows.length === 0 ? (
              <p className="text-sm text-muted-foreground text-center py-4">No flows available to import.</p>
            ) : (
              <div className="flex flex-col gap-1.5 max-h-64 overflow-y-auto">
                {importFlows.map((item) => (
                  <button
                    key={item.id}
                    onClick={() => void handleImportFlow(item)}
                    className="flex items-center gap-3 w-full text-left rounded-md border border-border p-3 hover:border-primary/30 hover:bg-accent/50 transition-colors"
                  >
                    <Workflow size={14} className="text-muted-foreground shrink-0" />
                    <div className="flex-1 min-w-0">
                      <div className="flex items-center gap-2">
                        <span className="text-xs font-semibold truncate">{item.name}</span>
                        <span className="text-[10px] px-1.5 py-px rounded bg-muted text-muted-foreground shrink-0">
                          {item.source}
                        </span>
                      </div>
                      {item.description && (
                        <div className="text-[10px] text-muted-foreground truncate mt-0.5">{item.description}</div>
                      )}
                    </div>
                    <span className="text-[10px] text-muted-foreground">{item.stepCount} steps</span>
                  </button>
                ))}
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
