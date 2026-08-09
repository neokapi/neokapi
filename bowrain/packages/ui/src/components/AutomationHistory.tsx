import { Badge, Button, Card } from "@neokapi/ui-primitives";
import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { useApi } from "../context/ApiContext";
import type { AutomationHistoryEntry } from "../types/api";
import { Loader2 } from "./icons";

/** Executions per request; the server pages on an opaque (started_at, id) cursor. */
const PAGE_SIZE = 25;

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
  // Cursors already walked, oldest page first. The cursor is opaque — the
  // server encodes (started_at, id) into it — so the only way forward is to
  // carry back the one the previous page returned.
  const [cursors, setCursors] = useState<string[]>([]);
  const cursor = cursors[cursors.length - 1];

  const {
    data: page,
    isLoading,
    isFetching,
    error,
  } = useQuery({
    queryKey: ["automations", "history", workspaceSlug, projectId, cursor ?? ""],
    queryFn: () =>
      api.listAutomationHistory(workspaceSlug, projectId, { limit: PAGE_SIZE, cursor }),
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

  const entries = page?.entries ?? [];
  if (entries.length === 0 && cursors.length === 0) {
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

      {(cursors.length > 0 || page?.next_cursor) && (
        <div className="flex items-center justify-between pt-2">
          <Button
            variant="outline"
            size="sm"
            disabled={cursors.length === 0 || isFetching}
            onClick={() => setCursors((prev) => prev.slice(0, -1))}
            data-testid="automation-history-newer"
          >
            Newer
          </Button>
          <span className="text-xs text-muted-foreground">
            {isFetching ? <Loader2 className="size-3.5 animate-spin" /> : null}
          </span>
          <Button
            variant="outline"
            size="sm"
            disabled={!page?.next_cursor || isFetching}
            onClick={() => {
              const next = page?.next_cursor;
              if (next) setCursors((prev) => [...prev, next]);
            }}
            data-testid="automation-history-older"
          >
            Older
          </Button>
        </div>
      )}
    </div>
  );
}
