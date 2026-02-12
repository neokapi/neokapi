interface HeaderProps {
  connected: boolean;
  sidebarCollapsed: boolean;
}

export function Header({ connected, sidebarCollapsed }: HeaderProps) {
  return (
    <header
      className="h-12 bg-card border-b border-border flex items-center justify-between"
      style={{
        padding: sidebarCollapsed ? "0 20px 0 80px" : "0 20px",
        // Wails: draggable title bar region
        // @ts-expect-error non-standard CSS property for Wails
        "--wails-draggable": "drag",
      }}
    >
      <span className="text-sm text-muted-foreground">
        Localization Workbench
      </span>
      <div className="flex items-center gap-3">
        <span
          className={`w-2 h-2 rounded-full inline-block ${connected ? "bg-green-500" : "bg-destructive"}`}
        />
        <span className="text-xs text-muted-foreground">
          {connected ? "Connected" : "Disconnected"}
        </span>
      </div>
    </header>
  );
}
