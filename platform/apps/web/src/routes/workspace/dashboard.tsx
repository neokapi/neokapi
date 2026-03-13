import { useCallback, useEffect } from "react";
import { useNavigate, useParams, useRouteContext } from "@tanstack/react-router";
import { useSuspenseQuery, useQueryClient } from "@tanstack/react-query";
import { ProjectDashboard, useApi, type ProjectInfo } from "@neokapi/ui";
import { projectsQueryOptions } from "../../queries";
import type { WorkspaceRouteContext } from "..";

export function ProjectDashboardRoute() {
  const navigate = useNavigate();
  const { workspace } = useParams({ strict: false });
  const adapter = useApi();
  const queryClient = useQueryClient();
  const { activeWorkspace } = useRouteContext({ strict: false }) as WorkspaceRouteContext;
  const ws = activeWorkspace.slug;

  useEffect(() => {
    document.title = `${activeWorkspace.name} — Bowrain`;
  }, [activeWorkspace.name]);

  const { data: projects } = useSuspenseQuery(projectsQueryOptions(adapter, ws));

  const handleCreateProject = useCallback(
    async (name: string, sourceLang: string, targetLangs: string[]) => {
      const info = await adapter.createProject(ws, name, sourceLang, targetLangs);
      void queryClient.invalidateQueries({ queryKey: ["projects", ws] });
      void navigate({
        to: "/$workspace/project/$projectId/stream/$stream",
        params: { workspace: workspace ?? ws, projectId: info.id, stream: "main" },
      });
    },
    [ws, workspace, adapter, navigate, queryClient],
  );

  const handleOpenProject = useCallback(
    (project: ProjectInfo) => {
      void navigate({
        to: "/$workspace/project/$projectId/stream/$stream",
        params: { workspace: workspace ?? ws, projectId: project.id, stream: "main" },
      });
    },
    [navigate, workspace, ws],
  );

  return (
    <ProjectDashboard
      projects={projects}
      onCreateProject={handleCreateProject}
      onOpenProject={handleOpenProject}
    />
  );
}
