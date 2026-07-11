import { useEffect, useRef, useState } from "react";
import {
  applyEvent,
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
 * Two properties make this robust:
 *
 *   - **Seq-dedupe.** Each frame carries a monotonic sequence (the SSE `id:`
 *     surfaced as `lastEventId`, or a `seq` field on the payload). EventSource
 *     does a native auto-reconnect on a transient blip, and the server always
 *     replays from seq 0 — so we drop any frame whose seq we've already applied,
 *     making a replay-after-reconnect idempotent. A reset-on-open backstop
 *     covers a backend that does not yet tag frames with a seq.
 *   - **Incremental fold.** We keep the folded model and apply only each new
 *     event (O(1) per frame) instead of re-reducing a growing array (O(n²) over
 *     a burst replay).
 *
 * Same-origin cookies authenticate the stream; a hard-closed live connection
 * reconnects with exponential backoff until the run is done.
 */
export function useConvergenceRunEvents(
  workspaceSlug: string | undefined,
  projectId: string | undefined,
  runId: string | null | undefined,
): { model: ConvergenceRunModel; connecting: boolean } {
  const [model, setModel] = useState<ConvergenceRunModel>(emptyRunModel);
  const [connecting, setConnecting] = useState(false);
  const modelRef = useRef<ConvergenceRunModel>(emptyRunModel());
  const lastSeqRef = useRef<number>(-1);
  const seqModeRef = useRef<boolean>(false);

  useEffect(() => {
    if (!workspaceSlug || !projectId || !runId) {
      modelRef.current = emptyRunModel();
      lastSeqRef.current = -1;
      seqModeRef.current = false;
      setModel(modelRef.current);
      setConnecting(false);
      return;
    }
    if (typeof EventSource === "undefined") return; // SSR / non-browser guard.

    // Fresh run: reset the fold and the dedupe high-water.
    modelRef.current = emptyRunModel();
    lastSeqRef.current = -1;
    seqModeRef.current = false;
    setModel(modelRef.current);

    let es: EventSource | null = null;
    let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
    let closed = false;
    let done = false;
    let backoff = 1000;
    const maxBackoff = 30_000;

    const url = `/api/v1/${encodeURIComponent(workspaceSlug)}/${encodeURIComponent(
      projectId,
    )}/convergence/runs/${encodeURIComponent(runId)}/events`;

    const publish = () => setModel(snapshot(modelRef.current));

    const connect = () => {
      if (closed) return;
      setConnecting(true);
      es = new EventSource(url, { withCredentials: true });

      es.onopen = () => {
        backoff = 1000;
        setConnecting(false);
        // Backstop for a backend that does not tag frames with a seq: a
        // reconnect replays from seq 0, so rebuild the fold from scratch. Once
        // any frame has carried a seq, dedupe governs replays and we must not
        // reset (that would drop the resume). On the first connect the model is
        // already empty, so this is a no-op then.
        if (!seqModeRef.current) {
          modelRef.current = emptyRunModel();
          lastSeqRef.current = -1;
          publish();
        }
      };

      es.onmessage = (e) => {
        try {
          const ev = JSON.parse(e.data) as ConvergenceEvent;
          const seq = frameSeq(e, ev);
          if (seq !== null) {
            seqModeRef.current = true;
            if (seq <= lastSeqRef.current) return; // already applied (replay).
            lastSeqRef.current = seq;
          }
          applyEvent(modelRef.current, ev);
          publish();
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
        // A transient blip leaves readyState === CONNECTING and EventSource
        // auto-reconnects itself (onopen fires again). Only a hard CLOSED needs
        // a manual backoff reconnect.
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

/**
 * frameSeq extracts a frame's monotonic sequence: a `seq` on the parsed payload
 * if present, else the SSE `id:` surfaced as `lastEventId`. Returns null when
 * neither is available (a backend not yet tagging frames).
 */
function frameSeq(e: MessageEvent, ev: ConvergenceEvent): number | null {
  const raw = (ev as { seq?: unknown }).seq;
  if (typeof raw === "number" && Number.isFinite(raw)) return raw;
  if (e.lastEventId) {
    const n = Number(e.lastEventId);
    if (Number.isFinite(n)) return n;
  }
  return null;
}

/**
 * snapshot returns a fresh model reference (new arrays + row objects) so React
 * re-renders. The accumulator in modelRef is mutated in place by applyEvent;
 * cloning here keeps the value handed to React from tearing under later folds.
 */
function snapshot(m: ConvergenceRunModel): ConvergenceRunModel {
  return {
    ...m,
    passes: m.passes.map((p) => ({ ...p, rows: p.rows.map((r) => ({ ...r })) })),
    // The shared model types logs as optional (a surface may not collect them);
    // the fold always initializes it, so this is a type-level guard, not a case
    // the stream reaches.
    logs: m.logs?.slice() ?? [],
  };
}
