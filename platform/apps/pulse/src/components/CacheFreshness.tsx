import { useState, useEffect } from "react";
import { useIsFetching, useQueryClient } from "@tanstack/react-query";
import { RefreshCw } from "lucide-react";

/**
 * Subtle cache freshness indicator for Pulse pages.
 * Shows when data was last fetched and a countdown to the next refresh.
 * Displays a spinning icon while any Pulse query is fetching.
 */
export function CacheFreshness({ queryKeyPrefix }: { queryKeyPrefix: string[] }) {
  const queryClient = useQueryClient();
  const isFetching = useIsFetching({ queryKey: queryKeyPrefix });
  const [, tick] = useState(0);

  // Re-render every 15s to keep the relative time fresh.
  useEffect(() => {
    const id = setInterval(() => tick((n) => n + 1), 15_000);
    return () => clearInterval(id);
  }, []);

  // Find the most recent dataUpdatedAt among matching queries.
  const queries = queryClient.getQueryCache().findAll({ queryKey: queryKeyPrefix });
  let latestUpdate = 0;
  let earliestStale = Infinity;

  for (const query of queries) {
    if (query.state.dataUpdatedAt > latestUpdate) {
      latestUpdate = query.state.dataUpdatedAt;
    }
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const staleTime = ((query.options as any).staleTime as number) ?? 0;
    const staleAt = query.state.dataUpdatedAt + staleTime;
    if (staleAt < earliestStale) {
      earliestStale = staleAt;
    }
  }

  if (latestUpdate === 0) return null;

  const ago = formatRelative(Date.now() - latestUpdate);
  const untilRefresh = earliestStale - Date.now();

  return (
    <div className="flex items-center gap-1.5 text-[11px] text-muted-foreground/60 select-none">
      <RefreshCw className={`h-3 w-3 ${isFetching ? "animate-spin text-primary/50" : ""}`} />
      <span>
        Updated {ago}
        {untilRefresh > 0 && !isFetching && <> · refreshes in {formatRelative(untilRefresh)}</>}
      </span>
    </div>
  );
}

function formatRelative(ms: number): string {
  const sec = Math.round(ms / 1000);
  if (sec < 5) return "just now";
  if (sec < 60) return `${sec}s`;
  const min = Math.round(sec / 60);
  if (min < 60) return `${min}m`;
  return `${Math.round(min / 60)}h`;
}
