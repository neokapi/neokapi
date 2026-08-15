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
  /**
   * Product analytics, backed by PostHog. The web shell always provides it
   * (key-gated); the desktop shell provides it gated on the persisted
   * telemetry setting (opt-out with first-run notice, decision D1). Call
   * sites must optional-chain (`platform.analytics?.capture(...)`). Event
   * names live in the shared `AnalyticsEvents` module (@neokapi/ui) — never
   * inline literals — and props carry ids/enum-ish values only.
   */
  analytics?: {
    identify(user: { id: string; email?: string; name?: string }): void;
    reset(): void;
    /**
     * Fire-and-forget product event ($pageview, feature_entered, …).
     *
     * `transport: "beacon"` is for the events a page fires on its way out. The
     * default transport batches, and a batch queued microseconds before a
     * top-level navigation is discarded with the document — which is exactly
     * the step worth measuring, since it is where a visitor leaves for the
     * identity provider. A shell that cannot honour it ignores the option and
     * still records the event.
     */
    capture(
      event: string,
      props?: Record<string, unknown>,
      options?: { transport?: "beacon" },
    ): void;
    /** Associate the session with a PostHog group (e.g. "workspace"). */
    group(type: string, key: string, props?: Record<string, unknown>): void;
    /**
     * Local-client telemetry opt-out control (desktop shells only; absent on
     * the web, where the privacy policy covers the cloud app). When present,
     * the settings page renders a usage-statistics toggle.
     */
    optOut?: {
      enabled(): boolean;
      setEnabled(enabled: boolean): void;
    };
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
   * Base URL of the connected server's web app. The desktop hosts the same
   * routes in a webview, but a few flows (WebAuthn passkeys, OIDC step-up) only
   * work at a real browser origin; a route uses this to `openExternal` the
   * equivalent page in the browser. Resolves null when not connected.
   */
  webBaseUrl?(): Promise<string | null>;
  /**
   * Sign out at the host level. The web app's default sign-out is an OIDC
   * cookie/redirect round-trip that has no meaning in the desktop webview
   * (Bearer-token auth, no browser origin); when present, the shared app calls
   * this instead. The desktop clears the keychain token and disconnects, which
   * returns the connection gate to the ServerConnect screen.
   */
  signOut?(): Promise<void>;
  /**
   * Watch a project for backend content-change events (desktop only). The web
   * shell reads freshness from a same-origin SSE EventSource; the desktop cannot
   * (keychain-Bearer auth over Wails), so the Go backend runs the SSE→Wails-events
   * watcher instead. Each change is normalized to the same `{ type }` the web SSE
   * relay emits, so the caller invalidates React Query identically. A reconnect
   * after an offline gap is reported as `{ type: "reconnected" }`. Returns an
   * unsubscribe that stops the watcher. Web omits this member.
   */
  watchProject?(
    projectId: string | undefined,
    onChange: (event: { type: string }) => void,
  ): () => void;
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
