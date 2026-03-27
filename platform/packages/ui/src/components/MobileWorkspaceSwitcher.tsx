import type { Workspace } from "../types/api";
import { WorkspaceIcon } from "./WorkspaceIcon";
import { ChevronsUpDown, Plus } from "./icons";
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
} from "./ui/dropdown-menu";
import { SidebarMenu, SidebarMenuButton, SidebarMenuItem, useSidebar } from "./ui/sidebar";

interface MobileWorkspaceSwitcherProps {
  workspaces: Workspace[];
  activeWorkspace: Workspace | null;
  onSelectWorkspace: (ws: Workspace) => void;
  onCreateWorkspace?: () => void;
}

export function MobileWorkspaceSwitcher({
  workspaces,
  activeWorkspace,
  onSelectWorkspace,
  onCreateWorkspace,
}: MobileWorkspaceSwitcherProps) {
  const { setOpenMobile } = useSidebar();

  return (
    <SidebarMenu>
      <SidebarMenuItem>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <SidebarMenuButton
              size="lg"
              className="data-[state=open]:bg-sidebar-accent data-[state=open]:text-sidebar-accent-foreground"
            >
              {activeWorkspace && (
                <div className="flex aspect-square size-8 items-center justify-center rounded-lg bg-sidebar-primary text-sidebar-primary-foreground">
                  <WorkspaceIcon
                    workspace={activeWorkspace}
                    active={false}
                    onClick={() => {}}
                    size={20}
                  />
                </div>
              )}
              <div className="grid flex-1 text-left text-sm leading-tight">
                <span className="truncate font-medium">
                  {activeWorkspace?.name || "No workspace"}
                </span>
                {activeWorkspace?.role && (
                  <span className="truncate text-xs text-muted-foreground">
                    {activeWorkspace.role}
                  </span>
                )}
              </div>
              <ChevronsUpDown className="ml-auto" />
            </SidebarMenuButton>
          </DropdownMenuTrigger>
          <DropdownMenuContent
            className="min-w-56 rounded-lg"
            align="start"
            side="bottom"
            sideOffset={4}
          >
            <DropdownMenuLabel className="text-xs text-muted-foreground">
              Workspaces
            </DropdownMenuLabel>
            {workspaces.map((ws) => (
              <DropdownMenuItem
                key={ws.id}
                onClick={() => {
                  onSelectWorkspace(ws);
                  setOpenMobile(false);
                }}
                className="gap-2 p-2"
              >
                <div className="flex size-6 items-center justify-center rounded-md border">
                  <WorkspaceIcon workspace={ws} active={false} onClick={() => {}} size={16} />
                </div>
                <span className="flex-1 truncate">{ws.name}</span>
              </DropdownMenuItem>
            ))}
            {onCreateWorkspace && (
              <>
                <DropdownMenuSeparator />
                <DropdownMenuItem
                  onClick={() => {
                    onCreateWorkspace();
                    setOpenMobile(false);
                  }}
                  className="gap-2 p-2"
                >
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
