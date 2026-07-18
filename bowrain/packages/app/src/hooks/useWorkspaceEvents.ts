import { useEffect } from "react";
import type { QueryClient } from "@tanstack/react-query";
import { useQueryClient } from "@tanstack/react-query";

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

  // Tasks → task lists.
  if (t.startsWith("task.")) {
    immediate(["tasks", ws], ["myTasks", ws], ["activities", ws]);
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

/**
 * Subscribe to the workspace's unified change-event stream and invalidate the
 * relevant React Query caches so no view shows stale state when content changes
 * from outside it (another user, a kapi push, a connector sync, an automation
 * or flow completion, a stream/member/brand/term change).
 *
 * Opens an EventSource to /api/v1/:ws/events (optionally scoped to one project),
 * reconnects with backoff on drop, and tears down on unmount or workspace
 * change. Same-origin cookies authenticate the stream. The Yjs collab WebSocket
 * keeps handling per-cursor presence — this layer is purely about data
 * freshness.
 */
export function useWorkspaceEvents(workspaceSlug: string | undefined, projectId?: string): void {
  const queryClient = useQueryClient();

  useEffect(() => {
    if (!workspaceSlug) return;
    if (typeof EventSource === "undefined") return; // SSR / non-browser guard.

    let es: EventSource | null = null;
    let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
    let closed = false;
    let backoff = 1000;
    const maxBackoff = 30_000;
    const invalidator = createEventInvalidator(queryClient, workspaceSlug);

    const url = projectId
      ? `/api/v1/${encodeURIComponent(workspaceSlug)}/events?project=${encodeURIComponent(projectId)}`
      : `/api/v1/${encodeURIComponent(workspaceSlug)}/events`;

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
        backoff = 1000; // reset backoff on a successful connection.
      };

      es.onerror = () => {
        // EventSource auto-reconnects on transient errors, but if the
        // connection is closed (e.g. server restart) we reconnect manually
        // with backoff to avoid a tight loop.
        if (es && es.readyState === EventSource.CLOSED) {
          es.close();
          es = null;
          if (closed) return;
          reconnectTimer = setTimeout(connect, backoff);
          backoff = Math.min(backoff * 2, maxBackoff);
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
  }, [workspaceSlug, projectId, queryClient]);
}
