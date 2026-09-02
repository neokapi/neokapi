import { useEffect } from "react";
import { useNavigate, useParams, useRouteContext } from "@tanstack/react-router";
import { useSuspenseQuery } from "@tanstack/react-query";
import {
  TranslationEditor,
  PresenceAvatars,
  useApi,
  useCollaboration,
  useStream,
} from "@neokapi/ui";
import { projectQueryOptions } from "../../queries";
import type { WorkspaceRouteContext } from "..";
import { useEditorSurfaceNav } from "./useEditorSurfaceNav";

export function TranslateRoute() {
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

  useEffect(() => {
    document.title = `${fileName} · ${project.name} · Bowrain`;
  }, [fileName, project.name]);

  const surfaceTabs = useEditorSurfaceNav("translate");

  // Set up collaborative editing via WebSocket + Yjs.
  const collabEnabled = !!fileName && !!ws;
  const { connectedUsers, connectionState, setSelectedBlock } = useCollaboration({
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
    enabled: collabEnabled,
  });

  return (
    <TranslationEditor
      project={project}
      fileName={fileName}
      surfaceTabs={surfaceTabs}
      onSelectedBlockChange={collabEnabled ? setSelectedBlock : undefined}
      onBack={() =>
        navigate({
          to: "/$workspace/p/$projectId/s/$stream",
          params: {
            workspace: workspace ?? ws,
            projectId: project.id,
            stream: activeStream,
          },
        })
      }
      presenceSlot={
        <div className="flex items-center gap-2">
          <PresenceAvatars users={connectedUsers} currentUserId={user.id} />
          {connectionState === "connecting" && (
            <span className="text-xs text-muted-foreground">Connecting...</span>
          )}
        </div>
      }
    />
  );
}
