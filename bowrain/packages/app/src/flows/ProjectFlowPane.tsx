// One flow open for editing: a bar with the way back, the flow's provenance,
// the save state and the flow-level actions, over the shared flow workbench
// (Steps, the linear editor, and Diagram, the read-only canvas). A built-in
// flow is read-only and offers a copy to edit instead.

import { AlertTriangle, Check, Copy, Loader2, Lock, X } from "lucide-react";
import { FlowTemplateLibrary, FlowViewTabs } from "@neokapi/flow-editor";
import type { FlowView, ToolInfo as EditorToolInfo } from "@neokapi/flow-editor";
import { Button, ConfirmDeleteButton, ErrorNotice, SimpleTooltip } from "@neokapi/ui";
import type { FlowDefinitionInfo, LinearFlowSpec } from "@neokapi/ui";

/** Where the open flow stands against the server. */
export type FlowSaveState = "saved" | "unsaved" | "saving" | "error";

export interface ProjectFlowPaneProps {
  flow: FlowDefinitionInfo;
  spec: LinearFlowSpec;
  /** The tool list in the editor's shape (see `toEditorTools`). */
  tools: EditorToolInfo[];
  readOnly: boolean;
  saveState: FlowSaveState;
  /** The failure behind an `error` save state. */
  saveError?: unknown;
  /** The view to open on; Steps unless told otherwise. */
  defaultView?: FlowView;
  onBack: () => void;
  onChange: (spec: LinearFlowSpec) => void;
  onRename?: (name: string) => void;
  /** Offered on a read-only flow: copy it into the project to edit. */
  onCopy?: () => void;
  onDelete?: () => void;
}

function SaveIndicator({ state }: { state: FlowSaveState }) {
  const base = "flex items-center gap-1 text-[11px]";
  switch (state) {
    case "saving":
      return (
        <span className={`${base} text-muted-foreground`} data-testid="flow-save-state">
          <Loader2 size={11} className="animate-spin" aria-hidden="true" />
          Saving...
        </span>
      );
    case "unsaved":
      return (
        <span className={`${base} text-muted-foreground`} data-testid="flow-save-state">
          Unsaved changes
        </span>
      );
    case "error":
      return (
        <span className={`${base} text-destructive`} data-testid="flow-save-state">
          <AlertTriangle size={11} aria-hidden="true" />
          Save failed
        </span>
      );
    default:
      return (
        <span className={`${base} text-muted-foreground`} data-testid="flow-save-state">
          <Check size={11} aria-hidden="true" />
          Saved
        </span>
      );
  }
}

export function ProjectFlowPane({
  flow,
  spec,
  tools,
  readOnly,
  saveState,
  saveError,
  defaultView,
  onBack,
  onChange,
  onRename,
  onCopy,
  onDelete,
}: ProjectFlowPaneProps) {
  return (
    <div className="flex flex-col gap-3" data-testid="flow-pane">
      <div className="flex flex-wrap items-center gap-3">
        <SimpleTooltip content="Back to flow list">
          <Button
            variant="ghost"
            size="icon-xs"
            onClick={onBack}
            aria-label="Back to flow list"
            data-testid="flow-back"
          >
            <X size={16} />
          </Button>
        </SimpleTooltip>
        {readOnly && (
          <span
            className="flex items-center gap-1 rounded bg-muted px-1.5 py-0.5 text-[11px] text-muted-foreground"
            data-testid="flow-read-only"
          >
            <Lock size={9} aria-hidden="true" /> Built-in (read-only)
          </span>
        )}
        {!readOnly && <SaveIndicator state={saveState} />}
        <div className="ml-auto flex items-center gap-2">
          {readOnly && onCopy && (
            <Button variant="outline" size="sm" onClick={onCopy} data-testid="copy-flow-btn">
              <Copy size={12} />
              Copy to edit
            </Button>
          )}
          {!readOnly && onDelete && (
            <ConfirmDeleteButton mode="text" size="sm" onDelete={onDelete} />
          )}
        </div>
      </div>

      {saveState === "error" && saveError != null && (
        <ErrorNotice error={saveError} title="Could not save the flow" variant="inline" />
      )}

      {/* Steps (authoring) and Diagram (the read-only canvas). The platform
          keeps no trace of a flow run, so there is no Run view. */}
      <div className="h-[640px] overflow-hidden rounded-lg border border-border bg-card shadow-sm">
        <FlowViewTabs
          key={flow.id}
          flowName={flow.name}
          flow={spec}
          tools={tools}
          onChange={onChange}
          readOnly={readOnly}
          onRename={readOnly ? undefined : onRename}
          defaultView={defaultView}
          templateLibrary={readOnly ? undefined : <FlowTemplateLibrary onSelect={onChange} />}
        />
      </div>
    </div>
  );
}
