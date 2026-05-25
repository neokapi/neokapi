import { useState, useCallback, useMemo, useRef } from "react";
import {
  ReactFlow,
  Controls,
  Background,
  MiniMap,
  addEdge,
  useNodesState,
  useEdgesState,
  BackgroundVariant,
  type Connection,
  type Node,
  type Edge,
  type NodeTypes,
  type NodeProps,
  Handle,
  Position,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import {
  Button,
  Input,
  Badge,
  cn,
  Label,
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  SchemaForm,
  Switch,
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
  type ComponentSchema,
} from "@neokapi/ui";
import { useFlowDefinitions, useFlowDefinitionApi, useTools, useToolSchema } from "../hooks/useApi";
import type { FlowDefinitionInfo, FlowNodeInfo, FlowEdgeInfo, ToolInfo } from "../types/api";

// --- Constants ---

const STAGE_SOURCE_TRANSFORM = "source-transform";

// --- Custom Node Components ---

const nodeColors: Record<
  string,
  { bg: string; border: string; label: string; text: string; sub: string }
> = {
  reader: {
    bg: "rgba(34, 197, 94, 0.12)",
    border: "#22c55e",
    label: "Input",
    text: "#e4e4e7",
    sub: "#86efac",
  },
  writer: {
    bg: "rgba(96, 165, 250, 0.12)",
    border: "#60a5fa",
    label: "Output",
    text: "#e4e4e7",
    sub: "#93c5fd",
  },
  tool: {
    bg: "rgba(148, 163, 184, 0.08)",
    border: "#64748b",
    label: "Tool",
    text: "#e4e4e7",
    sub: "#94a3b8",
  },
  // Source-transform tools get a distinct amber accent.
  sourceTransform: {
    bg: "rgba(245, 158, 11, 0.12)",
    border: "#f59e0b",
    label: "Source Transform",
    text: "#e4e4e7",
    sub: "#fcd34d",
  },
};

function ReaderNode({ data }: NodeProps) {
  const colors = nodeColors.reader;
  return (
    <div
      data-testid={`flow-node-${String(data.nodeId)}`}
      className="px-4 py-2.5 rounded-lg min-w-[140px] text-center text-[13px]"
      style={{ border: `2px solid ${colors.border}`, background: colors.bg, color: colors.text }}
    >
      <div className="text-[10px] font-semibold mb-0.5" style={{ color: colors.border }}>
        INPUT
      </div>
      <div className="font-semibold">{(data.label as string) || "Reader"}</div>
      <div className="text-[11px] mt-0.5" style={{ color: colors.sub }}>
        {data.formatName as string}
      </div>
      <Handle type="source" position={Position.Right} style={{ background: colors.border }} />
    </div>
  );
}

function WriterNode({ data }: NodeProps) {
  const colors = nodeColors.writer;
  return (
    <div
      data-testid={`flow-node-${String(data.nodeId)}`}
      className="px-4 py-2.5 rounded-lg min-w-[140px] text-center text-[13px]"
      style={{ border: `2px solid ${colors.border}`, background: colors.bg, color: colors.text }}
    >
      <Handle type="target" position={Position.Left} style={{ background: colors.border }} />
      <div className="text-[10px] font-semibold mb-0.5" style={{ color: colors.border }}>
        OUTPUT
      </div>
      <div className="font-semibold">{(data.label as string) || "Writer"}</div>
      <div className="text-[11px] mt-0.5" style={{ color: colors.sub }}>
        {data.formatName as string}
      </div>
    </div>
  );
}

function ToolNode({ data, selected }: NodeProps) {
  const isSourceTransform = data.stage === STAGE_SOURCE_TRANSFORM;
  const colors = isSourceTransform ? nodeColors.sourceTransform : nodeColors.tool;
  const activeBorder = isSourceTransform ? colors.border : "var(--accent, #6366f1)";

  return (
    <div
      data-testid={`flow-node-${String(data.nodeId)}`}
      data-stage={isSourceTransform ? STAGE_SOURCE_TRANSFORM : "main"}
      className={cn(
        "px-4 py-2.5 rounded-lg min-w-[140px] text-center text-[13px]",
        selected && "ring-2 ring-primary/30",
      )}
      style={{
        border: `2px solid ${selected ? activeBorder : colors.border}`,
        background: selected
          ? isSourceTransform
            ? "rgba(245, 158, 11, 0.22)"
            : "rgba(99, 102, 241, 0.15)"
          : colors.bg,
        color: colors.text,
      }}
    >
      <Handle type="target" position={Position.Left} style={{ background: colors.border }} />
      <div className="text-[10px] font-semibold mb-0.5" style={{ color: colors.sub }}>
        {isSourceTransform ? "SOURCE TRANSFORM" : "TOOL"}
      </div>
      <div className="font-semibold">{(data.label as string) || (data.toolName as string)}</div>
      {data.description ? (
        <div className="text-[11px] mt-0.5" style={{ color: colors.sub }}>
          {data.description as string}
        </div>
      ) : null}
      {isSourceTransform && (
        <div className="mt-1.5 flex justify-center">
          <span
            className="inline-flex items-center px-1.5 py-0.5 rounded text-[9px] font-bold uppercase tracking-wider"
            style={{
              background: "rgba(245, 158, 11, 0.25)",
              color: "#f59e0b",
              border: "1px solid rgba(245, 158, 11, 0.4)",
            }}
          >
            source-transform
          </span>
        </div>
      )}
      <Handle type="source" position={Position.Right} style={{ background: colors.border }} />
    </div>
  );
}

const nodeTypes: NodeTypes = {
  reader: ReaderNode,
  writer: WriterNode,
  tool: ToolNode,
};

// --- Conversion between API types and React Flow types ---

function defToReactFlow(def: FlowDefinitionInfo): { nodes: Node[]; edges: Edge[] } {
  const nodes: Node[] = def.nodes.map((n: FlowNodeInfo) => ({
    id: n.id,
    type: n.type,
    position: { x: n.position.x, y: n.position.y },
    data: {
      label: n.label || n.name,
      toolName: n.name,
      formatName: n.name === "auto" ? "Auto-detect" : n.name,
      nodeId: n.id,
      config: n.config || {},
      stage: n.stage || "",
    },
  }));
  const edges: Edge[] = def.edges.map((e: FlowEdgeInfo) => ({
    id: e.id,
    source: e.source,
    target: e.target,
    animated: true,
    style: { stroke: "#6366f1", strokeWidth: 2 },
  }));
  return { nodes, edges };
}

function reactFlowToDef(
  id: string,
  name: string,
  description: string,
  nodes: Node[],
  edges: Edge[],
  source: string,
): FlowDefinitionInfo {
  return {
    id,
    name,
    description,
    source,
    nodes: nodes.map((n) => ({
      id: n.id,
      type: (n.type || "tool") as "tool" | "reader" | "writer",
      name: (n.data.toolName as string) || n.id,
      label: (n.data.label as string) || "",
      stage: (n.data.stage as string) || undefined,
      config: (n.data.config as Record<string, unknown>) || undefined,
      position: { x: n.position.x, y: n.position.y },
    })),
    edges: edges.map((e) => ({
      id: e.id,
      source: e.source,
      target: e.target,
    })),
  };
}

// --- Tool Palette ---

function ToolPalette({
  tools,
  onAddTool,
}: {
  tools: ToolInfo[];
  onAddTool: (tool: ToolInfo) => void;
}) {
  return (
    <div
      data-testid="tool-palette"
      className="p-3 border-b border-border flex gap-2 flex-wrap items-center"
    >
      <span className="text-xs font-semibold text-muted-foreground">Add Tool:</span>
      {tools.map((tool) => (
        <Button
          key={tool.name}
          data-testid={`add-tool-${tool.name}`}
          onClick={() => onAddTool(tool)}
          variant="outline"
          size="sm"
          className="text-xs"
        >
          {tool.name}
        </Button>
      ))}
    </div>
  );
}

// --- Flow List ---

function FlowList({
  definitions,
  activeId,
  onSelect,
  onNew,
}: {
  definitions: FlowDefinitionInfo[];
  activeId: string | null;
  onSelect: (def: FlowDefinitionInfo) => void;
  onNew: () => void;
}) {
  return (
    <div
      data-testid="flow-list"
      className="w-60 border-r border-border flex flex-col overflow-hidden"
    >
      <div className="px-4 py-3 border-b border-border flex justify-between items-center">
        <span className="font-semibold text-sm text-foreground">Flows</span>
        <Button data-testid="new-flow-btn" onClick={onNew} size="sm">
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

// --- Tool Config Panel ---

function ToolConfigPanel({
  toolName,
  stage,
  isSourceTransformCapable,
  config,
  onChange,
  onStageChange,
}: {
  toolName: string;
  stage: string;
  isSourceTransformCapable: boolean;
  config: Record<string, unknown>;
  onChange: (config: Record<string, unknown>) => void;
  onStageChange: (stage: string) => void;
}) {
  const { schema, loading } = useToolSchema(toolName);
  const isSourceTransform = stage === STAGE_SOURCE_TRANSFORM;

  return (
    <div className="flex flex-col">
      {/* Stage toggle section */}
      <div className="px-4 py-3 border-b border-border">
        <div className="flex items-center justify-between gap-2">
          <div className="flex flex-col gap-0.5">
            <Label className="text-sm font-medium text-foreground">Source Transform</Label>
            <span className="text-[11px] text-muted-foreground leading-tight">
              Runs before main tools to settle the source model
            </span>
          </div>
          <TooltipProvider>
            <Tooltip>
              <TooltipTrigger asChild>
                {/* Wrap in a span so Tooltip works when Switch is disabled */}
                <span className="inline-flex">
                  <Switch
                    data-testid="source-transform-toggle"
                    checked={isSourceTransform}
                    disabled={!isSourceTransformCapable}
                    onCheckedChange={(checked: boolean) =>
                      onStageChange(checked ? STAGE_SOURCE_TRANSFORM : "")
                    }
                    aria-label="Enable source-transform stage"
                  />
                </span>
              </TooltipTrigger>
              <TooltipContent side="left" className="max-w-[200px] text-xs">
                {isSourceTransformCapable
                  ? "This tool can rewrite the source/model. Enable to run it in the source-transform stage before the main tools."
                  : "This tool does not support the source-transform stage. Only tools that rewrite source content (e.g. redact, simplify, normalize) can be placed here."}
              </TooltipContent>
            </Tooltip>
          </TooltipProvider>
        </div>
        {isSourceTransform && (
          <div
            className="mt-2 px-2 py-1.5 rounded text-[11px]"
            style={{
              background: "rgba(245, 158, 11, 0.1)",
              color: "#f59e0b",
              border: "1px solid rgba(245, 158, 11, 0.3)",
            }}
          >
            Runs ahead of main tools — downstream sees a settled source.
          </div>
        )}
      </div>

      {/* Tool schema config */}
      {loading ? (
        <div className="p-4 text-sm text-muted-foreground">Loading schema...</div>
      ) : !schema || !schema.properties || Object.keys(schema.properties).length === 0 ? (
        <div className="p-4 text-sm text-muted-foreground">
          No configurable parameters for <span className="font-mono">{toolName}</span>
        </div>
      ) : (
        <div className="p-4 space-y-4">
          <div>
            <h3 className="text-sm font-semibold text-foreground">{schema.title || toolName}</h3>
            {schema.description && (
              <p className="text-xs text-muted-foreground mt-1">{schema.description}</p>
            )}
          </div>
          <SchemaForm
            schema={schema as unknown as ComponentSchema}
            values={config}
            onChange={onChange}
          />
        </div>
      )}
    </div>
  );
}

// --- Main FlowBuilder Component ---

export function FlowBuilder() {
  const { definitions, refresh } = useFlowDefinitions();
  const { saveFlowDefinition, deleteFlowDefinition } = useFlowDefinitionApi();
  const { tools } = useTools();

  const [activeDef, setActiveDef] = useState<FlowDefinitionInfo | null>(null);
  const [editName, setEditName] = useState("");
  const [editDescription, setEditDescription] = useState("");
  const [dirty, setDirty] = useState(false);
  const [showNewFlowDialog, setShowNewFlowDialog] = useState(false);
  const [newFlowName, setNewFlowName] = useState("");
  const [newFlowDescription, setNewFlowDescription] = useState("");
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null);
  const nodeCounter = useRef(0);

  const [nodes, setNodes, onNodesChange] = useNodesState<Node>([]);
  const [edges, setEdges, onEdgesChange] = useEdgesState<Edge>([]);

  // Track the currently selected tool node
  const selectedToolNode = useMemo(() => {
    if (!selectedNodeId) return null;
    const node = nodes.find((n) => n.id === selectedNodeId);
    return node?.type === "tool" ? node : null;
  }, [selectedNodeId, nodes]);

  // Build a lookup map from tool name to ToolInfo for capability checks.
  const toolInfoMap = useMemo(() => {
    const map = new Map<string, ToolInfo>();
    for (const t of tools) {
      map.set(t.name, t);
    }
    return map;
  }, [tools]);

  const isBuiltIn = activeDef?.source === "built-in";

  const onConnect = useCallback(
    (connection: Connection) => {
      setEdges((eds) =>
        addEdge(connection, eds).map((e) => ({
          ...e,
          animated: true,
          style: { stroke: "#6366f1", strokeWidth: 2 },
        })),
      );
      setDirty(true);
    },
    [setEdges],
  );

  const handleSelect = useCallback(
    (def: FlowDefinitionInfo) => {
      setActiveDef(def);
      setEditName(def.name);
      setEditDescription(def.description || "");
      const { nodes: n, edges: e } = defToReactFlow(def);
      setNodes(n);
      setEdges(e);
      setDirty(false);
    },
    [setNodes, setEdges],
  );

  const handleNew = useCallback(
    (name: string, description: string) => {
      const id = `custom-flow-${Date.now()}`;
      const def: FlowDefinitionInfo = {
        id,
        name,
        description,
        source: "user",
        nodes: [
          {
            id: "reader",
            type: "reader",
            name: "auto",
            label: "Input",
            position: { x: 0, y: 100 },
          },
          {
            id: "writer",
            type: "writer",
            name: "auto",
            label: "Output",
            position: { x: 500, y: 100 },
          },
        ],
        edges: [],
      };
      setActiveDef(def);
      setEditName(def.name);
      setEditDescription(description);
      const { nodes: n, edges: e } = defToReactFlow(def);
      setNodes(n);
      setEdges(e);
      setDirty(true);
    },
    [setNodes, setEdges],
  );

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

  const handleAddTool = useCallback(
    (tool: ToolInfo) => {
      nodeCounter.current++;
      const id = `tool-${nodeCounter.current}-${Date.now()}`;
      const maxX = nodes.reduce((max, n) => Math.max(max, n.position.x), 0);
      const newNode: Node = {
        id,
        type: "tool",
        position: { x: Math.min(maxX + 200, 400), y: 100 },
        data: {
          label: tool.name,
          toolName: tool.name,
          description: tool.description,
          nodeId: id,
          stage: "",
          config: {},
        },
      };
      setNodes((prev) => [...prev, newNode]);
      setDirty(true);
    },
    [nodes, setNodes],
  );

  const handleSave = useCallback(async () => {
    if (!activeDef) return;
    const def = reactFlowToDef(activeDef.id, editName, editDescription, nodes, edges, "user");
    try {
      const saved = await saveFlowDefinition(def);
      setActiveDef(saved);
      setDirty(false);
      refresh();
    } catch (e) {
      console.error("Save flow failed:", e);
    }
  }, [activeDef, editName, editDescription, nodes, edges, saveFlowDefinition, refresh]);

  const handleDelete = useCallback(async () => {
    if (!activeDef || isBuiltIn) return;
    try {
      await deleteFlowDefinition(activeDef.id);
      setActiveDef(null);
      setNodes([]);
      setEdges([]);
      refresh();
    } catch (e) {
      console.error("Delete flow failed:", e);
    }
  }, [activeDef, isBuiltIn, deleteFlowDefinition, setNodes, setEdges, refresh]);

  const handleNodeClick = useCallback((_: React.MouseEvent, node: Node) => {
    setSelectedNodeId(node.id);
  }, []);

  const handlePaneClick = useCallback(() => {
    setSelectedNodeId(null);
  }, []);

  const handleToolConfigChange = useCallback(
    (config: Record<string, unknown>) => {
      if (!selectedToolNode) return;
      setNodes((prev) =>
        prev.map((n) => (n.id === selectedToolNode.id ? { ...n, data: { ...n.data, config } } : n)),
      );
      setDirty(true);
    },
    [selectedToolNode, setNodes],
  );

  const handleToolStageChange = useCallback(
    (stage: string) => {
      if (!selectedToolNode) return;
      setNodes((prev) =>
        prev.map((n) => (n.id === selectedToolNode.id ? { ...n, data: { ...n.data, stage } } : n)),
      );
      setDirty(true);
    },
    [selectedToolNode, setNodes],
  );

  const handleNodesChange = useCallback(
    (...args: Parameters<typeof onNodesChange>) => {
      onNodesChange(...args);
      setDirty(true);
    },
    [onNodesChange],
  );

  const handleEdgesChange = useCallback(
    (...args: Parameters<typeof onEdgesChange>) => {
      onEdgesChange(...args);
      setDirty(true);
    },
    [onEdgesChange],
  );

  const miniMapNodeColor = useMemo(
    () => (node: Node) => {
      if (node.type === "reader") return nodeColors.reader.border;
      if (node.type === "writer") return nodeColors.writer.border;
      if (node.data?.stage === STAGE_SOURCE_TRANSFORM) return nodeColors.sourceTransform.border;
      return nodeColors.tool.border;
    },
    [],
  );

  // Determine if the selected tool is source-transform capable.
  const selectedToolCapable = useMemo(() => {
    if (!selectedToolNode) return false;
    const toolName = selectedToolNode.data.toolName as string;
    return toolInfoMap.get(toolName)?.is_source_transform ?? false;
  }, [selectedToolNode, toolInfoMap]);

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
        {activeDef ? (
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
            {/* Tool palette (only for editable flows) */}
            {!isBuiltIn && <ToolPalette tools={tools} onAddTool={handleAddTool} />}
            {/* React Flow canvas + config panel */}
            <div className="flex flex-1 min-h-0">
              <div className="flex-1 min-h-0">
                <ReactFlow
                  nodes={nodes}
                  edges={edges}
                  onNodesChange={handleNodesChange}
                  onEdgesChange={handleEdgesChange}
                  onConnect={onConnect}
                  onNodeClick={handleNodeClick}
                  onPaneClick={handlePaneClick}
                  nodeTypes={nodeTypes}
                  fitView
                  nodesDraggable={!isBuiltIn}
                  nodesConnectable={!isBuiltIn}
                  elementsSelectable
                  deleteKeyCode={isBuiltIn ? null : "Backspace"}
                  proOptions={{ hideAttribution: true }}
                  className="bg-background"
                >
                  <Background variant={BackgroundVariant.Dots} gap={16} size={1} color="#3e4047" />
                  <Controls />
                  <MiniMap
                    nodeColor={miniMapNodeColor}
                    className="!bg-card"
                    style={{ height: 80 }}
                    maskColor="rgba(0, 0, 0, 0.4)"
                  />
                </ReactFlow>
              </div>
              {/* Tool config side panel */}
              {selectedToolNode && !isBuiltIn && (
                <div
                  data-testid="tool-config-panel"
                  className="w-72 border-l border-border overflow-auto"
                >
                  <ToolConfigPanel
                    toolName={selectedToolNode.data.toolName as string}
                    stage={(selectedToolNode.data.stage as string) || ""}
                    isSourceTransformCapable={selectedToolCapable}
                    config={(selectedToolNode.data.config as Record<string, unknown>) || {}}
                    onChange={handleToolConfigChange}
                    onStageChange={handleToolStageChange}
                  />
                </div>
              )}
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
