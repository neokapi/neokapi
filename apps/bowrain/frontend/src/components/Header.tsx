interface HeaderProps {
  connected: boolean;
}

export function Header({ connected }: HeaderProps) {
  return (
    <header
      style={{
        height: 48,
        backgroundColor: "var(--bg-secondary)",
        borderBottom: "1px solid var(--border)",
        display: "flex",
        alignItems: "center",
        justifyContent: "space-between",
        padding: "0 20px",
        // Wails: draggable title bar region
        // @ts-expect-error non-standard CSS property for Wails
        "--wails-draggable": "drag",
      }}
    >
      <span style={{ fontSize: 14, color: "var(--text-secondary)" }}>
        Localization Workbench
      </span>
      <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
        <span
          style={{
            width: 8,
            height: 8,
            borderRadius: "50%",
            backgroundColor: connected ? "var(--success)" : "var(--error)",
            display: "inline-block",
          }}
        />
        <span style={{ fontSize: 12, color: "var(--text-secondary)" }}>
          {connected ? "Connected" : "Disconnected"}
        </span>
      </div>
    </header>
  );
}
