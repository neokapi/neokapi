import { useCallback, useEffect, useRef, useState } from "react";
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

/** How often the queue depth and the failed list are read while mounted. */
export const CONNECTIVITY_POLL_MS = 4000;

/** The ids of a failed list, as one comparable string. */
function idsKey(ids: readonly number[]): string {
  return ids.join(",");
}

/**
 * Surface the desktop working copy's connectivity to the shared chrome: online/
 * offline state plus the offline-queue depth (pending) and the changes that did
 * not reach the server (failed). Backed by `platform.connectivity`, which the
 * desktop wires to the Go connection state and offline queue. No-op on web,
 * where the connectivity seam member is absent and every field stays undefined.
 *
 * The failed list is read whole only when its ids change: reading it decodes
 * every entry's payload, so each poll reads the ids alone when the host offers
 * them, and an unchanged list keeps the array the chrome already drew.
 */
export function useConnectivity(): ConnectivityStatus {
  const platform = usePlatform();
  const conn = platform.connectivity;

  const [state, setState] = useState<ConnectionState | undefined>(() => conn?.state());
  const [pendingChanges, setPendingChanges] = useState<number | undefined>(undefined);
  const [failedChanges, setFailedChanges] = useState<FailedChange[] | undefined>(undefined);
  const [refresh, setRefresh] = useState(0);
  // The ids of the list last read, so a poll whose ids match reads no list.
  const failedKey = useRef<string | undefined>(undefined);

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
        if (!cancelled) setPendingChanges(pending);
        if (!conn.failedChanges) return;
        if (conn.failedChangeIds && failedKey.current !== undefined) {
          const ids = await conn.failedChangeIds();
          if (idsKey(ids) === failedKey.current) return;
        }
        const failed = await conn.failedChanges();
        if (!cancelled) {
          failedKey.current = idsKey(failed.map((c) => c.id));
          setFailedChanges(failed);
        }
      } catch {
        /* transient binding failure — keep the last known counts */
      }
    };
    void poll();
    // Counts change as the offline queue drains or a sync fails; there is no
    // push event for them, so poll at a slow cadence while mounted.
    const id = setInterval(() => void poll(), CONNECTIVITY_POLL_MS);
    return () => {
      cancelled = true;
      clearInterval(id);
    };
  }, [conn, state, refresh]);

  // A dismissal that fails leaves the entry in place, so the next read shows
  // it again either way.
  const reread = useCallback(() => setRefresh((n) => n + 1), []);
  const retry = useCallback(() => {
    void conn?.retry?.();
  }, [conn]);
  const dismissFailedChange = useCallback(
    (id: number) => {
      void conn?.dismissFailedChange?.(id).then(reread, reread);
    },
    [conn, reread],
  );
  const dismissFailedChanges = useCallback(() => {
    void conn?.dismissFailedChanges?.().then(reread, reread);
  }, [conn, reread]);

  return {
    state,
    pendingChanges,
    failedChanges,
    dismissFailedChange: conn?.dismissFailedChange ? dismissFailedChange : undefined,
    dismissFailedChanges: conn?.dismissFailedChanges ? dismissFailedChanges : undefined,
    retry: conn?.retry ? retry : undefined,
  };
}
