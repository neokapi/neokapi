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
 * Translate a relayed change event into the set of React Query keys to
 * invalidate so every affected view refetches. Exported for unit testing.
 *
 * Keys mirror those defined across the web routes + queries.ts. We invalidate
 * by prefix (partial key match, React Query's default) so all streams/variants
 * of an entity refresh, not just one.
 */
export function invalidateForEvent(
  qc: Pick<QueryClient, "invalidateQueries">,
  ws: string,
  ev: WorkspaceChangeEvent,
): void {
  const t = ev.type ?? "";
  const invalidate = (queryKey: unknown[]) => void qc.invalidateQueries({ queryKey });

  // Block + per-block editor changes → the open project's blocks and the
  // translation dashboard counts.
  if (t.startsWith("block.") || t.startsWith("editor.block.")) {
    invalidate(["project", ws]);
    invalidate(["translationDashboard", ws]);
    return;
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
    invalidate(["projects", ws]);
    invalidate(["project", ws]);
    invalidate(["translationDashboard", ws]);
    invalidate(["archived-projects", ws]);
    return;
  }

  // Item add/remove → project (item list) + dashboard.
  if (t.startsWith("item.")) {
    invalidate(["project", ws]);
    invalidate(["projects", ws]);
    invalidate(["translationDashboard", ws]);
    return;
  }

  // Stream lifecycle → project (stream list lives on the project).
  if (t.startsWith("stream.")) {
    invalidate(["project", ws]);
    invalidate(["translationDashboard", ws]);
    return;
  }

  // Tasks → task lists.
  if (t.startsWith("task.")) {
    invalidate(["tasks", ws]);
    invalidate(["myTasks", ws]);
    invalidate(["activities", ws]);
    return;
  }

  // Membership → member lists (project + workspace).
  if (t.startsWith("member.")) {
    invalidate(["members", ws]);
    invalidate(["project", ws]);
    return;
  }

  // Brand voice / profile → brand candidates, profiles, drift, scores.
  if (t.startsWith("brand.")) {
    invalidate(["brand-candidates", ws]);
    invalidate(["brand-profiles", ws]);
    invalidate(["brand-drift", ws]);
    invalidate(["brand-scores", ws]);
    return;
  }

  // Connector pull/push/sync → connectors + project content + items.
  if (t.startsWith("connector.")) {
    invalidate(["connectors", ws]);
    invalidate(["project", ws]);
    invalidate(["projects", ws]);
    invalidate(["translationDashboard", ws]);
    invalidate(["activities", ws]);
    return;
  }

  // Flow / push-automations / source-review → flows + automation runs +
  // project (a flow typically mutates content).
  if (
    t.startsWith("flow.") ||
    t.startsWith("push.automations.") ||
    t.startsWith("source.review.")
  ) {
    invalidate(["flows", ws]);
    invalidate(["automation-runs", ws]);
    invalidate(["project", ws]);
    invalidate(["translationDashboard", ws]);
    return;
  }

  // Anything else still bumps the activity feed + audit log so they stay live.
  invalidate(["activities", ws]);
  invalidate(["auditlog", ws]);
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

    const url = projectId
      ? `/api/v1/${encodeURIComponent(workspaceSlug)}/events?project=${encodeURIComponent(projectId)}`
      : `/api/v1/${encodeURIComponent(workspaceSlug)}/events`;

    const connect = () => {
      if (closed) return;
      es = new EventSource(url, { withCredentials: true });

      es.addEventListener("change", (e) => {
        try {
          const ev = JSON.parse((e as MessageEvent).data) as WorkspaceChangeEvent;
          invalidateForEvent(queryClient, workspaceSlug, ev);
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
      es?.close();
    };
  }, [workspaceSlug, projectId, queryClient]);
}
