import { useEffect } from "react";
import { useParams, useRouteContext } from "@tanstack/react-router";
import { useSuspenseQuery } from "@tanstack/react-query";
import { TranslationDashboard, useApi, useStream } from "@neokapi/ui";
import { projectQueryOptions, translationDashboardQueryOptions } from "../../queries";
import type { WorkspaceRouteContext } from "..";

export function TranslationDashboardRoute() {
  const { projectId } = useParams({ strict: false });
  const adapter = useApi();
  const { activeWorkspace } = useRouteContext({ strict: false }) as WorkspaceRouteContext;
  const ws = activeWorkspace.slug;
  const { activeStream } = useStream();

  const { data: project } = useSuspenseQuery(
    projectQueryOptions(adapter, ws, projectId!, activeStream),
  );
  const { data: stats } = useSuspenseQuery(
    translationDashboardQueryOptions(adapter, ws, projectId!, activeStream),
  );

  useEffect(() => {
    document.title = `Dashboard — ${project.name} — ${activeWorkspace.name} — Bowrain`;
  }, [project.name, activeWorkspace.name]);

  return (
    <div className="mx-auto max-w-6xl p-6">
      <TranslationDashboard stats={stats} projectName={project.name} />
    </div>
  );
}
