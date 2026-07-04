import { useEffect, useRef, useState } from "react";
import {
  reduceRun,
  emptyRunModel,
  type ConvergenceEvent,
  type ConvergenceRunModel,
} from "@neokapi/ui";

/**
 * Subscribe to one convergence run's server-side event stream and fold it into
 * a render model. The endpoint (GET /api/v1/:ws/:projectId/convergence/runs/
 * :runId/events) replays the run's persisted events from seq 0, then follows
 * live and closes after the terminal `done` event.
 *
 * Because a reconnect replays from seq 0 again, each fresh connection rebuilds
 * the model from scratch (the accumulator resets on open) so passes are never
 * double-counted. Same-origin cookies authenticate the stream; a dropped live
 * connection reconnects with exponential backoff until the run is done.
 */
export function useConvergenceRunEvents(
  workspaceSlug: string | undefined,
  projectId: string | undefined,
  runId: string | null | undefined,
): { model: ConvergenceRunModel; connecting: boolean } {
  const [model, setModel] = useState<ConvergenceRunModel>(emptyRunModel);
  const [connecting, setConnecting] = useState(false);
  const eventsRef = useRef<ConvergenceEvent[]>([]);

  useEffect(() => {
    if (!workspaceSlug || !projectId || !runId) {
      setModel(emptyRunModel());
      setConnecting(false);
      return;
    }
    if (typeof EventSource === "undefined") return; // SSR / non-browser guard.

    let es: EventSource | null = null;
    let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
    let closed = false;
    let done = false;
    let backoff = 1000;
    const maxBackoff = 30_000;

    const url = `/api/v1/${encodeURIComponent(workspaceSlug)}/${encodeURIComponent(
      projectId,
    )}/convergence/runs/${encodeURIComponent(runId)}/events`;

    const connect = () => {
      if (closed) return;
      setConnecting(true);
      // A fresh connection replays from seq 0 — start the fold over.
      eventsRef.current = [];
      es = new EventSource(url, { withCredentials: true });

      es.onopen = () => {
        backoff = 1000;
        setConnecting(false);
      };

      es.onmessage = (e) => {
        try {
          const ev = JSON.parse(e.data) as ConvergenceEvent;
          eventsRef.current.push(ev);
          setModel(reduceRun(eventsRef.current));
          if (ev.type === "done") {
            done = true;
            closed = true;
            es?.close();
            es = null;
            setConnecting(false);
          }
        } catch {
          /* ignore malformed frames */
        }
      };

      es.onerror = () => {
        // The server closes the stream after `done`; that surfaces here as an
        // error we must not reconnect on.
        if (done || closed) {
          es?.close();
          es = null;
          return;
        }
        if (es && es.readyState === EventSource.CLOSED) {
          es.close();
          es = null;
          reconnectTimer = setTimeout(connect, backoff);
          backoff = Math.min(backoff * 2, maxBackoff);
        }
      };
    };

    connect();

    return () => {
      closed = true;
      if (reconnectTimer) clearTimeout(reconnectTimer);
      es?.close();
    };
  }, [workspaceSlug, projectId, runId]);

  return { model, connecting };
}
