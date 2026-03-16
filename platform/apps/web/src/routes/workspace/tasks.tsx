import { useEffect, useState, useCallback } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { TaskBoard, useWorkspace, useApi, useAuth, Card } from "@neokapi/ui";
import type { TaskInfo } from "@neokapi/ui";

export function TasksRoute() {
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
      document.title = `Tasks — ${activeWorkspace.name} — Bowrain`;
    }
  }, [activeWorkspace]);

  const { data, isFetching } = useQuery({
    queryKey: ["tasks", ws, cursor],
    queryFn: () => api.listTasks(ws, { limit: LIMIT, cursor: cursor || undefined }),
    enabled: !!ws,
    staleTime: 30_000,
  });

  useEffect(() => {
    if (data) {
      if (!cursor) {
        setAllTasks(data.tasks);
      } else {
        setAllTasks((prev) => [...prev, ...data.tasks]);
      }
      setHasMore(!!data.next_cursor);
    }
  }, [data, cursor]);

  const completeMutation = useMutation({
    mutationFn: (taskId: string) => api.completeTask(ws, taskId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["tasks", ws] });
      queryClient.invalidateQueries({ queryKey: ["myTasks", ws] });
    },
  });

  const cancelMutation = useMutation({
    mutationFn: (taskId: string) => api.cancelTask(ws, taskId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["tasks", ws] });
      queryClient.invalidateQueries({ queryKey: ["myTasks", ws] });
    },
  });

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
    <div className="mx-auto w-full max-w-5xl p-4 md:p-6">
      <h1 className="text-lg font-semibold mb-4">Tasks</h1>
      <TaskBoard
        tasks={allTasks}
        loading={isFetching}
        currentUserId={user?.id}
        onCompleteTask={(id) => completeMutation.mutate(id)}
        onCancelTask={(id) => cancelMutation.mutate(id)}
      />
    </div>
  );
}
