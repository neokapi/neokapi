import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  type ReactNode,
} from "react";
import { Events } from "@wailsio/runtime";

/**
 * Backend freshness events emitted by the Go backend's ProjectWatcher (over the
 * gRPC WatchProject stream) and reconnect logic. Each view subscribes to the
 * event(s) it cares about and refetches its own data; on `reconnected` every
 * subscriber refetches, since while offline we may have missed any number of
 * external changes.
 *
 * Mirrors the web app's EventSource → invalidateQueries layer (the desktop has
 * no React Query, so views own their refetch callbacks).
 */
export type BackendEventName =
  | "blocks-changed"
  | "presence-changed"
  | "project-changed"
  | "connector-sync"
  | "flow-changed"
  | "membership-changed"
  | "brand-voice-changed"
  | "termbase-changed"
  | "stream-changed"
  | "reconnected";

/** Every freshness event that should trigger a full refresh on reconnect. */
const REFRESHABLE_EVENTS: BackendEventName[] = [
  "blocks-changed",
  "presence-changed",
  "project-changed",
  "connector-sync",
  "flow-changed",
  "membership-changed",
  "brand-voice-changed",
  "termbase-changed",
  "stream-changed",
];

type Listener = (data: unknown) => void;

interface BackendEventsContextValue {
  /**
   * Subscribe a listener to one or more backend events. Returns an
   * unsubscribe function. A listener registered for any refreshable event is
   * also invoked on `reconnected` so the view pulls fresh state after an
   * offline gap.
   */
  subscribe: (events: BackendEventName | BackendEventName[], listener: Listener) => () => void;
}

const BackendEventsContext = createContext<BackendEventsContextValue | null>(null);

/**
 * Provider that bridges the single set of Wails `Events.On` subscriptions to
 * many view-level listeners. One Wails subscription per event type is shared
 * across all views via a ref-counted listener registry, avoiding a proliferation
 * of native event handlers.
 */
export function BackendEventsProvider({ children }: { children: ReactNode }) {
  // Map of event name → set of listeners.
  const listenersRef = useRef<Map<BackendEventName, Set<Listener>>>(new Map());

  const dispatch = useCallback((event: BackendEventName, data: unknown) => {
    const set = listenersRef.current.get(event);
    if (set) {
      for (const l of set) {
        try {
          l(data);
        } catch (err) {
          console.warn(`backend event listener for "${event}" threw`, err);
        }
      }
    }
    // A reconnect re-runs every refreshable listener so all open views pull
    // fresh authoritative state, not just whatever we queued offline.
    if (event === "reconnected") {
      for (const name of REFRESHABLE_EVENTS) {
        const refreshSet = listenersRef.current.get(name);
        if (!refreshSet) continue;
        for (const l of refreshSet) {
          try {
            l(data);
          } catch (err) {
            console.warn(`backend reconnect refresh for "${name}" threw`, err);
          }
        }
      }
    }
  }, []);

  // One native Wails subscription per event type, fanning out to listeners.
  useEffect(() => {
    const allEvents: BackendEventName[] = [...REFRESHABLE_EVENTS, "reconnected"];
    const cancels = allEvents.map((name) =>
      Events.On(name, (event: { data: unknown }) => dispatch(name, event?.data)),
    );
    return () => {
      for (const cancel of cancels) cancel?.();
    };
  }, [dispatch]);

  const subscribe = useCallback<BackendEventsContextValue["subscribe"]>((events, listener) => {
    const names = Array.isArray(events) ? events : [events];
    for (const name of names) {
      let set = listenersRef.current.get(name);
      if (!set) {
        set = new Set();
        listenersRef.current.set(name, set);
      }
      set.add(listener);
    }
    return () => {
      for (const name of names) {
        listenersRef.current.get(name)?.delete(listener);
      }
    };
  }, []);

  const value = useMemo<BackendEventsContextValue>(() => ({ subscribe }), [subscribe]);

  return <BackendEventsContext.Provider value={value}>{children}</BackendEventsContext.Provider>;
}

/**
 * Subscribe a view to one or more backend freshness events. The handler is
 * invoked whenever the event fires, and additionally on `reconnected` for any
 * refreshable event. Safe to call when no provider is mounted (no-op), so views
 * work in isolation (e.g. storybook).
 */
export function useBackendEvents(
  events: BackendEventName | BackendEventName[],
  handler: Listener,
): void {
  const ctx = useContext(BackendEventsContext);
  // Keep the latest handler without re-subscribing on every render.
  const handlerRef = useRef(handler);
  handlerRef.current = handler;

  useEffect(() => {
    if (!ctx) return;
    return ctx.subscribe(events, (data) => handlerRef.current(data));
    // events is intentionally spread so changing the set re-subscribes.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ctx, Array.isArray(events) ? events.join(",") : events]);
}
