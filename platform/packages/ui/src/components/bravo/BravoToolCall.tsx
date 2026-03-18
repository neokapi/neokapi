import { useState } from "react";
import { cn } from "../../lib/utils";
import type { BravoToolCall as BravoToolCallType } from "../../types/api";
import { Button } from "../ui/button";

export interface BravoToolCallProps {
  toolCall: BravoToolCallType;
  onApprove?: () => void;
  onDeny?: () => void;
}

function statusColor(status: BravoToolCallType["status"]): string {
  switch (status) {
    case "completed":
      return "text-green-600 dark:text-green-400";
    case "failed":
    case "denied":
      return "text-destructive";
    case "running":
      return "text-blue-600 dark:text-blue-400";
    case "needs_approval":
      return "text-amber-600 dark:text-amber-400";
    default:
      return "text-muted-foreground";
  }
}

function statusLabel(status: BravoToolCallType["status"]): string {
  switch (status) {
    case "completed":
      return "Completed";
    case "failed":
      return "Failed";
    case "running":
      return "Running...";
    case "needs_approval":
      return "Needs approval";
    case "denied":
      return "Denied";
    default:
      return "Pending";
  }
}

export function BravoToolCall({ toolCall, onApprove, onDeny }: BravoToolCallProps) {
  const [expanded, setExpanded] = useState(false);

  return (
    <div className="w-full max-w-[85%] rounded-md border bg-card text-card-foreground text-sm">
      <button
        onClick={() => setExpanded(!expanded)}
        className="flex w-full items-center gap-2 px-3 py-2 hover:bg-accent/50 transition-colors rounded-md"
      >
        <span className="font-mono text-xs font-medium truncate flex-1 text-left">
          {toolCall.tool_name}
        </span>
        <span className={cn("text-xs font-medium shrink-0", statusColor(toolCall.status))}>
          {statusLabel(toolCall.status)}
        </span>
        {toolCall.duration > 0 && (
          <span className="text-[10px] text-muted-foreground shrink-0">
            {(toolCall.duration / 1e6).toFixed(0)}ms
          </span>
        )}
        <span className="text-muted-foreground text-xs shrink-0">
          {expanded ? "▲" : "▼"}
        </span>
      </button>

      {expanded && (
        <div className="border-t px-3 py-2 space-y-2">
          {toolCall.input && Object.keys(toolCall.input).length > 0 && (
            <div>
              <div className="text-[10px] uppercase text-muted-foreground font-medium mb-1">
                Input
              </div>
              <pre className="text-xs bg-muted rounded p-2 overflow-x-auto whitespace-pre-wrap">
                {JSON.stringify(toolCall.input, null, 2)}
              </pre>
            </div>
          )}

          {toolCall.output && Object.keys(toolCall.output).length > 0 && (
            <div>
              <div className="text-[10px] uppercase text-muted-foreground font-medium mb-1">
                Output
              </div>
              <pre className="text-xs bg-muted rounded p-2 overflow-x-auto whitespace-pre-wrap">
                {JSON.stringify(toolCall.output, null, 2)}
              </pre>
            </div>
          )}

          {toolCall.error && (
            <div className="text-xs text-destructive">{toolCall.error}</div>
          )}
        </div>
      )}

      {toolCall.status === "needs_approval" && (onApprove || onDeny) && (
        <div className="border-t px-3 py-2 flex gap-2">
          {onApprove && (
            <Button size="sm" variant="default" onClick={onApprove}>
              Approve
            </Button>
          )}
          {onDeny && (
            <Button size="sm" variant="outline" onClick={onDeny}>
              Deny
            </Button>
          )}
        </div>
      )}
    </div>
  );
}
