import { LogOut } from "lucide-react";

interface HeaderProps {
  sidebarCollapsed: boolean;
  serverConnected: boolean;
  userName?: string;
  onDisconnect?: () => void;
}

export function Header({ sidebarCollapsed, serverConnected, userName, onDisconnect }: HeaderProps) {
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
        {serverConnected && userName && (
          <span className="text-xs text-muted-foreground">{userName}</span>
        )}
        <span
          className={`w-2 h-2 rounded-full inline-block ${serverConnected ? "bg-green-500" : "bg-muted-foreground/40"}`}
        />
        <span className="text-xs text-muted-foreground">
          {serverConnected ? "Connected" : "Local"}
        </span>
        {serverConnected && onDisconnect && (
          <button
            onClick={onDisconnect}
            className="text-muted-foreground hover:text-foreground transition-colors p-1"
            title="Disconnect from server"
            style={{
              // @ts-expect-error prevent drag on interactive element
              "--wails-draggable": "no-drag",
            }}
          >
            <LogOut className="w-3.5 h-3.5" />
          </button>
        )}
      </div>
    </header>
  );
}
