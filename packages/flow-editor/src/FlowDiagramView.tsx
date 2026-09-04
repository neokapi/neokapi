// The read-only diagram of a flow: the node canvas as a view, never an editor.
//
// Authoring happens in the shared linear step editor. The canvas earns its
// place as a picture of the same steps: per-branch parallel fan-out, the typed
// IO contract on every node with the unmet-input and placement diagnostics,
// and, when a run is loaded, the replay (edge playback, per-step inspector,
// scrub). It renders the canonical FlowEditor locked down, so the layout and
// the diagnostics stay one implementation; nothing here converts steps itself.

import { t } from "@neokapi/i18n-react/runtime";
import { cn } from "@neokapi/ui-primitives";
import { FlowEditor } from "./FlowEditor";
import type { FlowEditorProps, FlowSpec, ToolInfo, ComponentSchema, ToolDoc } from "./types";
import type { FlowTrace, TraceEvent } from "./traceTypes";

export interface FlowDiagramViewProps {
  /** The flow to draw, in steps format. */
  flow: FlowSpec;
  /** Tool metadata for the nodes (category, IO contract, transformer flags). */
  tools: ToolInfo[];
  /** Resolve a tool's option schema, shown read-only in the step inspector. */
  onGetSchema?: (toolName: string) => ComponentSchema | null;
  /** Resolve a tool's documentation for the step inspector. */
  onGetDoc?: (toolName: string) => ToolDoc | null;
  /** Project-level tool presets, badged on the nodes (see FlowEditorProps). */
  projectPresets?: Record<string, Record<string, unknown>>;
  /**
   * A recorded run of THIS flow. When set the view replays it: a playback
   * transport, parts flowing along the edges, and a run inspector per node.
   */
  trace?: FlowTrace;
  /** Pre-mapped trace events (`tool-<i>` node ids) for a host without full snapshots. */
  traceEvents?: TraceEvent[];
  /** Called when the reader dismisses the loaded run. */
  onTraceDismiss?: () => void;
  /** Host-rendered inspector for the Source / Sink endpoints (see FlowEditorProps). */
  renderEndpointPanel?: FlowEditorProps["renderEndpointPanel"];
  className?: string;
}

// The view never changes the flow; the editor's onChange is required, so it
// gets a handler that does nothing. Every authoring path checks `readOnly`
// before it would call this.
const ignoreChange = () => {};

const READ_ONLY = { readOnly: true, endpointsReadOnly: true } as const;

export function FlowDiagramView({
  flow,
  tools,
  onGetSchema,
  onGetDoc,
  projectPresets,
  trace,
  traceEvents,
  onTraceDismiss,
  renderEndpointPanel,
  className,
}: FlowDiagramViewProps) {
  if (flow.steps.length === 0) {
    return (
      <div
        className={cn("flex h-full items-center justify-center p-6", className)}
        data-testid="flow-diagram-view"
      >
        <p className="text-sm text-muted-foreground">
          {t("This flow has no steps yet. Add steps to see them as a diagram.")}
        </p>
      </div>
    );
  }
  return (
    <div className={cn("h-full min-h-0", className)} data-testid="flow-diagram-view">
      <FlowEditor
        flow={flow}
        tools={tools}
        onChange={ignoreChange}
        access={READ_ONLY}
        hideToolbar
        onGetSchema={onGetSchema}
        onGetDoc={onGetDoc}
        projectPresets={projectPresets}
        trace={trace}
        traceEvents={traceEvents}
        onTraceDismiss={onTraceDismiss}
        renderEndpointPanel={renderEndpointPanel}
      />
    </div>
  );
}
