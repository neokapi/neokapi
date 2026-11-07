import { useState, useEffect, useCallback, useRef } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Plus, X, Copy, Lock, FolderOpen, Download } from "lucide-react";
import {
  Button,
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  FlowCard,
  FlowsEmptyState,
  Label,
  Input,
  PageHeader,
  SectionHeading,
  SimpleTooltip,
  toast,
} from "@neokapi/ui-primitives";
import { api } from "../hooks/useApi";
import { qk } from "../lib/queryKeys";
import { useError } from "./ErrorBanner";
import { FlowPage } from "./FlowPage";
import type { RunFlowHandler } from "./CollectionsPanel";
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
  /**
   * Runs a flow, taking the caller to the runner. Absent in ad-hoc mode, where
   * there is no project for a run to act on; the editor's Run action appears
   * only when this is wired.
   */
  onRunFlow?: RunFlowHandler;
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
  /**
   * "built-in" | "user" | "project" (inline in the recipe) | "flows-dir" (a
   * file in the recipe's flows_dir:, edited in that file).
   */
  source: string;
  stepCount: number;
  /** Each step named, in order, for the card's chip strip. */
  steps: string[];
  /** True when this is the project's default flow. */
  isDefault: boolean;
}

export function FlowsPage({
  tabID,
  projectFlows: _projectFlows,
  onFlowChange,
  onFlowDelete,
  onRunFlow,
  flows: propFlows,
  adoptTabID,
  adoptProjectName,
}: FlowsPageProps) {
  const qc = useQueryClient();
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [selectedSpec, setSelectedSpec] = useState<FlowSpec | null>(null);
  const [selectedSource, setSelectedSource] = useState<string>("user");
  const [showCreateDialog, setShowCreateDialog] = useState(false);
  const [newFlowName, setNewFlowName] = useState("");

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
          toast.success(
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

  // Flow list as react-query server state: project flows keyed by tab, ad-hoc
  // user flows under a global key. Actions invalidate via refreshFlows().
  const flowsQuery = useQuery({
    queryKey: isProjectMode ? qk.flows(tabID!) : qk.userFlows(),
    enabled: !propFlows,
    queryFn: async (): Promise<FlowListItem[]> => {
      if (isProjectMode) {
        const result = await api.listFlows(tabID!);
        return (result ?? []).map((f) => ({
          id: f.name,
          name: f.name,
          description: f.description,
          source: f.source === "file" ? "flows-dir" : "project",
          stepCount: f.step_count,
          steps: f.steps ?? [],
          isDefault: f.default ?? false,
        }));
      }
      const result = await api.listUserFlows();
      return (result ?? []).map((f) => ({
        id: f.id,
        name: f.name,
        description: f.description,
        source: f.source,
        stepCount: f.step_count,
        steps: f.steps ?? [],
        isDefault: false,
      }));
    },
  });
  const flows: FlowListItem[] = propFlows ?? flowsQuery.data ?? [];
  const loading = !propFlows && flowsQuery.isLoading;

  // In a project, the built-in flows and the person's own flows are listed
  // below the project's as available to add, minus any the project already
  // holds under that name.
  const availableQuery = useQuery({
    queryKey: qk.userFlows(),
    enabled: isProjectMode && !propFlows,
    queryFn: async (): Promise<FlowListItem[]> => {
      const result = await api.listUserFlows();
      return (result ?? []).map((f) => ({
        id: f.id,
        name: f.name,
        description: f.description,
        source: f.source,
        stepCount: f.step_count,
        steps: f.steps ?? [],
        isDefault: false,
      }));
    },
  });
  const held = new Set(flows.map((f) => f.id));
  const available = isProjectMode ? (availableQuery.data ?? []).filter((f) => !held.has(f.id)) : [];

  useEffect(() => {
    if (flowsQuery.error) showError("Failed to load flows", flowsQuery.error);
  }, [flowsQuery.error, showError]);

  const refreshFlows = useCallback(() => {
    void qc.invalidateQueries({ queryKey: isProjectMode ? qk.flows(tabID!) : qk.userFlows() });
  }, [qc, isProjectMode, tabID]);

  const handleOpenFlow = useCallback(
    async (item: FlowListItem) => {
      try {
        if (isProjectMode) {
          const spec = await api.getFlow(tabID!, item.id);
          if (spec) {
            setSelectedId(item.id);
            setSelectedSpec(spec as FlowSpec);
            setSelectedSource(item.source);
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
            // Persist steps and inline options straight to the recipe on disk.
            await api.saveProjectFlow(tabID, selectedId, spec);
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

  const handleToggleDefault = useCallback(
    async (next: boolean) => {
      if (!tabID || !selectedId) return;
      try {
        await api.setDefaultFlow(tabID, next ? selectedId : "");
        refreshFlows();
      } catch (err) {
        showError("Failed to set the default flow", err);
      }
    },
    [tabID, selectedId, refreshFlows, showError],
  );

  const handleRenameFlow = useCallback(
    async (next: string) => {
      if (!tabID || !selectedId) return;
      const name = next.trim().replace(/\s+/g, "-").toLowerCase();
      if (!name || name === selectedId) return;
      try {
        await api.renameProjectFlow(tabID, selectedId, name);
        onFlowDelete?.(selectedId);
        onFlowChange?.(name, selectedSpec ?? { steps: [] });
        setSelectedId(name);
        refreshFlows();
      } catch (err) {
        showError("Failed to rename the flow", err);
      }
    },
    [tabID, selectedId, selectedSpec, onFlowChange, onFlowDelete, refreshFlows, showError],
  );

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
        refreshFlows();
      } catch (err) {
        showError("Failed to copy flow", err);
      }
    },
    [tabID, isProjectMode, onFlowChange, refreshFlows, showError],
  );

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
          setSelectedId(name);
          setSelectedSpec(spec);
          setSelectedSource("project");
          refreshFlows();
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
      refreshFlows();
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
        refreshFlows();
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
    refreshFlows();
  }, [refreshFlows]);

  // Editor view.
  if (selectedId && selectedSpec) {
    const isBuiltIn = selectedSource === "built-in";
    const isReadOnly = isBuiltIn || selectedSource === "flows-dir";
    const selectedIsDefault = flows.find((f) => f.id === selectedId)?.isDefault ?? false;
    const canGovern = isProjectMode && !isReadOnly;
    return (
      <div className="flex flex-col h-full">
        <div className="flex items-center gap-3 px-6 py-2 border-b border-border shrink-0">
          <SimpleTooltip content="Back to flow list">
            <Button
              variant="ghost"
              size="icon-xs"
              onClick={handleCloseEditor}
              aria-label="Back to flow list"
            >
              <X size={16} />
            </Button>
          </SimpleTooltip>
          {isReadOnly && (
            <span className="flex items-center gap-1 text-[11px] text-muted-foreground px-1.5 py-0.5 rounded bg-muted">
              <Lock size={9} />{" "}
              {isBuiltIn ? "Built-in (read-only)" : "Flow file (edit it in flows_dir)"}
            </span>
          )}
          <div className="ml-auto flex gap-2">
            {isBuiltIn && (
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
                    steps: [],
                    isDefault: false,
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
          </div>
        </div>

        <div className="flex-1 overflow-hidden">
          <FlowPage
            flowName={selectedId}
            flow={selectedSpec}
            onChange={isReadOnly ? () => {} : handleFlowChange}
            onRun={onRunFlow}
            readOnly={isReadOnly}
            tabID={tabID}
            isDefault={isProjectMode ? selectedIsDefault : undefined}
            onToggleDefault={canGovern ? handleToggleDefault : undefined}
            onRename={canGovern ? handleRenameFlow : undefined}
          />
        </div>
      </div>
    );
  }

  // Flow list view.
  return (
    <div className="p-6">
      <PageHeader
        title={isProjectMode ? "Flows" : "Flows"}
        actions={
          <div className="flex gap-2">
            {!isProjectMode && (
              <Button variant="outline" size="sm" onClick={() => void handleOpenFile()}>
                <FolderOpen size={12} />
                Open File...
              </Button>
            )}
            <Button size="sm" onClick={() => setShowCreateDialog(true)}>
              <Plus size={12} />
              New flow
            </Button>
          </div>
        }
      />

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
                    item.source !== "built-in" && item.source !== "flows-dir"
                      ? () => void handleDeleteFlow(item)
                      : undefined
                  }
                  onAdopt={canAdopt ? () => void handleAdoptFlow(item) : undefined}
                  adoptProjectName={adoptProjectName}
                />
              ))}
        </div>
      )}

      {!loading && flows.length === 0 && (
        <FlowsEmptyState projectMode={isProjectMode} onCreate={() => setShowCreateDialog(true)} />
      )}

      {available.length > 0 && (
        <section data-slot="flows-available" className="mt-8">
          <SectionHeading className="mb-2">Available to add</SectionHeading>
          <div className="grid grid-cols-1 gap-2 md:grid-cols-2">
            {available.map((item) => (
              <FlowCard
                key={item.id}
                item={item}
                onClick={() => void handleImportFlow(item)}
                onAdopt={() => void handleImportFlow(item)}
              />
            ))}
          </div>
        </section>
      )}

      <Dialog
        open={showCreateDialog}
        onOpenChange={(o) => {
          if (!o) {
            setShowCreateDialog(false);
            setNewFlowName("");
          }
        }}
      >
        <DialogContent showCloseButton={false}>
          <DialogHeader>
            <DialogTitle>New flow</DialogTitle>
          </DialogHeader>
          <div>
            <Label className="text-xs text-muted-foreground block mb-1">Flow name</Label>
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
            <p className="text-[11px] text-muted-foreground mb-4">
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
        </DialogContent>
      </Dialog>
    </div>
  );
}
