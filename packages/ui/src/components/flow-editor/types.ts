// Types for the shared linear flow editor.
//
// A flow is an ordered pipeline of tool steps. These shapes mirror the wire
// model both kapi and bowrain persist in the recipe (`core/flow.StepsSpec`), so
// a host can pass its own flow objects straight in. TypeScript's structural
// typing accepts any object that carries these fields, extra fields and all.
// The editor depends on no host's API types.

export interface FlowStep {
  /** The tool the step runs; empty on a parallel group, which fans out to `parallel`. */
  tool: string;
  /** The step's inline options, merged over the tool's defaults. */
  config?: Record<string, unknown>;
  /** An override label for the step; defaults to the tool's name. */
  label?: string;
  /** Fan-out: run these branches in parallel instead of a single tool. */
  parallel?: FlowStep[];
}

export interface FlowSpec {
  /** A one-line outcome for the flow, shown in the header. */
  description?: string;
  steps: FlowStep[];
  /** Where content enters the flow (a wire locator); left untouched by the editor. */
  source?: string;
  /** Where content leaves the flow (a wire locator); left untouched by the editor. */
  sink?: string;
}

/**
 * The subset of a tool's metadata the linear editor renders. A host's fuller
 * tool type (the desktop's `ToolInfo`, bowrain's equivalent) is assignable to
 * this because it carries at least these fields.
 */
export interface FlowTool {
  name: string;
  display_name?: string;
  description: string;
  /** Whether the tool has an option schema; the row shows options only if so. */
  has_schema?: boolean;
}
