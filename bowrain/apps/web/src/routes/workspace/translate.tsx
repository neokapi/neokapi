import { useNavigate, useParams, useRouteContext } from "@tanstack/react-router";
import { useSuspenseQuery } from "@tanstack/react-query";
import {
  TranslationEditor,
  PresenceAvatars,
  useApi,
  useCollaboration,
} from "@gokapi/ui";
import { projectQueryOptions } from "../../queries";
import type { WorkspaceRouteContext } from "..";

export function TranslateRoute() {
  const navigate = useNavigate();
  const { workspace, projectId, fileName } = useParams({ strict: false });
  const adapter = useApi();
  const { activeWorkspace, user } = useRouteContext({ strict: false }) as WorkspaceRouteContext;
  const ws = activeWorkspace.slug;

  const { data: project } = useSuspenseQuery(projectQueryOptions(adapter, ws, projectId!));

  // Set up collaborative editing via WebSocket + Yjs.
  const { connectedUsers, connectionState } = useCollaboration({
    serverUrl: window.location.origin,
    workspace: ws,
    projectId: projectId ?? "",
    fileName: fileName ?? "",
    locale: project.target_locales?.[0] ?? "",
    user: {
      userId: user.id,
      name: user.name,
      avatarUrl: user.avatar_url,
    },
    enabled: !!fileName && !!ws,
  });

  return (
    <TranslationEditor
      project={project}
      fileName={fileName!}
      onBack={() =>
        navigate({
          to: "/$workspace/project/$projectId",
          params: { workspace: workspace ?? ws, projectId: project.id },
        })
      }
      presenceSlot={
        <div className="flex items-center gap-2">
          <PresenceAvatars
            users={connectedUsers}
            currentUserId={user.id}
          />
          {connectionState === "connecting" && (
            <span className="text-xs text-muted-foreground">Connecting...</span>
          )}
        </div>
      }
    />
  );
}
