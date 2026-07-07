/**
 * Bridge Wails runtime events to react-query cache invalidation.
 *
 * Replaces the old hand-rolled "event fires → manually refetch into useState"
 * pattern: name the query keys a backend event should invalidate, and
 * react-query refetches the active ones. Outside Wails (Storybook / vitest) the
 * event never fires, so this is a no-op.
 */

import { useQueryClient, type QueryKey } from "@tanstack/react-query";
import { useWailsEvents } from "./useWailsEvent";

/**
 * Invalidate the given query keys whenever any of the named Wails events fires.
 *
 * Usage:
 *   useInvalidateOnEvent("registries-changed", [qk.formats(), qk.pluginDocsSummary()]);
 *   useInvalidateOnEvent(["plugins-changed", "registries-changed"], [qk.plugins()]);
 */
export function useInvalidateOnEvent(events: string | string[], keys: readonly QueryKey[]): void {
  const qc = useQueryClient();
  const names = Array.isArray(events) ? events : [events];
  useWailsEvents(names, () => {
    for (const key of keys) {
      void qc.invalidateQueries({ queryKey: key });
    }
  });
}
