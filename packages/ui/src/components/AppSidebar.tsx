import { cn } from "../lib/utils";
import type { Workspace, User } from "../types/api";
import { useTheme, type Theme } from "../context/ThemeContext";
import { Globe, BookOpen, Brain, Settings, ChevronLeft, ChevronRight, Sun, Moon, Sparkles, WifiOff } from "./icons";
import { WorkspaceSwitcher } from "./WorkspaceSwitcher";
import { AccountMenu } from "./AccountMenu";
import type { NavItem } from "./MainSidebar";

type ConnectionState = "disconnected" | "connecting" | "connected" | "offline";

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
  topSpacer?: number;
  collapsedWidth?: number;
  connectionState?: ConnectionState;
  pendingChanges?: number;
  /** Show theme toggle in sidebar footer (default true for web, false for desktop). */
  showThemeToggle?: boolean;
}

const defaultNavItems: NavItem[] = [
  { id: "translate", label: "Translate", icon: <Globe className="w-4 h-4" /> },
  { id: "termbase", label: "Termbase", icon: <BookOpen className="w-4 h-4" /> },
  { id: "memory", label: "Memory", icon: <Brain className="w-4 h-4" /> },
  { id: "settings", label: "Settings", icon: <Settings className="w-4 h-4" /> },
];

export function AppSidebar<V extends string = string>({
  workspaces,
  activeWorkspace,
  onSelectWorkspace,
  onCreateWorkspace,
  activeView,
  onViewChange,
  extraNavItems = [],
  user,
  onSignOut,
  collapsed,
  onCollapsedChange,
  topSpacer = 0,
  collapsedWidth = 0,
  connectionState,
  pendingChanges,
  showThemeToggle = true,
}: AppSidebarProps<V>) {
  const navItems = [
    ...defaultNavItems.slice(0, -1),
    ...extraNavItems,
    defaultNavItems[defaultNavItems.length - 1],
  ];

  const iconsOnly = collapsed && collapsedWidth > 0;
  const width = collapsed ? collapsedWidth : 220;

  return (
    <nav
      className="flex flex-col transition-[width] duration-200 ease-in-out overflow-hidden shrink-0"
      style={{
        width,
        background: "var(--sidebar-bg)",
        color: "var(--semantic-text)",
        backdropFilter: "blur(var(--sidebar-backdrop-blur, 16px))",
        WebkitBackdropFilter: "blur(var(--sidebar-backdrop-blur, 16px))",
        borderRight: collapsed && collapsedWidth === 0 ? "none" : "1px solid var(--semantic-border)",
        boxShadow: "var(--sidebar-glow, none)",
      }}
    >
      <div className="flex flex-col h-full">
        {/* Top spacer (e.g. macOS traffic lights) */}
        {topSpacer > 0 && <div style={{ height: topSpacer }} className="shrink-0" />}

        {/* Workspace switcher */}
        <div
          className="border-b shrink-0"
          style={{ borderColor: "var(--semantic-border)" }}
        >
          <WorkspaceSwitcher
            workspaces={workspaces}
            activeWorkspace={activeWorkspace}
            onSelectWorkspace={onSelectWorkspace}
            onCreateWorkspace={onCreateWorkspace}
            collapsed={iconsOnly}
          />
        </div>

        {/* Navigation items */}
        <div className="flex-1 py-2 px-2">
          <ul className="flex flex-col gap-1 list-none p-0 m-0">
            {navItems.map(({ id, label, icon }) => {
              const isActive = activeView === id;
              return (
                <li key={id} className="group/menu-item relative">
                  <button
                    data-testid={`nav-${id}`}
                    onClick={() => onViewChange(id as V)}
                    className={cn(
                      "flex w-full items-center gap-2 rounded-md border-none cursor-pointer text-sm text-left",
                      "transition-[background,color] duration-200 ease-linear outline-none",
                      iconsOnly ? "py-2.5 px-2 justify-center" : "px-3 py-2 justify-start",
                    )}
                    style={
                      isActive
                        ? {
                            background: "var(--semantic-primary)",
                            color: "var(--semantic-text-inverse)",
                          }
                        : {
                            background: "transparent",
                            color: "color-mix(in srgb, var(--semantic-text) 60%, transparent)",
                          }
                    }
                    onMouseEnter={(e) => {
                      if (!isActive) {
                        e.currentTarget.style.background = "var(--semantic-surface-elevated)";
                        e.currentTarget.style.color = "var(--semantic-text)";
                      }
                    }}
                    onMouseLeave={(e) => {
                      if (!isActive) {
                        e.currentTarget.style.background = "transparent";
                        e.currentTarget.style.color = "color-mix(in srgb, var(--semantic-text) 60%, transparent)";
                      }
                    }}
                  >
                    <span className="shrink-0">{icon}</span>
                    {!iconsOnly && <span>{label}</span>}
                  </button>
                </li>
              );
            })}
          </ul>
        </div>

        {/* Connection badge (when provided) */}
        {connectionState && connectionState !== "disconnected" && (
          <div
            className="px-4 py-2 border-t"
            style={{ borderColor: "var(--semantic-border)" }}
          >
            <ConnectionBadge state={connectionState} pendingChanges={pendingChanges} collapsed={iconsOnly} />
          </div>
        )}

        {/* Footer: theme toggle + collapse */}
        <div
          className={cn(
            "px-4 py-3 border-t flex items-center",
            iconsOnly ? "justify-center" : "justify-between",
          )}
          style={{ borderColor: "var(--semantic-border)" }}
        >
          {!iconsOnly && showThemeToggle && <ThemeToggle />}
          <button
            onClick={() => onCollapsedChange(!collapsed)}
            className="bg-transparent border-none cursor-pointer transition-colors"
            style={{ color: "color-mix(in srgb, var(--semantic-text) 60%, transparent)" }}
            onMouseEnter={(e) => { e.currentTarget.style.color = "var(--semantic-text)"; }}
            onMouseLeave={(e) => { e.currentTarget.style.color = "color-mix(in srgb, var(--semantic-text) 60%, transparent)"; }}
          >
            {collapsed ? <ChevronRight className="w-4 h-4" /> : <ChevronLeft className="w-4 h-4" />}
          </button>
        </div>

        {/* User account */}
        {user && onSignOut && (
          <div
            className="border-t shrink-0"
            style={{ borderColor: "var(--semantic-border)" }}
          >
            <AccountMenu
              user={user}
              onSignOut={onSignOut}
              variant="sidebar"
              collapsed={iconsOnly}
            />
          </div>
        )}
      </div>
    </nav>
  );
}

// ---------------------------------------------------------------------------
// Internal: ConnectionBadge
// ---------------------------------------------------------------------------

function ConnectionBadge({
  state,
  pendingChanges,
  collapsed,
}: {
  state: ConnectionState;
  pendingChanges?: number;
  collapsed: boolean;
}) {
  const isConnected = state === "connected";
  const isOffline = state === "offline";

  const dotColor = isConnected
    ? "bg-green-500"
    : isOffline
      ? "bg-amber-500"
      : "bg-muted-foreground/40";

  const label = isConnected
    ? "Connected"
    : isOffline
      ? "Offline"
      : "Connecting";

  return (
    <div className="flex items-center gap-2">
      <span className={`w-2 h-2 rounded-full inline-block shrink-0 ${dotColor}`} />
      {!collapsed && (
        <>
          <span
            className="text-xs"
            style={{ color: "color-mix(in srgb, var(--semantic-text) 60%, transparent)" }}
          >
            {label}
          </span>
          {isOffline && pendingChanges != null && pendingChanges > 0 && (
            <span className="flex items-center gap-1 text-xs text-amber-500 ml-auto">
              <WifiOff className="w-3 h-3" />
              {pendingChanges}
            </span>
          )}
        </>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Internal: ThemeToggle (extracted from MainSidebar)
// ---------------------------------------------------------------------------

const nextTheme: Record<Theme, Theme> = { glass: "light", light: "aurora", aurora: "glass" };
const themeIcons: Record<Theme, React.ReactNode> = {
  glass: <Moon className="w-4 h-4" />,
  light: <Sun className="w-4 h-4" />,
  aurora: <Sparkles className="w-4 h-4" />,
};
const themeLabels: Record<Theme, string> = {
  glass: "Glass",
  light: "Light",
  aurora: "Aurora",
};

function ThemeToggle() {
  const { theme, setTheme } = useTheme();
  return (
    <button
      data-testid="theme-toggle"
      onClick={() => setTheme(nextTheme[theme])}
      title={`Theme: ${themeLabels[theme]}`}
      className="bg-transparent border-none cursor-pointer p-0 leading-none transition-colors"
      style={{ color: "color-mix(in srgb, var(--semantic-text) 60%, transparent)" }}
      onMouseEnter={(e) => { e.currentTarget.style.color = "var(--semantic-text)"; }}
      onMouseLeave={(e) => { e.currentTarget.style.color = "color-mix(in srgb, var(--semantic-text) 60%, transparent)"; }}
    >
      {themeIcons[theme]}
    </button>
  );
}
