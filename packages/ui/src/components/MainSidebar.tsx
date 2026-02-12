import { cn } from "../lib/utils";
import type { Workspace } from "../types/api";
import { useTheme } from "../context/ThemeContext";
import { Separator } from "./ui/separator";

export type View = "translate" | "termbase" | "memory" | "settings";

export interface NavItem {
  id: string;
  label: string;
  icon: string;
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
  { id: "translate", label: "Translate", icon: "\u{1F30D}" },
  { id: "termbase", label: "Termbase", icon: "\u{1F4DA}" },
  { id: "memory", label: "Memory", icon: "\u{1F9E0}" },
  { id: "settings", label: "Settings", icon: "\u{2699}\u{FE0F}" },
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
      className="bg-sidebar flex flex-col transition-[width] duration-200 ease-in-out overflow-hidden shrink-0"
      style={{
        width,
        borderRight: collapsed && collapsedWidth === 0 ? "none" : undefined,
      }}
    >
      {/* Border only when visible */}
      {!(collapsed && collapsedWidth === 0) && (
        <div className="absolute inset-y-0 right-0 w-px bg-sidebar-border" style={{ display: "none" }} />
      )}
      <div className={cn("flex flex-col h-full", !(collapsed && collapsedWidth === 0) && "border-r border-sidebar-border")}>
        {/* Top spacer (e.g. macOS traffic lights) */}
        {topSpacer > 0 && <div style={{ height: topSpacer }} className="shrink-0" />}

        {/* Workspace name header */}
        {!iconsOnly && (
          <div className={cn("px-4 font-bold text-[15px] text-sidebar-foreground overflow-hidden text-ellipsis whitespace-nowrap border-b border-sidebar-border", topSpacer > 0 ? "pt-1 pb-2" : "pt-4 pb-2")}>
            {workspace?.name || "No workspace"}
          </div>
        )}

        {/* Navigation items */}
        <div className="flex-1 py-2">
          {navItems.map(({ id, label, icon }) => (
            <button
              key={id}
              data-testid={`nav-${id}`}
              onClick={() => onViewChange(id as V)}
              className={cn(
                "flex items-center gap-3 w-full border-none cursor-pointer text-sm text-left transition-colors",
                iconsOnly ? "py-3 justify-center" : "px-4 py-2.5 justify-start",
                activeView === id
                  ? "bg-sidebar-accent text-sidebar-accent-foreground border-l-[3px] border-l-sidebar-primary"
                  : "bg-transparent text-muted-foreground border-l-[3px] border-l-transparent hover:bg-sidebar-accent/50",
              )}
            >
              <span>{icon}</span>
              {!iconsOnly && <span>{label}</span>}
            </button>
          ))}
        </div>

        {/* Footer: theme toggle + collapse */}
        <div className={cn("px-4 py-3 border-t border-sidebar-border flex items-center", iconsOnly ? "justify-center" : "justify-between")}>
          {!iconsOnly && <ThemeToggle />}
          <button
            onClick={() => onCollapsedChange(!collapsed)}
            className="bg-transparent border-none text-muted-foreground cursor-pointer text-base hover:text-foreground transition-colors"
          >
            {collapsed ? "\u{25B6}" : "\u{25C0}"}
          </button>
        </div>
      </div>
    </nav>
  );
}

function ThemeToggle() {
  const { resolvedTheme, setTheme } = useTheme();
  return (
    <button
      data-testid="theme-toggle"
      onClick={() => setTheme(resolvedTheme === "dark" ? "light" : "dark")}
      title={resolvedTheme === "dark" ? "Switch to light mode" : "Switch to dark mode"}
      className="bg-transparent border-none text-muted-foreground cursor-pointer text-base p-0 leading-none hover:text-foreground transition-colors"
    >
      {resolvedTheme === "dark" ? "\u{2600}\u{FE0F}" : "\u{1F319}"}
    </button>
  );
}
