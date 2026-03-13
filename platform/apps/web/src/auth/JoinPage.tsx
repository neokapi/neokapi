import { useState, useEffect } from "react";
import {
  Card, CardContent, CardHeader, CardTitle,
  Button,
  useApi, useAuth, useWorkspace,
  type AcceptInviteResponse,
} from "@neokapi/ui";

interface JoinPageProps {
  code: string;
  onJoined: () => void;
}

/** Set a short-lived cookie so the server redirects back here after OIDC. */
function setReturnPathCookie(path: string) {
  document.cookie = `bowrain_return_path=${encodeURIComponent(path)}; path=/; max-age=600; SameSite=Lax`;
}

export function JoinPage({ code, onJoined }: JoinPageProps) {
  const api = useApi();
  const { user, setUser } = useAuth();
  const { setWorkspaces, setActiveWorkspace } = useWorkspace();
  const [checkingAuth, setCheckingAuth] = useState(true);
  const [accepting, setAccepting] = useState(false);
  const [error, setError] = useState("");
  const [result, setResult] = useState<AcceptInviteResponse | null>(null);

  const handleAccept = async () => {
    setAccepting(true);
    setError("");
    try {
      const resp = await api.acceptInvite(code);
      setResult(resp);

      // Refresh workspace list and switch to the joined workspace.
      const refreshed = await api.listWorkspaces();
      setWorkspaces(refreshed);
      const joined = refreshed.find((ws) => ws.slug === resp.workspace_slug);
      if (joined) {
        setActiveWorkspace(joined);
      }
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "Failed to accept invitation");
    } finally {
      setAccepting(false);
    }
  };

  // On mount, try to fetch the current user (the session cookie may already be set).
  useEffect(() => {
    if (user) {
      setCheckingAuth(false);
      return;
    }
    void (async () => {
      try {
        const currentUser = await api.getCurrentUser();
        if (currentUser) {
          setUser(currentUser);
        }
      } catch {
        // No session — user stays null.
      } finally {
        setCheckingAuth(false);
      }
    })();
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  // Auto-accept once user is resolved.
  useEffect(() => {
    if (user && !result && !error && !accepting) {
      void handleAccept();
    }
  }, [user]); // eslint-disable-line react-hooks/exhaustive-deps

  // Still checking whether the user has an active session.
  if (checkingAuth) {
    return (
      <div className="flex items-center justify-center h-screen text-muted-foreground">
        Loading...
      </div>
    );
  }

  // Not authenticated: show a friendly landing page with a "Sign in to join" button.
  if (!user) {
    return (
      <div className="flex items-center justify-center h-screen flex-col gap-6 text-foreground">
        <Card className="min-w-[360px] glass-surface">
          <CardHeader className="items-center text-center">
            <CardTitle className="text-xl font-semibold">Join Workspace</CardTitle>
            <p className="text-sm text-muted-foreground">
              You have been invited to join a workspace. Sign in to accept the invitation.
            </p>
          </CardHeader>
          <CardContent className="flex flex-col gap-4">
            <Button
              onClick={() => {
                setReturnPathCookie(`/join/${code}`);
                window.location.href = "/api/v1/auth/login";
              }}
              className="w-full"
              size="lg"
            >
              Sign in to join
            </Button>
          </CardContent>
        </Card>
      </div>
    );
  }

  if (result) {
    return (
      <div className="flex items-center justify-center h-screen flex-col gap-6 text-foreground">
        <Card className="min-w-[360px] glass-surface">
          <CardHeader className="items-center text-center">
            <CardTitle className="text-xl font-semibold">Joined!</CardTitle>
            <p className="text-sm text-muted-foreground">
              You are now a {result.role} of <strong>{result.workspace_name}</strong>
            </p>
          </CardHeader>
          <CardContent className="flex flex-col gap-4">
            <Button onClick={onJoined} className="w-full" size="lg">
              Go to workspace
            </Button>
          </CardContent>
        </Card>
      </div>
    );
  }

  return (
    <div className="flex items-center justify-center h-screen flex-col gap-6 text-foreground">
      <Card className="min-w-[360px] glass-surface">
        <CardHeader className="items-center text-center">
          <CardTitle className="text-xl font-semibold">Join Workspace</CardTitle>
          <p className="text-sm text-muted-foreground">
            {accepting ? "Accepting invitation..." : "Accept this workspace invitation"}
          </p>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          {error && (
            <div className="text-destructive text-sm text-center">{error}</div>
          )}
          {!accepting && error && (
            <Button onClick={handleAccept} className="w-full" size="lg">
              Try again
            </Button>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
