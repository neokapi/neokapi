import { useCallback, useEffect, useState } from "react";
import type { ConnectionState, FailedChange } from "@neokapi/ui";
import { usePlatform } from "../platform";

export interface ConnectivityStatus {
  /** Undefined on web (no connectivity seam) — the chrome then shows nothing. */
  state?: ConnectionState;
  pendingChanges?: number;
  /** The offline changes that did not reach the server. */
  failedChanges?: FailedChange[];
  /** Present only when the host keeps a list of failed changes. */
  dismissFailedChange?: (id: number) => void;
  dismissFailedChanges?: () => void;
  /** Present only when the host can attempt a reconnection on demand. */
  retry?: () => void;
}

/**
 * Surface the desktop working copy's connectivity to the shared chrome: online/
 * offline state plus the offline-queue depth (pending) and the changes that did
 * not reach the server (failed). Backed by `platform.connectivity`, which the
 * desktop wires to the Go connection state and offline queue. No-op on web,
 * where the connectivity seam member is absent and every field stays undefined.
 */
export function useConnectivity(): ConnectivityStatus {
  const platform = usePlatform();
  const conn = platform.connectivity;

  const [state, setState] = useState<ConnectionState | undefined>(() => conn?.state());
  const [pendingChanges, setPendingChanges] = useState<number | undefined>(undefined);
  const [failedChanges, setFailedChanges] = useState<FailedChange[] | undefined>(undefined);
  const [refresh, setRefresh] = useState(0);

  useEffect(() => {
    if (!conn) return;
    setState(conn.state());
    return conn.onChange(setState);
  }, [conn]);

  useEffect(() => {
    if (!conn) return;
    let cancelled = false;
    const poll = async () => {
      try {
        const pending = conn.pendingCount ? await conn.pendingCount() : undefined;
        const failed = conn.failedChanges ? await conn.failedChanges() : undefined;
        if (!cancelled) {
          setPendingChanges(pending);
          setFailedChanges(failed);
        }
      } catch {
        /* transient binding failure — keep the last known counts */
      }
    };
    void poll();
    // Counts change as the offline queue drains or a sync fails; there is no
    // push event for them, so poll at a slow cadence while mounted.
    const id = setInterval(() => void poll(), 4000);
    return () => {
      cancelled = true;
      clearInterval(id);
    };
  }, [conn, state, refresh]);

  const retry = useCallback(() => {
    void conn?.retry?.();
  }, [conn]);
  const dismissFailedChange = useCallback(
    (id: number) => {
      void conn?.dismissFailedChange?.(id).then(() => setRefresh((n) => n + 1));
    },
    [conn],
  );
  const dismissFailedChanges = useCallback(() => {
    void conn?.dismissFailedChanges?.().then(() => setRefresh((n) => n + 1));
  }, [conn]);

  return {
    state,
    pendingChanges,
    failedChanges,
    dismissFailedChange: conn?.dismissFailedChange ? dismissFailedChange : undefined,
    dismissFailedChanges: conn?.dismissFailedChanges ? dismissFailedChanges : undefined,
    retry: conn?.retry ? retry : undefined,
  };
}
