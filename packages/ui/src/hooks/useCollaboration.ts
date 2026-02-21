import { useEffect, useRef, useState, useCallback } from "react";
import * as Y from "yjs";
import { WebsocketProvider } from "y-websocket";

/** Presence state for each connected user. */
export interface CollabUser {
  userId: string;
  name: string;
  avatarUrl?: string;
  /** Currently selected block ID, if any. */
  selectedBlock?: string;
  color: string;
}

/** Connection state for the collaboration session. */
export type CollabConnectionState = "disconnected" | "connecting" | "connected";

/** Options for the useCollaboration hook. */
export interface UseCollaborationOptions {
  /** Base URL of the bowrain server (e.g. "http://localhost:8080"). */
  serverUrl: string;
  /** Workspace slug. */
  workspace: string;
  /** Project ID. */
  projectId: string;
  /** File name being edited. */
  fileName: string;
  /** Target locale being edited. */
  locale: string;
  /** Current user info. */
  user: {
    userId: string;
    name: string;
    avatarUrl?: string;
  };
  /** Auth token for WebSocket connection. */
  authToken?: string;
  /** Whether collaboration is enabled. Default true. */
  enabled?: boolean;
}

// Deterministic color palette for user avatars.
const COLORS = [
  "#f43f5e", "#8b5cf6", "#3b82f6", "#06b6d4", "#10b981",
  "#f59e0b", "#ef4444", "#6366f1", "#0ea5e9", "#14b8a6",
];

function colorForUser(userId: string): string {
  let hash = 0;
  for (let i = 0; i < userId.length; i++) {
    hash = ((hash << 5) - hash + userId.charCodeAt(i)) | 0;
  }
  return COLORS[Math.abs(hash) % COLORS.length];
}

/**
 * React hook for collaborative editing via Yjs + WebSocket.
 *
 * Sets up a Y.Doc + WebsocketProvider connected to the bowrain server's
 * collab endpoint. Returns the shared document, connected users, and
 * connection state.
 */
export function useCollaboration(options: UseCollaborationOptions) {
  const {
    serverUrl,
    workspace,
    projectId,
    fileName,
    locale,
    user,
    authToken,
    enabled = true,
  } = options;

  const [connectionState, setConnectionState] = useState<CollabConnectionState>("disconnected");
  const [connectedUsers, setConnectedUsers] = useState<CollabUser[]>([]);

  const ydocRef = useRef<Y.Doc | null>(null);
  const providerRef = useRef<WebsocketProvider | null>(null);

  // Update local awareness state (e.g. selected block).
  const setSelectedBlock = useCallback((blockId: string | undefined) => {
    const provider = providerRef.current;
    if (!provider) return;

    provider.awareness.setLocalStateField("selectedBlock", blockId);
  }, []);

  useEffect(() => {
    if (!enabled) return;

    const ydoc = new Y.Doc();
    ydocRef.current = ydoc;

    // Build WebSocket URL for the collab endpoint.
    const wsBase = serverUrl.replace(/^http/, "ws");
    const wsUrl = `${wsBase}/api/v1/workspaces/${workspace}/editor/projects/${projectId}/collab/${encodeURIComponent(fileName)}?locale=${encodeURIComponent(locale)}`;

    // Room name used by y-websocket protocol.
    const roomName = `${workspace}:${projectId}:${fileName}:${locale}`;

    const provider = new WebsocketProvider(wsUrl, roomName, ydoc, {
      connect: true,
      // Pass auth token as protocol parameter.
      params: authToken ? { token: authToken } : {},
      protocols: ["yjs"],
    });
    providerRef.current = provider;

    // Set local awareness state.
    provider.awareness.setLocalState({
      userId: user.userId,
      name: user.name,
      avatarUrl: user.avatarUrl,
      color: colorForUser(user.userId),
    });

    // Track connection state.
    const handleStatus = ({ status }: { status: string }) => {
      if (status === "connected") setConnectionState("connected");
      else if (status === "connecting") setConnectionState("connecting");
      else setConnectionState("disconnected");
    };
    provider.on("status", handleStatus);

    // Track connected users from awareness protocol.
    const handleAwareness = () => {
      const states = provider.awareness.getStates();
      const users: CollabUser[] = [];
      states.forEach((state) => {
        if (state && state.userId) {
          users.push({
            userId: state.userId as string,
            name: (state.name as string) || "Unknown",
            avatarUrl: state.avatarUrl as string | undefined,
            selectedBlock: state.selectedBlock as string | undefined,
            color: (state.color as string) || colorForUser(state.userId as string),
          });
        }
      });
      setConnectedUsers(users);
    };
    provider.awareness.on("change", handleAwareness);

    return () => {
      provider.off("status", handleStatus);
      provider.awareness.off("change", handleAwareness);
      provider.destroy();
      ydoc.destroy();
      ydocRef.current = null;
      providerRef.current = null;
      setConnectionState("disconnected");
      setConnectedUsers([]);
    };
  }, [enabled, serverUrl, workspace, projectId, fileName, locale, user.userId, user.name, user.avatarUrl, authToken]);

  return {
    /** The shared Yjs document. */
    ydoc: ydocRef.current,
    /** WebSocket provider for direct access. */
    provider: providerRef.current,
    /** Current connection state. */
    connectionState,
    /** All currently connected users (including self). */
    connectedUsers,
    /** Update the local user's selected block for presence tracking. */
    setSelectedBlock,
  };
}
