import { useCallback, useMemo } from "react";
import { useApi } from "../context/ApiContext";
import { useWorkspace } from "../context/WorkspaceContext";
import type { TaskInfo, CreateTaskRequest } from "../types/api";

export function useTasks() {
  const api = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";

  const listTasks = useCallback(
    async (query?: {
      project_id?: string;
      assignee_id?: string;
      status?: string;
      type?: string;
      priority?: string;
      cursor?: string;
      limit?: number;
    }): Promise<{ tasks: TaskInfo[]; next_cursor: string }> =>
      api.listTasks(ws, query),
    [api, ws],
  );

  const createTask = useCallback(
    async (task: CreateTaskRequest): Promise<TaskInfo> => api.createTask(ws, task),
    [api, ws],
  );

  const getTask = useCallback(
    async (taskId: string): Promise<TaskInfo> => api.getTask(ws, taskId),
    [api, ws],
  );

  const updateTask = useCallback(
    async (taskId: string, updates: Partial<CreateTaskRequest>): Promise<TaskInfo> =>
      api.updateTask(ws, taskId, updates),
    [api, ws],
  );

  const deleteTask = useCallback(
    async (taskId: string): Promise<void> => api.deleteTask(ws, taskId),
    [api, ws],
  );

  const assignTask = useCallback(
    async (taskId: string, assigneeId: string): Promise<void> =>
      api.assignTask(ws, taskId, assigneeId),
    [api, ws],
  );

  const completeTask = useCallback(
    async (taskId: string): Promise<void> => api.completeTask(ws, taskId),
    [api, ws],
  );

  const cancelTask = useCallback(
    async (taskId: string): Promise<void> => api.cancelTask(ws, taskId),
    [api, ws],
  );

  const listMyTasks = useCallback(
    async (query?: {
      status?: string;
      cursor?: string;
      limit?: number;
    }): Promise<{ tasks: TaskInfo[]; next_cursor: string }> =>
      api.listMyTasks(ws, query),
    [api, ws],
  );

  return useMemo(
    () => ({
      listTasks,
      createTask,
      getTask,
      updateTask,
      deleteTask,
      assignTask,
      completeTask,
      cancelTask,
      listMyTasks,
    }),
    [listTasks, createTask, getTask, updateTask, deleteTask, assignTask, completeTask, cancelTask, listMyTasks],
  );
}
