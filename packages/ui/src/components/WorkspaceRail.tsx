import type { Workspace, User } from "../types/api";
import { WorkspaceIcon } from "./WorkspaceIcon";

interface WorkspaceRailProps {
  workspaces: Workspace[];
  activeWorkspace: Workspace | null;
  onSelectWorkspace: (ws: Workspace) => void;
  onCreateWorkspace: () => void;
  user: User | null;
  onAvatarClick?: () => void;
}

export function WorkspaceRail({
  workspaces,
  activeWorkspace,
  onSelectWorkspace,
  onCreateWorkspace,
  user,
  onAvatarClick,
}: WorkspaceRailProps) {
  return (
    <div
      style={{
        width: 60,
        backgroundColor: "var(--bg-rail, #1a1a2e)",
        display: "flex",
        flexDirection: "column",
        alignItems: "center",
        paddingTop: 12,
        paddingBottom: 12,
        gap: 8,
        flexShrink: 0,
      }}
    >
      {/* Workspace icons */}
      <div style={{ flex: 1, display: "flex", flexDirection: "column", gap: 8, alignItems: "center" }}>
        {workspaces.map((ws) => (
          <WorkspaceIcon
            key={ws.id}
            workspace={ws}
            active={activeWorkspace?.id === ws.id}
            onClick={() => onSelectWorkspace(ws)}
          />
        ))}
      </div>

      {/* Create workspace button */}
      <button
        onClick={onCreateWorkspace}
        title="Create workspace"
        style={{
          width: 40,
          height: 40,
          borderRadius: "50%",
          border: "2px dashed var(--border, #444)",
          background: "transparent",
          color: "var(--text-secondary, #999)",
          cursor: "pointer",
          fontSize: 20,
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
        }}
      >
        +
      </button>

      {/* User avatar */}
      {user && (
        <button
          onClick={onAvatarClick}
          title={user.name || user.email}
          style={{
            width: 36,
            height: 36,
            borderRadius: "50%",
            border: "none",
            cursor: "pointer",
            backgroundColor: "var(--accent, #4A90D9)",
            color: "#fff",
            fontSize: 14,
            fontWeight: 700,
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            marginTop: 8,
            backgroundImage: user.avatar_url ? `url(${user.avatar_url})` : undefined,
            backgroundSize: "cover",
          }}
        >
          {!user.avatar_url && (user.name || "?")[0].toUpperCase()}
        </button>
      )}
    </div>
  );
}
