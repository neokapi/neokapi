// One flow open for editing: a bar with the way back, the flow's provenance,
// the save state and the flow-level actions, over the shared linear step
// editor. A built-in flow is read-only and offers a copy to edit instead.

import { AlertTriangle, Check, Copy, Loader2, Lock, X } from "lucide-react";
import { FlowTemplateLibrary } from "@neokapi/flow-editor";
import {
  Button,
  ConfirmDeleteButton,
  ErrorNotice,
  LinearFlowEditor,
  SimpleTooltip,
} from "@neokapi/ui";
import type { FlowDefinitionInfo, LinearFlowSpec, ToolInfo } from "@neokapi/ui";

/** Where the open flow stands against the server. */
export type FlowSaveState = "saved" | "unsaved" | "saving" | "error";

export interface ProjectFlowPaneProps {
  flow: FlowDefinitionInfo;
  spec: LinearFlowSpec;
  tools: ToolInfo[];
  readOnly: boolean;
  saveState: FlowSaveState;
  /** The failure behind an `error` save state. */
  saveError?: unknown;
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

      <div className="min-h-[520px] overflow-hidden rounded-lg border border-border bg-card shadow-sm">
        <LinearFlowEditor
          key={flow.id}
          flowName={flow.name}
          flow={spec}
          tools={tools}
          onChange={onChange}
          readOnly={readOnly}
          onRename={readOnly ? undefined : onRename}
          templateLibrary={readOnly ? undefined : <FlowTemplateLibrary onSelect={onChange} />}
        />
      </div>
    </div>
  );
}
