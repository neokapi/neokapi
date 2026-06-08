import { useState, useCallback, useMemo } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  Badge,
  Button,
  Card,
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Input,
  Label,
  cn,
  useApi,
  type FlowDefinitionInfo,
} from "@neokapi/ui";
import {
  FlowEditor,
  defToSpec,
  specToDef,
  type FlowSpec,
  type ToolInfo as EditorToolInfo,
  type FlowDefinitionInfo as EditorFlowDefinitionInfo,
} from "@neokapi/flow-editor";

/**
 * Web superset flow editor.
 *
 * Lists a project's flow definitions (built-in flows merged with server-stored
 * project flows) and edits them on the canonical `@neokapi/flow-editor`
 * <FlowEditor> canvas — the same component kapi-desktop and the Bowrain desktop
 * app render. This component owns the list / new / save / delete UX and the
 * REST data calls; it bridges the server's node/edge FlowDefinitionInfo to the
 * editor's steps-based FlowSpec via the shared defToSpec / specToDef adapter.
 *
 * Flows are connector-agnostic: they apply to content from any connector. The
 * flow graph never names a connector (Kapi is one source among many).
 */
export function ProjectFlowsEditor({
  workspaceSlug,
  projectId,
}: {
  workspaceSlug: string;
  projectId: string;
}) {
  const api = useApi();
  const queryClient = useQueryClient();
  const flowsKey = ["flows", workspaceSlug, projectId];

  const { data: flows, isLoading } = useQuery({
    queryKey: flowsKey,
    queryFn: () => api.listFlowDefinitions(workspaceSlug, projectId),
    staleTime: 15_000,
  });

  const { data: tools } = useQuery({
    queryKey: ["tools"],
    queryFn: () => api.listTools(),
    staleTime: 5 * 60_000,
  });

  const editorTools = useMemo<EditorToolInfo[]>(
    () =>
      (tools ?? []).map((t) => ({
        name: t.name,
        description: t.description,
        category: t.category,
        consumes: t.consumes,
        produces: t.produces,
        isSourceTransform: t.is_source_transform,
      })),
    [tools],
  );

  const [activeDef, setActiveDef] = useState<FlowDefinitionInfo | null>(null);
  const [activeSpec, setActiveSpec] = useState<FlowSpec | null>(null);
  const [editName, setEditName] = useState("");
  const [editDescription, setEditDescription] = useState("");
  const [dirty, setDirty] = useState(false);
  const [isNew, setIsNew] = useState(false);
  const [showNewDialog, setShowNewDialog] = useState(false);
  const [newName, setNewName] = useState("");
  const [newDescription, setNewDescription] = useState("");

  const isBuiltIn = activeDef?.source === "built-in";

  const invalidate = useCallback(
    () => queryClient.invalidateQueries({ queryKey: flowsKey }),
    [queryClient, flowsKey],
  );

  const createMutation = useMutation({
    mutationFn: (def: FlowDefinitionInfo) =>
      api.createFlowDefinition(workspaceSlug, projectId, def),
    onSuccess: (saved) => {
      void invalidate();
      handleSelect(saved);
    },
  });

  const updateMutation = useMutation({
    mutationFn: ({ flowId, def }: { flowId: string; def: FlowDefinitionInfo }) =>
      api.updateFlowDefinition(workspaceSlug, projectId, flowId, def),
    onSuccess: (saved) => {
      void invalidate();
      handleSelect(saved);
    },
  });

  const deleteMutation = useMutation({
    mutationFn: (flowId: string) => api.deleteFlowDefinition(workspaceSlug, projectId, flowId),
    onSuccess: () => {
      void invalidate();
      setActiveDef(null);
      setActiveSpec(null);
    },
  });

  const handleSelect = useCallback((def: FlowDefinitionInfo) => {
    setActiveDef(def);
    setActiveSpec(defToSpec(def as EditorFlowDefinitionInfo));
    setEditName(def.name);
    setEditDescription(def.description || "");
    setDirty(false);
    setIsNew(false);
  }, []);

  const handleNewCreate = useCallback(() => {
    const name = newName.trim();
    if (!name) return;
    const def: FlowDefinitionInfo = {
      id: `flow-${Date.now()}`,
      name,
      description: newDescription.trim(),
      source: "project",
      nodes: [],
      edges: [],
    };
    setActiveDef(def);
    setActiveSpec({ description: def.description, steps: [] });
    setEditName(name);
    setEditDescription(def.description || "");
    setDirty(true);
    setIsNew(true);
    setShowNewDialog(false);
    setNewName("");
    setNewDescription("");
  }, [newName, newDescription]);

  const handleFlowChange = useCallback(
    (spec: FlowSpec) => {
      if (isBuiltIn) return;
      setActiveSpec(spec);
      setDirty(true);
    },
    [isBuiltIn],
  );

  const handleSave = useCallback(() => {
    if (!activeDef || !activeSpec) return;
    const def = specToDef(
      { ...activeSpec, description: editDescription },
      { id: activeDef.id, name: editName, source: "project", description: editDescription },
      editorTools,
    ) as FlowDefinitionInfo;
    if (isNew) {
      createMutation.mutate(def);
    } else {
      updateMutation.mutate({ flowId: activeDef.id, def });
    }
  }, [
    activeDef,
    activeSpec,
    editName,
    editDescription,
    editorTools,
    isNew,
    createMutation,
    updateMutation,
  ]);

  const handleDelete = useCallback(() => {
    if (!activeDef || isBuiltIn) return;
    if (isNew) {
      setActiveDef(null);
      setActiveSpec(null);
      return;
    }
    if (window.confirm(`Delete flow "${activeDef.name}"?`)) {
      deleteMutation.mutate(activeDef.id);
    }
  }, [activeDef, isBuiltIn, isNew, deleteMutation]);

  const saving = createMutation.isPending || updateMutation.isPending;

  return (
    <Card className="flex h-[640px] overflow-hidden p-0">
      {/* Flow list */}
      <div
        data-testid="flow-list"
        className="w-60 border-r border-border flex flex-col overflow-hidden"
      >
        <div className="px-4 py-3 border-b border-border flex justify-between items-center">
          <span className="font-semibold text-sm">Flows</span>
          <Button
            data-testid="new-flow-btn"
            size="sm"
            onClick={() => {
              setNewName("");
              setNewDescription("");
              setShowNewDialog(true);
            }}
          >
            + New
          </Button>
        </div>
        <div className="flex-1 overflow-auto py-1">
          {isLoading && (
            <div className="px-4 py-3 text-xs text-muted-foreground">Loading flows...</div>
          )}
          {(flows ?? []).map((def) => (
            <button
              key={def.id}
              data-testid={`flow-item-${def.id}`}
              onClick={() => handleSelect(def)}
              className={cn(
                "w-full px-4 py-2.5 text-left text-[13px] cursor-pointer border-l-[3px]",
                activeDef?.id === def.id && !isNew
                  ? "border-l-primary bg-accent"
                  : "border-l-transparent hover:bg-accent/50",
              )}
            >
              <div className="font-medium">{def.name}</div>
              <div className="text-[11px] text-muted-foreground mt-0.5">
                {def.source} &middot; {def.nodes.filter((n) => n.type === "tool").length} tool(s)
              </div>
            </button>
          ))}
        </div>
      </div>

      {/* New-flow dialog */}
      <Dialog open={showNewDialog} onOpenChange={setShowNewDialog}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>New Flow</DialogTitle>
          </DialogHeader>
          <div className="flex flex-col gap-4 py-2">
            <div className="flex flex-col gap-1.5">
              <Label>Name</Label>
              <Input
                data-testid="new-flow-name"
                value={newName}
                onChange={(e: React.ChangeEvent<HTMLInputElement>) => setNewName(e.target.value)}
                placeholder="My Flow"
                autoFocus
              />
            </div>
            <div className="flex flex-col gap-1.5">
              <Label>Description (optional)</Label>
              <Input
                value={newDescription}
                onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
                  setNewDescription(e.target.value)
                }
                placeholder="What this flow does..."
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setShowNewDialog(false)}>
              Cancel
            </Button>
            <Button onClick={handleNewCreate} disabled={!newName.trim()}>
              Create
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Editor */}
      <div className="flex-1 flex flex-col min-h-0">
        {activeDef && activeSpec ? (
          <>
            <div
              data-testid="flow-toolbar"
              className="px-4 py-2 border-b border-border flex gap-3 items-center"
            >
              <Input
                data-testid="flow-name-input"
                value={editName}
                onChange={(e: React.ChangeEvent<HTMLInputElement>) => {
                  setEditName(e.target.value);
                  setDirty(true);
                }}
                disabled={isBuiltIn}
                className="font-semibold flex-1 max-w-[300px]"
              />
              <Badge variant={isBuiltIn ? "secondary" : "default"}>{activeDef.source}</Badge>
              {!isBuiltIn && (
                <>
                  <Button
                    data-testid="save-flow-btn"
                    size="sm"
                    onClick={handleSave}
                    disabled={!dirty || saving}
                  >
                    {saving ? "Saving..." : "Save"}
                  </Button>
                  <Button
                    data-testid="delete-flow-btn"
                    size="sm"
                    variant="outline"
                    onClick={handleDelete}
                    className="border-destructive text-destructive hover:bg-destructive/10"
                  >
                    {isNew ? "Discard" : "Delete"}
                  </Button>
                </>
              )}
            </div>
            <div className="flex-1 min-h-0" data-testid="flow-editor">
              <FlowEditor
                key={activeDef.id}
                flow={activeSpec}
                tools={editorTools}
                onChange={handleFlowChange}
                readOnly={isBuiltIn}
              />
            </div>
          </>
        ) : (
          <div
            data-testid="flow-empty-state"
            className="flex-1 flex items-center justify-center text-muted-foreground text-sm"
          >
            Select a flow or create a new one. Flows run server-side on content from any connector.
          </div>
        )}
      </div>
    </Card>
  );
}
