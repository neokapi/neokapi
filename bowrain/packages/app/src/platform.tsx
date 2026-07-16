import { createContext, useContext, type ReactNode } from "react";

/**
 * Host-capability seam for the shared Bowrain app.
 *
 * The same route tree runs in two shells: the browser (`kind: "web"`) and the
 * Wails desktop (`kind: "desktop"`, Phase 2). Anything a route needs from its
 * host that differs between those shells is reached through this adapter rather
 * than a direct `window.*` / analytics call, so the app package never imports
 * shell-specific code.
 *
 * `openExternal` and `analytics` are consumed by the shared routes today. The
 * remaining members are desktop capabilities the Wails shell provides (Phase 2);
 * they are optional so the web shell can omit them and callers must
 * feature-detect before use. `onDeepLink` is the one wired into the router in
 * this package (see BowrainApp); the rest are consumed by components migrated
 * to the seam in later phases.
 */
export interface PlatformAdapter {
  kind: "web" | "desktop";
  /** Open a URL outside the app (a new browser tab, or the OS browser). */
  openExternal(url: string): void;
  /** Product analytics. Backed by PostHog on the web; absent on desktop. */
  analytics?: {
    identify(user: { id: string; email?: string; name?: string }): void;
    reset(): void;
  };
  /** Desktop working-copy connectivity. */
  connectivity?: {
    state(): "connected" | "offline";
    onChange(cb: (s: "connected" | "offline") => void): () => void;
    pendingCount?(): Promise<number>;
    failedCount?(): Promise<number>;
  };
  /** Native window file-drop (paths, not File objects). */
  onFilesDropped?(cb: (paths: string[]) => void): () => void;
  /** Reveal/open a path with the OS default handler. */
  openInOS?(path: string): Promise<void>;
  /** Presence-collaboration session (server URL + keychain token) for Yjs. */
  collabSession?(): Promise<{ serverUrl: string; authToken: string } | null>;
  /**
   * OS deep links (e.g. bowrain://…) normalized by the shell to an in-app route
   * path. BowrainApp subscribes and navigates the router. Returns an
   * unsubscribe.
   */
  onDeepLink?(cb: (path: string) => void): () => void;
  checkForUpdates?(): Promise<void>;
}

/**
 * Web shell implementation. `openExternal` opens a new tab; analytics are wired
 * from a hook the web entry passes in (keeping `posthog-js` out of this package).
 */
export function webPlatform(opts?: { analytics?: PlatformAdapter["analytics"] }): PlatformAdapter {
  return {
    kind: "web",
    openExternal: (url) => {
      window.open(url, "_blank", "noopener,noreferrer");
    },
    analytics: opts?.analytics,
  };
}

const PlatformContext = createContext<PlatformAdapter>(webPlatform());

export function PlatformProvider({
  platform,
  children,
}: {
  platform: PlatformAdapter;
  children: ReactNode;
}) {
  return <PlatformContext.Provider value={platform}>{children}</PlatformContext.Provider>;
}

export function usePlatform(): PlatformAdapter {
  return useContext(PlatformContext);
}
