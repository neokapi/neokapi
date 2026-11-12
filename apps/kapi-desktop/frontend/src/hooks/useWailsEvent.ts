/**
 * Hook for subscribing to Wails runtime events.
 *
 * Gracefully degrades in Storybook/vitest where the Wails runtime
 * isn't available — the callback simply never fires.
 */

import { useEffect, useRef } from "react";

type WailsEventCallback = (data: unknown) => void;

// Eagerly resolve the Wails Events module once at import time.
// This avoids race conditions where fast backend events arrive before
// the async import resolves.
type WailsEvents = { On: (name: string, cb: (e: { data: unknown }) => void) => () => void };
let eventsModule: WailsEvents | null = null;

const eventsReady: Promise<WailsEvents | null> = import("@wailsio/runtime")
  .then((mod) => {
    eventsModule = mod.Events;
    return eventsModule;
  })
  .catch(() => null);

function getEvents(): Promise<WailsEvents | null> {
  if (eventsModule) return Promise.resolve(eventsModule);
  return eventsReady;
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
  // The subscription lives as long as the name, and each event calls the
  // callback of the latest render. A callback captured once would read the
  // state of the first render: a handler that sends the project back to the
  // backend would send the project as it was when the component mounted.
  const latest = useRef(callback);
  latest.current = callback;
  useEffect(() => {
    let cleanup: (() => void) | null = null;
    let cancelled = false;
    void getEvents().then((events) => {
      if (cancelled) return;
      if (events) {
        cleanup = events.On(name, (e) => latest.current(e.data));
      }
    });
    return () => {
      cancelled = true;
      cleanup?.();
    };
  }, [name]);
}

/**
 * Subscribe to multiple Wails events with a single callback.
 */
export function useWailsEvents(names: string[], callback: WailsEventCallback) {
  const latest = useRef(callback);
  latest.current = callback;
  useEffect(() => {
    const cleanups: Array<() => void> = [];
    let cancelled = false;
    void getEvents().then((events) => {
      if (cancelled) return;
      if (events) {
        for (const name of names) {
          cleanups.push(events.On(name, (e) => latest.current(e.data)));
        }
      }
    });
    return () => {
      cancelled = true;
      cleanups.forEach((fn) => fn());
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [names.join(",")]);
}
