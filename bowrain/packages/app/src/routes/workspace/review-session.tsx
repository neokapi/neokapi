import { useEffect } from "react";
import { useNavigate, useParams, useRouteContext } from "@tanstack/react-router";
import { useSuspenseQuery } from "@tanstack/react-query";
import { ReviewSession, useApi } from "@neokapi/ui";
import { projectQueryOptions, translationDashboardQueryOptions } from "../../queries";
import type { WorkspaceRouteContext } from "..";

/**
 * ReviewSessionRoute is the project-level governed review session — the
 * dedicated surface (not a mode of the translation editor) that walks every
 * pending block across the project's items and locales in one focused,
 * keyboard-first flow. Every "review" entry point (run hand-off, dashboard
 * card, per-item review) routes here.
 */
export function ReviewSessionRoute() {
  const navigate = useNavigate();
  const { workspace, projectId, stream } = useParams({ strict: false });
  const adapter = useApi();
  const { activeWorkspace } = useRouteContext({ strict: false }) as WorkspaceRouteContext;
  const ws = activeWorkspace.slug;
  const wsParam = workspace ?? ws;
  const streamParam = stream ?? "main";

  const { data: project } = useSuspenseQuery(
    projectQueryOptions(adapter, ws, projectId!, streamParam),
  );
  const { data: stats } = useSuspenseQuery(
    translationDashboardQueryOptions(adapter, ws, projectId!, streamParam),
  );

  useEffect(() => {
    document.title = `Review — ${project.name} — Bowrain`;
  }, [project.name]);

  const baseParams = { workspace: wsParam, projectId: project.id, stream: streamParam };

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <ReviewSession
        project={project}
        dashboardStats={stats}
        stream={streamParam}
        onBack={() => navigate({ to: "/$workspace/p/$projectId/s/$stream", params: baseParams })}
        onOpenDelivery={() =>
          navigate({ to: "/$workspace/p/$projectId/s/$stream/dashboard", params: baseParams })
        }
        onOpenRuns={() =>
          navigate({ to: "/$workspace/p/$projectId/s/$stream/runs", params: baseParams })
        }
        onOpenChangeset={(id) =>
          navigate({
            to: "/$workspace/context/changes/$id",
            params: { workspace: wsParam, id },
          })
        }
      />
    </div>
  );
}
