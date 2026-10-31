import { useEffect } from "react";
import { useNavigate, useParams, useRouteContext } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import {
  ReviewInbox,
  useApi,
  type ReviewInboxProject,
  type ReviewInboxRules,
  type ReviewInboxTask,
} from "@neokapi/ui";
import type { WorkspaceRouteContext } from "..";
import { myReviewTasksQueryOptions } from "../../queries";

/**
 * ReviewInboxRoute is the workspace-level roll-up of pending review: every
 * project awaiting review, most-pending first, each linking into its focused
 * review session. It reads the loop rollup's per-project ship standing —
 * project-scoped, not assignee-scoped — so pooled and owner-assigned review is
 * visible, not only what's assigned to the current user.
 */
/**
 * Where a project's Rules opens from the inbox: on the questions assigned to
 * the member when they hold any there, else on every question.
 */
export function rulesSearch(
  rules: readonly ReviewInboxRules[],
  projectId: string,
): { assigned?: "me" } {
  const assigned = rules.find((r) => r.projectId === projectId)?.assigned ?? 0;
  return assigned > 0 ? { assigned: "me" } : {};
}

export function ReviewInboxRoute() {
  const navigate = useNavigate();
  const { workspace } = useParams({ strict: false });
  const adapter = useApi();
  const { activeWorkspace } = useRouteContext({ strict: false }) as WorkspaceRouteContext;
  const ws = activeWorkspace.slug;
  const wsParam = workspace ?? ws;

  useEffect(() => {
    document.title = `Review inbox · ${activeWorkspace.name} · Bowrain`;
  }, [activeWorkspace.name]);

  const { data: rollup, isLoading } = useQuery({
    queryKey: ["loopRollup", ws],
    queryFn: () => adapter.getLoopRollup(ws),
    enabled: !!ws,
    staleTime: 30_000,
  });

  // The same open-task set the dashboard counts as "review task(s) for you":
  // its card links here, so the filter runs server-side on both surfaces and
  // what is counted there is exactly what is listed here.
  const { data: reviewTasksData } = useQuery({
    ...myReviewTasksQueryOptions(adapter, ws),
    enabled: !!ws,
  });
  const tasks: ReviewInboxTask[] = (reviewTasksData?.tasks ?? []).map((t) => ({
    id: t.id,
    title: t.title,
    type: t.type,
    changesetId: t.data?.changeset_id,
    projectId: t.project_id || undefined,
    stream: t.stream,
  }));

  const projects: ReviewInboxProject[] = (rollup?.ship?.projects ?? []).map((p) => ({
    projectId: p.project_id,
    projectName: p.project_name ?? p.project_id,
    stream: p.stream ?? "main",
    pending: p.pending,
    established: p.established,
    translated: p.translated,
  }));

  // The questions waiting for a decision in each project. Rules opens on the
  // stream the project's review standing names, or main.
  const { data: queue } = useQuery({
    queryKey: ["contextQueue", ws],
    queryFn: () => adapter.getContextQueue(ws),
    enabled: !!ws,
    staleTime: 30_000,
  });
  const streamOf = new Map(projects.map((p) => [p.projectId, p.stream]));
  const rules: ReviewInboxRules[] = (queue?.projects ?? []).map((q) => ({
    projectId: q.project_id,
    projectName: q.project_name || q.project_id,
    stream: streamOf.get(q.project_id) ?? "main",
    waiting: q.waiting,
    assigned: q.assigned,
  }));

  const ship = rollup?.ship;
  const coverageNote =
    ship && ship.counted_projects < ship.total_projects
      ? `Showing ${ship.counted_projects} of ${ship.total_projects} projects with a cached rollup.`
      : undefined;

  return (
    <ReviewInbox
      tasks={tasks}
      onOpenTask={(t) => {
        if (t.changesetId) {
          void navigate({
            to: "/$workspace/context/changes/$id",
            params: { workspace: wsParam, id: t.changesetId },
          });
        } else if (t.projectId) {
          void navigate({
            to: "/$workspace/p/$projectId/s/$stream/review",
            params: { workspace: wsParam, projectId: t.projectId, stream: t.stream ?? "main" },
          });
        }
      }}
      projects={projects}
      rules={rules}
      onOpenRules={(projectId, stream) =>
        navigate({
          to: "/$workspace/p/$projectId/s/$stream/rules",
          params: { workspace: wsParam, projectId, stream },
          search: rulesSearch(rules, projectId),
        })
      }
      loading={isLoading}
      coverageNote={coverageNote}
      onOpenReview={(projectId, stream) =>
        navigate({
          to: "/$workspace/p/$projectId/s/$stream/review",
          params: { workspace: wsParam, projectId, stream },
        })
      }
    />
  );
}
