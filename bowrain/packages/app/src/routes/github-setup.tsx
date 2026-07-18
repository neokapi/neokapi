import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate, useSearch, Link } from "@tanstack/react-router";
import {
  AnalyticsEvents,
  Badge,
  Button,
  Card,
  ChevronDown,
  ErrorNotice,
  GitPullRequest,
  Input,
  Label,
  Loader2,
  Plus,
  useAnalytics,
  useApi,
  useAuth,
  type InstallationRepo,
  type ProjectFormData,
  type Workspace,
} from "@neokapi/ui";
import { projectsQueryOptions, workspacesQueryOptions } from "../queries";
import { importPhaseFromStatus } from "./import-phase";
import { coerceInstallationId } from "./installation-id";

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
 *
 * GitHub calls this page with `setup_action=install` (first install) or
 * `setup_action=update` (repository access changed, via Redirect on update).
 * Both are handled identically — the repo list is re-read either way; the
 * value only feeds diagnostics when the installation id is missing.
 *
 * History: the original page used shadcn Selects and ProjectFormDialog, which
 * rendered nothing in production (#1348). The cause was NOT this route's
 * layout — the @neokapi/kapi-react build transform recognised ICU authoring
 * components by bare identifier, so the controlled `<Select value={...}>`
 * widgets were serialised to empty ICU `{value, select, }` templates and
 * deleted from the compiled chunk. That transform bug is fixed (shape-based
 * recognition + regression guard in radix-select-transform.test.tsx); the
 * portal-free wizard below (#1353) is kept because cards and inline forms are
 * the better fit for this first-run surface.
 */

/** Short-lived cookie so the server redirects back here after OIDC. */
function setReturnPathCookie(path: string) {
  document.cookie = `bowrain_return_path=${encodeURIComponent(path)}; path=/; max-age=600; SameSite=Lax`;
}

/** Same handle rules the server enforces (see WelcomePage). */
const SLUG_PATTERN = /^[a-z0-9](?:[a-z0-9-]{0,62}[a-z0-9])?$/;

/** Client-side suggestion: lowercase, strip accents, hyphenate the rest. */
function slugify(name: string): string {
  return name
    .toLowerCase()
    .normalize("NFKD")
    .replace(/[\u0300-\u036f]/g, "")
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 64);
}

/**
 * What ProjectFormDialog submits when its fields are left untouched and the
 * workspace has no configured languages: English source, one French target,
 * defined-list mode. The server requires only name + default_source_language;
 * the languages stay editable from the project's settings afterwards.
 */
function newProjectData(name: string): ProjectFormData {
  return {
    name,
    default_source_language: "en",
    target_languages: ["fr"],
    target_language_mode: "defined",
  };
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

/**
 * Workspace chooser: one selectable card per workspace plus a "New workspace"
 * card that expands an inline create form. Rendered even with a single
 * workspace so the user sees where the project will land.
 */
function WorkspaceSection({
  workspaces,
  activeSlug,
  onSelect,
}: {
  workspaces: Workspace[];
  activeSlug: string;
  onSelect: (slug: string) => void;
}) {
  const api = useApi();
  const queryClient = useQueryClient();
  const [creatingOpen, setCreatingOpen] = useState(false);
  const [name, setName] = useState("");
  const [slug, setSlug] = useState("");
  const [slugTouched, setSlugTouched] = useState(false);
  const [error, setError] = useState<unknown>(null);

  const slugInvalid = slug.length > 0 && (!SLUG_PATTERN.test(slug) || slug.length < 2);

  const resetForm = () => {
    setName("");
    setSlug("");
    setSlugTouched(false);
    setError(null);
  };

  const createWorkspace = useMutation({
    mutationFn: () => api.createWorkspace(name.trim(), slug),
    onSuccess: (ws) => {
      // Seed the cache so the new card shows immediately, then refetch for
      // the server's authoritative list.
      queryClient.setQueryData<Workspace[]>(["workspaces"], (prev) => [
        ...(prev ?? []).filter((w) => w.id !== ws.id),
        ws,
      ]);
      void queryClient.invalidateQueries({ queryKey: ["workspaces"] });
      onSelect(ws.slug);
      setCreatingOpen(false);
      resetForm();
    },
    onError: (e) => setError(e),
  });

  const submitDisabled =
    createWorkspace.isPending || !name.trim() || slug.length < 2 || !SLUG_PATTERN.test(slug);

  return (
    <div className="mb-5">
      <div className="text-sm text-muted-foreground">Workspace</div>
      <p className="mt-0.5 text-xs text-muted-foreground">
        Connected repositories create their projects in this workspace.
      </p>
      <div className="mt-2 flex flex-wrap gap-2">
        {workspaces.map((ws) => {
          const selected = ws.slug === activeSlug;
          return (
            <button
              key={ws.slug}
              type="button"
              aria-pressed={selected}
              data-testid={`workspace-card-${ws.slug}`}
              onClick={() => onSelect(ws.slug)}
              className={`flex flex-col items-start rounded-lg border px-3 py-2 text-left transition-colors cursor-pointer bg-transparent ${
                selected
                  ? "border-primary/50 bg-primary/5 ring-1 ring-primary/20"
                  : "border-border/50 hover:border-border"
              }`}
            >
              <span className="text-sm font-medium">{ws.name}</span>
              <span className="text-xs text-muted-foreground">{ws.slug}</span>
            </button>
          );
        })}
        <button
          type="button"
          aria-pressed={creatingOpen}
          data-testid="workspace-card-new"
          onClick={() => {
            setCreatingOpen((v) => !v);
            setError(null);
          }}
          className={`flex items-center gap-1.5 rounded-lg border border-dashed px-3 py-2 text-sm transition-colors cursor-pointer bg-transparent ${
            creatingOpen
              ? "border-primary/50 bg-primary/5 text-foreground ring-1 ring-primary/20"
              : "border-border/50 text-muted-foreground hover:border-border hover:text-foreground"
          }`}
        >
          <Plus className="h-4 w-4" />
          New workspace…
        </button>
      </div>

      {creatingOpen && (
        <div className="mt-3 rounded-lg border border-border/50 p-3">
          <div className="flex flex-col gap-3 sm:flex-row">
            <div className="flex-1">
              <Label htmlFor="new-workspace-name" className="text-muted-foreground">
                Workspace name
              </Label>
              <Input
                id="new-workspace-name"
                data-testid="new-workspace-name"
                className="mt-1"
                value={name}
                autoFocus
                onChange={(e: React.ChangeEvent<HTMLInputElement>) => {
                  setName(e.target.value);
                  if (!slugTouched) setSlug(slugify(e.target.value));
                }}
                placeholder="My Team"
              />
            </div>
            <div className="flex-1">
              <Label htmlFor="new-workspace-slug" className="text-muted-foreground">
                Handle
              </Label>
              <Input
                id="new-workspace-slug"
                data-testid="new-workspace-slug"
                className="mt-1"
                value={slug}
                onChange={(e: React.ChangeEvent<HTMLInputElement>) => {
                  setSlug(e.target.value.toLowerCase());
                  setSlugTouched(true);
                }}
                placeholder="my-team"
                autoComplete="off"
                spellCheck={false}
              />
            </div>
          </div>
          {slugInvalid && (
            <p className="mt-2 text-xs text-destructive">
              Use 2–64 lowercase letters, numbers, and hyphens.
            </p>
          )}
          {error != null && (
            <ErrorNotice
              error={error}
              variant="inline"
              className="mt-2"
              data-testid="new-workspace-error"
            />
          )}
          <div className="mt-3 flex items-center gap-2">
            <Button
              size="sm"
              data-testid="new-workspace-create"
              disabled={submitDisabled}
              onClick={() => createWorkspace.mutate()}
            >
              {createWorkspace.isPending ? "Creating…" : "Create workspace"}
            </Button>
            <Button
              size="sm"
              variant="ghost"
              onClick={() => {
                setCreatingOpen(false);
                resetForm();
              }}
            >
              Cancel
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}

export function GithubSetupRoute() {
  const { installation_id: rawInstallationId, setup_action: setupAction } = useSearch({
    strict: false,
  }) as {
    installation_id?: string | number;
    setup_action?: "install" | "update";
  };
  const installationId = coerceInstallationId(rawInstallationId);
  const api = useApi();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { user, setUser } = useAuth();
  const { capture } = useAnalytics();

  // A GitHub redirect always carries setup_action; landing with it but
  // without an installation id means the handoff lost the id — count it so
  // a broken redirect shows up in analytics rather than as silent churn.
  useEffect(() => {
    if (setupAction && !installationId) {
      capture(AnalyticsEvents.githubSetupInstallationMissing, { setup_action: setupAction });
    }
    // Mount-only: the search params are fixed for the lifetime of the visit.
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

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

  // --- No installation id: opened out of context or the redirect lost it. --
  if (!installationId) {
    return (
      <Shell>
        <p className="text-sm text-muted-foreground">
          {setupAction
            ? "GitHub sent you here without the installation id, so the repositories cannot be listed yet. It comes back with one round-trip through the installation's settings:"
            : "This page is where GitHub sends you after installing the app. If the app is already installed, reopen this page from the installation's settings:"}
        </p>
        <ol
          className="mt-3 list-decimal space-y-1.5 pl-5 text-sm text-muted-foreground"
          data-testid="installation-id-recovery"
        >
          <li>
            Open{" "}
            <a
              className="underline underline-offset-2"
              href="https://github.com/settings/installations"
            >
              github.com/settings/installations
            </a>{" "}
            (for an organization install: the organization&apos;s Settings → GitHub Apps).
          </li>
          <li>
            Choose <span className="font-medium text-foreground">Configure</span> next to Bowrain.
          </li>
          <li>
            Press <span className="font-medium text-foreground">Save</span> — GitHub redirects back
            here with the installation id filled in.
          </li>
        </ol>
        <p className="mt-4 text-sm text-muted-foreground">
          Not installed yet? Install{" "}
          <a
            className="underline underline-offset-2"
            href="https://github.com/apps/bowrain-cloud/installations/new"
          >
            the Bowrain app
          </a>{" "}
          on a repository and GitHub brings you back here automatically.
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
      <WorkspaceSection
        workspaces={workspaces.data ?? []}
        activeSlug={activeSlug}
        onSelect={setWorkspaceSlug}
      />

      {repos.isLoading && <Spinner />}
      {repos.isError && (
        <ErrorNotice
          error={repos.error}
          title="Could not list the installation's repositories"
          variant="inline"
        />
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
  const navigate = useNavigate();
  // Default to creating a project named after the repo — the first-time path
  // must be one obvious click; picking an existing project stays available.
  const [projectId, setProjectId] = useState("__new__");
  const [error, setError] = useState<unknown>(null);
  // Set once this session's Connect succeeded: the row then tracks the
  // server's background ingest instead of showing a bare "connected" badge.
  const [importing, setImporting] = useState<{ connectorId: string; projectId: string } | null>(
    null,
  );

  const repoShortName = repo.full_name.slice(repo.full_name.lastIndexOf("/") + 1);
  const [newName, setNewName] = useState(repoShortName);

  const bind = useMutation({
    mutationFn: (pid: string) =>
      api.bindInstallationRepo(workspaceSlug, installationId, {
        repository: repo.full_name,
        project_id: pid,
      }),
    onSuccess: (res) => {
      setImporting({ connectorId: res.connector_id, projectId: res.project_id });
      onChanged();
    },
    onError: (e) => setError(e),
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
    onSuccess: (res) => {
      setImporting({ connectorId: res.connector_id, projectId: res.project_id });
      onChanged();
    },
    onError: (e) => setError(e),
  });

  // While the background ingest runs, poll the connector's status — the same
  // surface the connectors panel reads. A successful first fetch stamps
  // lastSync; a failure lands in errors.
  const importStatus = useQuery({
    queryKey: ["github-setup-import", workspaceSlug, importing?.connectorId],
    queryFn: () => api.getConnectorStatus(workspaceSlug, importing?.connectorId ?? ""),
    enabled: !!importing,
    refetchInterval: 2000,
  });

  // Retry a failed import through the same fetch the connectors panel's
  // "Fetch now" uses; success stamps lastSync and the poll takes it from there.
  const retryFetch = useMutation({
    mutationFn: () =>
      api.fetchConnector(workspaceSlug, importing?.connectorId ?? "", importing?.projectId ?? ""),
    onSettled: () => {
      void importStatus.refetch();
    },
  });

  const importPhase = !importing
    ? null
    : retryFetch.isPending
      ? "pending"
      : importPhaseFromStatus(importStatus.data);

  // The repository's content is in: hand over to the project page, where the
  // dashboard and live run progress pick the story up.
  useEffect(() => {
    if (!importing || importPhase !== "ready") return;
    void navigate({
      to: "/$workspace/p/$projectId/s/$stream",
      params: { workspace: workspaceSlug, projectId: importing.projectId, stream: "main" },
    });
  }, [importing, importPhase, navigate, workspaceSlug]);

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
        {error != null && (
          <ErrorNotice
            error={error}
            variant="inline"
            className="mt-1"
            data-testid={`connect-error-${repo.full_name}`}
          />
        )}
        {importPhase === "failed" && (
          <div className="mt-1 text-xs text-destructive">
            {importStatus.data?.errors[0] ?? "The import did not complete."} The repository stays
            connected — retry here or from the{" "}
            <Link
              to="/$workspace/p/$projectId/s/$stream/connectors"
              params={{
                workspace: workspaceSlug,
                projectId: importing?.projectId ?? "",
                stream: "main",
              }}
              className="underline underline-offset-2"
            >
              connectors panel
            </Link>
            .
          </div>
        )}
      </div>

      {importing ? (
        importPhase === "failed" ? (
          <div className="flex shrink-0 items-center gap-2">
            <Badge variant="destructive">import failed</Badge>
            <Button
              size="sm"
              variant="outline"
              onClick={() => retryFetch.mutate()}
              data-testid={`retry-import-${repo.full_name}`}
            >
              Retry import
            </Button>
          </div>
        ) : (
          <div
            className="flex shrink-0 items-center gap-2 text-sm text-muted-foreground"
            data-testid={`importing-${repo.full_name}`}
          >
            <Loader2 className="h-4 w-4 animate-spin" />
            Importing your repo…
          </div>
        )
      ) : repo.connector_id ? (
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
          {projects.length > 0 && (
            <div className="relative">
              <select
                value={projectId}
                onChange={(e: React.ChangeEvent<HTMLSelectElement>) => setProjectId(e.target.value)}
                aria-label="Project"
                data-testid={`project-select-${repo.full_name}`}
                className="h-8 w-44 cursor-pointer appearance-none rounded-lg border border-input bg-transparent pl-2.5 pr-8 text-sm text-foreground outline-none transition-colors focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50 dark:bg-input/30 dark:hover:bg-input/50"
              >
                {projects.map((p) => (
                  <option key={p.id} value={p.id}>
                    {p.name}
                  </option>
                ))}
                <option value="__new__">+ New project…</option>
              </select>
              <ChevronDown className="pointer-events-none absolute right-2 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
            </div>
          )}
          {projectId === "__new__" ? (
            <>
              <Input
                value={newName}
                onChange={(e: React.ChangeEvent<HTMLInputElement>) => setNewName(e.target.value)}
                aria-label="Project name"
                data-testid={`new-project-name-${repo.full_name}`}
                className="h-8 w-40"
              />
              <Button
                size="sm"
                disabled={!newName.trim() || createAndBind.isPending || bind.isPending}
                onClick={() => createAndBind.mutate(newProjectData(newName.trim()))}
                data-testid={`create-and-connect-${repo.full_name}`}
              >
                {createAndBind.isPending ? "Creating…" : "Create & connect"}
              </Button>
            </>
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
    </div>
  );
}
