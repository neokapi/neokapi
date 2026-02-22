import { WifiOff } from "lucide-react";
import { cn } from "@gokapi/ui";

type ConnectionState = "disconnected" | "connecting" | "connected" | "offline";

interface HeaderProps {
  sidebarCollapsed: boolean;
  connectionState: ConnectionState;
  pendingChanges?: number;
}

export function Header({ sidebarCollapsed, connectionState, pendingChanges }: HeaderProps) {
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
      className={cn(
        "h-12 border-b border-border flex items-center justify-between glass-surface bg-card/80 pr-5",
        sidebarCollapsed ? "pl-20" : "pl-5",
      )}
      style={{
        // @ts-expect-error non-standard CSS property for Wails
        "--wails-draggable": "drag",
      }}
    >
      <span className="text-sm text-muted-foreground">
        Localization Workbench
      </span>
      <div className="flex items-center gap-3">
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
          title={stateLabel}
        />
      </div>
    </header>
  );
}
