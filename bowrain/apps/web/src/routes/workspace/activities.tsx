import { useEffect, useState, useCallback } from "react";
import { useQuery } from "@tanstack/react-query";
import { ActivityFeed, useWorkspace, useApi, Card } from "@neokapi/ui";
import type { ActivityInfo } from "@neokapi/ui";

export function ActivitiesRoute() {
  const { activeWorkspace } = useWorkspace();
  const api = useApi();
  const ws = activeWorkspace?.slug ?? "";

  const [allActivities, setAllActivities] = useState<ActivityInfo[]>([]);
  const [cursor, setCursor] = useState<string>("");
  const [hasMore, setHasMore] = useState(false);
  const LIMIT = 50;

  useEffect(() => {
    if (activeWorkspace) {
      document.title = `Activity — ${activeWorkspace.name} — Bowrain`;
    }
  }, [activeWorkspace]);

  const { data, isFetching } = useQuery({
    queryKey: ["activities", ws, cursor],
    queryFn: () => api.listActivities(ws, { limit: LIMIT, cursor: cursor || undefined }),
    enabled: !!ws,
    staleTime: 30_000,
  });

  useEffect(() => {
    if (data) {
      if (!cursor) {
        setAllActivities(data.activities);
      } else {
        setAllActivities((prev) => [...prev, ...data.activities]);
      }
      setHasMore(!!data.next_cursor);
    }
  }, [data, cursor]);

  const handleLoadMore = useCallback(() => {
    if (data?.next_cursor) {
      setCursor(data.next_cursor);
    }
  }, [data]);

  if (!activeWorkspace) {
    return (
      <Card className="mt-8 max-w-md mx-auto p-8 text-center text-muted-foreground text-sm">
        Select a workspace
      </Card>
    );
  }

  return (
    <div className="mx-auto w-full max-w-3xl p-4 md:p-6">
      <h1 className="text-lg font-semibold mb-4">Activity</h1>
      <ActivityFeed
        activities={allActivities}
        loading={isFetching}
        hasMore={hasMore}
        onLoadMore={handleLoadMore}
      />
    </div>
  );
}
