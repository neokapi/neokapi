import { cn } from "../../lib/utils";
import { Button } from "../ui/button";

export interface BravoApprovalCardProps {
  toolCallId: string;
  toolName: string;
  input?: Record<string, unknown>;
  onApprove: (toolCallId: string) => void;
  onDeny: (toolCallId: string) => void;
  loading?: boolean;
}

export function BravoApprovalCard({
  toolCallId,
  toolName,
  input,
  onApprove,
  onDeny,
  loading,
}: BravoApprovalCardProps) {
  return (
    <div className="w-full max-w-[85%] rounded-lg border-2 border-amber-300 dark:border-amber-600 bg-amber-50 dark:bg-amber-950/20 p-3 space-y-3">
      <div className="flex items-center gap-2">
        <span className="text-amber-600 dark:text-amber-400 text-sm font-medium">
          Approval required
        </span>
      </div>

      <div className="text-sm">
        <span className="text-muted-foreground">@bravo wants to run </span>
        <span className="font-mono font-medium">{toolName}</span>
      </div>

      {input && Object.keys(input).length > 0 && (
        <pre className="text-xs bg-background/50 rounded p-2 overflow-x-auto whitespace-pre-wrap border">
          {JSON.stringify(input, null, 2)}
        </pre>
      )}

      <div className="flex gap-2">
        <Button
          size="sm"
          onClick={() => onApprove(toolCallId)}
          disabled={loading}
          className={cn(loading && "opacity-50")}
        >
          Approve
        </Button>
        <Button size="sm" variant="outline" onClick={() => onDeny(toolCallId)} disabled={loading}>
          Deny
        </Button>
      </div>
    </div>
  );
}
