import { useState } from "react";
import type { Workspace, User } from "../types/api";
import { Globe, BookOpen, Brain, Settings, ChevronLeft, ChevronRight } from "./icons";
import { WorkspaceSwitcher } from "./WorkspaceSwitcher";
import { useSidebar } from "./ui/sidebar";

export type View = "translate" | "termbase" | "memory" | "settings";

export interface NavItem {
  id: string;
  label: string;
  icon: React.ReactNode;
}

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
  collapsedWidth?: number;
  connectionState?: ConnectionState;
  pendingChanges?: number;
  /** Show theme toggle in sidebar footer (default true for web, false for desktop). */
  showThemeToggle?: boolean;
}

const defaultNavItems: NavItem[] = [
  { id: "translate", label: "Translate", icon: <Globe className="w-5 h-5" /> },
  { id: "termbase", label: "Termbase", icon: <BookOpen className="w-5 h-5" /> },
  { id: "memory", label: "Memory", icon: <Brain className="w-5 h-5" /> },
  { id: "settings", label: "Settings", icon: <Settings className="w-5 h-5" /> },
];

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
}: AppSidebarProps<V>) {
  const [hovered, setHovered] = useState(false);
  const { state } = useSidebar();
  const iconsOnly = collapsed;
  const navItems = [
    ...defaultNavItems.slice(0, -1),
    ...extraNavItems,
    defaultNavItems[defaultNavItems.length - 1],
  ];

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

      {/* Navigation */}
      <nav className="flex-1 overflow-hidden px-2 py-2">
        <ul className="flex flex-col gap-1 list-none p-0 m-0">
          {navItems.map(({ id, label, icon }) => {
            const isActive = activeView === id;
            return (
              <li key={id}>
                <button
                  data-testid={`nav-${id}`}
                  onClick={() => onViewChange(id as V)}
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
                      e.currentTarget.style.background =
                        "var(--sidebar-accent, var(--semantic-surface-elevated))";
                      e.currentTarget.style.color = "var(--sidebar-foreground)";
                    }
                  }}
                  onMouseLeave={(e) => {
                    if (!isActive) {
                      e.currentTarget.style.background = "transparent";
                      e.currentTarget.style.color =
                        "color-mix(in srgb, var(--sidebar-foreground) 60%, transparent)";
                    }
                  }}
                >
                  <span className="shrink-0 flex items-center justify-center">{icon}</span>
                  {!iconsOnly && <span>{label}</span>}
                </button>
              </li>
            );
          })}
        </ul>
      </nav>

      {/* Floating collapse/expand button on sidebar edge */}
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
