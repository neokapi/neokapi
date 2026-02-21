import { useState, useEffect, useCallback } from "react";
import { useNavigate, useParams } from "@tanstack/react-router";
import { ProjectDashboard, useApi, useWorkspace, type ProjectInfo } from "@gokapi/ui";

export function ProjectDashboardRoute() {
  const navigate = useNavigate();
  const { workspace } = useParams({ strict: false });
  const adapter = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";

  const [projects, setProjects] = useState<ProjectInfo[]>([]);

  useEffect(() => {
    if (!ws) return;
    adapter.listProjects(ws).then(setProjects).catch(() => setProjects([]));
  }, [ws, adapter]);

  const handleCreateProject = useCallback(
    async (name: string, sourceLang: string, targetLangs: string[]) => {
      const info = await adapter.createProject(ws, name, sourceLang, targetLangs);
      setProjects((prev) => [...prev, info]);
      navigate({
        to: "/$workspace/project/$projectId",
        params: { workspace: workspace ?? ws, projectId: info.id },
      });
    },
    [ws, workspace, adapter, navigate],
  );

  const handleOpenProject = useCallback(
    (project: ProjectInfo) => {
      navigate({
        to: "/$workspace/project/$projectId",
        params: { workspace: workspace ?? ws, projectId: project.id },
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
