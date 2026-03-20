import { useEffect, useState } from "react";
import { useNavigate, useSearch } from "@tanstack/react-router";
import { handleCallback, login, isAuthenticated } from "../auth";

export function AuthCallbackRoute() {
  const navigate = useNavigate();
  const search = useSearch({ strict: false }) as { action?: string };
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    async function processCallback() {
      // If this is a login trigger (no code in URL), start the OIDC flow
      if (search.action === "login") {
        await login();
        return;
      }

      const params = new URLSearchParams(window.location.search);
      const code = params.get("code");

      if (!code) {
        // Already authenticated, redirect to dashboard
        if (isAuthenticated()) {
          void navigate({ to: "/", replace: true });
          return;
        }
        // No code and not authenticated — start login
        await login();
        return;
      }

      try {
        await handleCallback(code);
        void navigate({ to: "/", replace: true });
      } catch (err) {
        setError(err instanceof Error ? err.message : "Authentication failed");
      }
    }

    void processCallback();
  }, [navigate, search.action]);

  if (error) {
    return (
      <div className="flex items-center justify-center h-screen bg-background">
        <div className="text-center space-y-4">
          <h2 className="text-lg font-semibold text-foreground">Authentication Error</h2>
          <p className="text-sm text-muted-foreground">{error}</p>
          <button
            onClick={() => void login()}
            className="px-4 py-2 text-sm rounded-md bg-primary text-primary-foreground hover:bg-primary/90 cursor-pointer"
          >
            Try again
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className="flex items-center justify-center h-screen bg-background">
      <p className="text-sm text-muted-foreground">Authenticating...</p>
    </div>
  );
}
