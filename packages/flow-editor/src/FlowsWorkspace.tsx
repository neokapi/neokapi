import { useCallback, useMemo, useState } from "react";
import {
  Badge,
  Button,
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Input,
  Label,
  cn,
} from "@neokapi/ui-primitives";
import { FlowEditor } from "./FlowEditor";
import { defToSpec, specToDef } from "./defAdapter";
import type { ComponentSchema, FlowDefinitionInfo, FlowSpec, ToolDoc, ToolInfo } from "./types";

/**
 * Data layer for {@link FlowsWorkspace}, supplied by each host app. The
 * container owns all list / new / save / delete UX and the canvas wiring; the
 * adapter is the only seam where the backend differs — kapi-desktop and the
 * Bowrain desktop use Wails bindings, the Bowrain web app uses REST + react-query.
 *
 * `saveFlow`/`deleteFlow` are responsible for refreshing whatever feeds `flows`
 * (query invalidation, an explicit refresh, …). The container reselects the flow
 * returned by `saveFlow`, so it must be the persisted, server-canonical record.
 */
export interface FlowsDataAdapter {
  /** Current flow definitions — built-in catalog merged with user/project flows. */
  flows: FlowDefinitionInfo[];
  /** Whether the flow list is still loading. */
  isLoading?: boolean;
  /** Persist a new (`isNew`) or existing flow; resolves to the saved record. */
  saveFlow: (def: FlowDefinitionInfo, isNew: boolean) => Promise<FlowDefinitionInfo>;
  /** Delete a persisted flow by id. */
  deleteFlow: (id: string) => Promise<void>;
}

export interface FlowsWorkspaceProps {
  /** Available tools for the palette + node enrichment (already in editor shape). */
  tools: ToolInfo[];
  /** Backend seam — see {@link FlowsDataAdapter}. */
  adapter: FlowsDataAdapter;
  /** Provenance assigned to newly-created flows. Default `"user"`. */
  newFlowSource?: FlowDefinitionInfo["source"];
  /** Generates a fresh flow id for new flows. Default `() => \`flow-${Date.now()}\``. */
  makeFlowId?: () => string;
  /** Flow sources rendered read-only (not editable/deletable). Default `["built-in"]`. */
  readOnlySources?: string[];
  /** Whether authoring new flows is allowed (e.g. a project is selected). Default `true`. */
  canAuthor?: boolean;
  /** Confirm before deleting a persisted flow. Default `false`. */
  confirmDelete?: boolean;
  /** Show a description input alongside the name in the toolbar. Default `false`. */
  showDescriptionInput?: boolean;
  /** Empty-state copy shown when no flow is selected. */
  emptyStateText?: string;
  /** Optional tool-schema resolver, threaded to the canvas config panel. */
  onGetSchema?: (toolName: string) => ComponentSchema | null;
  /** Optional tool-doc resolver, threaded to the canvas. */
  onGetDoc?: (toolName: string) => ToolDoc | null;
  /** Optional run handler; when present the canvas shows a Run button. */
  onRun?: (flow: FlowSpec) => void;
  /** Whether a run is in progress (disables the Run button). */
  runDisabled?: boolean;
  /** Outer container className (sizing / border / background). */
  className?: string;
}

interface FlowListProps {
  definitions: FlowDefinitionInfo[];
  activeId: string | null;
  isLoading?: boolean;
  canAuthor: boolean;
  onSelect: (def: FlowDefinitionInfo) => void;
  onNew: () => void;
}

function FlowList({ definitions, activeId, isLoading, canAuthor, onSelect, onNew }: FlowListProps) {
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
        {isLoading && (
          <div className="px-4 py-3 text-xs text-muted-foreground">Loading flows...</div>
        )}
        {definitions.map((def) => (
          <button
            key={def.id}
            data-testid={`flow-item-${def.id}`}
            onClick={() => onSelect(def)}
            className={cn(
              "w-full px-4 py-2.5 text-left border-none cursor-pointer text-[13px] text-foreground border-l-[3px]",
              activeId === def.id
                ? "border-l-primary bg-accent"
                : "border-l-transparent bg-transparent hover:bg-accent/50",
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

/**
 * Shared flow-management workspace: a flow list, new-flow dialog, edit toolbar,
 * and the canonical `@neokapi/flow-editor` <FlowEditor> canvas. The only thing
 * that varies between host apps is the data layer, supplied via {@link
 * FlowsDataAdapter}; everything else — selection, new/save/delete state, the
 * spec↔definition bridging — lives here so kapi-desktop, the Bowrain desktop
 * app, and the Bowrain web app share one implementation.
 */
export function FlowsWorkspace({
  tools,
  adapter,
  newFlowSource = "user",
  makeFlowId,
  readOnlySources = ["built-in"],
  canAuthor = true,
  confirmDelete = false,
  showDescriptionInput = false,
  emptyStateText = "Select a flow from the list or create a new one",
  onGetSchema,
  onGetDoc,
  onRun,
  runDisabled,
  className,
}: FlowsWorkspaceProps) {
  const [activeDef, setActiveDef] = useState<FlowDefinitionInfo | null>(null);
  const [activeSpec, setActiveSpec] = useState<FlowSpec | null>(null);
  const [editName, setEditName] = useState("");
  const [editDescription, setEditDescription] = useState("");
  const [dirty, setDirty] = useState(false);
  const [isNew, setIsNew] = useState(false);
  const [saving, setSaving] = useState(false);
  const [showNewDialog, setShowNewDialog] = useState(false);
  const [newName, setNewName] = useState("");
  const [newDescription, setNewDescription] = useState("");

  const isReadOnly = !!activeDef && readOnlySources.includes(activeDef.source);

  const handleSelect = useCallback((def: FlowDefinitionInfo) => {
    setActiveDef(def);
    setActiveSpec(defToSpec(def));
    setEditName(def.name);
    setEditDescription(def.description || "");
    setDirty(false);
    setIsNew(false);
  }, []);

  const handleNewDialogChange = useCallback((open: boolean) => {
    if (!open) {
      setNewName("");
      setNewDescription("");
    }
    setShowNewDialog(open);
  }, []);

  const handleNewCreate = useCallback(() => {
    const name = newName.trim();
    if (!name) return;
    const description = newDescription.trim();
    const id = makeFlowId ? makeFlowId() : `flow-${Date.now()}`;
    const def: FlowDefinitionInfo = {
      id,
      name,
      description,
      source: newFlowSource,
      nodes: [],
      edges: [],
    };
    setActiveDef(def);
    setActiveSpec({ description, steps: [] });
    setEditName(name);
    setEditDescription(description);
    setDirty(true);
    setIsNew(true);
    setShowNewDialog(false);
    setNewName("");
    setNewDescription("");
  }, [newName, newDescription, makeFlowId, newFlowSource]);

  const handleFlowChange = useCallback(
    (spec: FlowSpec) => {
      if (isReadOnly) return;
      setActiveSpec(spec);
      setDirty(true);
    },
    [isReadOnly],
  );

  const handleSave = useCallback(async () => {
    if (!activeDef || !activeSpec) return;
    const def = specToDef(
      { ...activeSpec, description: editDescription },
      {
        id: activeDef.id,
        name: editName,
        source: activeDef.source,
        description: editDescription,
      },
      tools,
    );
    setSaving(true);
    try {
      const saved = await adapter.saveFlow(def, isNew);
      handleSelect(saved);
    } catch (e) {
      console.error("Save flow failed:", e);
    } finally {
      setSaving(false);
    }
  }, [activeDef, activeSpec, editName, editDescription, tools, isNew, adapter, handleSelect]);

  const handleDelete = useCallback(async () => {
    if (!activeDef || isReadOnly) return;
    if (isNew) {
      setActiveDef(null);
      setActiveSpec(null);
      return;
    }
    if (confirmDelete && !window.confirm(`Delete flow "${activeDef.name}"?`)) return;
    try {
      await adapter.deleteFlow(activeDef.id);
      setActiveDef(null);
      setActiveSpec(null);
    } catch (e) {
      console.error("Delete flow failed:", e);
    }
  }, [activeDef, isReadOnly, isNew, confirmDelete, adapter]);

  const editorTools = useMemo(() => tools, [tools]);

  return (
    <div
      data-testid="flow-builder"
      className={cn(
        "flex min-h-0 overflow-hidden",
        className ?? "flex-1 rounded-lg border border-border",
      )}
    >
      <FlowList
        definitions={adapter.flows}
        activeId={activeDef?.id ?? null}
        isLoading={adapter.isLoading}
        canAuthor={canAuthor}
        onSelect={handleSelect}
        onNew={() => {
          setNewName("");
          setNewDescription("");
          setShowNewDialog(true);
        }}
      />

      <Dialog open={showNewDialog} onOpenChange={handleNewDialogChange}>
        <DialogContent onInteractOutside={(e: Event) => e.preventDefault()}>
          <DialogHeader>
            <DialogTitle>New Flow</DialogTitle>
          </DialogHeader>
          <div className="flex flex-col gap-4 py-2">
            <div className="flex flex-col gap-1.5">
              <Label className="text-muted-foreground">Name</Label>
              <Input
                data-testid="new-flow-name"
                value={newName}
                onChange={(e: React.ChangeEvent<HTMLInputElement>) => setNewName(e.target.value)}
                placeholder="My Flow"
                autoFocus
              />
            </div>
            <div className="flex flex-col gap-1.5">
              <Label className="text-muted-foreground">Description (optional)</Label>
              <Input
                data-testid="new-flow-description"
                value={newDescription}
                onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
                  setNewDescription(e.target.value)
                }
                placeholder="What this flow does..."
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => handleNewDialogChange(false)}>
              Cancel
            </Button>
            <Button onClick={handleNewCreate} disabled={!newName.trim()}>
              Create
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

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
                disabled={isReadOnly}
                className={cn(
                  "font-semibold text-base flex-1 max-w-[300px]",
                  isReadOnly && "border-none bg-transparent",
                )}
              />
              {showDescriptionInput && (
                <Input
                  data-testid="flow-description-input"
                  value={editDescription}
                  onChange={(e: React.ChangeEvent<HTMLInputElement>) => {
                    setEditDescription(e.target.value);
                    setDirty(true);
                  }}
                  placeholder="Description..."
                  disabled={isReadOnly}
                  className={cn("text-sm flex-1", isReadOnly && "border-none bg-transparent")}
                />
              )}
              <Badge variant={isReadOnly ? "secondary" : "default"}>{activeDef.source}</Badge>
              {!isReadOnly && (
                <>
                  <Button
                    data-testid="save-flow-btn"
                    onClick={handleSave}
                    disabled={!dirty || saving}
                    size="sm"
                  >
                    {saving ? "Saving..." : "Save"}
                  </Button>
                  <Button
                    data-testid="delete-flow-btn"
                    onClick={handleDelete}
                    variant="outline"
                    size="sm"
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
                onGetSchema={onGetSchema}
                onGetDoc={onGetDoc}
                onRun={onRun}
                runDisabled={runDisabled}
                readOnly={isReadOnly}
              />
            </div>
          </>
        ) : (
          <div
            data-testid="flow-empty-state"
            className="flex-1 flex items-center justify-center text-muted-foreground text-sm"
          >
            {emptyStateText}
          </div>
        )}
      </div>
    </div>
  );
}
