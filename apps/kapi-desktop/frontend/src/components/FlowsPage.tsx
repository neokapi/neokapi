import { useState, useEffect, useCallback, useRef } from "react";
import { t } from "@neokapi/kapi-react/runtime";
import {
  Workflow,
  Plus,
  X,
  Save,
  Copy,
  Lock,
  Import,
  FolderOpen,
  Download,
  FolderInput,
  CheckCircle2,
} from "lucide-react";
import {
  Button,
  Skeleton,
  Label,
  Input,
  ScrollArea,
  ItemCard,
  ConfirmDeleteButton,
  PageHeader,
  EmptyState,
} from "@neokapi/ui-primitives";
import { api } from "../hooks/useApi";
import { useError } from "./ErrorBanner";
import { FlowPage } from "./FlowPage";
import type { FlowSpec } from "../types/api";

export interface FlowsPageProps {
  /** Project tab ID — if provided, flows are scoped to the project. */
  tabID?: string;
  /** Project flows map — used for project-mode flow data. */
  projectFlows?: Record<string, FlowSpec>;
  /** Called when a project flow is modified. */
  onFlowChange?: (name: string, spec: FlowSpec) => void;
  /** Called when a flow is deleted from the project. */
  onFlowDelete?: (name: string) => void;
  /** Pre-loaded flow list for Storybook — skips api.listUserFlows()/api.listFlows(). */
  flows?: FlowListItem[];
  /**
   * In ad-hoc mode, the active project tab id (if any project is open). When
   * set, user/ad-hoc flows gain an "Add to project" action that copies the flow
   * into that project's recipe via AdoptUserFlowIntoProject.
   */
  adoptTabID?: string;
  /** Display name of the adopt target project (for the action label/tooltip). */
  adoptProjectName?: string;
}

export interface FlowListItem {
  id: string;
  name: string;
  description: string;
  source: string; // "built-in" | "user" | "project"
  stepCount: number;
}

export function FlowsPage({
  tabID,
  projectFlows: _projectFlows,
  onFlowChange,
  onFlowDelete,
  flows: propFlows,
  adoptTabID,
  adoptProjectName,
}: FlowsPageProps) {
  const [flows, setFlows] = useState<FlowListItem[]>(propFlows ?? []);
  const [loading, setLoading] = useState(!propFlows);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [selectedSpec, setSelectedSpec] = useState<FlowSpec | null>(null);
  const [selectedSource, setSelectedSource] = useState<string>("user");
  const [showCreateDialog, setShowCreateDialog] = useState(false);
  const [showImportDialog, setShowImportDialog] = useState(false);
  const [importFlows, setImportFlows] = useState<FlowListItem[]>([]);
  const [newFlowName, setNewFlowName] = useState("");
  const [adoptNotice, setAdoptNotice] = useState<string | null>(null);

  const { showError } = useError();
  const isProjectMode = !!tabID;
  // Adopt is only offered in ad-hoc mode when a project tab is open.
  const canAdopt = !isProjectMode && !!adoptTabID;

  const handleAdoptFlow = useCallback(
    async (item: FlowListItem) => {
      if (!adoptTabID) return;
      try {
        const result = await api.adoptUserFlowIntoProject(adoptTabID, item.id);
        if (result) {
          const target = adoptProjectName ? ` to ${adoptProjectName}` : " to project";
          setAdoptNotice(
            result.renamed
              ? `Added "${item.name}"${target} as "${result.name}" (renamed to avoid a clash)`
              : `Added "${result.name}"${target}`,
          );
        }
      } catch (err) {
        showError("Failed to add flow to project", err);
      }
    },
    [adoptTabID, adoptProjectName, showError],
  );

  // Auto-dismiss the adopt notice after a few seconds.
  useEffect(() => {
    if (!adoptNotice) return;
    const id = setTimeout(() => setAdoptNotice(null), 5000);
    return () => clearTimeout(id);
  }, [adoptNotice]);

  const refreshFlows = useCallback(async () => {
    if (propFlows) return;
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
  }, [tabID, isProjectMode, showError, propFlows]);

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
            setSelectedSpec({
              description: detail.description,
              steps: detail.steps as FlowSpec["steps"],
            });
            setSelectedSource(detail.source);
          }
        }
      } catch (err) {
        showError("Failed to open flow", err);
      }
    },
    [tabID, isProjectMode, showError],
  );

  // Debounce persistence — update local state immediately, save to backend after 500ms idle.
  const saveTimerRef = useRef<ReturnType<typeof setTimeout>>(undefined);

  const handleFlowChange = useCallback(
    (spec: FlowSpec) => {
      if (!selectedId) return;
      // Immediate local update — no async, no re-render delay.
      setSelectedSpec(spec);
      onFlowChange?.(selectedId, spec);

      // Debounced save to backend.
      clearTimeout(saveTimerRef.current);
      saveTimerRef.current = setTimeout(async () => {
        try {
          if (isProjectMode && tabID) {
            await api.saveFlow(tabID, selectedId, spec);
          } else if (selectedSource === "user") {
            await api.saveUserFlow({
              id: selectedId,
              name: selectedId,
              description: spec.description ?? "",
              steps: spec.steps,
            });
          }
        } catch (err) {
          showError("Failed to save flow", err);
        }
      }, 500);
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
            const spec: FlowSpec = {
              description: detail.description,
              steps: detail.steps as FlowSpec["steps"],
            };
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
              setSelectedSpec({
                description: detail.description,
                steps: detail.steps as FlowSpec["steps"],
              });
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
          const spec: FlowSpec = {
            description: detail.description,
            steps: detail.steps as FlowSpec["steps"],
          };
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

  const handleOpenFile = useCallback(async () => {
    try {
      const detail = await api.openFlowFileDialog();
      if (detail) {
        setSelectedId(detail.id);
        setSelectedSpec({
          description: detail.description,
          steps: detail.steps as FlowSpec["steps"],
        });
        setSelectedSource("file");
      }
    } catch (err) {
      showError("Failed to open flow file", err);
    }
  }, [showError]);

  const handleSaveAs = useCallback(async () => {
    if (!selectedId || !selectedSpec) return;
    try {
      await api.saveFlowFileDialog(selectedId, selectedSpec.steps);
    } catch (err) {
      showError("Failed to save flow file", err);
    }
  }, [selectedId, selectedSpec, showError]);

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
          <Button
            variant="ghost"
            size="icon-xs"
            onClick={handleCloseEditor}
            title="Back to flow list"
          >
            <X size={16} />
          </Button>
          <Workflow size={16} className="text-muted-foreground" />
          <h1 className="text-sm font-semibold">{selectedId}</h1>
          {isReadOnly && (
            <span className="flex items-center gap-1 text-[10px] text-muted-foreground px-1.5 py-0.5 rounded bg-muted">
              <Lock size={9} /> Built-in (read-only)
            </span>
          )}
          <div className="ml-auto flex gap-2">
            {isReadOnly && (
              <Button
                variant="outline"
                size="sm"
                onClick={() =>
                  void handleCopyBuiltIn({
                    id: selectedId,
                    name: selectedId,
                    description: "",
                    source: "built-in",
                    stepCount: 0,
                  })
                }
              >
                <Copy size={12} />
                Copy to edit
              </Button>
            )}
            {!isReadOnly && (
              <Button variant="outline" size="sm" onClick={() => void handleSaveAs()}>
                <Download size={12} />
                Save As...
              </Button>
            )}
            {tabID && !isReadOnly && (
              <Button variant="outline" size="sm" onClick={() => void handleSaveProject()}>
                <Save size={12} />
                Save
              </Button>
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
            tabID={tabID}
          />
        </div>
      </div>
    );
  }

  // Flow list view.
  return (
    <div className="p-6">
      <PageHeader
        title={isProjectMode ? "Project Flows" : "Flows"}
        actions={
          <div className="flex gap-2">
            {!isProjectMode && (
              <Button variant="outline" size="sm" onClick={() => void handleOpenFile()}>
                <FolderOpen size={12} />
                Open File...
              </Button>
            )}
            {isProjectMode && (
              <Button variant="outline" size="sm" onClick={() => void handleOpenImportDialog()}>
                <Import size={12} />
                Import Flow
              </Button>
            )}
            <Button size="sm" onClick={() => setShowCreateDialog(true)}>
              <Plus size={12} />
              New Flow
            </Button>
          </div>
        }
      />

      {adoptNotice && (
        <div
          className="mb-4 flex items-center gap-2 rounded-md border border-green-500/30 bg-green-500/5 px-3 py-2 text-sm text-green-700 dark:text-green-400"
          role="status"
        >
          <CheckCircle2 size={14} className="shrink-0" />
          {adoptNotice}
        </div>
      )}

      {(loading || flows.length > 0) && (
        <div className="grid grid-cols-1 md:grid-cols-2 gap-2">
          {loading
            ? [0, 1, 2].map((i) => <FlowCard key={i} loading />)
            : flows.map((item) => (
                <FlowCard
                  key={item.id}
                  item={item}
                  onClick={() => void handleOpenFlow(item)}
                  onCopy={
                    item.source === "built-in" ? () => void handleCopyBuiltIn(item) : undefined
                  }
                  onDelete={
                    item.source !== "built-in" ? () => void handleDeleteFlow(item) : undefined
                  }
                  onAdopt={canAdopt ? () => void handleAdoptFlow(item) : undefined}
                  adoptProjectName={adoptProjectName}
                />
              ))}
        </div>
      )}

      {!loading && flows.length === 0 && (
        <EmptyState
          icon={<Workflow size={24} className="text-muted-foreground/50" />}
          title={
            isProjectMode
              ? "No flows defined in this project yet."
              : "Create a flow or copy a built-in to get started."
          }
          action={
            <Button size="sm" onClick={() => setShowCreateDialog(true)}>
              Create Flow
            </Button>
          }
        />
      )}

      {showCreateDialog && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40">
          <div className="w-full max-w-sm rounded-xl border border-border bg-background p-6 shadow-lg">
            <h2 className="text-lg font-semibold mb-3">New Flow</h2>
            <Label className="text-xs text-muted-foreground block mb-1">Flow Name</Label>
            <Input
              type="text"
              value={newFlowName}
              onChange={(e) => setNewFlowName(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter") void handleCreateFlow();
              }}
              placeholder="translate-and-qa"
              autoFocus
              className="mb-1"
            />
            <p className="text-[10px] text-muted-foreground mb-4">
              You can start from a template in the editor.
            </p>
            <div className="flex gap-2">
              <Button
                size="sm"
                onClick={() => void handleCreateFlow()}
                disabled={!newFlowName.trim()}
              >
                Create
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={() => {
                  setShowCreateDialog(false);
                  setNewFlowName("");
                }}
              >
                Cancel
              </Button>
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
              <Button variant="ghost" size="icon-xs" onClick={() => setShowImportDialog(false)}>
                <X size={14} />
              </Button>
            </div>
            <p className="text-xs text-muted-foreground mb-4">
              Copy a built-in or user flow into this project. The flow will be independent — changes
              won't affect the original.
            </p>
            {importFlows.length === 0 ? (
              <p className="text-sm text-muted-foreground text-center py-4">
                No flows available to import.
              </p>
            ) : (
              <ScrollArea className="max-h-64">
                <div className="flex flex-col gap-1.5">
                  {importFlows.map((item) => (
                    <Button
                      key={item.id}
                      variant="outline"
                      onClick={() => void handleImportFlow(item)}
                      className="flex items-center gap-3 w-full h-auto text-left p-3 hover:border-primary/30 hover:bg-accent/50"
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
                          <div className="text-[10px] text-muted-foreground truncate mt-0.5">
                            {item.description}
                          </div>
                        )}
                      </div>
                      <span className="text-[10px] text-muted-foreground">
                        {item.stepCount} steps
                      </span>
                    </Button>
                  ))}
                </div>
              </ScrollArea>
            )}
          </div>
        </div>
      )}
    </div>
  );
}

// ── FlowCard ────────────────────────────────────────────────────

interface FlowCardItem {
  id: string;
  name: string;
  description?: string;
  stepCount: number;
  source?: string;
}

function FlowCard({
  item,
  loading,
  onClick,
  onCopy,
  onDelete,
  onAdopt,
  adoptProjectName,
}: {
  item?: FlowCardItem;
  loading?: boolean;
  onClick?: () => void;
  onCopy?: () => void;
  onDelete?: () => void;
  onAdopt?: () => void;
  adoptProjectName?: string;
}) {
  if (loading) {
    return (
      <ItemCard>
        <div className="flex items-start gap-3">
          <Skeleton className="mt-0.5 h-5 w-5 shrink-0 rounded" />
          <div className="min-w-0 flex-1">
            <Skeleton className="h-4 w-1/2" />
            <Skeleton className="mt-1.5 h-3 w-3/4" />
            <Skeleton className="mt-2.5 h-3 w-16" />
          </div>
        </div>
      </ItemCard>
    );
  }

  if (!item) return null;

  return (
    <ItemCard clickable onClick={onClick}>
      <div className="flex items-start gap-3">
        <Workflow
          size={18}
          className="mt-0.5 shrink-0 text-muted-foreground transition-colors group-hover:text-primary"
        />
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <span className="truncate text-sm font-semibold text-foreground transition-colors group-hover:text-primary">
              {item.name}
            </span>
            {item.source === "built-in" && (
              <span className="shrink-0 rounded bg-muted px-1.5 py-px text-[10px] text-muted-foreground">
                built-in
              </span>
            )}
          </div>
          {item.description && (
            <div className="mt-0.5 truncate text-[11px] text-muted-foreground">
              {item.description}
            </div>
          )}
          <div className="mt-2 flex items-center gap-3 text-[11px] text-muted-foreground">
            <span>{t("{count} step(s)", { count: item.stepCount })}</span>
          </div>
        </div>

        <div
          className="flex gap-1 opacity-0 transition-opacity group-hover:opacity-100"
          onClick={(e: React.MouseEvent) => e.stopPropagation()}
        >
          {onAdopt && (
            <Button
              variant="ghost"
              size="icon-xs"
              onClick={onAdopt}
              title={adoptProjectName ? `Add to project: ${adoptProjectName}` : "Add to project"}
              aria-label="Add to project"
            >
              <FolderInput size={12} />
            </Button>
          )}
          {onCopy && (
            <Button variant="ghost" size="icon-xs" onClick={onCopy} title="Copy to edit">
              <Copy size={12} />
            </Button>
          )}
          {onDelete && <ConfirmDeleteButton onDelete={onDelete} mode="icon" />}
        </div>
      </div>
    </ItemCard>
  );
}
