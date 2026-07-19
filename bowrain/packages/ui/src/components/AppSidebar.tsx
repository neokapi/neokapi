import {
  Sidebar,
  SidebarContent,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarSeparator,
  useSidebar,
} from "@neokapi/ui-primitives";
import { useMemo } from "react";
import { t } from "@neokapi/i18n-react/runtime";
import type { Workspace, User, ProjectInfo, StreamInfo } from "../types/api";
import {
  Brain,
  Settings,
  Palette,
  Home,
  LayoutDashboard,
  Sparkles,
  Clock,
  Trash2,
  ArrowLeft,
  Globe,
  Users,
  KeyRound,
  Info,
  CreditCard,
  Shield,
  Lock,
  Network,
  FlaskConical,
  Activity,
  Rocket,
  Plug,
} from "./icons";
import { WorkspaceSwitcher } from "./WorkspaceSwitcher";
import { MobileWorkspaceSwitcher } from "./MobileWorkspaceSwitcher";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export type View = "translate" | "brand" | "termbase" | "memory" | "auditlog" | "bin" | "settings";

export interface NavItem {
  id: string;
  label: string;
  icon: React.ReactNode;
}

export interface SubNavItem {
  id: string;
  label: string;
  icon: React.ReactNode;
}

type ConnectionState = "disconnected" | "connecting" | "connected" | "offline";

export type ProjectView = "dashboard" | "automations" | "runs" | "connectors";

export type SidebarContext =
  | { level: "workspace"; activeView: View }
  | {
      level: "project";
      project: ProjectInfo;
      activeStream: string;
      activeProjectView: ProjectView;
      onBack: () => void;
      onOpenDashboard: () => void;
      onOpenFile: (fileName: string) => void;
      onStreamChange: (stream: string) => void;
      onCreateStream?: () => void;
      onEditStream?: (stream: StreamInfo) => void;
      onMergeStream?: (streamName: string) => void;
      onDiffStream?: (streamName: string) => void;
      onDeleteStream?: (streamName: string) => void;
      onOpenAutomations?: () => void;
      onOpenRuns?: () => void;
      onOpenConnectors?: () => void;
    };

export interface AppSidebarProps<V extends string = string> {
  workspaces: Workspace[];
  activeWorkspace: Workspace | null;
  onSelectWorkspace: (ws: Workspace) => void;
  onCreateWorkspace?: () => void;
  activeView: V;
  onViewChange: (view: V) => void;
  extraNavItems?: NavItem[];
  user: User | null;
  onSignOut?: () => void;
  collapsed: boolean;
  onCollapsedChange: (collapsed: boolean) => void;
  collapsedWidth?: number;
  connectionState?: ConnectionState;
  pendingChanges?: number;
  showThemeToggle?: boolean;
  sidebarContext?: SidebarContext;
  activeSubNav?: string;
  onSubNavChange?: (id: string) => void;
  /**
   * Sub-nav item ids to hide from the secondary menu (e.g. gate a feature-flagged
   * item like the @bravo settings entry). Filtered out of the rendered sub-nav.
   */
  hiddenSubNavIds?: string[];
}

// ---------------------------------------------------------------------------
// Nav data
// ---------------------------------------------------------------------------
// Getter functions, not module-level consts: the labels go through t() so the
// neokapi-i18n transform extracts them and looks them up in the active locale's
// dictionary at CALL time. Evaluating these per render (the components call
// them below) picks up a locale switch; a module-level const would freeze the
// labels in whatever locale was active at import time.

const workspaceNavItems = (): NavItem[] => [
  { id: "translate", label: t("Projects"), icon: <Home /> },
  { id: "brand", label: t("Brand"), icon: <Palette /> },
  { id: "memory", label: t("Content memory"), icon: <Brain /> },
];

const workspaceBottomItems = (): NavItem[] => [
  { id: "settings", label: t("Settings"), icon: <Settings /> },
];

/** Sub-navigation items for views that have secondary menus. Exported for AppShell. */
export const subNavConfig = (): Record<string, SubNavItem[]> => ({
  brand: [
    { id: "concepts", label: t("Concepts"), icon: <Network /> },
    { id: "voice", label: t("Voice"), icon: <Palette /> },
    { id: "experiments", label: t("Experiments"), icon: <FlaskConical /> },
    { id: "activity", label: t("Activity"), icon: <Activity /> },
    { id: "dashboard", label: t("Dashboard"), icon: <LayoutDashboard /> },
  ],
  settings: [
    { id: "general", label: t("General"), icon: <Settings /> },
    { id: "languages", label: t("Languages"), icon: <Globe /> },
    { id: "members", label: t("Members"), icon: <Users /> },
    { id: "roles", label: t("Roles"), icon: <Shield /> },
    { id: "governance", label: t("Governance"), icon: <Lock /> },
    { id: "providers", label: t("Providers"), icon: <KeyRound /> },
    { id: "tokens", label: t("API Tokens"), icon: <KeyRound /> },
    { id: "auditlog", label: t("Audit Log"), icon: <Clock /> },
    { id: "bin", label: t("Recycle Bin"), icon: <Trash2 /> },
    { id: "bravo", label: t("@bravo Agent"), icon: <Sparkles /> },
    { id: "billing", label: t("Billing"), icon: <CreditCard /> },
    { id: "system", label: t("System Info"), icon: <Info /> },
  ],
});

// ---------------------------------------------------------------------------
// Desktop: Icon-only rail
// ---------------------------------------------------------------------------

function IconNav<V extends string>({
  activeView,
  onViewChange,
  extraNavItems = [],
  sidebarContext,
}: {
  activeView: V;
  onViewChange: (view: V) => void;
  extraNavItems?: NavItem[];
  sidebarContext: SidebarContext;
}) {
  const mainItems = [...workspaceNavItems(), ...extraNavItems];
  const isProject = sidebarContext.level === "project";

  if (isProject) {
    const ctx = sidebarContext as Extract<SidebarContext, { level: "project" }>;
    return (
      <>
        <SidebarGroup>
          <SidebarGroupContent>
            <SidebarMenu>
              <SidebarMenuItem>
                <SidebarMenuButton tooltip="Back" onClick={ctx.onBack} data-testid="sidebar-home">
                  <ArrowLeft />
                </SidebarMenuButton>
              </SidebarMenuItem>
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>
        <SidebarSeparator />
        <SidebarGroup>
          <SidebarGroupContent>
            <SidebarMenu>
              <SidebarMenuItem>
                <SidebarMenuButton
                  tooltip="Dashboard"
                  isActive={ctx.activeProjectView === "dashboard"}
                  onClick={ctx.onOpenDashboard}
                  data-testid="sidebar-dashboard"
                >
                  <LayoutDashboard />
                </SidebarMenuButton>
              </SidebarMenuItem>
              {ctx.onOpenAutomations && (
                <SidebarMenuItem>
                  <SidebarMenuButton
                    tooltip="Automations"
                    isActive={ctx.activeProjectView === "automations"}
                    onClick={ctx.onOpenAutomations}
                    data-testid="sidebar-automations"
                  >
                    <Sparkles />
                  </SidebarMenuButton>
                </SidebarMenuItem>
              )}
              {ctx.onOpenRuns && (
                <SidebarMenuItem>
                  <SidebarMenuButton
                    tooltip="Runs"
                    isActive={ctx.activeProjectView === "runs"}
                    onClick={ctx.onOpenRuns}
                    data-testid="sidebar-runs"
                  >
                    <Rocket />
                  </SidebarMenuButton>
                </SidebarMenuItem>
              )}
              {ctx.onOpenConnectors && (
                <SidebarMenuItem>
                  <SidebarMenuButton
                    tooltip="Connectors"
                    isActive={ctx.activeProjectView === "connectors"}
                    onClick={ctx.onOpenConnectors}
                    data-testid="sidebar-connectors"
                  >
                    <Plug />
                  </SidebarMenuButton>
                </SidebarMenuItem>
              )}
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>
      </>
    );
  }

  return (
    <>
      <SidebarGroup>
        <SidebarGroupContent>
          <SidebarMenu>
            {mainItems.map(({ id, label, icon }) => (
              <SidebarMenuItem key={id}>
                <SidebarMenuButton
                  tooltip={label}
                  isActive={activeView === id}
                  onClick={() => onViewChange(id as V)}
                  data-testid={`nav-${id}`}
                >
                  {icon}
                </SidebarMenuButton>
              </SidebarMenuItem>
            ))}
          </SidebarMenu>
        </SidebarGroupContent>
      </SidebarGroup>

      <SidebarGroup className="mt-auto">
        <SidebarGroupContent>
          <SidebarMenu>
            {workspaceBottomItems().map(({ id, label, icon }) => (
              <SidebarMenuItem key={id}>
                <SidebarMenuButton
                  tooltip={label}
                  isActive={activeView === id}
                  onClick={() => onViewChange(id as V)}
                  data-testid={`nav-${id}`}
                >
                  {icon}
                </SidebarMenuButton>
              </SidebarMenuItem>
            ))}
          </SidebarMenu>
        </SidebarGroupContent>
      </SidebarGroup>
    </>
  );
}

// ---------------------------------------------------------------------------
// Mobile: Full nav with labels + inline sub-nav
// ---------------------------------------------------------------------------

function MobileNav<V extends string>({
  activeView,
  onViewChange,
  extraNavItems = [],
  sidebarContext,
  activeSubNav,
  onSubNavChange,
  hiddenSubNavIds,
}: {
  activeView: V;
  onViewChange: (view: V) => void;
  extraNavItems?: NavItem[];
  sidebarContext: SidebarContext;
  activeSubNav?: string;
  onSubNavChange?: (id: string) => void;
  hiddenSubNavIds?: string[];
}) {
  const { setOpenMobile } = useSidebar();
  const mainItems = [...workspaceNavItems(), ...extraNavItems];
  const isProject = sidebarContext.level === "project";

  const handleNav = (id: string) => {
    onViewChange(id as V);
    setOpenMobile(false);
  };

  const handleSubNav = (id: string) => {
    onSubNavChange?.(id);
    setOpenMobile(false);
  };

  if (isProject) {
    const ctx = sidebarContext as Extract<SidebarContext, { level: "project" }>;
    return (
      <>
        <SidebarGroup>
          <SidebarGroupContent>
            <SidebarMenu>
              <SidebarMenuItem>
                <SidebarMenuButton
                  onClick={() => {
                    ctx.onBack();
                    setOpenMobile(false);
                  }}
                  data-testid="sidebar-home"
                >
                  <ArrowLeft />
                  <span>Back</span>
                </SidebarMenuButton>
              </SidebarMenuItem>
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>
        <SidebarSeparator />
        <SidebarGroup>
          <SidebarGroupLabel>{ctx.project.name}</SidebarGroupLabel>
          <SidebarGroupContent>
            <SidebarMenu>
              <SidebarMenuItem>
                <SidebarMenuButton
                  isActive={ctx.activeProjectView === "dashboard"}
                  onClick={() => {
                    ctx.onOpenDashboard();
                    setOpenMobile(false);
                  }}
                  data-testid="sidebar-dashboard"
                >
                  <LayoutDashboard />
                  <span>Dashboard</span>
                </SidebarMenuButton>
              </SidebarMenuItem>
              {ctx.onOpenAutomations && (
                <SidebarMenuItem>
                  <SidebarMenuButton
                    isActive={ctx.activeProjectView === "automations"}
                    onClick={() => {
                      ctx.onOpenAutomations!();
                      setOpenMobile(false);
                    }}
                    data-testid="sidebar-automations"
                  >
                    <Sparkles />
                    <span>Automations</span>
                  </SidebarMenuButton>
                </SidebarMenuItem>
              )}
              {ctx.onOpenRuns && (
                <SidebarMenuItem>
                  <SidebarMenuButton
                    isActive={ctx.activeProjectView === "runs"}
                    onClick={() => {
                      ctx.onOpenRuns!();
                      setOpenMobile(false);
                    }}
                    data-testid="sidebar-runs"
                  >
                    <Rocket />
                    <span>Runs</span>
                  </SidebarMenuButton>
                </SidebarMenuItem>
              )}
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>
      </>
    );
  }

  // Workspace-level: show main nav items. Keep undefined (not []) when a view has
  // no sub-nav, so the secondary menu stays hidden for those views.
  const subItems = subNavConfig()[activeView as string]?.filter(
    (item) => !hiddenSubNavIds?.includes(item.id),
  );

  return (
    <>
      <SidebarGroup>
        <SidebarGroupLabel>Workspace</SidebarGroupLabel>
        <SidebarGroupContent>
          <SidebarMenu>
            {mainItems.map(({ id, label, icon }) => (
              <SidebarMenuItem key={id}>
                <SidebarMenuButton
                  isActive={activeView === id}
                  onClick={() => handleNav(id)}
                  data-testid={`nav-${id}`}
                >
                  {icon}
                  <span>{label}</span>
                </SidebarMenuButton>
              </SidebarMenuItem>
            ))}
          </SidebarMenu>
        </SidebarGroupContent>
      </SidebarGroup>

      {/* Settings + bottom items */}
      <SidebarGroup>
        <SidebarGroupContent>
          <SidebarMenu>
            {workspaceBottomItems().map(({ id, label, icon }) => (
              <SidebarMenuItem key={id}>
                <SidebarMenuButton
                  isActive={activeView === id}
                  onClick={() => handleNav(id)}
                  data-testid={`nav-${id}`}
                >
                  {icon}
                  <span>{label}</span>
                </SidebarMenuButton>
              </SidebarMenuItem>
            ))}
          </SidebarMenu>
        </SidebarGroupContent>
      </SidebarGroup>

      {/* Inline sub-nav for current view (e.g. settings sub-items) */}
      {subItems && onSubNavChange && (
        <>
          <SidebarSeparator />
          <SidebarGroup>
            <SidebarGroupLabel>
              {activeView.charAt(0).toUpperCase() + (activeView as string).slice(1)}
            </SidebarGroupLabel>
            <SidebarGroupContent>
              <SidebarMenu>
                {subItems.map((item) => (
                  <SidebarMenuItem key={item.id}>
                    <SidebarMenuButton
                      isActive={activeSubNav === item.id}
                      onClick={() => handleSubNav(item.id)}
                    >
                      {item.icon}
                      <span>{item.label}</span>
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                ))}
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
        </>
      )}
    </>
  );
}

// ---------------------------------------------------------------------------
// Main component
// ---------------------------------------------------------------------------

export function AppSidebar<V extends string = string>({
  workspaces,
  activeWorkspace,
  onSelectWorkspace,
  onCreateWorkspace,
  activeView,
  onViewChange,
  extraNavItems = [],
  sidebarContext,
  activeSubNav,
  onSubNavChange,
  hiddenSubNavIds,
  // Consumed but not passed to Sidebar DOM
  user: _user,
  onSignOut: _onSignOut,
  collapsed: _collapsed,
  onCollapsedChange: _onCollapsedChange,
  collapsedWidth: _collapsedWidth,
  connectionState: _connectionState,
  pendingChanges: _pendingChanges,
  showThemeToggle: _showThemeToggle,
  ...props
}: AppSidebarProps<V> & Omit<React.ComponentProps<typeof Sidebar>, keyof AppSidebarProps>) {
  const effectiveContext = useMemo<SidebarContext>(
    () => sidebarContext ?? { level: "workspace", activeView: activeView as View },
    [sidebarContext, activeView],
  );

  const { isMobile } = useSidebar();

  if (isMobile) {
    // Mobile: full sidebar in a Sheet drawer with text labels
    return (
      <Sidebar collapsible="offcanvas" {...props}>
        <SidebarHeader>
          <MobileWorkspaceSwitcher
            workspaces={workspaces}
            activeWorkspace={activeWorkspace}
            onSelectWorkspace={onSelectWorkspace}
            onCreateWorkspace={onCreateWorkspace}
          />
        </SidebarHeader>
        <SidebarContent>
          <MobileNav
            activeView={activeView}
            onViewChange={onViewChange}
            extraNavItems={extraNavItems}
            sidebarContext={effectiveContext}
            activeSubNav={activeSubNav}
            onSubNavChange={onSubNavChange}
            hiddenSubNavIds={hiddenSubNavIds}
          />
        </SidebarContent>
      </Sidebar>
    );
  }

  // Desktop: icon-only rail
  return (
    <Sidebar collapsible="none" className="!w-(--sidebar-width-icon)" {...props}>
      {/* Reserve a top "safe area" so the workspace switcher clears the macOS
          traffic lights — both the real desktop shell (html.bw-desktop-mac, set
          by the Wails frontend on macOS) and the recorded web capture
          (html.pw-recording-tl, set by the harness). Plain browser visits get
          no class and no gutter. Mirrors kapi-desktop's traffic-light gutter. */}
      <SidebarHeader className="[html.bw-desktop-mac_&]:mt-9 [html.pw-recording-tl_&]:mt-10">
        <WorkspaceSwitcher
          workspaces={workspaces}
          activeWorkspace={activeWorkspace}
          onSelectWorkspace={onSelectWorkspace}
          onCreateWorkspace={onCreateWorkspace}
        />
      </SidebarHeader>
      <SidebarContent className="[&_[data-slot=sidebar-menu]]:gap-1 [&_[data-slot=sidebar-menu-button]]:justify-center [&_[data-slot=sidebar-menu-button]]:aspect-square [&_[data-slot=sidebar-menu-button]]:p-0 [&_[data-slot=sidebar-menu-button]_svg]:size-5 [&_svg]:stroke-[1.5]">
        <IconNav
          activeView={activeView}
          onViewChange={onViewChange}
          extraNavItems={extraNavItems}
          sidebarContext={effectiveContext}
        />
      </SidebarContent>
    </Sidebar>
  );
}
