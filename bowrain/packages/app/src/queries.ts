import { queryOptions } from "@tanstack/react-query";
import type { ApiAdapter, TranslationDashboardItemOpts } from "@neokapi/ui";

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

export const brandProfilesQueryOptions = (api: ApiAdapter, workspaceSlug: string) =>
  queryOptions({
    queryKey: ["brand-profiles", workspaceSlug],
    queryFn: () => api.listBrandProfiles(workspaceSlug),
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

export const convergenceRunsQueryOptions = (
  api: ApiAdapter,
  workspaceSlug: string,
  projectId: string,
) =>
  queryOptions({
    queryKey: ["convergenceRuns", workspaceSlug, projectId],
    queryFn: () => api.listConvergenceRuns(workspaceSlug, projectId, 20),
    staleTime: 15_000,
    refetchInterval: 30_000,
  });

// The provider-free pre-flight estimate for a project's next convergence run
// (epic 019): fetched when the run-now consent dialog opens. Short staleTime so a
// re-open reflects a freshly-settled source or an added credit pack.
export const convergenceEstimateQueryOptions = (
  api: ApiAdapter,
  workspaceSlug: string,
  projectId: string,
) =>
  queryOptions({
    queryKey: ["convergenceEstimate", workspaceSlug, projectId],
    queryFn: () => api.estimateConvergence(workspaceSlug, projectId),
    staleTime: 5_000,
  });

export const myTasksQueryOptions = (api: ApiAdapter, workspaceSlug: string) =>
  queryOptions({
    queryKey: ["myTasks", workspaceSlug],
    queryFn: () => api.listMyTasks(workspaceSlug, { status: "open" }),
    staleTime: 30_000,
    refetchInterval: 60_000,
  });

/**
 * The change-sets waiting on a review. The nav wears this count so a workspace
 * says it has governed work waiting before anyone opens the page it is waiting
 * on. The key matches the one @neokapi/ui's useChangesets uses, so a lifecycle
 * mutation invalidates both at once.
 */
export const pendingChangesetsQueryOptions = (api: ApiAdapter, workspaceSlug: string) =>
  queryOptions({
    queryKey: ["changesets", workspaceSlug, "in_review"],
    queryFn: () => api.listChangesets(workspaceSlug, "in_review"),
    staleTime: 30_000,
    refetchInterval: 60_000,
  });

/** Page size for the dashboard's per-file table (server-sorted, lazy-loaded). */
export const DASHBOARD_ITEM_PAGE_SIZE = 100;

/**
 * Default item paging for the dashboard query: first page, name ascending.
 * The route loader primes the cache with exactly these options so the
 * component's initial render is a cache hit.
 */
export const defaultDashboardItemOpts: TranslationDashboardItemOpts = {
  itemLimit: DASHBOARD_ITEM_PAGE_SIZE,
  itemSort: "name",
  itemDir: "asc",
};

export const translationDashboardQueryOptions = (
  api: ApiAdapter,
  workspaceSlug: string,
  projectId: string,
  stream?: string,
  itemOpts: TranslationDashboardItemOpts = defaultDashboardItemOpts,
) =>
  queryOptions({
    queryKey: [
      "translationDashboard",
      workspaceSlug,
      projectId,
      stream ?? "main",
      itemOpts.itemLimit ?? 0,
      itemOpts.itemOffset ?? 0,
      itemOpts.itemSort ?? "name",
      itemOpts.itemDir ?? "asc",
    ],
    queryFn: () => api.getTranslationDashboard(workspaceSlug, projectId, stream, itemOpts),
    staleTime: 30_000,
    // Freshness is SSE-driven: useWorkspaceEvents invalidates the
    // ["translationDashboard", ws, id] prefix (debounced) on block/item/
    // stream/flow/connector change events. The long interval is only a
    // safety net for missed frames (dropped SSE connection).
    refetchInterval: 300_000,
  });

// Measured steerability: the project's per-locale model sweep measurements
// plus the instance gate. Refreshed on demand from the settings panel (no
// polling — a sweep lands on the next visit or explicit refetch).
export const modelRecommendationsQueryOptions = (
  api: ApiAdapter,
  workspaceSlug: string,
  projectId: string,
) =>
  queryOptions({
    queryKey: ["modelRecommendations", workspaceSlug, projectId],
    queryFn: () => api.getModelRecommendations(workspaceSlug, projectId),
    staleTime: 30_000,
  });
