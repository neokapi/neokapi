// The flow workbench every surface renders: a view switch over one flow.
//
// Steps is the authoring surface (the shared linear step editor). Diagram is
// the same steps drawn as a read-only canvas with the IO diagnostics. Run
// appears when a recorded run of the flow is loaded and replays it on the
// canvas. The composition owns nothing about persistence: the host passes the
// flow, the tools and the callbacks exactly as it would to the linear editor.

import { useEffect, useRef, useState, type ReactNode } from "react";
import { t } from "@neokapi/i18n-react/runtime";
import {
  LinearFlowEditor,
  ViewTab,
  ViewTabGroup,
  type SchemaFormHost,
} from "@neokapi/ui-primitives";
import { FlowDiagramView } from "./FlowDiagramView";
import type { FlowSpec, ToolInfo, ComponentSchema, ToolDoc } from "./types";
import type { FlowTrace, TraceEvent } from "./traceTypes";

export type FlowView = "steps" | "diagram" | "run";

export interface FlowViewTabsProps {
  flowName: string;
  flow: FlowSpec;
  tools: ToolInfo[];
  onChange: (flow: FlowSpec) => void;
  /** Resolve a tool's option schema (cached by the host). */
  onGetSchema?: (toolName: string) => ComponentSchema | null;
  /** Resolve a tool's documentation for the diagram's step inspector. */
  onGetDoc?: (toolName: string) => ToolDoc | null;
  host?: SchemaFormHost;
  onRun?: () => void;
  runDisabled?: boolean;
  readOnly?: boolean;
  /** True when this is the project's default flow. Absent hides the toggle. */
  isDefault?: boolean;
  onToggleDefault?: (next: boolean) => void;
  /** Absent leaves the name read-only. */
  onRename?: (next: string) => void;
  /** Rendered in the step editor's empty state (see LinearFlowEditorProps). */
  templateLibrary?: ReactNode;
  /** Project-level tool presets, badged on the diagram's nodes. */
  projectPresets?: Record<string, Record<string, unknown>>;
  /** A recorded run of this flow; its presence adds the Run view. */
  trace?: FlowTrace;
  /** Pre-mapped trace events for a host without full snapshots; also adds the Run view. */
  traceEvents?: TraceEvent[];
  /** Called when the reader dismisses the loaded run. */
  onTraceDismiss?: () => void;
  /** The view to show; omit to let the switch keep its own state. */
  view?: FlowView;
  /** The initial view when uncontrolled (default "steps"). */
  defaultView?: FlowView;
  /** Called whenever the view changes, by a click or by a run arriving. */
  onViewChange?: (view: FlowView) => void;
}

export function FlowViewTabs({
  flowName,
  flow,
  tools,
  onChange,
  onGetSchema,
  onGetDoc,
  host,
  onRun,
  runDisabled,
  readOnly,
  isDefault,
  onToggleDefault,
  onRename,
  templateLibrary,
  projectPresets,
  trace,
  traceEvents,
  onTraceDismiss,
  view,
  defaultView = "steps",
  onViewChange,
}: FlowViewTabsProps) {
  const hasRun = !!trace || (traceEvents?.length ?? 0) > 0;
  const [inner, setInner] = useState<FlowView>(defaultView);
  const requested = view ?? inner;
  // Run is only a view while there is a run to show.
  const active: FlowView = requested === "run" && !hasRun ? "diagram" : requested;

  const select = (next: FlowView) => {
    if (view === undefined) setInner(next);
    onViewChange?.(next);
  };

  // A run arriving after mount is the reader's cue to watch it: switch to the
  // Run view once, the way the lab replays a fresh trace. A run present from
  // the start leaves the chosen view alone.
  const hadRun = useRef(hasRun);
  const selectRef = useRef(select);
  selectRef.current = select;
  useEffect(() => {
    if (hasRun && !hadRun.current) selectRef.current("run");
    hadRun.current = hasRun;
  }, [hasRun]);

  return (
    <div className="flex h-full min-h-0 flex-col" data-testid="flow-view-tabs">
      <div className="flex shrink-0 items-center border-b px-4 py-1.5">
        <ViewTabGroup aria-label={t("Flow view")}>
          <ViewTab
            active={active === "steps"}
            onClick={() => select("steps")}
            data-testid="flow-view-steps"
          >
            {t("Steps")}
          </ViewTab>
          <ViewTab
            active={active === "diagram"}
            onClick={() => select("diagram")}
            data-testid="flow-view-diagram"
          >
            {t("Diagram")}
          </ViewTab>
          {hasRun && (
            <ViewTab
              active={active === "run"}
              onClick={() => select("run")}
              data-testid="flow-view-run"
            >
              {t("Run")}
            </ViewTab>
          )}
        </ViewTabGroup>
      </div>
      <div className="min-h-0 flex-1">
        {active === "steps" && (
          <LinearFlowEditor
            flowName={flowName}
            flow={flow}
            tools={tools}
            onChange={onChange}
            onGetSchema={onGetSchema}
            host={host}
            onRun={onRun}
            runDisabled={runDisabled}
            readOnly={readOnly}
            isDefault={isDefault}
            onToggleDefault={onToggleDefault}
            onRename={onRename}
            templateLibrary={templateLibrary}
          />
        )}
        {active === "diagram" && (
          <FlowDiagramView
            flow={flow}
            tools={tools}
            onGetSchema={onGetSchema}
            onGetDoc={onGetDoc}
            projectPresets={projectPresets}
          />
        )}
        {active === "run" && (
          <FlowDiagramView
            flow={flow}
            tools={tools}
            onGetSchema={onGetSchema}
            onGetDoc={onGetDoc}
            projectPresets={projectPresets}
            trace={trace}
            traceEvents={traceEvents}
            onTraceDismiss={onTraceDismiss}
          />
        )}
      </div>
    </div>
  );
}
