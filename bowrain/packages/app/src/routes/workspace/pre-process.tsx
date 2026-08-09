import { useEffect } from "react";
import { useNavigate, useParams, useRouteContext } from "@tanstack/react-router";
import { useSuspenseQuery } from "@tanstack/react-query";
import { PreProcessSurface, useApi, useStream } from "@neokapi/ui";
import { projectQueryOptions } from "../../queries";
import type { WorkspaceRouteContext } from "..";
import { useEditorSurfaceNav } from "./useEditorSurfaceNav";

/**
 * PreProcessRoute is the pre-flight surface — file-wide source-prep (pseudo,
 * bulk content-memory leverage, AI draft) run before per-block translation. Sibling of the
 * translate route, scoped to the same file.
 */
export function PreProcessRoute() {
  const navigate = useNavigate();
  const { workspace, projectId, itemName } = useParams({ strict: false });
  const adapter = useApi();
  const { activeWorkspace } = useRouteContext({ strict: false }) as WorkspaceRouteContext;
  const ws = activeWorkspace.slug;
  const { activeStream } = useStream();

  const { data: project } = useSuspenseQuery(
    projectQueryOptions(adapter, ws, projectId!, activeStream),
  );

  const fileName = itemName ?? "";

  const surfaceTabs = useEditorSurfaceNav("pre-process");

  useEffect(() => {
    document.title = `Pre-process · ${fileName} — ${project.name} — Bowrain`;
  }, [fileName, project.name]);

  return (
    <PreProcessSurface
      project={project}
      fileName={fileName}
      surfaceTabs={surfaceTabs}
      onBack={() =>
        navigate({
          to: "/$workspace/p/$projectId/s/$stream/$itemName/translate",
          params: {
            workspace: workspace ?? ws,
            projectId: project.id,
            stream: activeStream,
            itemName: fileName,
          },
        })
      }
    />
  );
}
