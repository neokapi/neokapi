import { useState, useEffect, useCallback } from "react";
import { Workflow, Plus, Play, Trash2, Pencil, X } from "lucide-react";
import { api } from "../hooks/useApi";
import { useError } from "./ErrorBanner";
import { FlowPage } from "./FlowPage";
import type { FlowSpec, FlowInfo } from "../types/api";

interface FlowsPageProps {
  /** Project tab ID — if provided, flows are scoped to the project. */
  tabID?: string;
  /** Project flows map — used for project-mode flow data. */
  projectFlows?: Record<string, FlowSpec>;
  /** Called when a project flow is modified (to sync back to project state). */
  onFlowChange?: (name: string, spec: FlowSpec) => void;
  /** Called when a flow is deleted from the project. */
  onFlowDelete?: (name: string) => void;
}

export function FlowsPage({
  tabID,
  projectFlows,
  onFlowChange,
  onFlowDelete,
}: FlowsPageProps) {
  const [flows, setFlows] = useState<FlowInfo[]>([]);
  const [loading, setLoading] = useState(true);
  const [selectedFlow, setSelectedFlow] = useState<string | null>(null);
  const [selectedSpec, setSelectedSpec] = useState<FlowSpec | null>(null);
  const [showCreateDialog, setShowCreateDialog] = useState(false);
  const [newFlowName, setNewFlowName] = useState("");
  const [renamingFlow, setRenamingFlow] = useState<string | null>(null);
  const [renameValue, setRenameValue] = useState("");
  const [deleteConfirm, setDeleteConfirm] = useState<string | null>(null);

  const { showError } = useError();
  const isProjectMode = !!tabID;

  const refreshFlows = useCallback(async () => {
    setLoading(true);
    try {
      if (tabID) {
        const result = await api.listFlows(tabID);
        setFlows(result ?? []);
      } else if (projectFlows) {
        setFlows(
          Object.entries(projectFlows).map(([name, spec]) => ({
            name,
            description: spec.description ?? "",
            step_count: spec.steps.length,
          })),
        );
      }
    } catch (err) {
      showError("Failed to load flows", err);
    } finally {
      setLoading(false);
    }
  }, [tabID, projectFlows, showError]);

  useEffect(() => {
    void refreshFlows();
  }, [refreshFlows]);

  const handleOpenFlow = useCallback(
    async (name: string) => {
      try {
        if (tabID) {
          const spec = await api.getFlow(tabID, name);
          if (spec) {
            setSelectedFlow(name);
            setSelectedSpec(spec as FlowSpec);
          }
        } else if (projectFlows?.[name]) {
          setSelectedFlow(name);
          setSelectedSpec(projectFlows[name]);
        }
      } catch (err) {
        showError("Failed to open flow", err);
      }
    },
    [tabID, projectFlows, showError],
  );

  const handleFlowChange = useCallback(
    async (spec: FlowSpec) => {
      if (!selectedFlow) return;
      setSelectedSpec(spec);
      try {
        if (tabID) {
          await api.saveFlow(tabID, selectedFlow, spec);
        }
        onFlowChange?.(selectedFlow, spec);
      } catch (err) {
        showError("Failed to save flow", err);
      }
    },
    [selectedFlow, tabID, onFlowChange, showError],
  );

  const handleRunFlow = useCallback(
    async (flowName: string) => {
      if (!tabID) {
        showError("Run requires a project", "Open a project to run flows with input files.");
        return;
      }
      try {
        // For now, run with empty inputs — the runner page handles file selection.
        // This triggers the RunFlow backend which emits flow:event Wails events.
        await api.runFlow(tabID, flowName, [], "");
      } catch (err) {
        showError("Failed to run flow", err);
      }
    },
    [tabID, showError],
  );

  const handleCreateFlow = useCallback(async () => {
    const name = newFlowName.trim().replace(/\s+/g, "-").toLowerCase();
    if (!name) return;
    const spec: FlowSpec = { steps: [] };
    try {
      if (tabID) {
        await api.saveFlow(tabID, name, spec);
      }
      onFlowChange?.(name, spec);
      setNewFlowName("");
      setShowCreateDialog(false);
      setSelectedFlow(name);
      setSelectedSpec(spec);
      void refreshFlows();
    } catch (err) {
      showError("Failed to create flow", err);
    }
  }, [newFlowName, tabID, onFlowChange, refreshFlows, showError]);

  const handleDeleteFlow = useCallback(
    async (name: string) => {
      try {
        if (tabID) {
          await api.deleteFlow(tabID, name);
        }
        onFlowDelete?.(name);
        setDeleteConfirm(null);
        if (selectedFlow === name) {
          setSelectedFlow(null);
          setSelectedSpec(null);
        }
        void refreshFlows();
      } catch (err) {
        showError("Failed to delete flow", err);
      }
    },
    [tabID, onFlowDelete, selectedFlow, refreshFlows, showError],
  );

  const handleCloseEditor = useCallback(() => {
    setSelectedFlow(null);
    setSelectedSpec(null);
    void refreshFlows();
  }, [refreshFlows]);

  // Editor view — a flow is selected.
  if (selectedFlow && selectedSpec) {
    return (
      <div className="flex flex-col h-full">
        {/* Header */}
        <div className="flex items-center gap-3 px-6 py-3 border-b border-border shrink-0">
          <button
            onClick={handleCloseEditor}
            className="p-1 rounded hover:bg-accent text-muted-foreground hover:text-foreground transition-colors"
            title="Back to flow list"
          >
            <X size={16} />
          </button>
          <Workflow size={16} className="text-muted-foreground" />
          <h1 className="text-sm font-semibold">{selectedFlow}</h1>
          {selectedSpec.description && (
            <span className="text-xs text-muted-foreground">{selectedSpec.description}</span>
          )}
        </div>

        {/* Flow editor fills remaining space */}
        <div className="flex-1 overflow-hidden">
          <FlowPage
            flowName={selectedFlow}
            flow={selectedSpec}
            onChange={handleFlowChange}
            onRun={tabID ? (_name, _spec) => void handleRunFlow(selectedFlow) : undefined}
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
        <button
          onClick={() => setShowCreateDialog(true)}
          className="flex items-center gap-1.5 rounded-md bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground hover:bg-primary/90"
        >
          <Plus size={12} />
          New Flow
        </button>
      </div>

      {/* Loading */}
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

      {/* Flow list */}
      {!loading && flows.length > 0 && (
        <div className="grid grid-cols-1 md:grid-cols-2 gap-2">
          {flows.map((flow) => (
            <div
              key={flow.name}
              className="group rounded-lg border border-border bg-card p-4 transition-all hover:border-primary/30 hover:shadow-md cursor-pointer"
              onClick={() => void handleOpenFlow(flow.name)}
            >
              <div className="flex items-start gap-3">
                <Workflow size={18} className="mt-0.5 text-muted-foreground group-hover:text-primary transition-colors shrink-0" />
                <div className="flex-1 min-w-0">
                  {renamingFlow === flow.name ? (
                    <div className="flex gap-1 mb-1">
                      <input
                        type="text"
                        value={renameValue}
                        onChange={(e) => setRenameValue(e.target.value)}
                        className="flex-1 rounded border border-input bg-transparent px-2 py-0.5 text-sm outline-none focus:ring-1 focus:ring-ring"
                        autoFocus
                        onClick={(e) => e.stopPropagation()}
                        onKeyDown={(e) => {
                          if (e.key === "Escape") setRenamingFlow(null);
                        }}
                      />
                      <button
                        onClick={(e) => { e.stopPropagation(); setRenamingFlow(null); }}
                        className="text-xs text-muted-foreground"
                      >
                        Cancel
                      </button>
                    </div>
                  ) : (
                    <div className="text-sm font-semibold text-foreground group-hover:text-primary transition-colors truncate">
                      {flow.name}
                    </div>
                  )}
                  {flow.description && (
                    <div className="text-[11px] text-muted-foreground truncate mt-0.5">
                      {flow.description}
                    </div>
                  )}
                  <div className="flex items-center gap-3 mt-2 text-[11px] text-muted-foreground">
                    <span>{flow.step_count} step{flow.step_count !== 1 ? "s" : ""}</span>
                  </div>
                </div>

                {/* Actions */}
                <div
                  className="flex gap-1 opacity-0 group-hover:opacity-100 transition-opacity"
                  onClick={(e) => e.stopPropagation()}
                >
                  {tabID && (
                    <button
                      onClick={() => void handleRunFlow(flow.name)}
                      className="p-1.5 rounded hover:bg-accent text-muted-foreground hover:text-foreground transition-colors"
                      title="Run flow"
                    >
                      <Play size={12} />
                    </button>
                  )}
                  {deleteConfirm === flow.name ? (
                    <div className="flex gap-1">
                      <button
                        onClick={() => void handleDeleteFlow(flow.name)}
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
                      onClick={() => setDeleteConfirm(flow.name)}
                      className="p-1.5 rounded hover:bg-destructive/10 text-muted-foreground hover:text-destructive transition-colors"
                      title="Delete flow"
                    >
                      <Trash2 size={12} />
                    </button>
                  )}
                </div>
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Empty state */}
      {!loading && flows.length === 0 && (
        <div className="rounded-lg border border-dashed border-border p-8 text-center">
          <Workflow size={24} className="mx-auto mb-2 text-muted-foreground/50" />
          <p className="text-sm text-muted-foreground mb-3">
            {isProjectMode
              ? "No flows defined in this project yet."
              : "Create a flow to design localization pipelines."}
          </p>
          <button
            onClick={() => setShowCreateDialog(true)}
            className="rounded-md bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground hover:bg-primary/90"
          >
            Create Flow
          </button>
        </div>
      )}

      {/* Create dialog */}
      {showCreateDialog && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40">
          <div className="w-full max-w-sm rounded-xl border border-border bg-background p-6 shadow-lg">
            <h2 className="text-lg font-semibold mb-3">New Flow</h2>
            <label className="text-xs text-muted-foreground block mb-1">Flow Name</label>
            <input
              type="text"
              value={newFlowName}
              onChange={(e) => setNewFlowName(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter") void handleCreateFlow();
              }}
              placeholder="translate-and-qa"
              autoFocus
              className="w-full rounded-md border border-input bg-transparent px-3 py-2 text-sm outline-none focus:ring-1 focus:ring-ring mb-1"
            />
            <p className="text-[10px] text-muted-foreground mb-4">
              Spaces will be converted to dashes. You can start from a template in the editor.
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
    </div>
  );
}
