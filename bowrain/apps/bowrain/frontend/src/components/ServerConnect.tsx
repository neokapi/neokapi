import { useState, useEffect, useRef } from "react";
import { Button, Input, cn } from "@gokapi/ui";
import { Loader2, Globe, ExternalLink } from "lucide-react";
import type { ConnectionInfo, DeviceAuthInfo } from "../hooks/useApi";

import { Browser } from "@wailsio/runtime";

interface ServerConnectProps {
  info: ConnectionInfo;
  onConnect: (serverURL: string) => Promise<ConnectionInfo>;
  onStartLogin: (serverURL: string) => Promise<DeviceAuthInfo>;
  onPollLogin: (deviceCode: string, interval: number) => Promise<boolean>;
  onCancelLogin: () => Promise<void>;
  onSkip: () => void;
}

type Stage = "url" | "auth" | "polling";

export function ServerConnect({
  info,
  onConnect,
  onStartLogin,
  onPollLogin,
  onCancelLogin,
  onSkip,
}: ServerConnectProps) {
  const [stage, setStage] = useState<Stage>("url");
  const [serverURL, setServerURL] = useState(info.server_url || "");
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [authInfo, setAuthInfo] = useState<DeviceAuthInfo | null>(null);
  const pollingRef = useRef(false);

  // Try connecting with stored auth first.
  const handleConnect = async () => {
    if (!serverURL.trim()) return;
    setLoading(true);
    setError(null);
    try {
      const result = await onConnect(serverURL.trim());
      if (result.state === "connected") {
        // Success — App.tsx will handle navigation to workspace selector.
        return;
      }
    } catch {
      // No stored auth or expired — proceed to login.
    }
    // Start device auth.
    try {
      const auth = await onStartLogin(serverURL.trim());
      setAuthInfo(auth);
      setStage("auth");
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  };

  // Open verification URL and start polling.
  const handleOpenAndPoll = () => {
    if (!authInfo) return;
    try {
      Browser.OpenURL(authInfo.verification_uri);
    } catch {
      // If Browser.OpenURL isn't available, continue anyway.
    }
    setStage("polling");
  };

  // Poll for authorization.
  useEffect(() => {
    if (stage !== "polling" || !authInfo) return;
    pollingRef.current = true;

    const interval = (authInfo.expires_in > 0 ? Math.min(authInfo.expires_in, 5) : 5) * 1000;
    const timer = setInterval(async () => {
      if (!pollingRef.current) return;
      try {
        const authorized = await onPollLogin(authInfo.user_code, interval / 1000);
        if (authorized) {
          pollingRef.current = false;
          // Try connecting now.
          await onConnect(serverURL.trim());
        }
      } catch (e) {
        pollingRef.current = false;
        setError(e instanceof Error ? e.message : String(e));
        setStage("url");
      }
    }, interval);

    return () => {
      pollingRef.current = false;
      clearInterval(timer);
    };
  }, [stage, authInfo, onPollLogin, onConnect, serverURL]);

  const handleCancel = async () => {
    pollingRef.current = false;
    await onCancelLogin();
    setStage("url");
    setAuthInfo(null);
    setError(null);
  };

  return (
    <div className="flex flex-col items-center justify-center h-full gap-8">
      <div className="text-center">
        <Globe className="w-12 h-12 text-primary mx-auto mb-4" />
        <h2 className="text-2xl font-semibold mb-2">Connect to Server</h2>
        <p className="text-muted-foreground text-sm max-w-md">
          Connect to a Bowrain Server to collaborate with your team on translation projects.
        </p>
      </div>

      <div className="w-full max-w-md space-y-4">
        {stage === "url" && (
          <>
            <div className="space-y-2">
              <label className="text-sm font-medium">Server URL</label>
              <Input
                placeholder="https://bowrain.example.com"
                value={serverURL}
                onChange={(e) => setServerURL(e.target.value)}
                onKeyDown={(e) => e.key === "Enter" && handleConnect()}
                disabled={loading}
              />
            </div>

            {error && (
              <p className="text-sm text-destructive">{error}</p>
            )}

            <div className="flex gap-3">
              <Button
                className="flex-1"
                onClick={handleConnect}
                disabled={!serverURL.trim() || loading}
              >
                {loading && <Loader2 className="w-4 h-4 mr-2 animate-spin" />}
                Connect
              </Button>
            </div>
          </>
        )}

        {stage === "auth" && authInfo && (
          <div className="space-y-4 text-center">
            <p className="text-sm text-muted-foreground">
              Enter this code in your browser to authorize:
            </p>
            <div className="bg-muted rounded-lg p-4">
              <code className="text-2xl font-mono font-bold tracking-widest">
                {authInfo.user_code}
              </code>
            </div>
            <Button
              className="w-full"
              onClick={handleOpenAndPoll}
            >
              <ExternalLink className="w-4 h-4 mr-2" />
              Open in Browser
            </Button>
          </div>
        )}

        {stage === "polling" && (
          <div className="space-y-4 text-center">
            <Loader2 className="w-8 h-8 animate-spin text-primary mx-auto" />
            <p className="text-sm text-muted-foreground">
              Waiting for authorization...
            </p>
            {authInfo && (
              <div className="bg-muted rounded-lg p-3">
                <code className="text-lg font-mono font-bold tracking-widest">
                  {authInfo.user_code}
                </code>
              </div>
            )}
            <Button
              variant="outline"
              onClick={handleCancel}
            >
              Cancel
            </Button>
          </div>
        )}

        <div className="text-center pt-2">
          <button
            onClick={onSkip}
            className={cn(
              "text-sm text-muted-foreground hover:text-foreground transition-colors",
              "underline underline-offset-2",
            )}
          >
            Work Offline
          </button>
        </div>
      </div>
    </div>
  );
}
