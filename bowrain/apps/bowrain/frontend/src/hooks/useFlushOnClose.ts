import { useCallback, useEffect, useState } from "react";
import { Events } from "@wailsio/runtime";
import { flushPendingSaves, hasPendingSaves, subscribePendingSaves } from "@neokapi/bowrain-app";
import { Backend } from "../api/backend";

/** The event the backend emits when it holds a window close or a quit. */
export const CLOSE_REQUESTED_EVENT = "app:close-requested";

/** How long a close waits for the waiting saves to reach the server. */
export const FLUSH_TIMEOUT_MS = 10_000;

/** Why the waiting saves did not all reach the server. */
export interface FlushFailure {
  timedOut: boolean;
  errors: unknown[];
}

/**
 * useFlushOnClose saves every waiting edit before the window goes.
 *
 * It tells the backend whether any surface has a save waiting, so the backend
 * holds a window close or a quit only then. On a held close it acknowledges
 * at once, flushes the waiting saves for at most FLUSH_TIMEOUT_MS, and lets
 * the close finish when they all reached the server. When a save fails or the
 * wait runs out it keeps the window open and returns the failure, so the
 * shell can show it and offer Close anyway or Stay.
 */
export function useFlushOnClose() {
  const [failure, setFailure] = useState<FlushFailure | null>(null);
  const [flushing, setFlushing] = useState(false);

  // Report the waiting saves whenever they change, and stop holding the window
  // when the page unmounts or unloads.
  useEffect(() => {
    const report = () =>
      void Promise.resolve(Backend.SetPendingSaves(hasPendingSaves())).catch(() => undefined);
    const clear = () => void Promise.resolve(Backend.SetPendingSaves(false)).catch(() => undefined);
    report();
    const unsubscribe = subscribePendingSaves(report);
    window.addEventListener("pagehide", clear);
    return () => {
      unsubscribe();
      window.removeEventListener("pagehide", clear);
      clear();
    };
  }, []);

  const onCloseRequested = useCallback(async () => {
    void Promise.resolve(Backend.AcknowledgeClose()).catch(() => undefined);
    setFlushing(true);
    try {
      const result = await flushPendingSaves(FLUSH_TIMEOUT_MS);
      if (result.ok) {
        setFailure(null);
        await Backend.ConfirmClose();
        return;
      }
      setFailure({ timedOut: result.timedOut, errors: result.errors });
    } finally {
      setFlushing(false);
    }
  }, []);

  useEffect(() => {
    const cancel = Events.On(CLOSE_REQUESTED_EVENT, () => void onCloseRequested());
    return () => cancel?.();
  }, [onCloseRequested]);

  const closeAnyway = useCallback(async () => {
    setFailure(null);
    await Backend.ConfirmClose();
  }, []);

  const stay = useCallback(async () => {
    setFailure(null);
    await Backend.CancelClose();
  }, []);

  return { failure, flushing, closeAnyway, stay, onCloseRequested };
}
