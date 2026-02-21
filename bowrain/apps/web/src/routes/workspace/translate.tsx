import { useState, useEffect } from "react";
import { useNavigate, useParams } from "@tanstack/react-router";
import {
  TranslationEditor,
  PresenceAvatars,
  useApi,
  useAuth,
  useWorkspace,
  useCollaboration,
  type ProjectInfo,
} from "@gokapi/ui";

export function TranslateRoute() {
  const navigate = useNavigate();
  const { workspace, projectId, fileName } = useParams({ strict: false });
  const adapter = useApi();
  const { user } = useAuth();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";

  const [project, setProject] = useState<ProjectInfo | null>(null);

  useEffect(() => {
    if (!ws || !projectId) return;
    adapter.getProject(ws, projectId).then(setProject).catch(() => setProject(null));
  }, [ws, projectId, adapter]);

  // Set up collaborative editing via WebSocket + Yjs.
  const { connectedUsers, connectionState } = useCollaboration({
    serverUrl: window.location.origin,
    workspace: ws,
    projectId: projectId ?? "",
    fileName: fileName ?? "",
    locale: project?.target_locales?.[0] ?? "",
    user: {
      userId: user?.id ?? "anonymous",
      name: user?.name ?? "Anonymous",
      avatarUrl: user?.avatar_url,
    },
    enabled: !!project && !!fileName && !!ws,
  });

  if (!project || !fileName) {
    return (
      <div className="flex items-center justify-center h-full text-muted-foreground text-sm">
        Loading editor...
      </div>
    );
  }

  return (
    <TranslationEditor
      project={project}
      fileName={fileName}
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
            currentUserId={user?.id}
          />
          {connectionState === "connecting" && (
            <span className="text-xs text-muted-foreground">Connecting...</span>
          )}
        </div>
      }
    />
  );
}
