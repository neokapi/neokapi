/**
 * Hook for subscribing to Wails runtime events.
 *
 * Gracefully degrades in Storybook/vitest where the Wails runtime
 * isn't available — the callback simply never fires.
 */

import { useEffect } from "react";

type WailsEventCallback = (data: unknown) => void;

// Lazily resolve the Wails Events module once.
let eventsModule: { On: (name: string, cb: (e: { data: unknown }) => void) => () => void } | null = null;
let eventsLoaded = false;

async function getEvents() {
  if (eventsModule) return eventsModule;
  if (eventsLoaded) return null;
  try {
    const mod = await import("@wailsio/runtime");
    eventsModule = mod.Events;
    eventsLoaded = true;
    return eventsModule;
  } catch {
    eventsLoaded = true;
    return null;
  }
}

/**
 * Subscribe to a Wails event. The callback fires whenever the backend
 * emits the named event. Automatically cleans up on unmount.
 *
 * Usage:
 *   useWailsEvent("plugins-changed", () => refreshList());
 *   useWailsEvent("flow:event", (data) => handleEvent(data));
 */
export function useWailsEvent(name: string, callback: WailsEventCallback) {
  useEffect(() => {
    let cleanup: (() => void) | null = null;
    getEvents().then((events) => {
      if (events) {
        cleanup = events.On(name, (e) => callback(e.data));
      }
    });
    return () => { cleanup?.(); };
    // eslint-disable-next-line react-hooks/exhaustive-deps -- callback identity managed by caller
  }, [name]);
}

/**
 * Subscribe to multiple Wails events with a single callback.
 */
export function useWailsEvents(names: string[], callback: WailsEventCallback) {
  useEffect(() => {
    const cleanups: Array<() => void> = [];
    getEvents().then((events) => {
      if (events) {
        for (const name of names) {
          cleanups.push(events.On(name, (e) => callback(e.data)));
        }
      }
    });
    return () => cleanups.forEach((fn) => fn());
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [names.join(",")]);
}
