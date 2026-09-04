import { useEffect, useState } from "react";
import type { AutomationRun, AutomationStep } from "../types/api";

/**
 * One frame of an automation run's event stream. The server sends the same
 * shape for the opening snapshot, for each transition it pushes as the run
 * manager persists it (`run.started`, `step.started`, `step.progress`,
 * `step.finished`, `run.finished`), and for the snapshot it repeats every
 * few seconds as a safety net. Each carries the run and its steps as stored,
 * so a client replaces rather than merges.
 */
export interface AutomationRunFrame {
  type: string;
  run: AutomationRun;
  steps: AutomationStep[];
}

/** What a run's stream has said so far. */
export interface AutomationRunLive {
  /** The run as last streamed; null before the first frame. */
  run: AutomationRun | null;
  /** The steps as last streamed; null before the first frame. */
  steps: AutomationStep[] | null;
  /** The server closed the stream after the run settled. */
  done: boolean;
  /** A stream is open for the run. */
  connected: boolean;
}

const idle: AutomationRunLive = { run: null, steps: null, done: false, connected: false };

/**
 * Follow one automation run over its server-sent event stream
 * (GET /api/v1/:ws/:projectId/automations/runs/:runId/events).
 *
 * The stream is a same-origin EventSource authenticated by the session
 * cookie. Every frame replaces the run and its steps, so a reconnect needs
 * no replay: the opening snapshot of the new connection is the current
 * state. A hard-closed connection reconnects with exponential backoff until
 * the server sends `done`, which it does once a snapshot finds the run
 * settled. Pass `enabled: false` where the stream cannot be opened (the
 * desktop app reaches the server without a cookie session).
 */
export function useAutomationRunEvents(
  workspaceSlug: string | undefined,
  projectId: string | undefined,
  runId: string | null | undefined,
  enabled = true,
): AutomationRunLive {
  const [live, setLive] = useState<AutomationRunLive>(idle);

  useEffect(() => {
    // Every branch that opens no stream resets to idle, including the SSR /
    // non-browser guard, so a consumer never sees a stale run.
    setLive(idle);
    if (!enabled || !workspaceSlug || !projectId || !runId) return;
    if (typeof EventSource === "undefined") return;

    let es: EventSource | null = null;
    let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
    let closed = false;
    let backoff = 1000;
    const maxBackoff = 30_000;

    const url = `/api/v1/${encodeURIComponent(workspaceSlug)}/${encodeURIComponent(
      projectId,
    )}/automations/runs/${encodeURIComponent(runId)}/events`;

    const connect = () => {
      if (closed) return;
      const source = new EventSource(url, { withCredentials: true });
      es = source;

      source.onopen = () => {
        backoff = 1000;
        setLive((prev) => ({ ...prev, connected: true }));
      };

      source.addEventListener("message", (e: MessageEvent) => {
        let frame: AutomationRunFrame;
        try {
          frame = JSON.parse(e.data) as AutomationRunFrame;
        } catch {
          return; // a malformed frame is dropped; the next snapshot carries the state
        }
        if (!frame || typeof frame !== "object" || !frame.run) return;
        setLive((prev) => ({
          ...prev,
          run: frame.run,
          steps: Array.isArray(frame.steps) ? frame.steps : [],
          connected: true,
        }));
      });

      source.addEventListener("done", () => {
        closed = true;
        source.close();
        es = null;
        setLive((prev) => ({ ...prev, done: true, connected: false }));
      });

      source.onerror = () => {
        // The server closes the stream after `done`; that surfaces here as an
        // error that must not reconnect.
        if (closed) {
          source.close();
          es = null;
          return;
        }
        // A transient blip leaves readyState === CONNECTING and EventSource
        // reconnects itself (onopen fires again). Only a hard CLOSED needs a
        // manual backoff reconnect.
        if (source.readyState === EventSource.CLOSED) {
          source.close();
          es = null;
          setLive((prev) => ({ ...prev, connected: false }));
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
  }, [enabled, workspaceSlug, projectId, runId]);

  return live;
}
