import { useState, useEffect } from "react";
import {
  Card, CardContent, CardHeader, CardTitle,
  Button,
  useApi, useAuth, useWorkspace,
  type AcceptInviteResponse,
} from "@gokapi/ui";

interface JoinPageProps {
  code: string;
  onJoined: () => void;
}

export function JoinPage({ code, onJoined }: JoinPageProps) {
  const api = useApi();
  const { user } = useAuth();
  const { workspaces, setWorkspaces, setActiveWorkspace } = useWorkspace();
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

  // Auto-accept on mount if user is authenticated.
  useEffect(() => {
    if (user && !result && !error) {
      handleAccept();
    }
  }, [user]); // eslint-disable-line react-hooks/exhaustive-deps

  if (!user) {
    // Not authenticated — redirect to login, then come back.
    return (
      <div className="flex items-center justify-center h-screen flex-col gap-6 bg-background text-foreground">
        <Card className="min-w-[360px]">
          <CardHeader className="items-center text-center">
            <CardTitle className="text-xl font-semibold">Join Workspace</CardTitle>
            <p className="text-sm text-muted-foreground">
              Sign in to accept this invitation
            </p>
          </CardHeader>
          <CardContent className="flex flex-col gap-4">
            <Button
              onClick={() => {
                // Preserve the join code in URL so we return here after auth.
                const returnUrl = `${window.location.origin}/join/${code}`;
                window.location.href = `/api/v1/auth/login?return_url=${encodeURIComponent(returnUrl)}`;
              }}
              className="w-full"
              size="lg"
            >
              Sign in to continue
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
    <div className="flex items-center justify-center h-screen flex-col gap-6 bg-background text-foreground">
      <Card className="min-w-[360px]">
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
