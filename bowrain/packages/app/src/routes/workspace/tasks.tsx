import { useEffect, useState, useCallback } from "react";
import { useNavigate, useParams } from "@tanstack/react-router";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { TaskBoard, useWorkspace, useApi, useAuth, Card } from "@neokapi/ui";
import type { TaskInfo } from "@neokapi/ui";
import { taskCountsQueryOptions } from "../../queries";

export function TasksRoute() {
  const navigate = useNavigate();
  const { workspace } = useParams({ strict: false });
  const { activeWorkspace } = useWorkspace();
  const { user } = useAuth();
  const api = useApi();
  const queryClient = useQueryClient();
  const ws = activeWorkspace?.slug ?? "";

  const [allTasks, setAllTasks] = useState<TaskInfo[]>([]);
  const [cursor, setCursor] = useState<string>("");
  const [hasMore, setHasMore] = useState(false);
  const LIMIT = 50;

  useEffect(() => {
    if (activeWorkspace) {
      document.title = `Tasks · ${activeWorkspace.name} · Bowrain`;
    }
  }, [activeWorkspace]);

  const { data, isFetching } = useQuery({
    queryKey: ["tasks", ws, cursor],
    queryFn: () => api.listTasks(ws, { limit: LIMIT, cursor: cursor || undefined }),
    enabled: !!ws,
    staleTime: 30_000,
  });

  // Column totals over the whole workspace, not over the pages loaded so far.
  const { data: counts } = useQuery({
    ...taskCountsQueryOptions(api, ws),
    enabled: !!ws,
  });

  useEffect(() => {
    if (data) {
      if (!cursor) {
        setAllTasks(data.tasks);
      } else {
        // Dedupe by id: invalidation/focus refetches re-deliver the same page.
        setAllTasks((prev) => {
          const seen = new Set(prev.map((t) => t.id));
          return [...prev, ...data.tasks.filter((t) => !seen.has(t.id))];
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

  const completeMutation = useMutation({
    mutationFn: (taskId: string) => api.completeTask(ws, taskId),
    onSuccess: () => {
      // Restart pagination from the first page so the accumulated list
      // reflects the change instead of appending stale pages.
      setCursor("");
      void queryClient.invalidateQueries({ queryKey: ["tasks", ws] });
      void queryClient.invalidateQueries({ queryKey: ["myTasks", ws] });
      void queryClient.invalidateQueries({ queryKey: ["taskCounts", ws] });
      void queryClient.invalidateQueries({ queryKey: ["myTaskCounts", ws] });
    },
  });

  const cancelMutation = useMutation({
    mutationFn: (taskId: string) => api.cancelTask(ws, taskId),
    onSuccess: () => {
      setCursor("");
      void queryClient.invalidateQueries({ queryKey: ["tasks", ws] });
      void queryClient.invalidateQueries({ queryKey: ["myTasks", ws] });
      void queryClient.invalidateQueries({ queryKey: ["taskCounts", ws] });
      void queryClient.invalidateQueries({ queryKey: ["myTaskCounts", ws] });
    },
  });

  if (!activeWorkspace) {
    return (
      <Card className="mt-8 max-w-md mx-auto p-8 text-center text-muted-foreground text-sm">
        Select a workspace
      </Card>
    );
  }

  return (
    <div className="mx-auto w-full max-w-5xl p-4 md:p-6">
      <h1 className="text-lg font-semibold mb-4">Tasks</h1>
      <TaskBoard
        tasks={allTasks}
        loading={isFetching}
        statusCounts={counts?.by_status}
        hasMore={hasMore}
        onLoadMore={handleLoadMore}
        currentUserId={user?.id}
        onCompleteTask={(id) => completeMutation.mutate(id)}
        onCancelTask={(id) => cancelMutation.mutate(id)}
        onTaskClick={(task: TaskInfo) => {
          if (!task.project_id) return;
          // Review-type tasks deep-link into the dedicated review session, not
          // the project page (or the moded editor).
          const isReview =
            task.type === "review" || task.type === "review_terms" || task.type === "source_review";
          void navigate({
            to: isReview
              ? "/$workspace/p/$projectId/s/$stream/review"
              : "/$workspace/p/$projectId/s/$stream",
            params: {
              workspace: workspace ?? ws,
              projectId: task.project_id,
              stream: task.stream || "main",
            },
          });
        }}
      />
    </div>
  );
}
