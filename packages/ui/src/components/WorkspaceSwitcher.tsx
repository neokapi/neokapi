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
import { useSidebar } from "./ui/sidebar";

interface WorkspaceSwitcherProps {
  workspaces: Workspace[];
  activeWorkspace: Workspace | null;
  onSelectWorkspace: (ws: Workspace) => void;
  onCreateWorkspace?: () => void;
}

/** Stacked workspace icons — active in front, another peeking behind. */
function StackedIcons({ active, other }: { active: Workspace; other: Workspace | null }) {
  return (
    <div className="relative" style={{ width: 40, height: 42 }}>
      {/* Back icon — offset to bottom-right, faded */}
      {other && (
        <div className="absolute" style={{ top: 3, left: 2, opacity: 0.35 }}>
          <WorkspaceIcon workspace={other} active={false} onClick={() => {}} size={32} />
        </div>
      )}
      {/* Front icon */}
      <div className="absolute" style={{ top: 0, left: 0, zIndex: 1 }}>
        <WorkspaceIcon workspace={active} active={false} onClick={() => {}} size={32} />
      </div>
    </div>
  );
}

export function WorkspaceSwitcher({
  workspaces,
  activeWorkspace,
  onSelectWorkspace,
  onCreateWorkspace,
}: WorkspaceSwitcherProps) {
  const { state } = useSidebar();
  const collapsed = state === "collapsed";

  // Find a second workspace to show behind the active one
  const otherWs = workspaces.find((ws) => ws.id !== activeWorkspace?.id) ?? null;

  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        className={
          collapsed
            ? "flex justify-center w-full py-3 bg-transparent border-none cursor-pointer transition-colors outline-none"
            : "flex items-center gap-2 w-full px-3 py-2.5 bg-transparent border-none cursor-pointer transition-colors outline-none text-sidebar-foreground hover:bg-sidebar-accent rounded-md"
        }
      >
        {collapsed && activeWorkspace ? (
          <StackedIcons active={activeWorkspace} other={otherWs} />
        ) : (
          activeWorkspace && (
            <WorkspaceIcon workspace={activeWorkspace} active={false} onClick={() => {}} size={28} />
          )
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
