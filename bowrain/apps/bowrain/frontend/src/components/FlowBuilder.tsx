import { useState, useCallback, useMemo, useRef, useReducer } from "react";
import {
  Button,
  Input,
  Badge,
  cn,
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  Label,
} from "@neokapi/ui";
import {
  FlowEditor,
  defToSpec,
  specToDef,
  type FlowSpec,
  type ToolInfo as EditorToolInfo,
  type ComponentSchema,
} from "@neokapi/flow-editor";
import { useFlowDefinitions, useFlowDefinitionApi, useTools } from "../hooks/useApi";
import type { FlowDefinitionInfo } from "../types/api";

// Wails v3 bindings — used directly for the optional tool-schema lookup, which
// the FlowEditor requests synchronously and we resolve via a cache.
// eslint-disable-next-line @typescript-eslint/ban-ts-comment
// @ts-ignore – generated .js bindings outside the TS project root
import * as Backend from "../../bindings/github.com/neokapi/neokapi/bowrain/apps/bowrain/backend/app.js";
import { optionalBinding } from "../api/optionalBinding";

// --- Flow List ---------------------------------------------------------------

function FlowList({
  definitions,
  activeId,
  onSelect,
  onNew,
  canAuthor,
}: {
  definitions: FlowDefinitionInfo[];
  activeId: string | null;
  onSelect: (def: FlowDefinitionInfo) => void;
  onNew: () => void;
  canAuthor: boolean;
}) {
  return (
    <div
      data-testid="flow-list"
      className="w-60 border-r border-border flex flex-col overflow-hidden"
    >
      <div className="px-4 py-3 border-b border-border flex justify-between items-center">
        <span className="font-semibold text-sm text-foreground">Flows</span>
        <Button
          data-testid="new-flow-btn"
          onClick={onNew}
          size="sm"
          disabled={!canAuthor}
          title={canAuthor ? undefined : "Select a project to author flows"}
        >
          + New
        </Button>
      </div>
      <div className="flex-1 overflow-auto py-1">
        {definitions.map((def) => (
          <button
            key={def.id}
            data-testid={`flow-item-${def.id}`}
            onClick={() => onSelect(def)}
            className={cn(
              "w-full px-4 py-2.5 text-left border-none cursor-pointer text-[13px] text-foreground",
              activeId === def.id
                ? "border-l-[3px] border-l-primary bg-accent"
                : "border-l-[3px] border-l-transparent bg-transparent hover:bg-accent/50",
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
  );
}

// --- Main FlowBuilder Component ----------------------------------------------

/**
 * Visual flow builder for the Bowrain desktop app.
 *
 * The editing canvas is the shared `@neokapi/flow-editor` <FlowEditor>, the same
 * component kapi-desktop uses. This component owns the list / new / delete / save
 * UX; the underlying Wails data calls (List/Get/Save/DeleteFlowDefinition) are
 * project-scoped and proxy to the Bowrain server's flow-definition REST API
 * (#766) — the desktop no longer authors flows to a local store. It bridges the
 * backend's node/edge FlowDefinitionInfo to the editor's steps-based FlowSpec
 * via the shared defToSpec / specToDef adapter.
 *
 * Flows are connector-agnostic, project-scoped server resources. A project must
 * be selected to author flows; without one, only the built-in catalog shows.
 */
export function FlowBuilder({ projectId }: { projectId?: string }) {
  const { definitions, refresh } = useFlowDefinitions(projectId ?? "");
  const { saveFlowDefinition, deleteFlowDefinition } = useFlowDefinitionApi(projectId ?? "");
  const { tools } = useTools();

  const [activeDef, setActiveDef] = useState<FlowDefinitionInfo | null>(null);
  const [activeSpec, setActiveSpec] = useState<FlowSpec | null>(null);
  const [editName, setEditName] = useState("");
  const [editDescription, setEditDescription] = useState("");
  const [dirty, setDirty] = useState(false);
  const [showNewFlowDialog, setShowNewFlowDialog] = useState(false);
  const [newFlowName, setNewFlowName] = useState("");
  const [newFlowDescription, setNewFlowDescription] = useState("");

  const isBuiltIn = activeDef?.source === "built-in";

  // Map bowrain's ToolInfo (snake_case is_source_transform) onto the editor's
  // ToolInfo (camelCase isSourceTransform). The editor uses this to gate the
  // source-transform stage toggle and to enrich nodes with category/description.
  const editorTools = useMemo<EditorToolInfo[]>(
    () =>
      tools.map((t) => ({
        name: t.name,
        description: t.description,
        category: t.category,
        tags: t.tags,
        requires: t.requires,
        cardinality: t.cardinality as EditorToolInfo["cardinality"],
        default_locale: t.default_locale,
        side_effects: t.side_effects,
        consumes: t.consumes,
        produces: t.produces,
        isSourceTransform: t.is_source_transform,
      })),
    [tools],
  );

  // Tool-schema cache. The backend may not expose GetToolSchema yet (the
  // generated bindings omit it); onGetSchema returns null until it does.
  const schemasRef = useRef<Record<string, ComponentSchema | null>>({});
  const fetchingRef = useRef<Set<string>>(new Set());
  const [, forceUpdate] = useReducer((x: number) => x + 1, 0);

  const handleGetSchema = useCallback((toolName: string): ComponentSchema | null => {
    if (toolName in schemasRef.current) {
      return schemasRef.current[toolName] ?? null;
    }
    if (fetchingRef.current.has(toolName)) return null;
    const fn = optionalBinding<(name: string) => Promise<ComponentSchema | null>>(
      Backend,
      "GetToolSchema",
    );
    if (!fn) {
      schemasRef.current[toolName] = null;
      return null;
    }
    fetchingRef.current.add(toolName);
    void fn(toolName)
      .then((result) => {
        schemasRef.current[toolName] = result ?? null;
      })
      .catch(() => {
        schemasRef.current[toolName] = null;
      })
      .finally(() => {
        fetchingRef.current.delete(toolName);
        forceUpdate();
      });
    return null;
  }, []);

  const handleSelect = useCallback((def: FlowDefinitionInfo) => {
    setActiveDef(def);
    setActiveSpec(defToSpec(def));
    setEditName(def.name);
    setEditDescription(def.description || "");
    setDirty(false);
  }, []);

  const handleNew = useCallback((name: string, description: string) => {
    const id = `custom-flow-${Date.now()}`;
    const def: FlowDefinitionInfo = {
      id,
      name,
      description,
      source: "user",
      nodes: [],
      edges: [],
    };
    setActiveDef(def);
    setActiveSpec({ description, steps: [] });
    setEditName(name);
    setEditDescription(description);
    setDirty(true);
  }, []);

  const handleNewFlowDialogOpen = useCallback(() => {
    setNewFlowName("");
    setNewFlowDescription("");
    setShowNewFlowDialog(true);
  }, []);

  const handleNewFlowDialogClose = useCallback((open: boolean) => {
    if (!open) {
      setNewFlowName("");
      setNewFlowDescription("");
    }
    setShowNewFlowDialog(open);
  }, []);

  const handleNewFlowCreate = useCallback(() => {
    if (!newFlowName.trim()) return;
    handleNew(newFlowName.trim(), newFlowDescription.trim());
    setShowNewFlowDialog(false);
    setNewFlowName("");
    setNewFlowDescription("");
  }, [newFlowName, newFlowDescription, handleNew]);

  // FlowEditor edits the spec; mark dirty and stash the latest spec.
  const handleFlowChange = useCallback(
    (spec: FlowSpec) => {
      if (isBuiltIn) return;
      setActiveSpec(spec);
      setDirty(true);
    },
    [isBuiltIn],
  );

  const handleSave = useCallback(async () => {
    if (!activeDef || !activeSpec) return;
    const def = specToDef(
      { ...activeSpec, description: editDescription },
      { id: activeDef.id, name: editName, source: "user", description: editDescription },
      editorTools,
    );
    try {
      const saved = await saveFlowDefinition(def);
      setActiveDef(saved);
      setActiveSpec(defToSpec(saved));
      setDirty(false);
      refresh();
    } catch (e) {
      console.error("Save flow failed:", e);
    }
  }, [activeDef, activeSpec, editName, editDescription, editorTools, saveFlowDefinition, refresh]);

  const handleDelete = useCallback(async () => {
    if (!activeDef || isBuiltIn) return;
    try {
      await deleteFlowDefinition(activeDef.id);
      setActiveDef(null);
      setActiveSpec(null);
      refresh();
    } catch (e) {
      console.error("Delete flow failed:", e);
    }
  }, [activeDef, isBuiltIn, deleteFlowDefinition, refresh]);

  return (
    <div
      data-testid="flow-builder"
      className="flex flex-1 min-h-0 rounded-lg border border-border overflow-hidden"
    >
      <FlowList
        definitions={definitions}
        activeId={activeDef?.id || null}
        onSelect={handleSelect}
        onNew={handleNewFlowDialogOpen}
        canAuthor={!!projectId}
      />
      <Dialog open={showNewFlowDialog} onOpenChange={handleNewFlowDialogClose}>
        <DialogContent onInteractOutside={(e: Event) => e.preventDefault()}>
          <DialogHeader>
            <DialogTitle>New Flow</DialogTitle>
          </DialogHeader>
          <div className="flex flex-col gap-4 py-2">
            <div className="flex flex-col gap-1">
              <Label className="text-muted-foreground">Name</Label>
              <Input
                value={newFlowName}
                onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
                  setNewFlowName(e.target.value)
                }
                placeholder="My Flow"
                data-testid="new-flow-name"
                autoFocus
              />
            </div>
            <div className="flex flex-col gap-1">
              <Label className="text-muted-foreground">Description (optional)</Label>
              <Input
                value={newFlowDescription}
                onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
                  setNewFlowDescription(e.target.value)
                }
                placeholder="What this flow does..."
                data-testid="new-flow-description"
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => handleNewFlowDialogClose(false)}>
              Cancel
            </Button>
            <Button onClick={handleNewFlowCreate} disabled={!newFlowName.trim()}>
              Create
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <div className="flex-1 flex flex-col min-h-0">
        {activeDef && activeSpec ? (
          <>
            {/* Toolbar */}
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
                className={cn(
                  "font-semibold text-base flex-1 max-w-[300px]",
                  isBuiltIn && "border-none bg-transparent",
                )}
              />
              <Input
                data-testid="flow-description-input"
                value={editDescription}
                onChange={(e: React.ChangeEvent<HTMLInputElement>) => {
                  setEditDescription(e.target.value);
                  setDirty(true);
                }}
                placeholder="Description..."
                disabled={isBuiltIn}
                className={cn("text-sm flex-1", isBuiltIn && "border-none bg-transparent")}
              />
              <Badge variant={isBuiltIn ? "secondary" : "default"}>{activeDef.source}</Badge>
              {!isBuiltIn && (
                <>
                  <Button
                    data-testid="save-flow-btn"
                    onClick={handleSave}
                    disabled={!dirty}
                    size="sm"
                  >
                    Save
                  </Button>
                  <Button
                    data-testid="delete-flow-btn"
                    onClick={handleDelete}
                    variant="outline"
                    size="sm"
                    className="border-destructive text-destructive hover:bg-destructive/10"
                  >
                    Delete
                  </Button>
                </>
              )}
            </div>
            {/* Shared flow editor canvas */}
            <div className="flex-1 min-h-0" data-testid="flow-editor">
              <FlowEditor
                key={activeDef.id}
                flow={activeSpec}
                tools={editorTools}
                onChange={handleFlowChange}
                onGetSchema={handleGetSchema}
                readOnly={isBuiltIn}
              />
            </div>
          </>
        ) : (
          <div
            data-testid="flow-empty-state"
            className="flex-1 flex items-center justify-center text-muted-foreground text-sm"
          >
            Select a flow from the list or create a new one
          </div>
        )}
      </div>
    </div>
  );
}
