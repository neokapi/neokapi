import { useEffect } from "react";
import { useNavigate, useParams, useRouteContext } from "@tanstack/react-router";
import { useSuspenseQuery } from "@tanstack/react-query";
import { ReviewSurface, PresenceAvatars, useApi, useCollaboration, useStream } from "@neokapi/ui";
import { projectQueryOptions } from "../../queries";
import type { WorkspaceRouteContext } from "..";
import { useEditorSurfaceNav } from "./useEditorSurfaceNav";

/**
 * ReviewRoute is the block-level translation review surface — a sibling of the
 * translate route, scoped to the same file. It lists blocks by status, runs QA,
 * and supports bulk review actions. Brand-rule promotion stays in /brand/review.
 */
export function ReviewRoute() {
  const navigate = useNavigate();
  const { workspace, projectId, itemId } = useParams({ strict: false });
  const adapter = useApi();
  const { activeWorkspace, user } = useRouteContext({ strict: false }) as WorkspaceRouteContext;
  const ws = activeWorkspace.slug;
  const { activeStream } = useStream();

  const { data: project } = useSuspenseQuery(
    projectQueryOptions(adapter, ws, projectId!, activeStream),
  );

  const item = project.items?.find((i) => i.id === itemId);
  const fileName = item?.name ?? itemId ?? "";

  const surfaceTabs = useEditorSurfaceNav("review");

  useEffect(() => {
    document.title = `Review · ${fileName} — ${project.name} — Bowrain`;
  }, [fileName, project.name]);

  const { connectedUsers } = useCollaboration({
    serverUrl: window.location.origin,
    workspace: ws,
    projectId: projectId ?? "",
    fileName,
    locale: project.target_languages?.[0] ?? "",
    user: {
      userId: user.id,
      name: user.name,
      avatarUrl: user.avatar_url,
    },
    enabled: !!fileName && !!ws,
  });

  return (
    <ReviewSurface
      project={project}
      fileName={fileName}
      surfaceTabs={surfaceTabs}
      onBack={() =>
        navigate({
          to: "/$workspace/p/$projectId/s/$stream/$itemId/translate",
          params: {
            workspace: workspace ?? ws,
            projectId: project.id,
            stream: activeStream,
            itemId: itemId ?? "",
          },
        })
      }
      presenceSlot={<PresenceAvatars users={connectedUsers} currentUserId={user.id} />}
    />
  );
}
