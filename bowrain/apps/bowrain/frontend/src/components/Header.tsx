import { LogOut, WifiOff } from "lucide-react";

type ConnectionState = "disconnected" | "connecting" | "connected" | "offline";

interface HeaderProps {
  sidebarCollapsed: boolean;
  connectionState: ConnectionState;
  userName?: string;
  pendingChanges?: number;
  onDisconnect?: () => void;
}

export function Header({ sidebarCollapsed, connectionState, userName, pendingChanges, onDisconnect }: HeaderProps) {
  const isConnected = connectionState === "connected";
  const isOffline = connectionState === "offline";

  const dotColor = isConnected
    ? "bg-green-500"
    : isOffline
      ? "bg-amber-500"
      : "bg-muted-foreground/40";

  const stateLabel = isConnected
    ? "Connected"
    : isOffline
      ? "Offline"
      : "Local";

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
        {(isConnected || isOffline) && userName && (
          <span className="text-xs text-muted-foreground">{userName}</span>
        )}
        {isOffline && (
          <span className="flex items-center gap-1 text-xs text-amber-500">
            <WifiOff className="w-3 h-3" />
            {pendingChanges != null && pendingChanges > 0 && (
              <span>{pendingChanges} pending</span>
            )}
          </span>
        )}
        <span
          className={`w-2 h-2 rounded-full inline-block ${dotColor}`}
        />
        <span className="text-xs text-muted-foreground">
          {stateLabel}
        </span>
        {(isConnected || isOffline) && onDisconnect && (
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
