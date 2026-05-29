import { useCallback, useEffect, useState } from "react";
import { CloudUpload, Loader2, CheckCircle2, ExternalLink, Unplug } from "lucide-react";
import { Button, Input, Label, Badge } from "@neokapi/ui-primitives";
import { t } from "@neokapi/kapi-react/runtime";
import type { BowrainConnection, ConnectBowrainResult, PublishBowrainResult } from "../types/api";

/**
 * Phases of the optional one-way "Connect / Publish to Bowrain" flow.
 *
 * idle        → not connected; collecting the server URL
 * connecting  → browser OAuth in progress (server-brokered OIDC + PKCE)
 * connected   → authenticated + server: block written; ready to publish
 * publishing  → creating/claiming the server-side project
 * published   → project provisioned; ongoing sync delegated to `kapi sync`
 */
export type ConnectPhase = "idle" | "connecting" | "connected" | "publishing" | "published";

export interface ConnectBowrainDialogProps {
  /** Project tab ID to connect. */
  tabID: string;
  /** Whether the project has been saved to disk (required before connecting). */
  saved: boolean;
  onClose: () => void;
  /** Injectable API surface (defaults to the real Wails bindings). */
  api?: ConnectApi;
}

/** The minimal API surface this dialog needs — easy to mock in tests. */
export interface ConnectApi {
  getBowrainConnection: (tabID: string) => Promise<BowrainConnection | null>;
  connectBowrain: (tabID: string, serverURL: string) => Promise<ConnectBowrainResult | null>;
  publishBowrain: (tabID: string) => Promise<PublishBowrainResult | null>;
  disconnectBowrain: (tabID: string) => Promise<void | null>;
}

const DEFAULT_SERVER = "https://bowrain.cloud";

export function ConnectBowrainDialog({
  tabID,
  saved,
  onClose,
  api: injected,
}: ConnectBowrainDialogProps) {
  // Lazy-load the real bindings only when no api is injected (keeps tests
  // free of the Wails runtime import).
  const [api, setApi] = useState<ConnectApi | undefined>(injected);
  useEffect(() => {
    if (injected) return;
    let cancelled = false;
    void import("../hooks/useApi").then((m) => {
      if (!cancelled) setApi(m.api as unknown as ConnectApi);
    });
    return () => {
      cancelled = true;
    };
  }, [injected]);

  const [phase, setPhase] = useState<ConnectPhase>("idle");
  const [serverURL, setServerURL] = useState(DEFAULT_SERVER);
  const [conn, setConn] = useState<BowrainConnection | null>(null);
  const [publishResult, setPublishResult] = useState<PublishBowrainResult | null>(null);
  const [error, setError] = useState<string | null>(null);

  // Load the current connection on open.
  useEffect(() => {
    if (!api) return;
    void api.getBowrainConnection(tabID).then((c) => {
      if (!c) return;
      setConn(c);
      if (c.server_url) setServerURL(c.server_url);
      if (c.connected && c.authenticated) setPhase("connected");
    });
  }, [api, tabID]);

  const handleConnect = useCallback(async () => {
    if (!api) return;
    setError(null);
    setPhase("connecting");
    try {
      const res = await api.connectBowrain(tabID, serverURL);
      if (!res) throw new Error(t("Connect is only available in the desktop app."));
      const fresh = await api.getBowrainConnection(tabID);
      setConn(fresh);
      setPhase("connected");
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
      setPhase("idle");
    }
  }, [api, tabID, serverURL]);

  const handlePublish = useCallback(async () => {
    if (!api) return;
    setError(null);
    setPhase("publishing");
    try {
      const res = await api.publishBowrain(tabID);
      if (!res) throw new Error(t("Publish is only available in the desktop app."));
      setPublishResult(res);
      const fresh = await api.getBowrainConnection(tabID);
      setConn(fresh);
      setPhase("published");
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
      setPhase("connected");
    }
  }, [api, tabID]);

  const handleDisconnect = useCallback(async () => {
    if (!api) return;
    setError(null);
    try {
      await api.disconnectBowrain(tabID);
      setConn(null);
      setPublishResult(null);
      setPhase("idle");
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  }, [api, tabID]);

  const busy = phase === "connecting" || phase === "publishing";

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
      role="dialog"
      aria-modal="true"
      aria-label={t("Connect to Bowrain")}
    >
      <div className="w-full max-w-md rounded-xl border border-border bg-background p-6 shadow-lg">
        <div className="mb-4 flex items-center gap-2">
          <CloudUpload size={18} className="text-primary" />
          <h2 className="text-lg font-semibold">{t("Connect to Bowrain")}</h2>
        </div>

        <p className="mb-4 text-sm text-muted-foreground">
          {t(
            "Publish this local project up to a Bowrain server — like adding a git remote. This is one-way: ongoing collaboration happens in the Bowrain apps.",
          )}
        </p>

        {error && (
          <div className="mb-3 rounded-md border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive">
            {error}
          </div>
        )}

        {/* Step 1: server + authenticate */}
        {(phase === "idle" || phase === "connecting") && (
          <div className="space-y-3">
            <div>
              <Label htmlFor="bowrain-server" className="mb-1 block text-xs text-muted-foreground">
                {t("Bowrain server")}
              </Label>
              <Input
                id="bowrain-server"
                type="url"
                value={serverURL}
                onChange={(e: React.ChangeEvent<HTMLInputElement>) => setServerURL(e.target.value)}
                placeholder="https://bowrain.cloud"
                disabled={busy}
                autoFocus
              />
            </div>
            {!saved && (
              <p className="text-xs text-amber-600 dark:text-amber-500">
                {t("Save the project to disk before connecting.")}
              </p>
            )}
            <div className="flex gap-2">
              <Button
                onClick={handleConnect}
                disabled={busy || !saved || !serverURL.trim()}
                className="flex-1"
              >
                {phase === "connecting" ? (
                  <>
                    <Loader2 size={14} className="animate-spin" />
                    {t("Authenticating in browser…")}
                  </>
                ) : (
                  t("Connect & sign in")
                )}
              </Button>
              <Button variant="outline" onClick={onClose} disabled={busy}>
                {t("Cancel")}
              </Button>
            </div>
            {phase === "connecting" && (
              <p className="text-xs text-muted-foreground">
                {t("A browser window opened for sign-in. Return here when done.")}
              </p>
            )}
          </div>
        )}

        {/* Step 2: connected — ready to publish */}
        {(phase === "connected" || phase === "publishing") && conn && (
          <div className="space-y-3">
            <div className="flex items-center gap-2 text-sm">
              <CheckCircle2 size={16} className="text-green-600 dark:text-green-500" />
              <span>
                {t("Signed in to")} <Badge variant="secondary">{conn.server_url}</Badge>
              </span>
            </div>
            {conn.user_email && <p className="text-xs text-muted-foreground">{conn.user_email}</p>}
            <p className="text-sm text-muted-foreground">
              {t(
                "Publish creates the project on the server and links your recipe. Content is pushed afterwards with `kapi sync`.",
              )}
            </p>
            <div className="flex gap-2">
              <Button onClick={handlePublish} disabled={busy} className="flex-1">
                {phase === "publishing" ? (
                  <>
                    <Loader2 size={14} className="animate-spin" />
                    {t("Publishing…")}
                  </>
                ) : (
                  <>
                    <CloudUpload size={14} />
                    {t("Publish")}
                  </>
                )}
              </Button>
              <Button variant="outline" onClick={onClose} disabled={busy}>
                {t("Done")}
              </Button>
            </div>
            <Button
              variant="ghost"
              size="sm"
              onClick={handleDisconnect}
              disabled={busy}
              className="text-muted-foreground"
            >
              <Unplug size={14} />
              {t("Disconnect")}
            </Button>
          </div>
        )}

        {/* Step 3: published */}
        {phase === "published" && publishResult && (
          <div className="space-y-3">
            <div className="flex items-center gap-2 text-sm">
              <CheckCircle2 size={16} className="text-green-600 dark:text-green-500" />
              <span>{t("Published to Bowrain")}</span>
            </div>
            {publishResult.project_url && (
              <p className="break-all text-xs text-muted-foreground" translate="no">
                {publishResult.project_url}
              </p>
            )}
            <div className="rounded-md border border-border bg-muted/40 px-3 py-2 text-sm">
              <div className="flex items-start gap-2">
                <ExternalLink size={14} className="mt-0.5 shrink-0 text-muted-foreground" />
                <span>{publishResult.sync_hint || t("Push content with `kapi sync`.")}</span>
              </div>
            </div>
            <Button onClick={onClose} className="w-full">
              {t("Done")}
            </Button>
          </div>
        )}
      </div>
    </div>
  );
}
