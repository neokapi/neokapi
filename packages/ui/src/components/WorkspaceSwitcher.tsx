import type { Workspace } from "../types/api";
import { WorkspaceIcon } from "./WorkspaceIcon";
import { ChevronsUpDown, Plus } from "./icons";
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
} from "./ui/dropdown-menu";

interface WorkspaceSwitcherProps {
  workspaces: Workspace[];
  activeWorkspace: Workspace | null;
  onSelectWorkspace: (ws: Workspace) => void;
  onCreateWorkspace?: () => void;
  collapsed?: boolean;
}

export function WorkspaceSwitcher({
  workspaces,
  activeWorkspace,
  onSelectWorkspace,
  onCreateWorkspace,
  collapsed = false,
}: WorkspaceSwitcherProps) {
  const hasMultiple = workspaces.length > 1 || !!onCreateWorkspace;

  // Static label when single workspace and no create action
  if (!hasMultiple) {
    return (
      <div
        className="flex items-center gap-2 px-3 py-2.5"
        style={{ color: "var(--semantic-text)" }}
      >
        {activeWorkspace && (
          <WorkspaceIcon workspace={activeWorkspace} active={false} onClick={() => {}} size={28} />
        )}
        {!collapsed && (
          <span className="text-sm font-semibold truncate">
            {activeWorkspace?.name || "No workspace"}
          </span>
        )}
      </div>
    );
  }

  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        className="flex items-center gap-2 w-full px-3 py-2.5 bg-transparent border-none cursor-pointer transition-colors outline-none"
        style={{ color: "var(--semantic-text)" }}
        onMouseEnter={(e: React.MouseEvent<HTMLButtonElement>) => {
          e.currentTarget.style.background = "var(--semantic-surface-elevated)";
        }}
        onMouseLeave={(e: React.MouseEvent<HTMLButtonElement>) => {
          e.currentTarget.style.background = "transparent";
        }}
      >
        {activeWorkspace && (
          <WorkspaceIcon workspace={activeWorkspace} active={false} onClick={() => {}} size={28} />
        )}
        {!collapsed && (
          <>
            <span className="flex-1 text-sm font-semibold truncate text-left">
              {activeWorkspace?.name || "No workspace"}
            </span>
            <ChevronsUpDown className="w-4 h-4 shrink-0 opacity-50" />
          </>
        )}
      </DropdownMenuTrigger>
      <DropdownMenuContent
        align="start"
        className="w-[200px] backdrop-blur-lg"
      >
        {workspaces.map((ws) => (
          <DropdownMenuItem
            key={ws.id}
            onClick={() => onSelectWorkspace(ws)}
            className="flex items-center gap-2"
          >
            <WorkspaceIcon workspace={ws} active={false} onClick={() => {}} size={24} />
            <span className="flex-1 truncate text-sm">{ws.name}</span>
            {ws.role && (
              <span className="text-[10px] text-muted-foreground uppercase tracking-wide">
                {ws.role}
              </span>
            )}
          </DropdownMenuItem>
        ))}
        {onCreateWorkspace && (
          <>
            <DropdownMenuSeparator />
            <DropdownMenuItem onClick={onCreateWorkspace} className="flex items-center gap-2">
              <Plus className="w-4 h-4" />
              <span className="text-sm">Create workspace</span>
            </DropdownMenuItem>
          </>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
