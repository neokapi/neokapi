import { useEffect, useState } from "react";
import { useNavigate, useSearch } from "@tanstack/react-router";
import { useQueryClient } from "@tanstack/react-query";
import { handleCallback, login, fetchAdminSession, ADMIN_SESSION_QUERY_KEY } from "../auth";

export function AuthCallbackRoute() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
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
        // No code: if a session already exists, go to the dashboard.
        const session = await queryClient.ensureQueryData({
          queryKey: ADMIN_SESSION_QUERY_KEY,
          queryFn: fetchAdminSession,
        });
        if (session) {
          void navigate({ to: "/", replace: true });
          return;
        }
        // Not authenticated — start login.
        await login();
        return;
      }

      try {
        await handleCallback(code);
        // The exchange set the session cookie; refresh the cached probe so the
        // guard and layout see the new session.
        await queryClient.invalidateQueries({ queryKey: ADMIN_SESSION_QUERY_KEY });
        void navigate({ to: "/", replace: true });
      } catch (err) {
        setError(err instanceof Error ? err.message : "Authentication failed");
      }
    }

    void processCallback();
  }, [navigate, queryClient, search.action]);

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
