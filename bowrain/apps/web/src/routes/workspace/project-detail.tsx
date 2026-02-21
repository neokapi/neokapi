import { useState, useEffect, useCallback } from "react";
import { useNavigate, useParams } from "@tanstack/react-router";
import { ProjectView, useApi, useWorkspace, type ProjectInfo } from "@gokapi/ui";

export function ProjectDetailRoute() {
  const navigate = useNavigate();
  const { workspace, projectId } = useParams({ strict: false });
  const adapter = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";

  const [project, setProject] = useState<ProjectInfo | null>(null);

  useEffect(() => {
    if (!ws || !projectId) return;
    adapter.getProject(ws, projectId).then(setProject).catch(() => setProject(null));
  }, [ws, projectId, adapter]);

  const handleUploadFiles = useCallback(
    async (files: File[]) => {
      if (!project) return;
      const updated = await adapter.uploadFiles(ws, project.id, files);
      setProject(updated);
    },
    [ws, adapter, project],
  );

  const handleRemoveFile = useCallback(
    async (fileName: string) => {
      if (!project) return;
      const updated = await adapter.removeFile(ws, project.id, fileName);
      setProject(updated);
    },
    [ws, adapter, project],
  );

  if (!project) {
    return (
      <div className="flex items-center justify-center h-full text-muted-foreground text-sm">
        Loading project...
      </div>
    );
  }

  return (
    <ProjectView
      project={project}
      onBack={() => navigate({ to: "/$workspace", params: { workspace: workspace ?? ws } })}
      onOpenFile={(f) =>
        navigate({
          to: "/$workspace/project/$projectId/translate/$fileName",
          params: { workspace: workspace ?? ws, projectId: project.id, fileName: f },
        })
      }
      onUploadFiles={handleUploadFiles}
      onRemoveFile={handleRemoveFile}
      onOpenTM={() =>
        navigate({ to: "/$workspace/memory", params: { workspace: workspace ?? ws } })
      }
      onOpenTerms={() =>
        navigate({ to: "/$workspace/termbase", params: { workspace: workspace ?? ws } })
      }
      serverMode={ws ? { serverURL: window.location.origin, workspaceSlug: ws } : undefined}
    />
  );
}
