import { useEffect } from "react";
import type { QueryClient } from "@tanstack/react-query";
import { useQueryClient } from "@tanstack/react-query";
import { useApi } from "@neokapi/ui";

/**
 * The change-event shape relayed by the server's SSE endpoint
 * (GET /api/v1/:ws/events). Mirrors event.ChangeEvent in the Go server.
 */
export interface WorkspaceChangeEvent {
  type: string;
  projectId?: string;
  stream?: string;
  itemName?: string;
  blockId?: string;
  changedBy?: string;
  changeType?: string;
  actor?: string;
}

/**
 * The invalidations one change event should trigger, split by urgency.
 *
 * `immediate` keys refetch right away (cheap, targeted queries — a single
 * project's detail, task lists, member lists). `debounced` keys are the
 * heavier list/aggregate queries (the workspace projects list, the
 * translation dashboard) that a burst of events — e.g. per-block frames
 * during an active sync — must not refetch once per frame; they are coalesced
 * with a trailing debounce by createEventInvalidator.
 */
export interface InvalidationPlan {
  immediate: unknown[][];
  debounced: unknown[][];
}

/**
 * Translate a relayed change event into the query keys to invalidate.
 * Pure — exported for unit testing.
 *
 * Keys mirror those defined across the web routes + queries.ts. We invalidate
 * by prefix (partial key match, React Query's default). When the event carries
 * a projectId, project-scoped keys are narrowed to that project (["project",
 * ws, id] instead of ["project", ws]) so one project's sync does not refetch
 * every open project view.
 */
export function planInvalidations(ws: string, ev: WorkspaceChangeEvent): InvalidationPlan {
  const t = ev.type ?? "";
  const id = ev.projectId;
  // Project-scoped prefix: narrowed to one project when the event names it.
  const projectKey = id ? ["project", ws, id] : ["project", ws];
  const dashboardKey = id ? ["translationDashboard", ws, id] : ["translationDashboard", ws];
  const plan: InvalidationPlan = { immediate: [], debounced: [] };
  const immediate = (...keys: unknown[][]) => plan.immediate.push(...keys);
  const debounced = (...keys: unknown[][]) => plan.debounced.push(...keys);

  // Presence frames ("editor.presence.*") carry cursor/focus signals, not data
  // changes — the web app renders cursors over the Yjs awareness channel, so
  // there is nothing to refetch. Ignore them (they are high-frequency; letting
  // them fall through to the generic branch would thrash every query).
  if (t.startsWith("editor.presence.")) {
    return plan;
  }

  // Block + per-block editor changes → the open project's blocks and the
  // translation dashboard counts. Per-block frames arrive in bursts during
  // syncs and runs, so the dashboard aggregate rides the debounce.
  if (t.startsWith("block.") || t.startsWith("editor.block.")) {
    immediate(projectKey);
    debounced(dashboardKey);
    return plan;
  }

  // Project lifecycle, collection, extraction, quality gate, version, and any
  // other generic change → project list + the project itself.
  if (
    t.startsWith("project.") ||
    t.startsWith("collection.") ||
    t.startsWith("extraction.") ||
    t.startsWith("quality.gate.") ||
    t.startsWith("version.")
  ) {
    immediate(projectKey);
    debounced(["projects", ws], dashboardKey, ["archived-projects", ws]);
    return plan;
  }

  // Item add/remove → project (item list) + list aggregates + dashboard.
  if (t.startsWith("item.")) {
    immediate(projectKey);
    debounced(["projects", ws], dashboardKey);
    return plan;
  }

  // Stream lifecycle → project (stream list lives on the project).
  if (t.startsWith("stream.")) {
    immediate(projectKey);
    debounced(dashboardKey);
    return plan;
  }

  // Tasks → task lists and the server-counted badges beside them.
  if (t.startsWith("task.")) {
    immediate(
      ["tasks", ws],
      ["myTasks", ws],
      ["taskCounts", ws],
      ["myTaskCounts", ws],
      ["activities", ws],
    );
    return plan;
  }

  // Membership → member lists (project + workspace).
  if (t.startsWith("member.")) {
    immediate(["members", ws], projectKey);
    return plan;
  }

  // Brand voice / profile → brand candidates, profiles, drift, scores.
  if (t.startsWith("brand.")) {
    immediate(
      ["brand-candidates", ws],
      ["brand-profiles", ws],
      ["brand-drift", ws],
      ["brand-scores", ws],
    );
    return plan;
  }

  // Connector pull/push/sync → connectors + project content + items.
  if (t.startsWith("connector.")) {
    immediate(["connectors", ws], projectKey, ["activities", ws]);
    debounced(["projects", ws], dashboardKey);
    return plan;
  }

  // Convergence run lifecycle (the bus announces terminal states) → the run
  // lists, the workspace loop rollup (the home's latest-run/ship cards), and
  // the dashboard aggregates a completed run's translations moved. All
  // debounced: a run burst must not refetch once per frame.
  if (t.startsWith("convergence.")) {
    immediate(["activities", ws]);
    debounced(
      id ? ["convergenceRuns", ws, id] : ["convergenceRuns", ws],
      ["loopRollup", ws],
      dashboardKey,
    );
    return plan;
  }

  // Flow / push-automations / source-review → flows + automation runs +
  // project (a flow typically mutates content).
  if (
    t.startsWith("flow.") ||
    t.startsWith("push.automations.") ||
    t.startsWith("source.review.")
  ) {
    immediate(["flows", ws], ["automation-runs", ws], projectKey);
    debounced(dashboardKey);
    return plan;
  }

  // Anything else still bumps the activity feed + audit log so they stay live.
  immediate(["activities", ws], ["auditlog", ws]);
  return plan;
}

/** Trailing debounce interval for the heavy list/aggregate invalidations. */
export const LIST_INVALIDATE_DEBOUNCE_MS = 2000;

/**
 * Stateful executor of InvalidationPlans: immediate keys invalidate at once;
 * debounced keys coalesce per query key with a trailing timer, so a burst of
 * events (an active sync emitting one frame per block) triggers a single
 * list/dashboard refetch ~2s after the burst instead of one per frame.
 * Exported for unit testing; `dispose` cancels pending timers on teardown.
 */
export function createEventInvalidator(
  qc: Pick<QueryClient, "invalidateQueries">,
  ws: string,
  debounceMs: number = LIST_INVALIDATE_DEBOUNCE_MS,
): { handle: (ev: WorkspaceChangeEvent) => void; dispose: () => void } {
  const timers = new Map<string, ReturnType<typeof setTimeout>>();
  let disposed = false;

  const invalidate = (queryKey: unknown[]) => void qc.invalidateQueries({ queryKey });

  const scheduleDebounced = (queryKey: unknown[]) => {
    const slot = JSON.stringify(queryKey);
    const pending = timers.get(slot);
    if (pending) clearTimeout(pending); // trailing: restart on every new event
    timers.set(
      slot,
      setTimeout(() => {
        timers.delete(slot);
        if (!disposed) invalidate(queryKey);
      }, debounceMs),
    );
  };

  return {
    handle: (ev) => {
      if (disposed) return;
      const plan = planInvalidations(ws, ev);
      for (const key of plan.immediate) invalidate(key);
      for (const key of plan.debounced) scheduleDebounced(key);
    },
    dispose: () => {
      disposed = true;
      for (const timer of timers.values()) clearTimeout(timer);
      timers.clear();
    },
  };
}

// ---------------------------------------------------------------------------
// Reconnect / session-refresh policy (pure — exported for unit testing)
// ---------------------------------------------------------------------------

/** Backoff bounds for manual reconnects after a hard-closed stream. */
export const SSE_INITIAL_BACKOFF_MS = 1000;
export const SSE_MAX_BACKOFF_MS = 30_000;

/**
 * How many CONSECUTIVE failed session refreshes stop the reconnect loop.
 *
 * An EventSource cannot see HTTP status codes, so after session expiry the
 * stream just hard-closes on every reconnect (the server 401s) and the old
 * loop churned forever at the backoff cap while live updates were silently
 * dead. The refresh attempt before each reconnect disambiguates: a refresh
 * that keeps failing means the session is gone and re-login is required —
 * nothing a further reconnect can fix — so the loop stops instead of churning.
 * (The web app has no persistent "disconnected" chrome to surface; the next
 * user-driven fetch hits the normal 401 → refresh → login redirect path.)
 * A transient outage (server restart, network blip) refreshes fine, keeps the
 * counter at zero, and retries forever, exactly as before.
 */
export const SSE_MAX_REFRESH_FAILURES = 3;

export interface SseReconnectState {
  /** Delay before the next reconnect attempt (exponential, capped). */
  backoffMs: number;
  /** Consecutive refreshSession() failures since the last success/open. */
  refreshFailures: number;
}

export function initialSseReconnectState(): SseReconnectState {
  return { backoffMs: SSE_INITIAL_BACKOFF_MS, refreshFailures: 0 };
}

export type SseReconnectPlan =
  | { action: "retry"; delayMs: number; next: SseReconnectState }
  | { action: "stop" };

/**
 * Decide what to do after the stream hard-closed: wait the current backoff and
 * try again (refresh first), or stop once the refresh budget is exhausted.
 */
export function sseClosed(s: SseReconnectState): SseReconnectPlan {
  if (s.refreshFailures >= SSE_MAX_REFRESH_FAILURES) return { action: "stop" };
  return {
    action: "retry",
    delayMs: s.backoffMs,
    next: { ...s, backoffMs: Math.min(s.backoffMs * 2, SSE_MAX_BACKOFF_MS) },
  };
}

/**
 * Fold the result of the pre-reconnect session refresh into the state: success
 * clears the failure streak (whatever closed the stream, it was not a dead
 * session), failure consumes one refresh attempt.
 */
export function sseRefreshResult(s: SseReconnectState, ok: boolean): SseReconnectState {
  return ok ? { ...s, refreshFailures: 0 } : { ...s, refreshFailures: s.refreshFailures + 1 };
}

/** True once the refresh budget is spent — do not reconnect again. */
export function sseShouldStop(s: SseReconnectState): boolean {
  return s.refreshFailures >= SSE_MAX_REFRESH_FAILURES;
}

/** A successful connection resets both the backoff and the refresh budget. */
export function sseOpened(): SseReconnectState {
  return initialSseReconnectState();
}

/**
 * Subscribe to the workspace's unified change-event stream and invalidate the
 * relevant React Query caches so no view shows stale state when content changes
 * from outside it (another user, a kapi push, a connector sync, an automation
 * or flow completion, a stream/member/brand/term change).
 *
 * Opens an EventSource to /api/v1/:ws/events (optionally scoped to one project),
 * reconnects with backoff on drop, and tears down on unmount or workspace
 * change. Same-origin cookies authenticate the stream. Because an EventSource
 * can neither send headers nor observe a 401, every manual reconnect is
 * preceded by a session refresh through the adapter's normal auth path
 * (ApiAdapter.refreshSession); bounded consecutive refresh failures stop the
 * loop (see SSE_MAX_REFRESH_FAILURES). The Yjs collab WebSocket keeps handling
 * per-cursor presence — this layer is purely about data freshness.
 */
export function useWorkspaceEvents(workspaceSlug: string | undefined, projectId?: string): void {
  const queryClient = useQueryClient();
  const api = useApi();

  useEffect(() => {
    if (!workspaceSlug) return;
    if (typeof EventSource === "undefined") return; // SSR / non-browser guard.

    let es: EventSource | null = null;
    let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
    let closed = false;
    let state = initialSseReconnectState();
    const invalidator = createEventInvalidator(queryClient, workspaceSlug);

    const url = projectId
      ? `/api/v1/${encodeURIComponent(workspaceSlug)}/events?project=${encodeURIComponent(projectId)}`
      : `/api/v1/${encodeURIComponent(workspaceSlug)}/events`;

    // Refresh the cookie/token session through the adapter's normal auth path.
    // An adapter without the capability (desktop Bearer transport) has nothing
    // to refresh — treat as fine so reconnect behavior is unchanged there.
    const refreshSession = async (): Promise<boolean> => {
      const attempt = api.refreshSession?.();
      if (!attempt) return true;
      try {
        return await attempt;
      } catch {
        return false;
      }
    };

    const connect = () => {
      if (closed) return;
      es = new EventSource(url, { withCredentials: true });

      es.addEventListener("change", (e) => {
        try {
          const ev = JSON.parse((e as MessageEvent).data) as WorkspaceChangeEvent;
          invalidator.handle(ev);
        } catch {
          /* ignore malformed frames */
        }
      });

      es.onopen = () => {
        state = sseOpened(); // reset backoff + refresh budget.
      };

      es.onerror = () => {
        // EventSource auto-reconnects on transient errors, but if the
        // connection is closed (server restart, auth rejection) we reconnect
        // manually with backoff to avoid a tight loop.
        if (es && es.readyState === EventSource.CLOSED) {
          es.close();
          es = null;
          if (closed) return;
          const plan = sseClosed(state);
          if (plan.action === "stop") return; // dead session: stop churning.
          state = plan.next;
          reconnectTimer = setTimeout(() => {
            void (async () => {
              // Session refresh BEFORE reconnecting: the EventSource itself
              // cannot recover from an expired session (it can't see the 401,
              // let alone fix it), but the normal fetch-path refresh can.
              state = sseRefreshResult(state, await refreshSession());
              if (closed || sseShouldStop(state)) return;
              connect();
            })();
          }, plan.delayMs);
        }
      };
    };

    connect();

    return () => {
      closed = true;
      if (reconnectTimer) clearTimeout(reconnectTimer);
      invalidator.dispose();
      es?.close();
    };
  }, [workspaceSlug, projectId, queryClient, api]);
}
