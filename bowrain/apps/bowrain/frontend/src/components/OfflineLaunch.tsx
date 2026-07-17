import { useCallback, useEffect, useRef, useState } from "react";
import { Button } from "@neokapi/ui";
import { Loader2, RotateCw, WifiOff } from "lucide-react";

interface OfflineLaunchProps {
  /** The server the app is configured to reach (shown so the user knows what's down). */
  serverURL: string;
  /**
   * Re-probe connectivity. Resolves true when the server is reachable again — the
   * gate then enters the app (unmounting this screen). Resolves false to keep
   * waiting. The gate owns the actual transition; this screen only drives retries.
   */
  onRetry: () => Promise<boolean>;
  /** Switch to the normal ServerConnect form to point at a different server. */
  onUseDifferentServer: () => void;
}

const INITIAL_DELAY_MS = 2_000;
const MAX_DELAY_MS = 30_000;

/**
 * The offline-launch gate state (#1284). The desktop needs its server to load a
 * workspace; when it can't be reached at launch, this replaces the dead-end (a
 * blank screen, a raw fetch error, or a sign-in form that can't work) with a
 * calm, designed state: it says plainly what's wrong, shows the server it's
 * trying, retries automatically with backoff, and enters the app on its own the
 * moment connectivity returns. Distinct from an expired/invalid session, which
 * still routes to ServerConnect to sign in again.
 */
export function OfflineLaunch({ serverURL, onRetry, onUseDifferentServer }: OfflineLaunchProps) {
  const [retrying, setRetrying] = useState(true);
  // Keep the latest onRetry without restarting the backoff loop on every render.
  const retryRef = useRef(onRetry);
  retryRef.current = onRetry;

  // Automatic retry with exponential backoff. On success the gate advances and
  // unmounts this component; the cleanup flag stops any in-flight loop.
  useEffect(() => {
    let cancelled = false;
    let timer: ReturnType<typeof setTimeout>;
    let delay = INITIAL_DELAY_MS;

    const attempt = async () => {
      if (cancelled) return;
      setRetrying(true);
      let reachable = false;
      try {
        reachable = await retryRef.current();
      } catch {
        reachable = false;
      }
      if (cancelled || reachable) return;
      setRetrying(false);
      delay = Math.min(delay * 2, MAX_DELAY_MS);
      timer = setTimeout(attempt, delay);
    };

    // Probe immediately on entering the state, then back off.
    timer = setTimeout(attempt, 0);
    return () => {
      cancelled = true;
      clearTimeout(timer);
    };
  }, []);

  const handleManualRetry = useCallback(async () => {
    setRetrying(true);
    try {
      await retryRef.current();
    } finally {
      setRetrying(false);
    }
  }, []);

  return (
    <div className="flex flex-col items-center justify-center h-full gap-8">
      <div className="flex flex-col items-center text-center gap-3">
        <div className="w-12 h-12 rounded-full bg-muted flex items-center justify-center">
          <WifiOff className="w-6 h-6 text-muted-foreground" />
        </div>
        <h2 className="text-2xl font-semibold">Can't reach your server</h2>
        <p className="text-muted-foreground text-sm max-w-md">
          Bowrain needs to reach{" "}
          <span className="font-medium text-foreground break-all">{serverURL}</span> to load your
          workspace. It keeps trying and opens the moment the connection is back.
        </p>
      </div>

      <div className="w-full max-w-md flex flex-col items-center gap-4">
        <div
          className="flex items-center gap-2 text-sm text-muted-foreground"
          data-testid="offline-retry-status"
        >
          <Loader2 className={`w-4 h-4 ${retrying ? "animate-spin" : ""}`} />
          {retrying ? "Reconnecting…" : "Waiting to retry…"}
        </div>

        <Button
          className="w-full"
          onClick={handleManualRetry}
          disabled={retrying}
          data-testid="offline-try-again"
        >
          <RotateCw className="w-4 h-4 mr-2" />
          Try again
        </Button>

        <button
          type="button"
          onClick={onUseDifferentServer}
          className="text-xs text-muted-foreground hover:text-foreground transition-colors cursor-pointer"
          data-testid="offline-use-different-server"
        >
          Use a different server
        </button>
      </div>
    </div>
  );
}
