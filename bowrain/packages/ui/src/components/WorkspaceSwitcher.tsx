import type { Workspace } from "../types/api";
import { WorkspaceIcon } from "./WorkspaceIcon";
import { Plus } from "./icons";
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
} from "@neokapi/ui-primitives/components/ui/dropdown-menu";
import {
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from "@neokapi/ui-primitives/components/ui/sidebar";

interface WorkspaceSwitcherProps {
  workspaces: Workspace[];
  activeWorkspace: Workspace | null;
  onSelectWorkspace: (ws: Workspace) => void;
  onCreateWorkspace?: () => void;
}

export function WorkspaceSwitcher({
  workspaces,
  activeWorkspace,
  onSelectWorkspace,
  onCreateWorkspace,
}: WorkspaceSwitcherProps) {
  return (
    <SidebarMenu>
      <SidebarMenuItem>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <SidebarMenuButton
              className="data-[state=open]:bg-sidebar-accent data-[state=open]:text-sidebar-accent-foreground justify-center !p-1"
              tooltip={activeWorkspace?.name ?? "Select workspace"}
            >
              {activeWorkspace ? (
                <WorkspaceIcon
                  workspace={activeWorkspace}
                  active={false}
                  onClick={() => {}}
                  size={28}
                />
              ) : (
                <span className="text-xs font-bold">?</span>
              )}
            </SidebarMenuButton>
          </DropdownMenuTrigger>
          <DropdownMenuContent
            className="min-w-56 rounded-lg"
            align="start"
            side="right"
            sideOffset={8}
          >
            <DropdownMenuLabel className="text-xs text-muted-foreground">
              Workspaces
            </DropdownMenuLabel>
            {workspaces.map((ws) => (
              <DropdownMenuItem
                key={ws.id}
                onClick={() => onSelectWorkspace(ws)}
                className="gap-2 p-2"
              >
                <div className="flex size-6 items-center justify-center rounded-md border">
                  <WorkspaceIcon workspace={ws} active={false} onClick={() => {}} size={16} />
                </div>
                <span className="flex-1 truncate">{ws.name}</span>
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
                <DropdownMenuItem onClick={onCreateWorkspace} className="gap-2 p-2">
                  <div className="flex size-6 items-center justify-center rounded-md border bg-transparent">
                    <Plus className="size-4" />
                  </div>
                  <span className="font-medium text-muted-foreground">Create workspace</span>
                </DropdownMenuItem>
              </>
            )}
          </DropdownMenuContent>
        </DropdownMenu>
      </SidebarMenuItem>
    </SidebarMenu>
  );
}
