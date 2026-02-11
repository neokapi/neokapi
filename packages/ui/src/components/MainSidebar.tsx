import type { Workspace } from "../types/api";
import { useTheme } from "../context/ThemeContext";

export type View = "translate" | "termbase" | "memory" | "settings";

interface MainSidebarProps {
  workspace: Workspace | null;
  activeView: View;
  onViewChange: (view: View) => void;
  collapsed: boolean;
  onCollapsedChange: (collapsed: boolean) => void;
}

const navItems: { view: View; label: string; icon: string }[] = [
  { view: "translate", label: "Translate", icon: "\u{1F30D}" },
  { view: "termbase", label: "Termbase", icon: "\u{1F4DA}" },
  { view: "memory", label: "Memory", icon: "\u{1F9E0}" },
  { view: "settings", label: "Settings", icon: "\u{2699}\u{FE0F}" },
];

export function MainSidebar({
  workspace,
  activeView,
  onViewChange,
  collapsed,
  onCollapsedChange,
}: MainSidebarProps) {
  return (
    <nav
      style={{
        width: collapsed ? 0 : 220,
        backgroundColor: "var(--bg-secondary)",
        borderRight: collapsed ? "none" : "1px solid var(--border)",
        display: "flex",
        flexDirection: "column",
        transition: "width 0.2s ease",
        overflow: "hidden",
        flexShrink: 0,
      }}
    >
      {/* Workspace name header */}
      <div
        style={{
          padding: "16px 16px 8px",
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

      {/* Navigation items */}
      <div style={{ flex: 1, padding: "8px 0" }}>
        {navItems.map(({ view, label, icon }) => (
          <button
            key={view}
            data-testid={`nav-${view}`}
            onClick={() => onViewChange(view)}
            style={{
              display: "flex",
              alignItems: "center",
              gap: 12,
              width: "100%",
              padding: "10px 16px",
              background: activeView === view ? "var(--bg-tertiary)" : "transparent",
              border: "none",
              borderLeft: activeView === view
                ? "3px solid var(--accent)"
                : "3px solid transparent",
              color: activeView === view
                ? "var(--text-primary)"
                : "var(--text-secondary)",
              cursor: "pointer",
              fontSize: 14,
              textAlign: "left",
            }}
          >
            <span>{icon}</span>
            <span>{label}</span>
          </button>
        ))}
      </div>

      {/* Footer: theme toggle + collapse */}
      <div
        style={{
          padding: "12px 16px",
          borderTop: "1px solid var(--border)",
          display: "flex",
          justifyContent: "space-between",
          alignItems: "center",
        }}
      >
        <ThemeToggle />
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
          {"\u{25C0}"}
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
