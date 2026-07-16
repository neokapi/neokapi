import { useState, useCallback, useEffect } from "react";
import { ThemeProvider } from "@neokapi/ui";
import { BowrainApp } from "@neokapi/bowrain-app";
import { createHashHistory } from "@tanstack/react-router";
import { QueryClientProvider } from "@tanstack/react-query";
import { Loader2 } from "lucide-react";
import { Events } from "@wailsio/runtime";
import { ServerConnect } from "./components/ServerConnect";
import { useConnection } from "./hooks/useApi";
import { createDesktopAdapter } from "./api/desktopAdapter";
import { createDesktopPlatform } from "./api/desktopPlatform";
import { queryClient } from "./lib/queryClient";

type AppMode = "loading" | "connecting" | "ready";

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

  // Keep the gate responsive to backend connection changes (the auto-connect
  // race, where the first refresh() returns disconnected): refresh the snapshot
  // whenever the backend reports a change.
  useEffect(() => {
    const cancel = Events.On("connection-state-changed", () => {
      void connection.refresh();
    });
    return () => cancel?.();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Promote to the app once the backend reports connected.
  useEffect(() => {
    if (connection.info.state === "connected" && mode === "connecting") {
      setMode("ready");
    }
  }, [connection.info.state, mode]);

  // Demote back to the gate when the session drops after mounting — the desktop
  // sign-out (platform.signOut → Logout) disconnects, and this returns the user
  // to ServerConnect. A transient network blip reports "offline" (still a valid
  // working copy), so only a true disconnect/unauthenticated state demotes.
  useEffect(() => {
    if (
      mode === "ready" &&
      connection.info.state !== "connected" &&
      connection.info.state !== "offline"
    ) {
      setMode("connecting");
    }
  }, [connection.info.state, mode]);

  // Initial probe: reuse a stored/auto session if present, else show the gate.
  useEffect(() => {
    connection
      .refresh()
      .then(async (ci) => {
        if (ci.state === "connected" || ci.state === "offline") {
          await ensureWorkspaceSelected(ci);
          setMode("ready");
        } else if ((window as { __skipConnection?: boolean }).__skipConnection) {
          setMode("ready");
        } else {
          setMode("connecting");
        }
      })
      .catch(() => {
        setMode(
          (window as { __skipConnection?: boolean }).__skipConnection ? "ready" : "connecting",
        );
      });
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
