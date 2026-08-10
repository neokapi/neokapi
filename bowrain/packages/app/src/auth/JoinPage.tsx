import { useState, useEffect } from "react";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  CardDescription,
  Button,
  ErrorNotice,
  useApi,
  useAuth,
  useWorkspace,
  Users,
  CircleCheck,
  Loader2,
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

/** "an admin", "a member" — the roles are ordinary English words. */
function article(word: string): string {
  return /^[aeiou]/i.test(word) ? "an" : "a";
}

/**
 * What the join actually granted. The role and workspace come from the accept
 * response; either may be absent if the server could not name the workspace it
 * had already joined the user to, and a join that happened is still confirmed
 * rather than reported as a blank.
 */
export function JoinedDescription({ result }: { result: AcceptInviteResponse }) {
  const workspaceName = result.workspace_name?.trim();
  const role = result.role?.trim();

  if (workspaceName && role) {
    return (
      <>
        You are now {article(role)} {role} of <strong>{workspaceName}</strong>
      </>
    );
  }
  if (workspaceName) {
    return (
      <>
        You have joined <strong>{workspaceName}</strong>
      </>
    );
  }
  return <>You have joined the workspace</>;
}

export function JoinPage({ code, onJoined }: JoinPageProps) {
  const api = useApi();
  const { user, setUser } = useAuth();
  const { setWorkspaces, setActiveWorkspace } = useWorkspace();
  const [checkingAuth, setCheckingAuth] = useState(true);
  const [accepting, setAccepting] = useState(false);
  const [error, setError] = useState<{ title: string; cause?: unknown } | null>(null);
  const [result, setResult] = useState<AcceptInviteResponse | null>(null);

  const handleAccept = async () => {
    setAccepting(true);
    setError(null);
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
      setError({ title: "Couldn't accept the invitation", cause: e });
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
      <div className="flex min-h-screen flex-col items-center justify-center p-4">
        <p className="mb-6 text-sm font-medium text-muted-foreground">Bowrain</p>
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    );
  }

  // Not authenticated: show a friendly landing page with a "Sign in to join" button.
  if (!user) {
    return (
      <div className="flex min-h-screen flex-col items-center justify-center p-4">
        <p className="mb-6 text-sm font-medium text-muted-foreground">Bowrain</p>
        <div className="w-full max-w-md">
          <Card>
            <CardHeader className="items-center text-center">
              <div className="mb-2 flex h-12 w-12 items-center justify-center rounded-full bg-primary/10">
                <Users className="h-6 w-6 text-primary" />
              </div>
              <CardTitle className="text-xl font-semibold">Join Workspace</CardTitle>
              <CardDescription>
                You have been invited to join a workspace. Sign in to accept the invitation.
              </CardDescription>
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
              <CardTitle className="text-xl font-semibold">Joined!</CardTitle>
              <CardDescription>
                <JoinedDescription result={result} />
              </CardDescription>
            </CardHeader>
            <CardContent className="flex flex-col gap-4">
              <Button onClick={onJoined} className="w-full" size="lg">
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
            <CardTitle className="text-xl font-semibold">Join Workspace</CardTitle>
            <CardDescription>
              {accepting ? "Accepting invitation..." : "Accept this workspace invitation"}
            </CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-4">
            {accepting && (
              <div className="flex justify-center">
                <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
              </div>
            )}
            {error && <ErrorNotice title={error.title} error={error.cause} variant="inline" />}
            {!accepting && error && (
              <Button onClick={handleAccept} className="w-full" size="lg">
                Try again
              </Button>
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
