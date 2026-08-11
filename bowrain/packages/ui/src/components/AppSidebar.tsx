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
  Globe,
  Users,
  KeyRound,
  Info,
  CreditCard,
  Shield,
  Lock,
  Layers,
  Network,
  FlaskConical,
  Activity,
  Rocket,
  Plug,
  FolderOpen,
} from "./icons";
import { WorkspaceSwitcher } from "./WorkspaceSwitcher";
import { MobileWorkspaceSwitcher } from "./MobileWorkspaceSwitcher";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export type View =
  | "translate"
  | "context"
  | "insights"
  | "terms"
  | "memory"
  | "auditlog"
  | "bin"
  | "settings";

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

/**
 * NavCount is the count pill a nav entry wears when work is waiting behind it —
 * the same quiet primary-tinted pill the governance inbox uses for its queue
 * length, so a count means the same thing wherever it appears. Nothing renders
 * at zero: a badge that is always there stops being a signal.
 */
export function NavCount({ count }: { count?: number }) {
  if (!count || count <= 0) return null;
  return (
    <span
      className="ml-auto shrink-0 rounded-full bg-primary/10 px-1.5 py-0.5 text-[11px] font-medium tabular-nums text-primary"
      aria-label={`${count} waiting`}
    >
      {count > 99 ? "99+" : count}
    </span>
  );
}

type ConnectionState = "disconnected" | "connecting" | "connected" | "offline";

export type ProjectView = "dashboard" | "source" | "automations" | "runs" | "connectors";

export type SidebarContext =
  | { level: "workspace"; activeView: View }
  | {
      level: "project";
      project: ProjectInfo;
      activeStream: string;
      activeProjectView: ProjectView;
      onOpenDashboard: () => void;
      /** The project's source content: collections, items and streams. */
      onOpenSource?: () => void;
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
  /**
   * Counts of work waiting behind a sub-nav entry, keyed by item id. A non-zero
   * count renders as a pill on that entry — the nav is where someone looks to
   * find out there is anything to do at all. Zero and absent both render nothing.
   */
  subNavCounts?: Record<string, number>;
}

// ---------------------------------------------------------------------------
// Nav data
// ---------------------------------------------------------------------------
// Getter functions, not module-level consts: the labels go through t() so the
// neokapi-i18n transform extracts them and looks them up in the active locale's
// dictionary at CALL time. Evaluating these per render (the components call
// them below) picks up a locale switch; a module-level const would freeze the
// labels in whatever locale was active at import time.

// Context is what you author and govern; Insights is what the graph tells you
// back. Keeping them as peers is the whole point of the re-root: three
// top-level entries that read as one product, not several.
const workspaceNavItems = (): NavItem[] => [
  { id: "translate", label: t("Projects"), icon: <Home /> },
  { id: "context", label: t("Context"), icon: <Palette /> },
  { id: "insights", label: t("Insights"), icon: <LayoutDashboard /> },
];

const workspaceBottomItems = (): NavItem[] => [
  { id: "settings", label: t("Settings"), icon: <Settings /> },
];

/**
 * The label for a top-level view, from the same definition the rail renders.
 *
 * Exported because the secondary panel's title names the section you are inside,
 * so it must be the string the rail shows. AppShell used to keep its own map,
 * which is a second place to forget: renaming the rail entry to "Context" left
 * the panel still reading "Brand", and that copy never passed through t(), so it
 * could not translate either.
 *
 * Views with no rail entry (reached from a sub-nav or a direct link) are named
 * here too, so one lookup answers for every view.
 */
export const viewLabel = (id: string): string | undefined =>
  [...workspaceNavItems(), ...workspaceBottomItems()].find((item) => item.id === id)?.label ??
  {
    memory: t("Content memory"),
    terms: t("Terms"),
    auditlog: t("Audit log"),
    bin: t("Recycle bin"),
  }[id];

/**
 * A project's own sections, for the secondary panel.
 *
 * A project is somewhere you are inside the workspace, not somewhere else: its
 * sections belong one level in from Projects, which is exactly what the
 * secondary panel is for. Putting them in the rail instead meant the rail had
 * to give up the workspace — Context, Insights and Settings all vanished the
 * moment a project opened, and the only way back to any of them was an
 * unlabelled arrow that led one step at a time.
 *
 * Entries whose callback the host leaves unset are dropped, so a project that
 * cannot run automations does not advertise them.
 */
export const projectSubNavConfig = (
  ctx: Extract<SidebarContext, { level: "project" }>,
): SubNavItem[] =>
  [
    { id: "dashboard", label: t("Overview"), icon: <LayoutDashboard />, on: ctx.onOpenDashboard },
    { id: "source", label: t("Source content"), icon: <FolderOpen />, on: ctx.onOpenSource },
    { id: "automations", label: t("Automations"), icon: <Sparkles />, on: ctx.onOpenAutomations },
    { id: "runs", label: t("Runs"), icon: <Rocket />, on: ctx.onOpenRuns },
    { id: "connectors", label: t("Connectors"), icon: <Plug />, on: ctx.onOpenConnectors },
  ]
    .filter((item) => item.on)
    .map(({ id, label, icon }) => ({ id, label, icon }));

/** Run the project section named by `id`. */
export function openProjectSection(
  ctx: Extract<SidebarContext, { level: "project" }>,
  id: string,
): void {
  const run = {
    dashboard: ctx.onOpenDashboard,
    source: ctx.onOpenSource,
    automations: ctx.onOpenAutomations,
    runs: ctx.onOpenRuns,
    connectors: ctx.onOpenConnectors,
  }[id];
  run?.();
}

/** Sub-navigation items for views that have secondary menus. Exported for AppShell. */
export const subNavConfig = (): Record<string, SubNavItem[]> => ({
  context: [
    // Profiles first, and the section's landing: a workspace's context is a set
    // of points — product, channel, market — before it is a set of concepts, and
    // everything below is read at one of them. "Brand" is the default point's
    // label here, not the name of the surface.
    { id: "profiles", label: t("Profiles"), icon: <Layers /> },
    { id: "concepts", label: t("Concepts"), icon: <Network /> },
    { id: "voice", label: t("Voice"), icon: <Palette /> },
    { id: "memory", label: t("Content memory"), icon: <Brain /> },
    // "Changes", not "Experiments": these are change-sets with a reach preview
    // and an optional pilot — the ONLY route a governed change can take, not
    // optional exploration. Nav labels name destinations, so a noun.
    { id: "changes", label: t("Changes"), icon: <FlaskConical /> },
    { id: "activity", label: t("Activity"), icon: <Activity /> },
  ],
  // What the graph tells you back. locale-demand moves in rather than leaving:
  // demand by market is evidence for a coordinate decision, not a stray report.
  insights: [
    { id: "dashboard", label: t("Overview"), icon: <LayoutDashboard /> },
    { id: "locale-demand", label: t("Locale demand"), icon: <Globe /> },
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

/**
 * The rail is the workspace, always.
 *
 * It does not change when a project opens. A project is reached through
 * Projects and stays under it, so Projects simply stays lit while the project's
 * own sections fill the panel beside the rail. The alternative — swapping the
 * rail's contents — is what made the sidebar confusing: the same strip of icons
 * meant different things depending on where you were, and three of the
 * workspace's four destinations were unreachable from inside any project.
 */
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
  // Inside a project, Projects is the section you are in — the rail says so
  // even though the URL is a project's, not the list's.
  const railView = sidebarContext.level === "project" ? "translate" : activeView;

  return (
    <>
      <SidebarGroup>
        <SidebarGroupContent>
          <SidebarMenu>
            {mainItems.map(({ id, label, icon }) => (
              <SidebarMenuItem key={id}>
                <SidebarMenuButton
                  tooltip={label}
                  tooltipWhenExpanded
                  isActive={railView === id}
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
                  tooltipWhenExpanded
                  isActive={railView === id}
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
  subNavCounts,
}: {
  activeView: V;
  onViewChange: (view: V) => void;
  extraNavItems?: NavItem[];
  sidebarContext: SidebarContext;
  activeSubNav?: string;
  onSubNavChange?: (id: string) => void;
  hiddenSubNavIds?: string[];
  subNavCounts?: Record<string, number>;
}) {
  const { setOpenMobile } = useSidebar();
  const mainItems = [...workspaceNavItems(), ...extraNavItems];
  const isProject = sidebarContext.level === "project";
  const projectCtx = isProject
    ? (sidebarContext as Extract<SidebarContext, { level: "project" }>)
    : undefined;
  // Inside a project, Projects is the section you are in.
  const railView = isProject ? "translate" : activeView;

  const handleNav = (id: string) => {
    onViewChange(id as V);
    setOpenMobile(false);
  };

  const handleSubNav = (id: string) => {
    onSubNavChange?.(id);
    setOpenMobile(false);
  };

  // The sub-nav header names the section you are inside, so it must be the same
  // string the rail shows — not a title-cased view id. Deriving it from the id
  // meant renaming a destination silently left the header behind (the Context
  // section still read "Brand"), and it could never translate, because an id is
  // not a catalog entry. Settings lives in the bottom group, so both are searched.
  const navLabelFor = (id: string): string | undefined =>
    mainItems.find((item) => item.id === id)?.label ?? viewLabel(id);

  // The workspace's own sub-nav, shown when no project is open — a project
  // fills this slot with its own sections instead. Keep undefined (not []) when
  // a view has no sub-nav, so the secondary menu stays hidden for those views.
  const subItems = isProject
    ? undefined
    : subNavConfig()[activeView as string]?.filter((item) => !hiddenSubNavIds?.includes(item.id));

  return (
    <>
      <SidebarGroup>
        <SidebarGroupLabel>Workspace</SidebarGroupLabel>
        <SidebarGroupContent>
          <SidebarMenu>
            {mainItems.map(({ id, label, icon }) => (
              <SidebarMenuItem key={id}>
                <SidebarMenuButton
                  isActive={railView === id}
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
                  isActive={railView === id}
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

      {/* The open project's sections, under its name. On mobile they sit below
          the workspace nav rather than replacing it, for the same reason the
          desktop rail no longer swaps: a project is a place inside the
          workspace, not a mode that hides it. */}
      {isProject && (
        <>
          <SidebarSeparator />
          <SidebarGroup>
            <SidebarGroupLabel>{projectCtx!.project.name}</SidebarGroupLabel>
            <SidebarGroupContent>
              <SidebarMenu>
                {projectSubNavConfig(projectCtx!).map((item) => (
                  <SidebarMenuItem key={item.id}>
                    <SidebarMenuButton
                      isActive={projectCtx!.activeProjectView === item.id}
                      onClick={() => {
                        openProjectSection(projectCtx!, item.id);
                        setOpenMobile(false);
                      }}
                      // The same id the desktop panel gives this entry: one
                      // control, one hook, whatever the viewport.
                      data-testid={`subnav-${item.id}`}
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

      {/* Inline sub-nav for current view (e.g. settings sub-items) */}
      {subItems && onSubNavChange && (
        <>
          <SidebarSeparator />
          <SidebarGroup>
            <SidebarGroupLabel>{navLabelFor(activeView as string)}</SidebarGroupLabel>
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
                      <NavCount count={subNavCounts?.[item.id]} />
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
  subNavCounts,
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
            subNavCounts={subNavCounts}
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
