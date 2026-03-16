import { useEffect } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { TaskBoard, useWorkspace, useApi, useAuth, Card } from "@neokapi/ui";

export function TasksRoute() {
  const { activeWorkspace } = useWorkspace();
  const { user } = useAuth();
  const api = useApi();
  const queryClient = useQueryClient();
  const ws = activeWorkspace?.slug ?? "";

  useEffect(() => {
    if (activeWorkspace) {
      document.title = `Tasks — ${activeWorkspace.name} — Bowrain`;
    }
  }, [activeWorkspace]);

  const { data, isFetching } = useQuery({
    queryKey: ["tasks", ws],
    queryFn: () => api.listTasks(ws, { limit: 50 }),
    enabled: !!ws,
    staleTime: 30_000,
  });

  const completeMutation = useMutation({
    mutationFn: (taskId: string) => api.completeTask(ws, taskId),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["tasks", ws] });
      void queryClient.invalidateQueries({ queryKey: ["myTasks", ws] });
    },
  });

  const cancelMutation = useMutation({
    mutationFn: (taskId: string) => api.cancelTask(ws, taskId),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["tasks", ws] });
      void queryClient.invalidateQueries({ queryKey: ["myTasks", ws] });
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
        tasks={data?.tasks ?? []}
        loading={isFetching}
        currentUserId={user?.id}
        onCompleteTask={(id) => completeMutation.mutate(id)}
        onCancelTask={(id) => cancelMutation.mutate(id)}
      />
    </div>
  );
}
