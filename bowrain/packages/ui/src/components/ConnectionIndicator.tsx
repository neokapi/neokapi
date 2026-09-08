import { Button } from "@neokapi/ui-primitives";
import { Loader2, RefreshCw, WifiOff } from "./icons";

/** The four states the desktop backend reports for its server connection. */
export type ConnectionState = "disconnected" | "connecting" | "connected" | "offline";

export interface ConnectionIndicatorProps {
  /** Undefined on the web, where there is no working copy to be offline from. */
  connectionState?: ConnectionState;
  /** Edits waiting in the offline queue. */
  pendingChanges?: number;
  /** Edits the server rejected outright on replay. */
  failedChanges?: number;
  /** Ask the backend to attempt a reconnection now. */
  onRetryConnection?: () => void;
}

/**
 * The chrome's account of the server connection: whether the working copy is
 * offline, how much work is waiting behind that, and whether an attempt to get
 * back is in flight.
 *
 * A connected app shows nothing here. Only the permanently-rejected count
 * outlives the outage that produced it, because those edits stay unapplied
 * until someone deals with them.
 */
export function ConnectionIndicator({
  connectionState,
  pendingChanges,
  failedChanges,
  onRetryConnection,
}: ConnectionIndicatorProps) {
  const isOffline = connectionState === "offline";
  const isConnecting = connectionState === "connecting";
  const hasPending = pendingChanges != null && pendingChanges > 0;
  const hasFailed = failedChanges != null && failedChanges > 0;

  return (
    <>
      {isConnecting && (
        <span
          className="flex items-center gap-1 text-xs text-muted-foreground"
          data-testid="connection-reconnecting"
        >
          <Loader2 className="size-3 animate-spin" />
          <span>Reconnecting</span>
        </span>
      )}

      {isOffline && (
        <span
          className="flex items-center gap-1 text-xs text-warning"
          data-testid="connection-offline"
        >
          <WifiOff className="size-3" />
          {hasPending ? (
            <span data-testid="offline-pending">{pendingChanges} pending</span>
          ) : (
            <span>Offline</span>
          )}
          {onRetryConnection && (
            <Button
              variant="ghost"
              size="sm"
              className="h-5 gap-1 px-1.5 text-xs"
              onClick={onRetryConnection}
              data-testid="connection-retry"
            >
              <RefreshCw className="size-3" />
              Retry
            </Button>
          )}
        </span>
      )}

      {hasFailed && (
        <span
          className="flex items-center gap-1 text-xs text-destructive"
          data-testid="connection-failed"
        >
          <WifiOff className="size-3" />
          <span>{failedChanges} failed</span>
        </span>
      )}
    </>
  );
}
