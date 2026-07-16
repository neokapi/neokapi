import { Browser, Events } from "@wailsio/runtime";
import type { PlatformAdapter } from "@neokapi/bowrain-app";
import { Backend } from "./backend";

/**
 * Desktop implementation of the shared app's host-capability seam
 * (`PlatformAdapter`). Every capability is backed by a Wails binding or runtime
 * call:
 *
 *   - openExternal   → Browser.OpenURL (OS default browser)
 *   - connectivity   → GetConnectionState + the connection-state-changed event,
 *                      plus the pending/failed offline-queue counts
 *   - onFilesDropped → the native window's files-dropped event (paths)
 *   - openInOS       → OpenFileInOS
 *   - collabSession  → GetCollabSession (server URL + keychain token for Yjs)
 *   - onDeepLink     → the deep-link-project event (bowrain://…), normalized to
 *                      an in-app route the router can navigate to
 *
 * `checkForUpdates` is intentionally absent: the desktop ships the Wails native
 * updater (backend InitUpdater, periodic background check) with no
 * frontend-callable binding, and the seam says not to invent one.
 *
 * `analytics` is absent too — the desktop does not run PostHog.
 */
export function createDesktopPlatform(): PlatformAdapter {
  // connectivity.state() is synchronous, but GetConnectionState is a binding
  // call. Keep a live cached value: prime it once, then follow the
  // connection-state-changed event for the app's lifetime.
  let lastState: "connected" | "offline" = "offline";
  const mapState = (s: string | undefined): "connected" | "offline" =>
    s === "connected" ? "connected" : "offline";

  try {
    void (Backend.GetConnectionState() as Promise<{ state?: string }>)
      .then((ci) => {
        lastState = mapState(ci?.state);
      })
      .catch(() => {});
    Events.On("connection-state-changed", (event: { data: unknown }) => {
      lastState = mapState((event.data as { state?: string } | undefined)?.state);
    });
  } catch {
    // No Wails runtime (e.g. a non-desktop harness) — stay "offline".
  }

  return {
    kind: "desktop",
    openExternal: (url) => {
      void Browser.OpenURL(url);
    },
    connectivity: {
      state: () => lastState,
      onChange: (cb) => {
        const cancel = Events.On("connection-state-changed", (event: { data: unknown }) => {
          cb(mapState((event.data as { state?: string } | undefined)?.state));
        });
        return () => cancel?.();
      },
      pendingCount: () => Backend.GetPendingChangesCount() as Promise<number>,
      failedCount: () => Backend.GetFailedChangesCount() as Promise<number>,
    },
    onFilesDropped: (cb) => {
      const cancel = Events.On("files-dropped", (event: { data: unknown }) => {
        cb((event.data as string[]) ?? []);
      });
      return () => cancel?.();
    },
    openInOS: (path) => Backend.OpenFileInOS(path) as Promise<void>,
    collabSession: async () => {
      try {
        const s = (await Backend.GetCollabSession()) as {
          serverUrl: string;
          authToken: string;
        } | null;
        return s ?? null;
      } catch {
        return null;
      }
    },
    onDeepLink: (cb) => {
      const cancel = Events.On("deep-link-project", (event: { data: unknown }) => {
        const info = event.data as { project_id?: string; workspace?: string } | undefined;
        if (!info?.project_id) return;
        const ws = info.workspace || "personal";
        cb(`/${ws}/p/${info.project_id}/s/main`);
      });
      return () => cancel?.();
    },
  };
}
