import { Badge, Card } from "@neokapi/ui-primitives";
import { useQuery } from "@tanstack/react-query";
import { useApi } from "../context/ApiContext";
import type { AutomationHistoryEntry } from "../types/api";

// ---------------------------------------------------------------------------
// Status badge
// ---------------------------------------------------------------------------

function StatusBadge({ status }: { status: AutomationHistoryEntry["status"] }) {
  const variant =
    status === "success" ? "default" : status === "failed" ? "destructive" : "secondary";

  return <Badge variant={variant}>{status}</Badge>;
}

// ---------------------------------------------------------------------------
// Relative time formatting
// ---------------------------------------------------------------------------

function relativeTime(iso: string): string {
  const now = Date.now();
  const then = new Date(iso).getTime();
  const diffMs = now - then;
  const diffSec = Math.floor(diffMs / 1000);
  if (diffSec < 60) return "just now";
  const diffMin = Math.floor(diffSec / 60);
  if (diffMin < 60) return `${diffMin}m ago`;
  const diffHr = Math.floor(diffMin / 60);
  if (diffHr < 24) return `${diffHr}h ago`;
  const diffDay = Math.floor(diffHr / 24);
  return `${diffDay}d ago`;
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

interface AutomationHistoryProps {
  workspaceSlug: string;
  projectId: string;
  ruleNames?: Record<string, string>;
}

export function AutomationHistory({ workspaceSlug, projectId, ruleNames }: AutomationHistoryProps) {
  const api = useApi();
  const {
    data: entries,
    isLoading,
    error,
  } = useQuery({
    queryKey: ["automations", "history", workspaceSlug, projectId],
    queryFn: () => api.listAutomationHistory(workspaceSlug, projectId),
    staleTime: 15_000,
  });

  if (isLoading) {
    return <div className="py-8 text-center text-sm text-muted-foreground">Loading history...</div>;
  }

  if (error) {
    return (
      <div className="py-8 text-center text-sm text-destructive">
        Failed to load history: {(error as Error).message}
      </div>
    );
  }

  if (!entries || entries.length === 0) {
    return (
      <div className="py-8 text-center text-sm text-muted-foreground">
        No executions yet. Rules will appear here when triggered.
      </div>
    );
  }

  return (
    <div className="space-y-2">
      {entries.map((entry) => (
        <Card key={entry.id} className="p-4">
          <div className="flex items-start justify-between gap-4">
            <div className="min-w-0 flex-1">
              <div className="flex items-center gap-2">
                <span className="text-sm font-medium truncate">
                  {ruleNames?.[entry.rule_id] ?? entry.rule_id}
                </span>
                <StatusBadge status={entry.status} />
              </div>
              <div className="mt-1 text-xs text-muted-foreground">Event: {entry.event_id}</div>
              {entry.status === "failed" && entry.error && (
                <div className="mt-2 text-xs text-destructive bg-destructive/10 rounded px-2 py-1 font-mono break-all">
                  {entry.error}
                </div>
              )}
            </div>
            <div className="text-xs text-muted-foreground whitespace-nowrap">
              {relativeTime(entry.started_at)}
            </div>
          </div>
        </Card>
      ))}
    </div>
  );
}
