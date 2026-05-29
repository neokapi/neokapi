import { useEffect, useState } from "react";
import { TranslationEditor, PresenceAvatars, useCollaboration } from "@neokapi/ui";
import type { ProjectInfo } from "@neokapi/ui";
import type { WailsApiAdapter, CollabSessionInfo } from "../api/WailsApiAdapter";

interface DesktopTranslateViewProps {
  adapter: WailsApiAdapter;
  project: ProjectInfo;
  fileName: string;
  /** Active workspace slug (server mode) used to scope the collab room. */
  workspaceSlug: string;
  onBack: () => void;
  onExport: (blob: Blob, fileName: string) => void;
  /** Cross-surface switcher slot (Pre-process/Translate/Review). */
  surfaceTabs?: React.ReactNode;
  /** Bumped by the backend-events layer to force a block reload on external change. */
  reloadSignal?: number;
}

/**
 * DesktopTranslateView is the desktop's parity counterpart to the web
 * translate route: it mounts the shared TranslationEditor and, when connected
 * to a server, layers presence co-editing on top via the same Yjs awareness
 * useCollaboration hook the web app uses.
 *
 * Unlike the web app, the desktop frontend has no access to window.location or
 * the auth token (it talks to its Go backend over Wails bindings, and the token
 * lives in the OS keychain). So it first asks the backend for a CollabSession —
 * server URL + keychain token + current user — and feeds that to the hook.
 * Block-content persistence stays server-authoritative via the existing
 * WailsApiAdapter.updateBlockTarget, exactly like web; presence is awareness
 * only. This supersedes the gRPC UpdatePresence path (no longer called from the
 * editor view) in favour of Yjs awareness.
 *
 * The Visual view renders the shared DocumentPreview (driven by the Wails
 * adapter's renderDocumentPreview), so the editor no longer needs a custom
 * renderPreview injection — both web and desktop share the same preview.
 */
export function DesktopTranslateView({
  adapter,
  project,
  fileName,
  workspaceSlug,
  onBack,
  onExport,
  surfaceTabs,
  reloadSignal,
}: DesktopTranslateViewProps) {
  const [session, setSession] = useState<CollabSessionInfo | null>(null);

  // Fetch the collab session from the backend. Failure (e.g. local/standalone
  // mode with no server) simply disables presence; the editor still works.
  useEffect(() => {
    let cancelled = false;
    adapter
      .getCollabSession()
      .then((s) => {
        if (!cancelled) setSession(s);
      })
      .catch(() => {
        if (!cancelled) setSession(null);
      });
    return () => {
      cancelled = true;
    };
  }, [adapter]);

  const collabEnabled = !!session?.serverUrl && !!workspaceSlug && !!fileName;

  const { connectedUsers, connectionState, setSelectedBlock } = useCollaboration({
    serverUrl: session?.serverUrl ?? "",
    workspace: workspaceSlug,
    projectId: project.id,
    fileName,
    locale: project.target_languages?.[0] ?? "",
    user: {
      userId: session?.user.userId ?? "",
      name: session?.user.name ?? "",
      avatarUrl: session?.user.avatarUrl,
    },
    authToken: session?.authToken,
    enabled: collabEnabled,
  });

  return (
    <TranslationEditor
      project={project}
      fileName={fileName}
      onBack={onBack}
      onExport={onExport}
      surfaceTabs={surfaceTabs}
      reloadSignal={reloadSignal}
      onSelectedBlockChange={collabEnabled ? setSelectedBlock : undefined}
      presenceSlot={
        collabEnabled ? (
          <div className="flex items-center gap-2">
            <PresenceAvatars users={connectedUsers} currentUserId={session?.user.userId} />
            {connectionState === "connecting" && (
              <span className="text-xs text-muted-foreground">Connecting…</span>
            )}
          </div>
        ) : undefined
      }
    />
  );
}
