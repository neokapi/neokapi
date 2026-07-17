import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useSearch, Link } from "@tanstack/react-router";
import {
  Badge,
  Button,
  Card,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  useApi,
} from "@neokapi/ui";
import type { InstallationRepo } from "@neokapi/ui";
import { projectsQueryOptions, workspacesQueryOptions } from "../queries";

/**
 * GitHub App post-install landing: GitHub redirects here (the app's Setup
 * URL) with the installation id. The page lists the repositories the
 * installation covers and binds each to a project — one selection, no
 * tokens, no CI configuration. Binding creates the server-side app-auth
 * forge connector; from the next push the loop delivers translations as a
 * pull request.
 */
export function GithubSetupRoute() {
  const { installation_id: installationId } = useSearch({ strict: false }) as {
    installation_id?: string;
  };
  const api = useApi();
  const queryClient = useQueryClient();

  const workspaces = useQuery(workspacesQueryOptions(api));
  const [workspaceSlug, setWorkspaceSlug] = useState<string>("");
  const activeSlug = workspaceSlug || workspaces.data?.[0]?.slug || "";

  const projects = useQuery({
    ...projectsQueryOptions(api, activeSlug),
    enabled: !!activeSlug,
  });
  const repos = useQuery({
    queryKey: ["github-installation-repos", activeSlug, installationId],
    queryFn: () => api.listInstallationRepos(activeSlug, installationId ?? ""),
    enabled: !!activeSlug && !!installationId,
  });

  if (!installationId) {
    return (
      <SetupShell>
        <p className="text-sm text-muted-foreground">
          This page is where GitHub sends you after installing the app. Open it from the app
          installation flow, or add <code>?installation_id=…</code> from the installation&apos;s
          settings page.
        </p>
      </SetupShell>
    );
  }

  return (
    <SetupShell>
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

      {repos.isLoading && <p className="text-sm text-muted-foreground">Loading repositories…</p>}
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
            projects={projects.data?.map((p) => ({ id: p.id, name: p.name })) ?? []}
            onBound={() =>
              void queryClient.invalidateQueries({
                queryKey: ["github-installation-repos", activeSlug, installationId],
              })
            }
          />
        ))}
        {repos.data?.length === 0 && (
          <p className="text-sm text-muted-foreground">
            The installation covers no repositories. Grant it access to at least one repository in
            the GitHub App&apos;s settings.
          </p>
        )}
      </div>
    </SetupShell>
  );
}

function SetupShell({ children }: { children: React.ReactNode }) {
  return (
    <div className="mx-auto mt-12 w-full max-w-2xl px-4">
      <Card className="p-8">
        <h1 className="text-xl font-semibold">Connect your repositories</h1>
        <p className="mt-1 mb-6 text-sm text-muted-foreground">
          The GitHub App is installed. Pick a project for each repository — from the next push,
          translations come back as one pull request the loop keeps up to date. No CI configuration,
          no tokens.
        </p>
        {children}
      </Card>
    </div>
  );
}

function RepoRow({
  repo,
  workspaceSlug,
  installationId,
  projects,
  onBound,
}: {
  repo: InstallationRepo;
  workspaceSlug: string;
  installationId: string;
  projects: { id: string; name: string }[];
  onBound: () => void;
}) {
  const api = useApi();
  const [projectId, setProjectId] = useState("");
  const [error, setError] = useState<string | null>(null);

  const bind = useMutation({
    mutationFn: () =>
      api.bindInstallationRepo(workspaceSlug, installationId, {
        repository: repo.full_name,
        project_id: projectId,
      }),
    onSuccess: onBound,
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
          <span className="truncate font-medium text-sm">{repo.full_name}</span>
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
            </SelectContent>
          </Select>
          <Button size="sm" disabled={!projectId || bind.isPending} onClick={() => bind.mutate()}>
            {bind.isPending ? "Connecting…" : "Connect"}
          </Button>
        </div>
      )}
    </div>
  );
}
