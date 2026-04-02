import { useState } from "react";
import { makeAssistantToolUI } from "@assistant-ui/react";
import { cn } from "@neokapi/ui-primitives";
import { Button } from "@neokapi/ui-primitives/components/ui/button";

// ---------------------------------------------------------------------------
// Generic tool-call renderer
//
// This replaces the old BravoToolCall component and is registered with
// assistant-ui as a catch-all tool UI via `makeAssistantToolUI`. It preserves
// the exact same visual design: expandable card with input/output JSON,
// status indicator, and approve/deny buttons for gated tools.
// ---------------------------------------------------------------------------

function statusColor(status: string): string {
  switch (status) {
    case "complete":
      return "text-green-600 dark:text-green-400";
    case "error":
      return "text-destructive";
    case "running":
    case "in_progress":
      return "text-blue-600 dark:text-blue-400";
    case "requires-action":
      return "text-amber-600 dark:text-amber-400";
    default:
      return "text-muted-foreground";
  }
}

function statusLabel(status: string, isError?: boolean): string {
  if (isError) return "Failed";
  switch (status) {
    case "complete":
      return "Completed";
    case "error":
      return "Failed";
    case "running":
    case "in_progress":
      return "Running...";
    case "requires-action":
      return "Needs approval";
    default:
      return "Pending";
  }
}

export function BravoToolCallRenderer({
  toolName,
  args,
  result,
  isError,
  status,
  addResult,
}: {
  toolName: string;
  toolCallId?: string;
  args?: Record<string, unknown>;
  result?: unknown;
  isError?: boolean;
  status?: { type: string } | string;
  addResult?: (result: unknown) => void;
}) {
  const [expanded, setExpanded] = useState(false);

  // Derive a display status from assistant-ui's status object.
  const displayStatus =
    typeof status === "object" && status !== null && "type" in status
      ? (status as { type: string }).type
      : typeof status === "string"
        ? status
        : "pending";

  // Check for needs_approval from the raw tool call metadata.
  const needsApproval = displayStatus === "requires-action";

  return (
    <div className="w-full rounded-md border bg-card text-card-foreground text-sm my-1">
      <button
        onClick={() => setExpanded(!expanded)}
        className="flex w-full items-center gap-2 px-3 py-2 hover:bg-accent/50 transition-colors rounded-md"
      >
        <span className="font-mono text-xs font-medium truncate flex-1 text-left">{toolName}</span>
        <span className={cn("text-xs font-medium shrink-0", statusColor(displayStatus))}>
          {statusLabel(displayStatus, isError)}
        </span>
        <span className="text-muted-foreground text-xs shrink-0">
          {expanded ? "\u25B2" : "\u25BC"}
        </span>
      </button>

      {expanded && (
        <div className="border-t px-3 py-2 space-y-2">
          {args && Object.keys(args).length > 0 && (
            <div>
              <div className="text-[10px] uppercase text-muted-foreground font-medium mb-1">
                Input
              </div>
              <pre className="text-xs bg-muted rounded p-2 overflow-x-auto whitespace-pre-wrap">
                {JSON.stringify(args, null, 2)}
              </pre>
            </div>
          )}

          {result != null &&
            (typeof result !== "object" ||
              Object.keys(result as Record<string, unknown>).length > 0) && (
              <div>
                <div className="text-[10px] uppercase text-muted-foreground font-medium mb-1">
                  Output
                </div>
                <pre className="text-xs bg-muted rounded p-2 overflow-x-auto whitespace-pre-wrap">
                  {typeof result === "string" ? result : JSON.stringify(result, null, 2)}
                </pre>
              </div>
            )}
        </div>
      )}

      {needsApproval && addResult && (
        <div className="border-t px-3 py-2">
          <div className="flex items-center gap-2 mb-2">
            <span className="text-amber-600 dark:text-amber-400 text-xs font-medium">
              @bravo wants to run this tool
            </span>
          </div>
          <div className="flex gap-2">
            <Button size="sm" variant="default" onClick={() => addResult("approved")}>
              Approve
            </Button>
            <Button size="sm" variant="outline" onClick={() => addResult("denied")}>
              Deny
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}

/**
 * Catch-all tool UI — renders for any tool name that doesn't have a specific
 * registered UI. This uses a wildcard pattern so it matches all tools.
 */
export const BravoFallbackToolUI = makeAssistantToolUI({
  toolName: "*",
  render: BravoToolCallRenderer,
});
