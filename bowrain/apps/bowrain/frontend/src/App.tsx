import { useState, useCallback, useEffect, useRef } from "react";
import { ThemeProvider } from "@neokapi/ui";
import { BowrainApp } from "@neokapi/bowrain-app";
import { createHashHistory } from "@tanstack/react-router";
import { QueryClientProvider } from "@tanstack/react-query";
import { Loader2 } from "lucide-react";
import { Events } from "@wailsio/runtime";
import { ServerConnect } from "./components/ServerConnect";
import { OfflineLaunch } from "./components/OfflineLaunch";
import { useConnection } from "./hooks/useApi";
import { createDesktopAdapter } from "./api/desktopAdapter";
import { createDesktopPlatform } from "./api/desktopPlatform";
import { queryClient } from "./lib/queryClient";

// "offline" is the designed launch state (#1284): the server is configured but
// unreachable, distinct from "connecting" (no/expired session → sign in).
type AppMode = "loading" | "connecting" | "offline" | "ready";

// One adapter + platform + history for the app's lifetime.
//
// The composite adapter runs the shared RestApiAdapter over a Wails ProxyRequest
// transport (real server, keychain auth), with the desktop's local-first
// surfaces served by bindings. It reports the server's real config, so the
// shared router runs in server mode against the connected server — the real
// server auth is the ServerConnect gate below.
const desktopAdapter = createDesktopAdapter();
const desktopPlatform = createDesktopPlatform();
// Hash history: the Wails webview serves the frontend from a static asset root,
// so browser history would 404 on refresh or deep navigation. Hash keeps every
// route change client-side.
const desktopHistory = createHashHistory();

/**
 * Desktop entry. A thin connection gate (loading spinner → ServerConnect PKCE
 * screen) precedes the shared @neokapi/bowrain-app, which mounts once the
 * backend reports a connected (or offline) working copy. Phase 2 of the
 * web/desktop unification (#1273): the desktop now renders the identical shared
 * route tree the web app does, over a WailsApiAdapter + desktop PlatformAdapter.
 */
function AppInner() {
  const connection = useConnection();
  const [mode, setMode] = useState<AppMode>("loading");
  // Mirror mode for async guards: a stale probe finishing after a user action
  // (e.g. "Use a different server") must not override the new state.
  const modeRef = useRef<AppMode>("loading");
  useEffect(() => {
    modeRef.current = mode;
  }, [mode]);

  // The shared app reads projects via the adapter, which proxies the backend's
  // currently-selected server workspace. Make sure one is selected so those
  // reads resolve when the connection didn't pin a workspace itself.
  const ensureWorkspaceSelected = useCallback(
    async (ci: { workspace?: string }) => {
      if (ci.workspace) return;
      try {
        const wsList = await connection.getServerWorkspaces();
        if (wsList.length > 0) {
          await connection.selectWorkspace(wsList[0].slug);
        }
      } catch {
        /* offline or single-tenant — the adapter still resolves locally */
      }
    },
    [connection],
  );

  // The backend reports "connected" optimistically from a stored token without a
  // round-trip, so a launch probe is what tells apart three outcomes:
  //   "app"     — reachable and authenticated (getConfig ok, getCurrentUser ok)
  //   "signin"  — reachable but the session is rejected (getConfig is public, so
  //               getCurrentUser returning null is the real signal)
  //   "offline" — the server is unreachable at all
  // getConfig seeds the shared config cache so the router's own first getConfig
  // reuses it (no second round-trip, no race); getCurrentUser seeds the user.
  const probeLaunch = useCallback(async (): Promise<"app" | "signin" | "offline"> => {
    let config;
    try {
      config = await desktopAdapter.getConfig();
    } catch {
      // Unreachable / transport failure — stay in the calm offline state.
      return "offline";
    }
    queryClient.setQueryData(["config"], config);
    // Reachable — is the stored session still valid? getConfig is unauthenticated,
    // so this is what distinguishes a rejected token (sign-in) from connected.
    const user = await desktopAdapter.getCurrentUser();
    if (!user) return "signin";
    queryClient.setQueryData(["currentUser"], user);
    return "app";
  }, []);

  // Act on a launch probe: enter the app, drop the dead session to sign-in, or
  // fall into the offline-launch state. Returns true once it has left the probing
  // state (so the offline retry loop can stop); false means "still offline".
  const applyProbeOutcome = useCallback(
    async (ci: { workspace?: string }): Promise<boolean> => {
      const outcome = await probeLaunch();
      // A concurrent user action may have moved us out of the launch/offline flow
      // (e.g. "Use a different server" → sign-in) while this probe was in flight —
      // don't let the stale result override it.
      if (modeRef.current !== "loading" && modeRef.current !== "offline") return true;
      if (outcome === "app") {
        await ensureWorkspaceSelected(ci);
        setMode("ready");
        return true;
      }
      if (outcome === "signin") {
        await connection.logout(); // clear the rejected token so sign-in starts clean
        setMode("connecting");
        return true;
      }
      setMode("offline");
      return false;
    },
    [probeLaunch, ensureWorkspaceSelected, connection],
  );

  const handleServerConnect = useCallback(
    async (serverURL: string) => {
      const ci = await connection.connect(serverURL);
      if (ci.state === "connected") {
        await ensureWorkspaceSelected(ci);
        setMode("ready");
      }
      return ci;
    },
    [connection, ensureWorkspaceSelected],
  );

  // Re-probe from the offline-launch state (auto-retry + manual "Try again").
  // Reachable → enter the app; a rejected session → sign-in. Either way the
  // retry loop stops (true); false keeps the offline state waiting.
  const handleOfflineRetry = useCallback(async (): Promise<boolean> => {
    const ci = await connection.refresh();
    if (ci.state === "disconnected") {
      setMode("connecting");
      return true;
    }
    return applyProbeOutcome(ci);
  }, [connection, applyProbeOutcome]);

  // "Use a different server": drop the (unreachable) connection so the optimistic
  // "connected" state can't auto-promote, then show the ServerConnect form.
  const handleUseDifferentServer = useCallback(async () => {
    await connection.disconnect();
    setMode("connecting");
  }, [connection]);

  // A rejected/expired session (sign-out, or a 401 the adapter could not refresh)
  // disconnects the backend — route that to ServerConnect sign-in, whether from
  // the app, the offline-launch state, or launch itself. A mid-session network
  // drop reports "offline" (a valid cached working copy) and stays in the app.
  useEffect(() => {
    const cancel = Events.On("connection-state-changed", () => {
      void connection.refresh().then((ci) => {
        if (ci.state === "disconnected") setMode("connecting");
      });
    });
    return () => cancel?.();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Entering the app always goes through a reachability probe (launch effect,
  // handleServerConnect, handleOfflineRetry) — never a bare "connected" state,
  // which is optimistic and would mount against an unreachable server.

  // Demote out of the mounted app when the session drops — the desktop sign-out
  // (platform.signOut → Logout) disconnects, returning the user to ServerConnect.
  // A transient network blip reports "offline" (still a valid working copy), so
  // only a true disconnect/unauthenticated state demotes.
  useEffect(() => {
    if (
      mode === "ready" &&
      connection.info.state !== "connected" &&
      connection.info.state !== "offline"
    ) {
      setMode("connecting");
    }
  }, [connection.info.state, mode]);

  // Launch: reuse a stored/auto session if present. "connected" is optimistic, so
  // verify reachability — reachable enters the app, unreachable shows the
  // offline-launch state. No/expired session shows sign-in.
  useEffect(() => {
    let aborted = false;
    connection
      .refresh()
      .then(async (ci) => {
        if (aborted) return;
        if (ci.state === "connected") {
          if (!aborted) await applyProbeOutcome(ci);
        } else if (ci.state === "offline") {
          // Backend already holds a mid-session offline working copy — mount it.
          await ensureWorkspaceSelected(ci);
          if (!aborted) setMode("ready");
        } else if ((window as { __skipConnection?: boolean }).__skipConnection) {
          setMode("ready");
        } else {
          setMode("connecting");
        }
      })
      .catch(() => {
        if (!aborted) {
          setMode(
            (window as { __skipConnection?: boolean }).__skipConnection ? "ready" : "connecting",
          );
        }
      });
    return () => {
      aborted = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  if (mode === "loading") {
    return (
      <ThemeProvider>
        <div className="flex items-center justify-center h-screen bg-background">
          <Loader2 className="w-8 h-8 animate-spin text-muted-foreground" />
        </div>
      </ThemeProvider>
    );
  }

  if (mode === "connecting") {
    return (
      <ThemeProvider>
        <div className="h-screen bg-background flex flex-col">
          <div
            className="h-10 shrink-0"
            style={{
              // @ts-expect-error non-standard CSS property for Wails
              "--wails-draggable": "drag",
            }}
          />
          <ServerConnect
            info={connection.info}
            onConnect={handleServerConnect}
            onStartLogin={connection.startLogin}
            onWaitForLogin={connection.waitForLogin}
            onCancelLogin={connection.cancelLogin}
          />
        </div>
      </ThemeProvider>
    );
  }

  if (mode === "offline") {
    return (
      <ThemeProvider>
        <div className="h-screen bg-background flex flex-col">
          <div
            className="h-10 shrink-0"
            style={{
              // @ts-expect-error non-standard CSS property for Wails
              "--wails-draggable": "drag",
            }}
          />
          <OfflineLaunch
            serverURL={connection.info.server_url || ""}
            onRetry={handleOfflineRetry}
            onUseDifferentServer={handleUseDifferentServer}
          />
        </div>
      </ThemeProvider>
    );
  }

  // Connected (or offline working copy): mount the shared app. It owns theme,
  // providers, and routing; the desktop passes the Wails data + platform seams
  // and a hash history.
  return (
    <BowrainApp
      api={desktopAdapter}
      platform={desktopPlatform}
      queryClient={queryClient}
      history={desktopHistory}
    />
  );
}

/**
 * App root. Provides the app-wide react-query client so the connection gate and
 * the shared app share one cache; the shared app's RootLayout re-provides the
 * same client, so invalidations are visible across both.
 */
function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <AppInner />
    </QueryClientProvider>
  );
}

export default App;
