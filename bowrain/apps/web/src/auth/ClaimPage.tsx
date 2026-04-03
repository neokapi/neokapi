import { useState, useEffect } from "react";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  CardDescription,
  Button,
  Alert,
  AlertDescription,
  useApi,
  useAuth,
  useWorkspace,
  Package,
  CircleCheck,
  Loader2,
  type ClaimProjectResponse,
} from "@neokapi/ui";

interface ClaimPageProps {
  token: string;
  onClaimed: () => void;
}

/** Set a short-lived cookie so the server redirects back here after OIDC. */
function setReturnPathCookie(path: string) {
  document.cookie = `bowrain_return_path=${encodeURIComponent(path)}; path=/; max-age=600; SameSite=Lax`;
}

export function ClaimPage({ token, onClaimed }: ClaimPageProps) {
  const api = useApi();
  const { user, setUser } = useAuth();
  const { setWorkspaces, setActiveWorkspace } = useWorkspace();
  const [checkingAuth, setCheckingAuth] = useState(true);
  const [claiming, setClaiming] = useState(false);
  const [error, setError] = useState("");
  const [result, setResult] = useState<ClaimProjectResponse | null>(null);

  const handleClaim = async () => {
    setClaiming(true);
    setError("");
    try {
      const resp = await api.claimProject(token);
      setResult(resp);

      // Refresh workspace list and switch to the workspace containing the claimed project.
      const refreshed = await api.listWorkspaces();
      setWorkspaces(refreshed);
      const claimedWs = refreshed.find((ws) => ws.slug === resp.workspace_slug);
      if (claimedWs) {
        setActiveWorkspace(claimedWs);
      }
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "Failed to claim project");
    } finally {
      setClaiming(false);
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

  // Auto-claim once user is resolved.
  useEffect(() => {
    if (user && !result && !error && !claiming) {
      void handleClaim();
    }
  }, [user]); // eslint-disable-line react-hooks/exhaustive-deps

  // Still checking whether the user has an active session.
  if (checkingAuth) {
    return (
      <div className="flex min-h-screen flex-col items-center justify-center p-4">
        <p className="mb-6 text-sm font-medium text-muted-foreground">Bowrain</p>
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    );
  }

  // Not authenticated: show a friendly landing page with a "Sign in to claim" button.
  if (!user) {
    return (
      <div className="flex min-h-screen flex-col items-center justify-center p-4">
        <p className="mb-6 text-sm font-medium text-muted-foreground">Bowrain</p>
        <div className="w-full max-w-md">
          <Card>
            <CardHeader className="items-center text-center">
              <div className="mb-2 flex h-12 w-12 items-center justify-center rounded-full bg-primary/10">
                <Package className="h-6 w-6 text-primary" />
              </div>
              <CardTitle className="text-xl font-semibold">Claim Project</CardTitle>
              <CardDescription>
                Sign in to claim this project and add it to your workspace.
              </CardDescription>
            </CardHeader>
            <CardContent className="flex flex-col gap-4">
              <Button
                onClick={() => {
                  setReturnPathCookie(`/claim/${token}`);
                  window.location.href = "/api/v1/auth/login";
                }}
                className="w-full"
                size="lg"
              >
                Sign in to claim
              </Button>
            </CardContent>
          </Card>
        </div>
      </div>
    );
  }

  if (result) {
    return (
      <div className="flex min-h-screen flex-col items-center justify-center p-4">
        <p className="mb-6 text-sm font-medium text-muted-foreground">Bowrain</p>
        <div className="w-full max-w-md">
          <Card>
            <CardHeader className="items-center text-center">
              <CircleCheck className="mb-2 h-12 w-12 text-emerald-500 dark:text-emerald-400" />
              <CardTitle className="text-xl font-semibold">Project Claimed!</CardTitle>
              <CardDescription>
                The project has been added to workspace <strong>{result.workspace_slug}</strong>.
              </CardDescription>
            </CardHeader>
            <CardContent className="flex flex-col gap-4">
              <Button onClick={onClaimed} className="w-full" size="lg">
                Go to workspace
              </Button>
            </CardContent>
          </Card>
        </div>
      </div>
    );
  }

  return (
    <div className="flex min-h-screen flex-col items-center justify-center p-4">
      <p className="mb-6 text-sm font-medium text-muted-foreground">Bowrain</p>
      <div className="w-full max-w-md">
        <Card>
          <CardHeader className="items-center text-center">
            <CardTitle className="text-xl font-semibold">Claim Project</CardTitle>
            <CardDescription>
              {claiming ? "Claiming project..." : "Claim this project"}
            </CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-4">
            {claiming && (
              <div className="flex justify-center">
                <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
              </div>
            )}
            {error && (
              <Alert variant="destructive">
                <AlertDescription>{error}</AlertDescription>
              </Alert>
            )}
            {!claiming && error && (
              <Button onClick={handleClaim} className="w-full" size="lg">
                Try again
              </Button>
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
