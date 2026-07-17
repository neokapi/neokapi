import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate, useSearch, Link } from "@tanstack/react-router";
import {
  Badge,
  Button,
  Card,
  GitPullRequest,
  Loader2,
  ProjectFormDialog,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  useApi,
  useAuth,
  type InstallationRepo,
  type ProjectFormData,
} from "@neokapi/ui";
import { projectsQueryOptions, workspacesQueryOptions } from "../queries";

/**
 * GitHub App post-install landing: GitHub redirects here (the app's Setup URL)
 * with the installation id. The page turns a fresh install into a connected
 * project — signing in or creating an account, a workspace and a project as
 * needed, then binding each repository. Binding creates the server-side
 * app-auth forge connector; from the next push the loop delivers translations
 * as a pull request.
 *
 * It owns its own auth (like the invite/claim pages): a logged-out visitor is
 * welcomed rather than silently bounced, and the installation id round-trips
 * through login via the return-path cookie.
 */

/** Short-lived cookie so the server redirects back here after OIDC. */
function setReturnPathCookie(path: string) {
  document.cookie = `bowrain_return_path=${encodeURIComponent(path)}; path=/; max-age=600; SameSite=Lax`;
}

function Shell({ children }: { children: React.ReactNode }) {
  return (
    <div className="mx-auto mt-12 w-full max-w-2xl px-4">
      <Card className="p-8">
        <div className="mb-6 flex items-center gap-3">
          <div className="flex h-10 w-10 items-center justify-center rounded-full bg-primary/10">
            <GitPullRequest className="h-5 w-5 text-primary" />
          </div>
          <div>
            <h1 className="text-xl font-semibold leading-tight">Connect your repositories</h1>
            <p className="text-sm text-muted-foreground">
              Translations come back as one pull request the loop keeps up to date — no CI, no
              tokens.
            </p>
          </div>
        </div>
        {children}
      </Card>
    </div>
  );
}

function Spinner() {
  return (
    <div className="flex justify-center py-8">
      <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
    </div>
  );
}

export function GithubSetupRoute() {
  const { installation_id: installationId } = useSearch({ strict: false }) as {
    installation_id?: string;
  };
  const api = useApi();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { user, setUser } = useAuth();

  // Own the auth check: this page is reachable logged-out (GitHub sends the
  // user straight here), so resolve the session before deciding what to show.
  const [checkingAuth, setCheckingAuth] = useState(!user);
  useEffect(() => {
    if (user) {
      setCheckingAuth(false);
      return;
    }
    void (async () => {
      try {
        const current = await api.getCurrentUser();
        if (current) setUser(current);
      } catch {
        // No session — stays logged out.
      } finally {
        setCheckingAuth(false);
      }
    })();
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  const setupPath = `/github/setup?installation_id=${installationId ?? ""}`;

  const workspaces = useQuery({ ...workspacesQueryOptions(api), enabled: !!user });
  const [workspaceSlug, setWorkspaceSlug] = useState<string>("");
  const activeSlug = workspaceSlug || workspaces.data?.[0]?.slug || "";

  const projects = useQuery({
    ...projectsQueryOptions(api, activeSlug),
    enabled: !!user && !!activeSlug,
  });
  const repos = useQuery({
    queryKey: ["github-installation-repos", activeSlug, installationId],
    queryFn: () => api.listInstallationRepos(activeSlug, installationId ?? ""),
    enabled: !!user && !!activeSlug && !!installationId,
  });

  // --- No installation id: opened out of context. -------------------------
  if (!installationId) {
    return (
      <Shell>
        <p className="text-sm text-muted-foreground">
          This page is where GitHub sends you after installing the app. Install{" "}
          <a
            className="underline underline-offset-2"
            href="https://github.com/apps/bowraincloud/installations/new"
          >
            the Bowrain app
          </a>{" "}
          on a repository, or open this page from the installation&apos;s settings.
        </p>
      </Shell>
    );
  }

  if (checkingAuth) {
    return (
      <Shell>
        <Spinner />
      </Shell>
    );
  }

  // --- Logged out: welcome + sign-in/sign-up. -----------------------------
  if (!user) {
    return (
      <Shell>
        <p className="text-sm text-muted-foreground">
          The GitHub App is installed. Sign in or create an account to connect the repositories to a
          Bowrain project.
        </p>
        <Button
          className="mt-5"
          size="lg"
          onClick={() => {
            setReturnPathCookie(setupPath);
            window.location.href = "/api/v1/auth/login";
          }}
        >
          Sign in or create an account
        </Button>
      </Shell>
    );
  }

  if (workspaces.isLoading) {
    return (
      <Shell>
        <Spinner />
      </Shell>
    );
  }

  // --- Signed in but no workspace yet: onboard first, then return here. ----
  if ((workspaces.data?.length ?? 0) === 0) {
    return (
      <Shell>
        <p className="text-sm text-muted-foreground">
          Welcome. Create your workspace to continue — you&apos;ll come right back here to connect
          your repositories.
        </p>
        <Button
          className="mt-5"
          size="lg"
          onClick={() => void navigate({ to: "/welcome", search: { return_to: setupPath } })}
        >
          Create your workspace
        </Button>
      </Shell>
    );
  }

  const projectList = projects.data?.map((p) => ({ id: p.id, name: p.name })) ?? [];

  return (
    <Shell>
      {(workspaces.data?.length ?? 0) > 1 && (
        <div className="mb-4 flex items-center gap-2">
          <span className="text-sm text-muted-foreground">Workspace</span>
          <Select value={activeSlug} onValueChange={setWorkspaceSlug}>
            <SelectTrigger className="w-56" data-testid="workspace-select">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {workspaces.data?.map((ws) => (
                <SelectItem key={ws.slug} value={ws.slug}>
                  {ws.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      )}

      {repos.isLoading && <Spinner />}
      {repos.isError && (
        <p className="text-sm text-destructive">
          Could not list the installation&apos;s repositories: {(repos.error as Error).message}
        </p>
      )}

      <div className="flex flex-col gap-3">
        {repos.data?.map((repo) => (
          <RepoRow
            key={repo.full_name}
            repo={repo}
            workspaceSlug={activeSlug}
            installationId={installationId}
            projects={projectList}
            onChanged={() => {
              void queryClient.invalidateQueries({
                queryKey: ["github-installation-repos", activeSlug, installationId],
              });
              void queryClient.invalidateQueries({
                queryKey: projectsQueryOptions(api, activeSlug).queryKey,
              });
            }}
          />
        ))}
        {repos.data?.length === 0 && (
          <p className="text-sm text-muted-foreground">
            The installation covers no repositories. Grant it access to at least one repository in
            the GitHub App&apos;s settings.
          </p>
        )}
      </div>
    </Shell>
  );
}

function RepoRow({
  repo,
  workspaceSlug,
  installationId,
  projects,
  onChanged,
}: {
  repo: InstallationRepo;
  workspaceSlug: string;
  installationId: string;
  projects: { id: string; name: string }[];
  onChanged: () => void;
}) {
  const api = useApi();
  const [projectId, setProjectId] = useState("");
  const [creating, setCreating] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const repoShortName = repo.full_name.slice(repo.full_name.lastIndexOf("/") + 1);

  const bind = useMutation({
    mutationFn: (pid: string) =>
      api.bindInstallationRepo(workspaceSlug, installationId, {
        repository: repo.full_name,
        project_id: pid,
      }),
    onSuccess: onChanged,
    onError: (e) => setError((e as Error).message),
  });

  // Create a project from the repo, then bind it in one step.
  const createAndBind = useMutation({
    mutationFn: async (data: ProjectFormData) => {
      const project = await api.createProject(
        workspaceSlug,
        data.name,
        data.default_source_language,
        data.target_languages,
      );
      return api.bindInstallationRepo(workspaceSlug, installationId, {
        repository: repo.full_name,
        project_id: project.id,
      });
    },
    onSuccess: () => {
      setCreating(false);
      onChanged();
    },
    onError: (e) => setError((e as Error).message),
  });

  const boundProject = useMemo(
    () => projects.find((p) => p.id === repo.project_id),
    [projects, repo.project_id],
  );

  return (
    <div
      className="flex items-center justify-between gap-3 rounded-lg border p-3"
      data-testid={`repo-${repo.full_name}`}
    >
      <div className="min-w-0">
        <div className="flex items-center gap-2">
          <span className="truncate text-sm font-medium">{repo.full_name}</span>
          {repo.private && <Badge variant="secondary">private</Badge>}
        </div>
        <div className="text-xs text-muted-foreground">tracked branch: {repo.default_branch}</div>
        {error && <div className="mt-1 text-xs text-destructive">{error}</div>}
      </div>

      {repo.connector_id ? (
        <div className="flex shrink-0 items-center gap-2">
          <Badge>connected</Badge>
          {boundProject && (
            <Link
              to="/$workspace"
              params={{ workspace: workspaceSlug }}
              className="text-sm text-muted-foreground underline-offset-2 hover:underline"
            >
              {boundProject.name}
            </Link>
          )}
        </div>
      ) : projects.length === 0 ? (
        <Button
          size="sm"
          className="shrink-0"
          disabled={bind.isPending || createAndBind.isPending}
          onClick={() => setCreating(true)}
        >
          Create a project
        </Button>
      ) : (
        <div className="flex shrink-0 items-center gap-2">
          <Select value={projectId} onValueChange={setProjectId}>
            <SelectTrigger className="w-44" data-testid={`project-select-${repo.full_name}`}>
              <SelectValue placeholder="Choose a project" />
            </SelectTrigger>
            <SelectContent>
              {projects.map((p) => (
                <SelectItem key={p.id} value={p.id}>
                  {p.name}
                </SelectItem>
              ))}
              <SelectItem value="__new__">+ New project…</SelectItem>
            </SelectContent>
          </Select>
          {projectId === "__new__" ? (
            <Button size="sm" onClick={() => setCreating(true)}>
              Create
            </Button>
          ) : (
            <Button
              size="sm"
              disabled={!projectId || bind.isPending}
              onClick={() => bind.mutate(projectId)}
            >
              {bind.isPending ? "Connecting…" : "Connect"}
            </Button>
          )}
        </div>
      )}

      <ProjectFormDialog
        open={creating}
        onOpenChange={setCreating}
        initialName={repoShortName}
        onSubmit={(data) => createAndBind.mutate(data)}
      />
    </div>
  );
}
