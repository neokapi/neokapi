import { useState } from "react";
import { cn } from "../lib/utils";
import type { Workspace, User } from "../types/api";
import { useTheme, type Theme } from "../context/ThemeContext";
import { Globe, BookOpen, Brain, Settings, ChevronLeft, ChevronRight, Sun, Moon, Sparkles } from "./icons";
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
  const [hovered, setHovered] = useState(false);

  return (
    <nav
      onMouseEnter={() => setHovered(true)}
      onMouseLeave={() => setHovered(false)}
      className="relative z-10 flex flex-col transition-[width] duration-200 ease-in-out overflow-visible shrink-0"
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
      <div className="flex flex-col h-full overflow-hidden">
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

        {/* Footer: theme toggle */}
        {!iconsOnly && showThemeToggle && (
          <div
            className="px-4 py-3 border-t flex items-center"
            style={{ borderColor: "var(--semantic-border)" }}
          >
            <ThemeToggle />
          </div>
        )}

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

      {/* Floating collapse/expand button on sidebar edge */}
      <button
        onClick={() => onCollapsedChange(!collapsed)}
        className="absolute top-1/2 -translate-y-1/2 rounded-full w-6 h-6 flex items-center justify-center border cursor-pointer transition-opacity duration-200 z-10 shadow-sm"
        style={{
          right: -12,
          opacity: hovered ? 1 : 0,
          pointerEvents: hovered ? "auto" : "none",
          background: "var(--glass-bg-strong, hsl(var(--card) / 0.8))",
          borderColor: "var(--semantic-border, hsl(var(--border)))",
          backdropFilter: "blur(8px)",
          WebkitBackdropFilter: "blur(8px)",
          color: "color-mix(in srgb, var(--semantic-text) 60%, transparent)",
        }}
        onMouseEnter={(e) => { e.currentTarget.style.color = "var(--semantic-text)"; }}
        onMouseLeave={(e) => { e.currentTarget.style.color = "color-mix(in srgb, var(--semantic-text) 60%, transparent)"; }}
      >
        {collapsed ? <ChevronRight className="w-3 h-3" /> : <ChevronLeft className="w-3 h-3" />}
      </button>
    </nav>
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
