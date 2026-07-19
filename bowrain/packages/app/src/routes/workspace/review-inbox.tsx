import { useEffect } from "react";
import { useNavigate, useParams, useRouteContext } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { ReviewInbox, useApi, type ReviewInboxProject } from "@neokapi/ui";
import type { WorkspaceRouteContext } from "..";

/**
 * ReviewInboxRoute is the workspace-level roll-up of pending review: every
 * project awaiting review, most-pending first, each linking into its focused
 * review session. It reads the loop rollup's per-project ship standing —
 * project-scoped, not assignee-scoped — so pooled and owner-assigned review is
 * visible, not only what's assigned to the current user.
 */
export function ReviewInboxRoute() {
  const navigate = useNavigate();
  const { workspace } = useParams({ strict: false });
  const adapter = useApi();
  const { activeWorkspace } = useRouteContext({ strict: false }) as WorkspaceRouteContext;
  const ws = activeWorkspace.slug;
  const wsParam = workspace ?? ws;

  useEffect(() => {
    document.title = `Review inbox — ${activeWorkspace.name} — Bowrain`;
  }, [activeWorkspace.name]);

  const { data: rollup, isLoading } = useQuery({
    queryKey: ["loopRollup", ws],
    queryFn: () => adapter.getLoopRollup(ws),
    enabled: !!ws,
    staleTime: 30_000,
  });

  const projects: ReviewInboxProject[] = (rollup?.ship?.projects ?? []).map((p) => ({
    projectId: p.project_id,
    projectName: p.project_name ?? p.project_id,
    stream: p.stream ?? "main",
    pending: p.pending,
    governed: p.governed,
    aiShippable: p.ai_shippable,
  }));

  const ship = rollup?.ship;
  const coverageNote =
    ship && ship.counted_projects < ship.total_projects
      ? `Showing ${ship.counted_projects} of ${ship.total_projects} projects with a cached rollup.`
      : undefined;

  return (
    <ReviewInbox
      projects={projects}
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
