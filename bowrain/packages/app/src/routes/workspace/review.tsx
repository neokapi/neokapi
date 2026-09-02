import { useEffect } from "react";
import { useNavigate, useParams, useRouteContext } from "@tanstack/react-router";
import { useSuspenseQuery } from "@tanstack/react-query";
import { ReviewSurface, PresenceAvatars, useApi, useCollaboration, useStream } from "@neokapi/ui";
import { projectQueryOptions } from "../../queries";
import type { WorkspaceRouteContext } from "..";
import { useEditorSurfaceNav } from "./useEditorSurfaceNav";

/**
 * ReviewRoute is the block-level translation review surface — a sibling of the
 * translate route, scoped to the same file. It lists blocks by status, runs the checks,
 * and supports bulk review actions. Brand-rule promotion stays in
 * /context/voice/review.
 */
export function ReviewRoute() {
  const navigate = useNavigate();
  const { workspace, projectId, _splat } = useParams({ strict: false });
  const adapter = useApi();
  const { activeWorkspace, user } = useRouteContext({ strict: false }) as WorkspaceRouteContext;
  const ws = activeWorkspace.slug;
  const { activeStream } = useStream();

  const { data: project } = useSuspenseQuery(
    projectQueryOptions(adapter, ws, projectId!, activeStream),
  );

  // The item name is the route's trailing splat, so a name with slashes stays
  // one coordinate — the file this surface edits needs no lookup, and no
  // dependence on the project response carrying every item.
  const fileName = _splat ?? "";

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
          to: "/$workspace/p/$projectId/s/$stream/translate/$",
          params: {
            workspace: workspace ?? ws,
            projectId: project.id,
            stream: activeStream,
            _splat: fileName,
          },
        })
      }
      presenceSlot={<PresenceAvatars users={connectedUsers} currentUserId={user.id} />}
    />
  );
}
