import { cn } from "../lib/utils";
import type { Workspace } from "../types/api";
import { useTheme, type Theme } from "../context/ThemeContext";
import { Globe, BookOpen, Brain, Settings, ChevronLeft, ChevronRight, Sun, Moon, Monitor } from "./icons";

export type View = "translate" | "termbase" | "memory" | "settings";

export interface NavItem {
  id: string;
  label: string;
  icon: React.ReactNode;
}

interface MainSidebarProps<V extends string = View> {
  workspace: Workspace | null;
  activeView: V;
  onViewChange: (view: V) => void;
  collapsed: boolean;
  onCollapsedChange: (collapsed: boolean) => void;
  /** Extra nav items inserted before Settings. */
  extraNavItems?: NavItem[];
  /** Top spacer height in px (e.g. 38 for macOS traffic lights). */
  topSpacer?: number;
  /** Width when collapsed. 0 = fully hidden (default), >0 = icons-only. */
  collapsedWidth?: number;
}

const defaultNavItems: NavItem[] = [
  { id: "translate", label: "Translate", icon: <Globe className="w-4 h-4" /> },
  { id: "termbase", label: "Termbase", icon: <BookOpen className="w-4 h-4" /> },
  { id: "memory", label: "Memory", icon: <Brain className="w-4 h-4" /> },
  { id: "settings", label: "Settings", icon: <Settings className="w-4 h-4" /> },
];

export function MainSidebar<V extends string = View>({
  workspace,
  activeView,
  onViewChange,
  collapsed,
  onCollapsedChange,
  extraNavItems = [],
  topSpacer = 0,
  collapsedWidth = 0,
}: MainSidebarProps<V>) {
  // Splice extra items before the last item (Settings).
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

        {/* Workspace name header */}
        {!iconsOnly && (
          <div
            className={cn(
              "px-4 font-bold text-[15px] overflow-hidden text-ellipsis whitespace-nowrap border-b",
              topSpacer > 0 ? "pt-1 pb-2" : "pt-4 pb-2",
            )}
            style={{
              color: "var(--semantic-text)",
              borderColor: "var(--semantic-border)",
            }}
          >
            {workspace?.name || "No workspace"}
          </div>
        )}

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

        {/* Footer: theme toggle + collapse */}
        <div
          className={cn(
            "px-4 py-3 border-t flex items-center",
            iconsOnly ? "justify-center" : "justify-between",
          )}
          style={{ borderColor: "var(--semantic-border)" }}
        >
          {!iconsOnly && <ThemeToggle />}
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
      </div>
    </nav>
  );
}

const nextTheme: Record<Theme, Theme> = { light: "dark", dark: "system", system: "light" };
const themeIcons: Record<Theme, React.ReactNode> = {
  light: <Sun className="w-4 h-4" />,
  dark: <Moon className="w-4 h-4" />,
  system: <Monitor className="w-4 h-4" />,
};
const themeLabels: Record<Theme, string> = {
  light: "Light",
  dark: "Dark",
  system: "System",
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
