type View = "formats" | "tools" | "flows" | "convert" | "translate";

interface SidebarProps {
  activeView: View;
  onViewChange: (view: View) => void;
  collapsed: boolean;
  onCollapsedChange: (collapsed: boolean) => void;
}

const navItems: { view: View; label: string; icon: string }[] = [
  { view: "formats", label: "Formats", icon: "\u{1F4C4}" },
  { view: "tools", label: "Tools", icon: "\u{1F527}" },
  { view: "flows", label: "Flows", icon: "\u{1F500}" },
  { view: "convert", label: "Convert", icon: "\u{1F504}" },
  { view: "translate", label: "Translate", icon: "\u{1F30D}" },
];

export function Sidebar({ activeView, onViewChange, collapsed, onCollapsedChange }: SidebarProps) {

  return (
    <nav
      style={{
        width: collapsed ? 60 : 220,
        backgroundColor: "var(--bg-secondary)",
        borderRight: "1px solid var(--border)",
        display: "flex",
        flexDirection: "column",
        transition: "width 0.2s ease",
        overflow: "hidden",
      }}
    >
      {/* Spacer for macOS traffic light buttons */}
      <div style={{ height: 38, flexShrink: 0 }} />
      <div style={{ flex: 1, padding: "8px 0" }}>
        {navItems.map(({ view, label, icon }) => (
          <button
            key={view}
            onClick={() => onViewChange(view)}
            style={{
              display: "flex",
              alignItems: "center",
              gap: 12,
              width: "100%",
              padding: collapsed ? "12px 0" : "12px 16px",
              justifyContent: collapsed ? "center" : "flex-start",
              background:
                activeView === view ? "var(--bg-tertiary)" : "transparent",
              border: "none",
              borderLeft:
                activeView === view
                  ? "3px solid var(--accent)"
                  : "3px solid transparent",
              color:
                activeView === view
                  ? "var(--text-primary)"
                  : "var(--text-secondary)",
              cursor: "pointer",
              fontSize: 14,
              textAlign: "left",
            }}
          >
            <span>{icon}</span>
            {!collapsed && <span>{label}</span>}
          </button>
        ))}
      </div>
      <div
        style={{
          padding: "12px 16px",
          borderTop: "1px solid var(--border)",
          display: "flex",
          alignItems: "center",
          justifyContent: collapsed ? "center" : "space-between",
        }}
      >
        {!collapsed && (
          <span style={{ fontWeight: 700, fontSize: 18 }}>Bowrain</span>
        )}
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

export type { View };
