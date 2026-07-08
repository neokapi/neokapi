/**
 * Bridge backend freshness events to react-query cache invalidation.
 *
 * Replaces the old hand-rolled "event fires → manually refetch into useState"
 * pattern: name the query keys a backend event should invalidate, and
 * react-query refetches the active ones. Outside Wails (Storybook / vitest) —
 * or when no BackendEventsProvider is mounted — the event never fires, so this
 * is a no-op. Mirrors kapi-desktop's useInvalidateOnEvent, over bowrain's
 * `useBackendEvents` fan-out instead of the raw Wails event.
 */

import { useQueryClient, type QueryKey } from "@tanstack/react-query";
import { useBackendEvents, type BackendEventName } from "./useBackendEvents";

/**
 * Invalidate the given query keys whenever any of the named backend events
 * fires (and on `reconnected`, via useBackendEvents' refresh fan-out).
 *
 * Usage:
 *   useInvalidateOnEvent("membership-changed", [qk.members(ws)]);
 *   useInvalidateOnEvent(["brand-voice-changed", "termbase-changed"], [qk.brandProfiles(ws)]);
 */
export function useInvalidateOnEvent(
  events: BackendEventName | BackendEventName[],
  keys: readonly QueryKey[],
): void {
  const qc = useQueryClient();
  useBackendEvents(events, () => {
    for (const key of keys) {
      void qc.invalidateQueries({ queryKey: key });
    }
  });
}
