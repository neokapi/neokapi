import { useCallback, useEffect, useState } from "react";
import { useNavigate, useParams, useRouteContext } from "@tanstack/react-router";
import { useSuspenseQuery, useQueryClient } from "@tanstack/react-query";
import { ProjectDashboard, ConfirmDialog, useApi, type ProjectInfo } from "@neokapi/ui";
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

  const invalidateProjects = useCallback(() => {
    void queryClient.invalidateQueries({ queryKey: ["projects", ws] });
  }, [queryClient, ws]);

  const handleCreateProject = useCallback(
    async (name: string, sourceLang: string, targetLangs: string[]) => {
      const info = await adapter.createProject(ws, name, sourceLang, targetLangs);
      invalidateProjects();
      void navigate({
        to: "/$workspace/p/$projectId/s/$stream",
        params: { workspace: workspace ?? ws, projectId: info.id, stream: "main" },
      });
    },
    [ws, workspace, adapter, navigate, invalidateProjects],
  );

  const handleOpenProject = useCallback(
    (project: ProjectInfo) => {
      void navigate({
        to: "/$workspace/p/$projectId/s/$stream",
        params: { workspace: workspace ?? ws, projectId: project.id, stream: "main" },
      });
    },
    [navigate, workspace, ws],
  );

  const handleCreateSampleProject = useCallback(async () => {
    const info = await adapter.createProject(ws, "Sample Project", "en", ["fr", "de", "ja"]);
    invalidateProjects();
    void navigate({
      to: "/$workspace/p/$projectId/s/$stream",
      params: { workspace: workspace ?? ws, projectId: info.id, stream: "main" },
    });
  }, [ws, workspace, adapter, navigate, invalidateProjects]);

  const handleEditProject = useCallback(
    async (projectId: string, data: { name?: string; target_languages?: string[] }) => {
      await adapter.updateProject(ws, projectId, data);
      invalidateProjects();
    },
    [ws, adapter, invalidateProjects],
  );

  const [archiveProjectId, setArchiveProjectId] = useState<string | null>(null);
  const confirmArchiveProject = useCallback(async () => {
    if (!archiveProjectId) return;
    await adapter.deleteProject(ws, archiveProjectId);
    setArchiveProjectId(null);
    invalidateProjects();
  }, [ws, adapter, archiveProjectId, invalidateProjects]);

  return (
    <>
      <ProjectDashboard
        projects={projects}
        onCreateProject={handleCreateProject}
        onOpenProject={handleOpenProject}
        onCreateSampleProject={handleCreateSampleProject}
        workspaceName={activeWorkspace.name}
        onEditProject={handleEditProject}
        onArchiveProject={setArchiveProjectId}
        workspaceLanguages={activeWorkspace.languages}
      />

      <ConfirmDialog
        open={archiveProjectId !== null}
        onOpenChange={(v) => { if (!v) setArchiveProjectId(null); }}
        title="Archive project"
        description="This project will be moved to the Bin. You can restore it at any time."
        confirmLabel="Archive"
        variant="destructive"
        onConfirm={confirmArchiveProject}
      />
    </>
  );
}
