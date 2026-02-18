import { useState } from "react";
import { Button, Input } from "@gokapi/ui";
import { Loader2, Globe } from "lucide-react";
import type { ConnectionInfo } from "../hooks/useApi";

interface ServerConnectProps {
  info: ConnectionInfo;
  onConnect: (serverURL: string) => Promise<ConnectionInfo>;
  onStartLogin: (serverURL: string) => Promise<void>;
  onWaitForLogin: () => Promise<boolean>;
  onCancelLogin: () => Promise<void>;
}

type Stage = "url" | "waiting";

export function ServerConnect({
  info,
  onConnect,
  onStartLogin,
  onWaitForLogin,
  onCancelLogin,
}: ServerConnectProps) {
  const [stage, setStage] = useState<Stage>("url");
  const [serverURL, setServerURL] = useState(info.server_url || "");
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  // Try connecting with stored auth first, then start PKCE login.
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
      // No stored auth or expired — proceed to PKCE login.
    }
    // Start PKCE auth — opens browser automatically.
    try {
      await onStartLogin(serverURL.trim());
      setStage("waiting");
      setLoading(false);
      // Wait for the PKCE callback.
      try {
        const authorized = await onWaitForLogin();
        if (authorized) {
          // Try connecting now that we have tokens.
          await onConnect(serverURL.trim());
        }
      } catch (e) {
        setError(e instanceof Error ? e.message : String(e));
        setStage("url");
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
      setLoading(false);
    }
  };

  const handleCancel = async () => {
    await onCancelLogin();
    setStage("url");
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
                Sign In
              </Button>
            </div>
          </>
        )}

        {stage === "waiting" && (
          <div className="space-y-4 text-center">
            <Loader2 className="w-8 h-8 animate-spin text-primary mx-auto" />
            <p className="text-sm text-muted-foreground">
              Completing sign-in in your browser...
            </p>
            {error && (
              <p className="text-sm text-destructive">{error}</p>
            )}
            <Button
              variant="outline"
              onClick={handleCancel}
            >
              Cancel
            </Button>
          </div>
        )}

      </div>
    </div>
  );
}
