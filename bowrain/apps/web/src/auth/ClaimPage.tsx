import { useState, useEffect } from "react";
import {
  Card, CardContent, CardHeader, CardTitle,
  Button,
  useApi, useAuth, useWorkspace,
  type ClaimProjectResponse,
} from "@gokapi/ui";

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
  const { user } = useAuth();
  const { setWorkspaces, setActiveWorkspace } = useWorkspace();
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

  // Auto-claim on mount if user is authenticated.
  useEffect(() => {
    if (user && !result && !error) {
      handleClaim();
    }
  }, [user]); // eslint-disable-line react-hooks/exhaustive-deps

  // Not authenticated: show a friendly landing page with a "Sign in to claim" button.
  if (!user) {
    return (
      <div className="flex items-center justify-center h-screen flex-col gap-6 bg-background text-foreground">
        <Card className="min-w-[360px]">
          <CardHeader className="items-center text-center">
            <CardTitle className="text-xl font-semibold">Claim Project</CardTitle>
            <p className="text-sm text-muted-foreground">
              Sign in to claim this project and add it to your workspace.
            </p>
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
    );
  }

  if (result) {
    return (
      <div className="flex items-center justify-center h-screen flex-col gap-6 bg-background text-foreground">
        <Card className="min-w-[360px]">
          <CardHeader className="items-center text-center">
            <CardTitle className="text-xl font-semibold">Project Claimed!</CardTitle>
            <p className="text-sm text-muted-foreground">
              The project has been added to workspace{" "}
              <strong>{result.workspace_slug}</strong>.
            </p>
          </CardHeader>
          <CardContent className="flex flex-col gap-4">
            <Button onClick={onClaimed} className="w-full" size="lg">
              Go to workspace
            </Button>
          </CardContent>
        </Card>
      </div>
    );
  }

  return (
    <div className="flex items-center justify-center h-screen flex-col gap-6 bg-background text-foreground">
      <Card className="min-w-[360px]">
        <CardHeader className="items-center text-center">
          <CardTitle className="text-xl font-semibold">Claim Project</CardTitle>
          <p className="text-sm text-muted-foreground">
            {claiming ? "Claiming project..." : "Claim this project"}
          </p>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          {error && (
            <div className="text-destructive text-sm text-center">{error}</div>
          )}
          {!claiming && error && (
            <Button onClick={handleClaim} className="w-full" size="lg">
              Try again
            </Button>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
