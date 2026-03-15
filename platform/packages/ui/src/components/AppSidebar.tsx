import { useState, useMemo } from "react";
import type { Workspace, User, ProjectInfo, StreamInfo } from "../types/api";
import {
  BookOpen,
  Brain,
  Settings,
  ChevronLeft,
  ChevronRight,
  Palette,
  Home,
  LayoutDashboard,
  Sparkles,
  Clock,
  Trash2,
} from "./icons";
import { WorkspaceSwitcher } from "./WorkspaceSwitcher";
import { useSidebar } from "./ui/sidebar";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export type View = "translate" | "brand" | "termbase" | "memory" | "auditlog" | "bin" | "settings";

export interface NavItem {
  id: string;
  label: string;
  icon: React.ReactNode;
}

type ConnectionState = "disconnected" | "connecting" | "connected" | "offline";

/**
 * Sidebar context describing what level the user is at.
 *
 * - "workspace": Dashboard or workspace-level pages (brand, termbase, etc.)
 * - "project": Inside a project (project detail or translation editor)
 */
/** Which project-level page is currently shown. */
export type ProjectView = "dashboard" | "automations";

export type SidebarContext =
  | { level: "workspace"; activeView: View }
  | {
      level: "project";
      project: ProjectInfo;
      activeStream: string;
      /** Which project sub-page is active in the sidebar. */
      activeProjectView: ProjectView;
      onBack: () => void;
      /** Navigate to project dashboard (file list). */
      onOpenDashboard: () => void;
      onOpenFile: (fileName: string) => void;
      onStreamChange: (stream: string) => void;
      onCreateStream?: () => void;
      onEditStream?: (stream: StreamInfo) => void;
      onMergeStream?: (streamName: string) => void;
      onDiffStream?: (streamName: string) => void;
      onDeleteStream?: (streamName: string) => void;
      onOpenAutomations?: () => void;
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
  /** Show theme toggle in sidebar footer (default true for web, false for desktop). */
  showThemeToggle?: boolean;
  /** Contextual sidebar content. When absent, uses flat nav based on activeView. */
  sidebarContext?: SidebarContext;
}

// ---------------------------------------------------------------------------
// Shared nav button
// ---------------------------------------------------------------------------

function NavButton({
  isActive,
  onClick,
  icon,
  label,
  iconsOnly,
  testId,
}: {
  isActive: boolean;
  onClick: () => void;
  icon: React.ReactNode;
  label: string;
  iconsOnly: boolean;
  testId?: string;
}) {
  return (
    <button
      data-testid={testId}
      onClick={onClick}
      className="flex w-full items-center gap-2 rounded-md border-none cursor-pointer text-sm text-left transition-[background,color] duration-200 ease-linear outline-none"
      style={{
        padding: iconsOnly ? "10px 0" : "8px 12px",
        justifyContent: iconsOnly ? "center" : "flex-start",
        ...(isActive
          ? {
              background: "var(--sidebar-primary, var(--semantic-primary))",
              color: "var(--sidebar-primary-foreground, var(--semantic-text-inverse))",
            }
          : {
              background: "transparent",
              color: "color-mix(in srgb, var(--sidebar-foreground) 60%, transparent)",
            }),
      }}
      onMouseEnter={(e) => {
        if (!isActive) {
          e.currentTarget.style.background = "var(--sidebar-accent, var(--semantic-surface-elevated))";
          e.currentTarget.style.color = "var(--sidebar-foreground)";
        }
      }}
      onMouseLeave={(e) => {
        if (!isActive) {
          e.currentTarget.style.background = "transparent";
          e.currentTarget.style.color = "color-mix(in srgb, var(--sidebar-foreground) 60%, transparent)";
        }
      }}
    >
      <span className="shrink-0 flex items-center justify-center">{icon}</span>
      {!iconsOnly && <span>{label}</span>}
    </button>
  );
}

// ---------------------------------------------------------------------------
// Workspace-level nav (default flat navigation)
// ---------------------------------------------------------------------------

const defaultNavItems: NavItem[] = [
  { id: "translate", label: "Projects", icon: <Home className="w-5 h-5" /> },
  { id: "brand", label: "Brand Voice", icon: <Palette className="w-5 h-5" /> },
  { id: "termbase", label: "Termbase", icon: <BookOpen className="w-5 h-5" /> },
  { id: "memory", label: "Memory", icon: <Brain className="w-5 h-5" /> },
  { id: "auditlog", label: "Audit Log", icon: <Clock className="w-5 h-5" /> },
  { id: "settings", label: "Settings", icon: <Settings className="w-5 h-5" /> },
];

/** Items pinned to the bottom of the sidebar, separated from the main nav. */
const bottomNavItems: NavItem[] = [
  { id: "bin", label: "Bin", icon: <Trash2 className="w-5 h-5" /> },
];

function WorkspaceNav<V extends string>({
  activeView,
  onViewChange,
  extraNavItems,
  iconsOnly,
}: {
  activeView: V;
  onViewChange: (view: V) => void;
  extraNavItems: NavItem[];
  iconsOnly: boolean;
}) {
  // Settings is always last in the main list
  const lastItem = defaultNavItems[defaultNavItems.length - 1];
  const mainItems = [
    ...defaultNavItems.slice(0, -1),
    ...extraNavItems,
    lastItem,
  ];

  return (
    <nav className="flex-1 overflow-hidden px-2 py-2 flex flex-col">
      <ul className="flex flex-col gap-1 list-none p-0 m-0 flex-1">
        {mainItems.map(({ id, label, icon }) => {
          const isActive = activeView === id;
          return (
            <li key={id}>
              <NavButton
                isActive={isActive}
                onClick={() => onViewChange(id as V)}
                icon={icon}
                label={label}
                iconsOnly={iconsOnly}
                testId={`nav-${id}`}
              />
            </li>
          );
        })}
      </ul>

      {/* Bottom items — pinned to bottom, visually separated */}
      {bottomNavItems.length > 0 && (
        <ul className="flex flex-col gap-1 list-none p-0 m-0 pt-2 mt-2 border-t border-border/20">
          {bottomNavItems.map(({ id, label, icon }) => {
            const isActive = activeView === id;
            return (
              <li key={id}>
                <NavButton
                  isActive={isActive}
                  onClick={() => onViewChange(id as V)}
                  icon={icon}
                  label={label}
                  iconsOnly={iconsOnly}
                  testId={`nav-${id}`}
                />
              </li>
            );
          })}
        </ul>
      )}
    </nav>
  );
}

// ---------------------------------------------------------------------------
// Project-level nav
// ---------------------------------------------------------------------------

function ProjectNav({
  ctx,
  iconsOnly,
}: {
  ctx: Extract<SidebarContext, { level: "project" }>;
  iconsOnly: boolean;
}) {
  const { activeProjectView } = ctx;

  return (
    <nav className="flex-1 overflow-hidden px-2 py-2">
      {/* Home button — goes up one level */}
      <NavButton
        isActive={false}
        onClick={ctx.onBack}
        icon={<Home className="w-5 h-5" />}
        label={activeProjectView !== "dashboard" ? ctx.project.name : "Home"}
        iconsOnly={iconsOnly}
        testId="sidebar-home"
      />

      {/* Divider */}
      <div
        className="mx-1 my-2"
        style={{ borderTop: "1px solid color-mix(in srgb, var(--sidebar-border) 80%, var(--sidebar-foreground) 20%)" }}
      />

      {/* Project-level menu items */}
      <ul className="flex flex-col gap-1 list-none p-0 m-0">
        <li>
          <NavButton
            isActive={activeProjectView === "dashboard"}
            onClick={ctx.onOpenDashboard}
            icon={<LayoutDashboard className="w-5 h-5" />}
            label="Dashboard"
            iconsOnly={iconsOnly}
            testId="sidebar-dashboard"
          />
        </li>
        {ctx.onOpenAutomations && (
          <li>
            <NavButton
              isActive={activeProjectView === "automations"}
              onClick={ctx.onOpenAutomations}
              icon={<Sparkles className="w-5 h-5" />}
              label="Automations"
              iconsOnly={iconsOnly}
              testId="sidebar-automations"
            />
          </li>
        )}
      </ul>
    </nav>
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
  collapsed,
  onCollapsedChange,
  sidebarContext,
}: AppSidebarProps<V>) {
  const [hovered, setHovered] = useState(false);
  const { state } = useSidebar();
  const iconsOnly = collapsed;

  const effectiveContext = useMemo<SidebarContext>(
    () => sidebarContext ?? { level: "workspace", activeView: activeView as View },
    [sidebarContext, activeView],
  );

  return (
    <aside
      data-sidebar="sidebar"
      data-side="left"
      data-variant="sidebar"
      data-collapsible="icon"
      data-state={state}
      className="group/sidebar relative flex flex-col shrink-0 transition-[width] duration-300 ease-in-out overflow-visible"
      style={{
        width: collapsed ? "var(--sidebar-width-icon)" : "var(--sidebar-width)",
        background: "var(--sidebar-bg)",
        color: "var(--sidebar-foreground)",
        borderRight: "1px solid var(--sidebar-border)",
        backdropFilter: "blur(var(--sidebar-backdrop-blur))",
        WebkitBackdropFilter: "blur(var(--sidebar-backdrop-blur))",
      }}
      onMouseEnter={() => setHovered(true)}
      onMouseLeave={() => setHovered(false)}
    >
      {/* Workspace switcher */}
      <div className="shrink-0 px-3 py-2.5">
        <WorkspaceSwitcher
          workspaces={workspaces}
          activeWorkspace={activeWorkspace}
          onSelectWorkspace={onSelectWorkspace}
          onCreateWorkspace={onCreateWorkspace}
        />
      </div>

      {/* Context-dependent navigation */}
      {effectiveContext.level === "workspace" ? (
        <WorkspaceNav
          activeView={effectiveContext.activeView as V}
          onViewChange={onViewChange}
          extraNavItems={extraNavItems}
          iconsOnly={iconsOnly}
        />
      ) : (
        <ProjectNav ctx={effectiveContext} iconsOnly={iconsOnly} />
      )}

      {/* Floating collapse/expand button */}
      <button
        onClick={() => onCollapsedChange(!collapsed)}
        className="absolute top-1/2 -translate-y-1/2 rounded-full w-6 h-6 flex items-center justify-center border cursor-pointer transition-opacity duration-200 z-20 shadow-sm"
        style={{
          right: -12,
          opacity: hovered ? 1 : 0,
          pointerEvents: hovered ? "auto" : "none",
          background: "var(--sidebar-bg, hsl(var(--card) / 0.8))",
          borderColor: "var(--sidebar-border, hsl(var(--border)))",
          backdropFilter: "blur(8px)",
          WebkitBackdropFilter: "blur(8px)",
          color: "var(--sidebar-foreground)",
        }}
      >
        {collapsed ? <ChevronRight className="w-3 h-3" /> : <ChevronLeft className="w-3 h-3" />}
      </button>
    </aside>
  );
}
