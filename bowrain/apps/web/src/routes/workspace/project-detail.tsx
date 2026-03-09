import { useCallback, useEffect } from "react";
import { useNavigate, useParams, useRouteContext } from "@tanstack/react-router";
import { useSuspenseQuery, useQueryClient } from "@tanstack/react-query";
import { ProjectView, useApi } from "@gokapi/ui";
import { projectQueryOptions } from "../../queries";
import type { WorkspaceRouteContext } from "..";

export function ProjectDetailRoute() {
  const navigate = useNavigate();
  const { workspace, projectId } = useParams({ strict: false });
  const adapter = useApi();
  const queryClient = useQueryClient();
  const { activeWorkspace } = useRouteContext({ strict: false }) as WorkspaceRouteContext;
  const ws = activeWorkspace.slug;

  const { data: project } = useSuspenseQuery(projectQueryOptions(adapter, ws, projectId!));

  useEffect(() => {
    document.title = `${project.name} — ${activeWorkspace.name} — Bowrain`;
  }, [project.name, activeWorkspace.name]);

  const handleUploadFiles = useCallback(
    async (files: File[]) => {
      await adapter.uploadFiles(ws, project.id, files);
      queryClient.invalidateQueries({ queryKey: ["project", ws, project.id] });
    },
    [ws, adapter, project.id, queryClient],
  );

  const handleRemoveFile = useCallback(
    async (fileName: string) => {
      await adapter.removeFile(ws, project.id, fileName);
      queryClient.invalidateQueries({ queryKey: ["project", ws, project.id] });
    },
    [ws, adapter, project.id, queryClient],
  );

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
