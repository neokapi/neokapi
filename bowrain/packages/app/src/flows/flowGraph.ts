// A platform flow is persisted as a tool-node graph (`FlowDefinitionInfo`) and
// edited as an ordered step list (`LinearFlowSpec`). Both are the same flow;
// this module carries a definition across that line in each direction through
// the one graph<->steps conversion `@neokapi/flow-editor` owns, and names the
// steps for the flow card's chip strip.

import { defToSpec, specToDef } from "@neokapi/flow-editor";
import type {
  FlowDefinitionInfo as EditorFlowDefinition,
  FlowNodeInfo as EditorFlowNode,
  ToolInfo as EditorToolInfo,
} from "@neokapi/flow-editor";
import { t } from "@neokapi/i18n-react/runtime";
import type { FlowDefinitionInfo, LinearFlowSpec, LinearFlowStep, ToolInfo } from "@neokapi/ui";

/** What naming a step needs of a tool; both the platform's and the editor's tool shapes carry it. */
export interface FlowToolName {
  name: string;
  display_name?: string;
}

/**
 * The platform's tool list in the flow editor's shape: the diagram reads the
 * IO contract and the transformer flag off it, the step editor the name and
 * description.
 */
export function toEditorTools(tools: ToolInfo[]): EditorToolInfo[] {
  return tools.map((tool) => ({
    name: tool.name,
    display_name: tool.display_name,
    description: tool.description,
    category: tool.category,
    source: tool.source,
    tags: tool.tags,
    requires: tool.requires,
    cardinality: tool.cardinality as EditorToolInfo["cardinality"],
    default_locale: tool.default_locale,
    consumes: tool.consumes,
    produces: tool.produces,
    side_effects: tool.side_effects,
    isSourceTransform: tool.is_source_transform,
  }));
}

/** The provenance values the conversion distinguishes. */
function editorSource(source: string): EditorFlowDefinition["source"] {
  if (source === "built-in" || source === "user") return source;
  return "project";
}

/**
 * The definition's tool-node graph in the shape the conversion reads. A flow
 * owns no I/O, so any reader or writer node a stored definition still carries
 * is left out, along with the edges that touch it.
 */
export function toEditorDefinition(def: FlowDefinitionInfo): EditorFlowDefinition {
  const nodes: EditorFlowNode[] = def.nodes
    .filter((n) => n.type === "tool")
    .map((n) => ({
      id: n.id,
      type: "tool",
      name: n.name,
      ...(n.label ? { label: n.label } : {}),
      ...(n.config ? { config: n.config } : {}),
      position: n.position,
    }));
  const ids = new Set(nodes.map((n) => n.id));
  return {
    id: def.id,
    name: def.name,
    ...(def.description ? { description: def.description } : {}),
    source: editorSource(def.source),
    nodes,
    edges: def.edges.filter((e) => ids.has(e.source) && ids.has(e.target)),
  };
}

/** The step list the linear editor edits, read from a persisted definition. */
export function definitionToSpec(def: FlowDefinitionInfo): LinearFlowSpec {
  return defToSpec(toEditorDefinition(def));
}

/**
 * The definition to persist for an edited step list, keeping the flow's id
 * and name. The server owns provenance and stamps every stored flow as a
 * project flow.
 */
export function specToDefinition(
  spec: LinearFlowSpec,
  flow: Pick<FlowDefinitionInfo, "id" | "name" | "description">,
): FlowDefinitionInfo {
  const def = specToDef(spec, {
    id: flow.id,
    name: flow.name,
    source: "project",
    description: flow.description,
  });
  return {
    id: def.id,
    name: def.name,
    description: def.description,
    source: def.source,
    nodes: def.nodes,
    edges: def.edges,
  };
}

/** A tool's name as shown to a reader: its display name, else its id. */
export function toolDisplayName(toolName: string, tools: FlowToolName[]): string {
  return tools.find((tool) => tool.name === toolName)?.display_name || toolName;
}

/** A step's name for a chip: its own label, else its tool's name, else the group. */
export function stepName(step: LinearFlowStep, tools: FlowToolName[]): string {
  if (step.label) return step.label;
  if (Array.isArray(step.parallel)) return t("Parallel group");
  return toolDisplayName(step.tool, tools);
}

/** The definition's steps as ordered names, for the flow card's chip strip. */
export function flowStepNames(def: FlowDefinitionInfo, tools: FlowToolName[]): string[] {
  return definitionToSpec(def).steps.map((step) => stepName(step, tools));
}
