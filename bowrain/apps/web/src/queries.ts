import { queryOptions } from "@tanstack/react-query";
import type { ApiAdapter } from "@neokapi/ui";

export const configQueryOptions = (api: ApiAdapter) =>
  queryOptions({
    queryKey: ["config"],
    queryFn: () => api.getConfig(),
    staleTime: Infinity,
  });

export const currentUserQueryOptions = (api: ApiAdapter) =>
  queryOptions({
    queryKey: ["currentUser"],
    queryFn: () => api.getCurrentUser(),
    staleTime: 60_000,
  });

export const workspacesQueryOptions = (api: ApiAdapter) =>
  queryOptions({
    queryKey: ["workspaces"],
    queryFn: () => api.listWorkspaces(),
    staleTime: 30_000,
  });

export const projectsQueryOptions = (api: ApiAdapter, workspaceSlug: string) =>
  queryOptions({
    queryKey: ["projects", workspaceSlug],
    queryFn: () => api.listProjects(workspaceSlug),
    staleTime: 30_000,
  });

export const projectQueryOptions = (
  api: ApiAdapter,
  workspaceSlug: string,
  projectId: string,
  stream?: string,
) =>
  queryOptions({
    queryKey: ["project", workspaceSlug, projectId, stream ?? "main"],
    queryFn: () => api.getProject(workspaceSlug, projectId, stream),
    staleTime: 30_000,
  });

export const activitiesQueryOptions = (api: ApiAdapter, workspaceSlug: string) =>
  queryOptions({
    queryKey: ["activities", workspaceSlug],
    queryFn: () => api.listActivities(workspaceSlug, { limit: 20 }),
    staleTime: 30_000,
    refetchInterval: 60_000,
  });

export const myTasksQueryOptions = (api: ApiAdapter, workspaceSlug: string) =>
  queryOptions({
    queryKey: ["myTasks", workspaceSlug],
    queryFn: () => api.listMyTasks(workspaceSlug, { status: "open" }),
    staleTime: 30_000,
    refetchInterval: 60_000,
  });

export const translationDashboardQueryOptions = (
  api: ApiAdapter,
  workspaceSlug: string,
  projectId: string,
  stream?: string,
) =>
  queryOptions({
    queryKey: ["translationDashboard", workspaceSlug, projectId, stream ?? "main"],
    queryFn: () => api.getTranslationDashboard(workspaceSlug, projectId, stream),
    staleTime: 30_000,
    refetchInterval: 60_000,
  });
