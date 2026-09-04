import { useEffect, useState, useCallback } from "react";
import { useQuery } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { ActivityFeed, useWorkspace, useApi, Card, PageHeader } from "@neokapi/ui";
import type { ActivityInfo } from "@neokapi/ui";

export function ActivitiesRoute() {
  const { activeWorkspace } = useWorkspace();
  const api = useApi();
  const navigate = useNavigate();
  const ws = activeWorkspace?.slug ?? "";

  const [allActivities, setAllActivities] = useState<ActivityInfo[]>([]);
  const [cursor, setCursor] = useState<string>("");
  const [hasMore, setHasMore] = useState(false);
  const LIMIT = 50;

  useEffect(() => {
    if (activeWorkspace) {
      document.title = `Activity · ${activeWorkspace.name} · Bowrain`;
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
        // Dedupe by id: focus/staleness refetches re-deliver the same page.
        setAllActivities((prev) => {
          const seen = new Set(prev.map((a) => a.id));
          return [...prev, ...data.activities.filter((a) => !seen.has(a.id))];
        });
      }
      setHasMore(!!data.next_cursor);
    }
  }, [data, cursor]);

  const handleLoadMore = useCallback(() => {
    if (data?.next_cursor) {
      setCursor(data.next_cursor);
    }
  }, [data]);

  // Deep-link an activity to the project (and stream) it touched. Workspace-level
  // activities with no project stay non-navigable (ActivityFeed only shows a
  // pointer cursor for rows we can route).
  const handleActivityClick = useCallback(
    (activity: ActivityInfo) => {
      if (!activity.project_id) return;
      void navigate({
        to: "/$workspace/p/$projectId/s/$stream",
        params: {
          workspace: ws,
          projectId: activity.project_id,
          stream: activity.stream || "main",
        },
      });
    },
    [navigate, ws],
  );

  if (!activeWorkspace) {
    return (
      <Card className="mt-8 max-w-md mx-auto p-8 text-center text-muted-foreground text-sm">
        Select a workspace
      </Card>
    );
  }

  return (
    <div className="mx-auto w-full max-w-3xl p-4 md:p-6">
      <PageHeader title="Activity" />
      <ActivityFeed
        activities={allActivities}
        loading={isFetching}
        hasMore={hasMore}
        onLoadMore={handleLoadMore}
        onActivityClick={handleActivityClick}
        canNavigate={(a) => !!a.project_id}
      />
    </div>
  );
}
