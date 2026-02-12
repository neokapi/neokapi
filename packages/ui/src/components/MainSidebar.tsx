import type { Workspace } from "../types/api";
import { useTheme } from "../context/ThemeContext";

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
      style={{
        width,
        backgroundColor: "var(--bg-secondary)",
        borderRight: collapsed && collapsedWidth === 0 ? "none" : "1px solid var(--border)",
        display: "flex",
        flexDirection: "column",
        transition: "width 0.2s ease",
        overflow: "hidden",
        flexShrink: 0,
      }}
    >
      {/* Top spacer (e.g. macOS traffic lights) */}
      {topSpacer > 0 && <div style={{ height: topSpacer, flexShrink: 0 }} />}

      {/* Workspace name header */}
      {!iconsOnly && (
        <div
          style={{
            padding: topSpacer > 0 ? "4px 16px 8px" : "16px 16px 8px",
            fontWeight: 700,
            fontSize: 15,
            color: "var(--text-primary)",
            overflow: "hidden",
            textOverflow: "ellipsis",
            whiteSpace: "nowrap",
            borderBottom: "1px solid var(--border)",
          }}
        >
          {workspace?.name || "No workspace"}
        </div>
      )}

      {/* Navigation items */}
      <div style={{ flex: 1, padding: "8px 0" }}>
        {navItems.map(({ id, label, icon }) => (
          <button
            key={id}
            data-testid={`nav-${id}`}
            onClick={() => onViewChange(id as V)}
            style={{
              display: "flex",
              alignItems: "center",
              gap: 12,
              width: "100%",
              padding: iconsOnly ? "12px 0" : "10px 16px",
              justifyContent: iconsOnly ? "center" : "flex-start",
              background: activeView === id ? "var(--bg-tertiary)" : "transparent",
              border: "none",
              borderLeft: activeView === id
                ? "3px solid var(--accent)"
                : "3px solid transparent",
              color: activeView === id
                ? "var(--text-primary)"
                : "var(--text-secondary)",
              cursor: "pointer",
              fontSize: 14,
              textAlign: "left",
            }}
          >
            <span>{icon}</span>
            {!iconsOnly && <span>{label}</span>}
          </button>
        ))}
      </div>

      {/* Footer: theme toggle + collapse */}
      <div
        style={{
          padding: "12px 16px",
          borderTop: "1px solid var(--border)",
          display: "flex",
          justifyContent: iconsOnly ? "center" : "space-between",
          alignItems: "center",
        }}
      >
        {!iconsOnly && <ThemeToggle />}
        <button
          onClick={() => onCollapsedChange(!collapsed)}
          style={{
            background: "none",
            border: "none",
            color: "var(--text-secondary)",
            cursor: "pointer",
            fontSize: 16,
          }}
        >
          {collapsed ? "\u{25B6}" : "\u{25C0}"}
        </button>
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
      style={{
        background: "none",
        border: "none",
        color: "var(--text-secondary)",
        cursor: "pointer",
        fontSize: 16,
        padding: 0,
        lineHeight: 1,
      }}
    >
      {resolvedTheme === "dark" ? "\u{2600}\u{FE0F}" : "\u{1F319}"}
    </button>
  );
}
