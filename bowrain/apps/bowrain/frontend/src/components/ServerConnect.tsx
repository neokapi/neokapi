import { useState, useEffect } from "react";
import {
  Button,
  Input,
  Label,
  Collapsible,
  CollapsibleTrigger,
  CollapsibleContent,
} from "@neokapi/ui";
import { Loader2, ChevronRight, ChevronDown } from "lucide-react";
import type { ConnectionInfo } from "../hooks/useApi";

import { Backend } from "../api/backend";

const FALLBACK_DEFAULT_URL = "https://bowrain.mymac";

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
  const [defaultURL, setDefaultURL] = useState(FALLBACK_DEFAULT_URL);
  const [serverURL, setServerURL] = useState(info.server_url || "");
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  // Has the user customised the URL away from the default?
  const isCustomServer = serverURL !== "" && serverURL !== defaultURL;
  const [serverOptionsOpen, setServerOptionsOpen] = useState(isCustomServer);

  // Fetch the default server URL from the backend on mount.
  useEffect(() => {
    Backend.GetDefaultServerURL()
      .then((url: string) => {
        if (url) {
          setDefaultURL(url);
          // Pre-fill the URL field if not already set by stored info.
          if (!info.server_url) {
            setServerURL(url);
          }
        }
      })
      .catch(() => {
        // Fall back to hardcoded default.
        if (!info.server_url) {
          setServerURL(FALLBACK_DEFAULT_URL);
        }
      });
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  // Auto-expand server options if returning user has a custom server.
  useEffect(() => {
    if (info.server_url && defaultURL && info.server_url !== defaultURL) {
      setServerOptionsOpen(true);
    }
  }, [info.server_url, defaultURL]);

  const effectiveURL = serverURL.trim() || defaultURL;

  // Try connecting with stored auth first, then start PKCE login.
  const handleConnect = async () => {
    setLoading(true);
    setError(null);
    try {
      const result = await onConnect(effectiveURL);
      if (result.state === "connected") {
        // Success — App.tsx will handle navigation to workspace selector.
        return;
      }
    } catch {
      // No stored auth or expired — proceed to PKCE login.
    }
    // Start PKCE auth — opens browser automatically.
    try {
      await onStartLogin(effectiveURL);
      setStage("waiting");
      setLoading(false);
      // Wait for the PKCE callback.
      try {
        const authorized = await onWaitForLogin();
        if (authorized) {
          // Try connecting now that we have tokens.
          await onConnect(effectiveURL);
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
        <h2 className="text-2xl font-semibold mb-2">Welcome to Bowrain</h2>
        <p className="text-muted-foreground text-sm max-w-md">
          Sign in to collaborate with your team on translation projects.
        </p>
      </div>

      <div className="w-full max-w-md space-y-4">
        {stage === "url" && (
          <>
            {error && <p className="text-sm text-destructive">{error}</p>}

            <Button className="w-full" onClick={handleConnect} disabled={loading}>
              {loading && <Loader2 className="w-4 h-4 mr-2 animate-spin" />}
              Sign In
            </Button>

            <Collapsible open={serverOptionsOpen} onOpenChange={setServerOptionsOpen}>
              <CollapsibleTrigger className="flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground transition-colors cursor-pointer">
                {serverOptionsOpen ? (
                  <ChevronDown className="w-3 h-3" />
                ) : (
                  <ChevronRight className="w-3 h-3" />
                )}
                Server options
              </CollapsibleTrigger>
              <CollapsibleContent className="pt-3">
                <div className="space-y-2">
                  <Label>Server URL</Label>
                  <Input
                    placeholder={defaultURL}
                    value={serverURL}
                    onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
                      setServerURL(e.target.value)
                    }
                    onKeyDown={(e: React.KeyboardEvent<HTMLInputElement>) =>
                      e.key === "Enter" && handleConnect()
                    }
                    disabled={loading}
                  />
                </div>
              </CollapsibleContent>
            </Collapsible>
          </>
        )}

        {stage === "waiting" && (
          <div className="space-y-4 text-center">
            <Loader2 className="w-8 h-8 animate-spin text-primary mx-auto" />
            <p className="text-sm text-muted-foreground">Completing sign-in in your browser...</p>
            {error && <p className="text-sm text-destructive">{error}</p>}
            <Button variant="outline" onClick={handleCancel}>
              Cancel
            </Button>
          </div>
        )}
      </div>
    </div>
  );
}
