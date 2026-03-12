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
    <div className="w-[60px] bg-sidebar flex flex-col items-center py-3 gap-2 shrink-0">
      {/* Workspace icons */}
      <div className="flex-1 flex flex-col gap-2 items-center">
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
        className="w-10 h-10 rounded-full border-2 border-dashed border-border bg-transparent text-muted-foreground cursor-pointer text-xl flex items-center justify-center hover:border-primary hover:text-foreground transition-colors"
      >
        +
      </button>

      {/* User avatar */}
      {user && (
        <button
          onClick={onAvatarClick}
          title={user.name || user.email}
          className="w-9 h-9 rounded-full border-none cursor-pointer bg-primary text-primary-foreground text-sm font-bold flex items-center justify-center mt-2"
          style={{
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
