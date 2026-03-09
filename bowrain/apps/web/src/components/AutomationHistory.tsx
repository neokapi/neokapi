import { useQuery } from "@tanstack/react-query";
import { Badge, GlassCard } from "@gokapi/ui";
import { api } from "../api";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface HistoryEntry {
  id: string;
  rule_id: string;
  project_id: string;
  event_id: string;
  status: "success" | "failed" | "skipped";
  error: string;
  started_at: string;
  ended_at: string;
}

// ---------------------------------------------------------------------------
// API helpers
// ---------------------------------------------------------------------------

async function fetchHistory(ws: string, projectId: string): Promise<HistoryEntry[]> {
  const resp = await fetch(
    `/api/v1/workspaces/${encodeURIComponent(ws)}/projects/${encodeURIComponent(projectId)}/automations/history`,
    { credentials: "same-origin" },
  );
  if (!resp.ok) {
    const body = await resp.text();
    throw new Error(`${resp.status}: ${body}`);
  }
  return resp.json();
}

// ---------------------------------------------------------------------------
// Status badge
// ---------------------------------------------------------------------------

function StatusBadge({ status }: { status: HistoryEntry["status"] }) {
  const variant =
    status === "success"
      ? "default"
      : status === "failed"
        ? "destructive"
        : "secondary";

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
  /** Optional map of rule IDs to rule names for display. */
  ruleNames?: Record<string, string>;
}

export function AutomationHistory({ workspaceSlug, projectId, ruleNames }: AutomationHistoryProps) {
  const { data: entries, isLoading, error } = useQuery({
    queryKey: ["automations", "history", workspaceSlug, projectId],
    queryFn: () => fetchHistory(workspaceSlug, projectId),
    staleTime: 15_000,
  });

  if (isLoading) {
    return (
      <div className="py-8 text-center text-sm text-muted-foreground">
        Loading history...
      </div>
    );
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
        <GlassCard key={entry.id} intensity="subtle" className="p-4">
          <div className="flex items-start justify-between gap-4">
            <div className="min-w-0 flex-1">
              <div className="flex items-center gap-2">
                <span className="text-sm font-medium truncate">
                  {ruleNames?.[entry.rule_id] ?? entry.rule_id}
                </span>
                <StatusBadge status={entry.status} />
              </div>
              <div className="mt-1 text-xs text-muted-foreground">
                Event: {entry.event_id}
              </div>
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
        </GlassCard>
      ))}
    </div>
  );
}
