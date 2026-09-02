import { useCallback, useEffect, useMemo, useState } from "react";
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
  StarterPromptCard,
  TEST_IDS,
  useAnalytics,
  useApi,
  useAuth,
  type InstallationRepo,
  type ProjectFormData,
  type RepoDetection,
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
 * layout — the @neokapi/i18n-react build transform recognised ICU authoring
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

/**
 * Where GitHub installs the app. Bowrain appends a signed `state` naming the
 * workspace that started the install; GitHub echoes it on the setup redirect,
 * which is what lets the returning visit claim the installation. An install
 * begun anywhere else (GitHub's own app page, the Marketplace) arrives without
 * state, and is reconnected by following this link from the workspace.
 */
const GITHUB_INSTALL_URL = "https://github.com/apps/bowrain-cloud/installations/new";

/** An installation this workspace does not own reads as a plain not-found. */
function isNotConnected(error: unknown): boolean {
  return (
    typeof error === "object" &&
    error !== null &&
    "status" in error &&
    (error as { status?: unknown }).status === 404
  );
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
              Translations come back as one pull request the loop keeps up to date, with no CI and
              no tokens.
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

/** After this many seconds of "Importing…" the row admits it is slow. */
const IMPORT_SLOW_AFTER_SECONDS = 45;

/**
 * How many consecutive failed status polls flip the row to "failed" when no
 * status body ever arrives (server unreachable, connector gone). A single
 * blip must not fail an import that is actually still running.
 */
const IMPORT_POLL_FAILURE_LIMIT = 3;

/** Debounce for re-counting matches while the user edits the patterns input. */
const PATTERNS_DEBOUNCE_MS = 400;

/** Human name for a monorepo marker file, for the detection conclusion. */
const MONOREPO_MARKER_NAMES: Record<string, string> = {
  "pnpm-workspace.yaml": "pnpm monorepo",
  "turbo.json": "Turborepo",
  "go.work": "Go workspace",
  "lerna.json": "Lerna monorepo",
};

/**
 * Workspace chooser: one selectable card per workspace plus a "New workspace"
 * card that expands an inline create form. Rendered even with a single
 * workspace so the user sees where the project will land. While a repository
 * import is in flight the whole section locks: switching workspaces would
 * tear down the rows mid-import.
 */
function WorkspaceSection({
  workspaces,
  activeSlug,
  onSelect,
  disabled = false,
}: {
  workspaces: Workspace[];
  activeSlug: string;
  onSelect: (slug: string) => void;
  disabled?: boolean;
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
    disabled ||
    createWorkspace.isPending ||
    !name.trim() ||
    slug.length < 2 ||
    !SLUG_PATTERN.test(slug);

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
              aria-disabled={disabled}
              disabled={disabled}
              data-testid={`workspace-card-${ws.slug}`}
              onClick={() => onSelect(ws.slug)}
              className={`flex flex-col items-start rounded-lg border px-3 py-2 text-left transition-colors bg-transparent disabled:cursor-not-allowed disabled:opacity-50 ${
                selected
                  ? "border-primary/50 bg-primary/5 ring-1 ring-primary/20"
                  : "border-border/50 hover:border-border"
              } ${disabled ? "" : "cursor-pointer"}`}
            >
              <span className="text-sm font-medium">{ws.name}</span>
              <span className="text-xs text-muted-foreground">{ws.slug}</span>
            </button>
          );
        })}
        <button
          type="button"
          aria-pressed={creatingOpen}
          aria-disabled={disabled}
          disabled={disabled}
          data-testid="workspace-card-new"
          onClick={() => {
            setCreatingOpen((v) => !v);
            setError(null);
          }}
          className={`flex items-center gap-1.5 rounded-lg border border-dashed px-3 py-2 text-sm transition-colors bg-transparent disabled:cursor-not-allowed disabled:opacity-50 ${
            creatingOpen
              ? "border-primary/50 bg-primary/5 text-foreground ring-1 ring-primary/20"
              : "border-border/50 text-muted-foreground hover:border-border hover:text-foreground"
          } ${disabled ? "" : "cursor-pointer"}`}
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
  const {
    installation_id: rawInstallationId,
    setup_action: setupAction,
    state: setupState,
  } = useSearch({
    strict: false,
  }) as {
    installation_id?: string | number;
    setup_action?: "install" | "update";
    state?: string;
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

  // The state has to survive the sign-in round trip too: a logged-out visitor
  // comes back through OIDC, and without it the claim below has nothing to
  // redeem.
  const setupPath =
    `/github/setup?installation_id=${installationId ?? ""}` +
    (setupState ? `&state=${encodeURIComponent(setupState)}` : "");

  const workspaces = useQuery({ ...workspacesQueryOptions(api), enabled: !!user });
  const [workspaceSlug, setWorkspaceSlug] = useState<string>("");
  const activeSlug = workspaceSlug || workspaces.data?.[0]?.slug || "";

  // Fresh state for an install started from here. It goes on the GitHub link
  // below so the installation comes back attributable to this workspace.
  const mintedState = useQuery({
    queryKey: ["github-setup-state", activeSlug],
    queryFn: () => api.githubSetupState(activeSlug),
    enabled: !!user && !!activeSlug,
    staleTime: 5 * 60 * 1000,
  });
  const installUrl = mintedState.data?.state
    ? `${GITHUB_INSTALL_URL}?state=${encodeURIComponent(mintedState.data.state)}`
    : GITHUB_INSTALL_URL;

  // Redeem the state GitHub echoed back, before anything reads the
  // installation. Every installation endpoint requires the workspace to own
  // the installation, and this is the only thing that makes it so; arriving
  // without state (installed straight from GitHub, say) simply leaves the
  // claim unmade, and the repo list below reports it.
  const claim = useQuery({
    queryKey: ["github-installation-claim", activeSlug, installationId, setupState],
    queryFn: () => api.claimInstallation(activeSlug, installationId ?? "", setupState ?? ""),
    enabled: !!user && !!activeSlug && !!installationId && !!setupState,
    retry: false,
    staleTime: Infinity,
  });
  const claimSettled = !setupState || claim.isSuccess || claim.isError;

  // One import at a time: while a repository's connect/import is in flight,
  // the rest of the wizard (workspace cards, other rows) locks so the user
  // can't fork the flow mid-import. Holds the importing repo's full_name.
  const [importingRepo, setImportingRepo] = useState<string | null>(null);

  // The repository this visit connected, and whether its content has landed.
  // Holding the user here is the point: a repository has just been handed over,
  // which is the one moment the assistant prompt below costs nothing to act on.
  const [connected, setConnected] = useState<{ repository: string; projectId: string } | null>(
    null,
  );
  const [imported, setImported] = useState(false);
  const handleConnected = useCallback((info: { repository: string; projectId: string }) => {
    setConnected(info);
    setImported(false);
  }, []);
  const handleImported = useCallback(() => setImported(true), []);

  const projects = useQuery({
    ...projectsQueryOptions(api, activeSlug),
    enabled: !!user && !!activeSlug,
  });
  const repos = useQuery({
    queryKey: ["github-installation-repos", activeSlug, installationId],
    queryFn: () => api.listInstallationRepos(activeSlug, installationId ?? ""),
    // Gated on the claim: listing before the installation belongs to this
    // workspace would just 404, and would cache that 404 as the answer.
    enabled: !!user && !!activeSlug && !!installationId && claimSettled,
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
            Press <span className="font-medium text-foreground">Save</span>. GitHub redirects back
            here with the installation id filled in.
          </li>
        </ol>
        <p className="mt-4 text-sm text-muted-foreground">
          Not installed yet? Install{" "}
          <a className="underline underline-offset-2" href={installUrl}>
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
          Welcome. Create your workspace to continue, and you&apos;ll come right back here to
          connect your repositories.
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
  const unboundCount = repos.data?.filter((r) => !r.connector_id).length ?? 0;
  const activeWorkspace = workspaces.data?.find((w) => w.slug === activeSlug);

  return (
    <Shell>
      <WorkspaceSection
        workspaces={workspaces.data ?? []}
        activeSlug={activeSlug}
        onSelect={setWorkspaceSlug}
        disabled={importingRepo !== null}
      />

      {(repos.isLoading || claim.isLoading) && <Spinner />}

      {/* The installation is not this workspace's — either it was installed
          outside Bowrain, or it belongs somewhere else entirely. Reconnecting
          from here sends the signed state along, which is what attributes it. */}
      {repos.isError && isNotConnected(repos.error) ? (
        <Card className="p-5" data-testid="installation-not-connected">
          <p className="text-sm text-muted-foreground">
            This GitHub installation is not connected to{" "}
            <span className="font-medium text-foreground">{activeSlug}</span>. Connect it from this
            workspace, and GitHub brings you straight back.
          </p>
          {/* Only offer the link once the state is minted: following it
              without one installs the app all over again and lands back
              here just as unattributed. */}
          {mintedState.data ? (
            <Button className="mt-4" asChild>
              <a href={installUrl}>Connect this installation</a>
            </Button>
          ) : (
            <Button className="mt-4" disabled>
              Connect this installation
            </Button>
          )}
        </Card>
      ) : (
        repos.isError && (
          <ErrorNotice
            error={repos.error}
            title="Could not list the installation's repositories"
            variant="inline"
          />
        )
      )}

      <div className="flex flex-col gap-3">
        {repos.data?.map((repo) => (
          <RepoRow
            key={repo.full_name}
            repo={repo}
            workspaceSlug={activeSlug}
            installationId={installationId}
            projects={projectList}
            autoExpand={unboundCount === 1}
            locked={importingRepo !== null && importingRepo !== repo.full_name}
            onConnected={handleConnected}
            onImported={handleImported}
            onImportStateChange={(active) => {
              setImportingRepo((current) => {
                if (active) return repo.full_name;
                return current === repo.full_name ? null : current;
              });
            }}
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

      {connected && (
        <div className="mt-5 flex flex-col gap-3" data-testid={TEST_IDS.onboarding.githubConnected}>
          <StarterPromptCard
            workspaceName={activeWorkspace?.name}
            serverUrl={window.location.origin}
            repoUrl={`https://github.com/${connected.repository}`}
          />
          {imported ? (
            <div>
              <Button
                onClick={() =>
                  void navigate({
                    to: "/$workspace/p/$projectId/s/$stream",
                    params: {
                      workspace: activeSlug,
                      projectId: connected.projectId,
                      stream: "main",
                    },
                  })
                }
                data-testid={TEST_IDS.onboarding.githubOpenProject}
              >
                Open the project
              </Button>
            </div>
          ) : (
            <p className="text-sm text-muted-foreground">
              The import runs in the background. The prompt above works while it does.
            </p>
          )}
        </div>
      )}
    </Shell>
  );
}

/** Debounce a changing value — live match feedback without a request per keystroke. */
function useDebounced<T>(value: T, delayMs: number): T {
  const [debounced, setDebounced] = useState(value);
  useEffect(() => {
    const t = setTimeout(() => setDebounced(value), delayMs);
    return () => clearTimeout(t);
  }, [value, delayMs]);
  return debounced;
}

/** The wizard's conclusion line for a detection, composed plainly. */
function detectionConclusion(d: RepoDetection): string {
  const parts: string[] = [];
  for (const marker of d.monorepo_markers ?? []) {
    const name = MONOREPO_MARKER_NAMES[marker];
    if (name) {
      parts.push(name);
      break;
    }
  }
  for (const sig of d.signals.slice(0, 2)) {
    parts.push(sig.dir ? `${sig.label} in ${sig.dir}` : sig.label);
  }
  const head = parts.length > 0 ? `Detected: ${parts.join(" · ")}` : "No known catalogs detected";
  const files = d.match_count === 1 ? "1 file matches" : `${d.match_count} files match`;
  return `${head} · ${files}`;
}

function RepoRow({
  repo,
  workspaceSlug,
  installationId,
  projects,
  autoExpand,
  locked,
  onConnected,
  onImported,
  onImportStateChange,
  onChanged,
}: {
  repo: InstallationRepo;
  workspaceSlug: string;
  installationId: string;
  projects: { id: string; name: string }[];
  /** Open the detection panel without a click (the single-repo first-run path). */
  autoExpand: boolean;
  /** Another repository's import is in flight: this row's controls lock. */
  locked: boolean;
  /** Reports the bind that tied this repository to a project. */
  onConnected: (info: { repository: string; projectId: string }) => void;
  /** Reports that this repository's content has landed in its project. */
  onImported: (info: { repository: string; projectId: string }) => void;
  /** Reports when this row's connect/import starts (true) or aborts (false). */
  onImportStateChange: (active: boolean) => void;
  onChanged: () => void;
}) {
  const api = useApi();
  // Default to creating a project named after the repo — the first-time path
  // must be one obvious click; picking an existing project stays available.
  const [projectId, setProjectId] = useState("__new__");
  const [error, setError] = useState<unknown>(null);
  // Set once this session's Connect succeeded: the row then tracks the
  // server's background ingest instead of showing a bare "connected" badge.
  const [importing, setImporting] = useState<{ connectorId: string; projectId: string } | null>(
    null,
  );
  // When the import started (Connect success or Retry), for honest elapsed
  // feedback while the clone runs.
  const [importStartedAt, setImportStartedAt] = useState<number | null>(null);
  const [now, setNow] = useState(() => Date.now());

  const repoShortName = repo.full_name.slice(repo.full_name.lastIndexOf("/") + 1);
  const [newName, setNewName] = useState(repoShortName);

  // --- Content detection (what would this bind import?) --------------------
  const [expanded, setExpanded] = useState(autoExpand && !repo.connector_id);
  const [scope, setScope] = useState("");
  const [patternsEdit, setPatternsEdit] = useState<string | null>(null);
  const debouncedPatterns = useDebounced(patternsEdit, PATTERNS_DEBOUNCE_MS);
  const detect = useQuery({
    queryKey: [
      "github-repo-detect",
      workspaceSlug,
      installationId,
      repo.full_name,
      scope,
      debouncedPatterns,
    ],
    queryFn: () =>
      api.detectInstallationRepo(workspaceSlug, installationId, repo.full_name, {
        scope: scope || undefined,
        patterns: debouncedPatterns ?? undefined,
      }),
    enabled: expanded && !repo.connector_id && !importing && !locked,
    staleTime: 60_000,
    placeholderData: (prev) => prev,
  });
  // What the bind sends: the user's edit, else the detected proposal. A row
  // bound without ever opening the panel sends none and the server defaults
  // apply.
  const bindPatterns = patternsEdit ?? detect.data?.proposed_patterns;

  const startImport = (res: { connector_id: string; project_id: string }) => {
    setImporting({ connectorId: res.connector_id, projectId: res.project_id });
    setImportStartedAt(Date.now());
    setNow(Date.now());
    onConnected({ repository: repo.full_name, projectId: res.project_id });
    onChanged();
  };

  const bind = useMutation({
    mutationFn: (pid: string) =>
      api.bindInstallationRepo(workspaceSlug, installationId, {
        repository: repo.full_name,
        project_id: pid,
        ...(bindPatterns ? { patterns: bindPatterns } : {}),
      }),
    onMutate: () => onImportStateChange(true),
    onSuccess: startImport,
    onError: (e) => {
      setError(e);
      onImportStateChange(false);
    },
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
        ...(bindPatterns ? { patterns: bindPatterns } : {}),
      });
    },
    onMutate: () => onImportStateChange(true),
    onSuccess: startImport,
    onError: (e) => {
      setError(e);
      onImportStateChange(false);
    },
  });

  // While the background ingest runs, poll the connector's status. This is
  // deliberately the CHEAP default read (stored row: last sync + recorded
  // ingest error — no probe flag), so a 2s poll never re-clones the repo
  // (#1362). A successful first fetch stamps lastSync; a recorded failure
  // lands in errors — so a broken import is never an endless "Importing…".
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
    onMutate: () => {
      setImportStartedAt(Date.now());
      setNow(Date.now());
    },
    onSettled: () => {
      void importStatus.refetch();
    },
  });

  const importPhase = !importing
    ? null
    : retryFetch.isPending
      ? "pending"
      : importStatus.data
        ? importPhaseFromStatus(importStatus.data)
        : // errorUpdateCount, not failureCount: failureCount is the per-fetch
          // retry counter and resets to 0 when each interval poll starts, so
          // it never accumulates across polls.
          importStatus.errorUpdateCount >= IMPORT_POLL_FAILURE_LIMIT
          ? // The poll itself keeps failing (server unreachable, connector
            // gone): stop pretending the import is progressing.
            "failed"
          : "pending";

  // Elapsed-time tick while the import is pending.
  useEffect(() => {
    if (!importStartedAt || importPhase !== "pending") return;
    const t = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(t);
  }, [importStartedAt, importPhase]);
  const elapsedSeconds = importStartedAt
    ? Math.max(0, Math.floor((now - importStartedAt) / 1000))
    : 0;

  // The repository's content is in. The page reports it and offers the way on
  // rather than jumping there itself: the hand-over screen is where the
  // assistant prompt lives, and a jump would take it away unread.
  useEffect(() => {
    if (!importing || importPhase !== "ready") return;
    onImported({ repository: repo.full_name, projectId: importing.projectId });
  }, [importing, importPhase, onImported, repo.full_name]);

  const boundProject = useMemo(
    () => projects.find((p) => p.id === repo.project_id),
    [projects, repo.project_id],
  );

  const showDetectPanel = expanded && !repo.connector_id && !importing;
  const previewLimit = 8;

  return (
    <div className="rounded-lg border p-3" data-testid={`repo-${repo.full_name}`}>
      <div className="flex items-center justify-between gap-3">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <span className="truncate text-sm font-medium">{repo.full_name}</span>
            {repo.private && <Badge variant="secondary">private</Badge>}
          </div>
          <div className="text-xs text-muted-foreground">tracked branch: {repo.default_branch}</div>
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
          ) : importPhase === "ready" ? (
            <div
              className="flex shrink-0 items-center gap-2"
              data-testid={`imported-${repo.full_name}`}
            >
              <Badge>imported</Badge>
            </div>
          ) : (
            <div
              className="flex shrink-0 items-center gap-2 text-sm text-muted-foreground"
              data-testid={`importing-${repo.full_name}`}
            >
              <Loader2 className="h-4 w-4 animate-spin" />
              <span>
                Importing your repo…{" "}
                <span className="tabular-nums" data-testid={`import-elapsed-${repo.full_name}`}>
                  {elapsedSeconds}s
                </span>
              </span>
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
            <button
              type="button"
              onClick={() => setExpanded((v) => !v)}
              aria-expanded={expanded}
              aria-disabled={locked}
              disabled={locked}
              data-testid={`detect-toggle-${repo.full_name}`}
              className="flex items-center gap-1 rounded-md px-1.5 py-1 text-xs text-muted-foreground transition-colors hover:text-foreground disabled:cursor-not-allowed disabled:opacity-50 bg-transparent cursor-pointer"
            >
              <ChevronDown
                className={`h-3.5 w-3.5 transition-transform ${expanded ? "rotate-180" : ""}`}
              />
              Preview import
            </button>
            {projects.length > 0 && (
              <div className="relative">
                <select
                  value={projectId}
                  onChange={(e: React.ChangeEvent<HTMLSelectElement>) =>
                    setProjectId(e.target.value)
                  }
                  aria-label="Project"
                  aria-disabled={locked}
                  disabled={locked}
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
                  aria-disabled={locked}
                  disabled={locked}
                  data-testid={`new-project-name-${repo.full_name}`}
                  className="h-8 w-40"
                />
                <Button
                  size="sm"
                  aria-disabled={locked}
                  disabled={locked || !newName.trim() || createAndBind.isPending || bind.isPending}
                  onClick={() => createAndBind.mutate(newProjectData(newName.trim()))}
                  data-testid={`create-and-connect-${repo.full_name}`}
                >
                  {createAndBind.isPending ? "Creating…" : "Create & connect"}
                </Button>
              </>
            ) : (
              <Button
                size="sm"
                aria-disabled={locked}
                disabled={locked || !projectId || bind.isPending}
                onClick={() => bind.mutate(projectId)}
              >
                {bind.isPending ? "Connecting…" : "Connect"}
              </Button>
            )}
          </div>
        )}
      </div>

      {error != null && (
        <ErrorNotice
          error={error}
          variant="inline"
          className="mt-2"
          data-testid={`connect-error-${repo.full_name}`}
        />
      )}

      {importing && importPhase === "pending" && elapsedSeconds >= IMPORT_SLOW_AFTER_SECONDS && (
        <p
          className="mt-2 text-xs text-muted-foreground"
          data-testid={`import-slow-${repo.full_name}`}
        >
          Taking longer than expected. Large repositories can take a few minutes.
        </p>
      )}

      {importPhase === "failed" && (
        <div className="mt-2" data-testid={`import-failed-${repo.full_name}`}>
          <ErrorNotice
            error={importStatus.data?.errors[0] ?? importStatus.error ?? undefined}
            title="The import failed"
            variant="inline"
            data-testid={`import-error-${repo.full_name}`}
          />
          <p className="mt-1 text-xs text-muted-foreground">
            The repository stays connected. Retry here or from the{" "}
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
          </p>
        </div>
      )}

      {showDetectPanel && (
        <div
          className="mt-3 rounded-lg border border-border/50 bg-muted/20 p-3"
          data-testid={`detect-panel-${repo.full_name}`}
        >
          {detect.isLoading && (
            <div className="flex items-center gap-2 text-xs text-muted-foreground">
              <Loader2 className="h-3.5 w-3.5 animate-spin" />
              Scanning the repository…
            </div>
          )}
          {detect.isError && (
            <ErrorNotice
              error={detect.error}
              title="Could not scan the repository"
              variant="inline"
              data-testid={`detect-error-${repo.full_name}`}
            />
          )}
          {detect.data && (
            <>
              <p className="text-xs" data-testid={`detect-conclusion-${repo.full_name}`}>
                {detectionConclusion(detect.data)}
              </p>

              {(detect.data.workspaces?.length ?? 0) > 0 && (
                <div className="mt-2 flex items-center gap-2">
                  <Label
                    htmlFor={`detect-scope-${repo.full_name}`}
                    className="text-xs text-muted-foreground"
                  >
                    Scope
                  </Label>
                  <div className="relative">
                    <select
                      id={`detect-scope-${repo.full_name}`}
                      value={scope}
                      onChange={(e: React.ChangeEvent<HTMLSelectElement>) => {
                        setScope(e.target.value);
                        // A new scope means a new proposal: drop the edit.
                        setPatternsEdit(null);
                      }}
                      aria-disabled={locked}
                      disabled={locked}
                      data-testid={`detect-scope-${repo.full_name}`}
                      className="h-7 cursor-pointer appearance-none rounded-md border border-input bg-transparent pl-2 pr-7 text-xs text-foreground outline-none transition-colors focus-visible:border-ring disabled:cursor-not-allowed disabled:opacity-50 dark:bg-input/30"
                    >
                      <option value="">Whole repository</option>
                      {(detect.data.workspaces ?? []).map((ws) => (
                        <option key={ws} value={ws}>
                          {ws}
                        </option>
                      ))}
                    </select>
                    <ChevronDown className="pointer-events-none absolute right-1.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
                  </div>
                </div>
              )}

              <div className="mt-2">
                <Label
                  htmlFor={`detect-patterns-${repo.full_name}`}
                  className="text-xs text-muted-foreground"
                >
                  Content patterns
                </Label>
                <Input
                  id={`detect-patterns-${repo.full_name}`}
                  value={patternsEdit ?? detect.data.proposed_patterns}
                  onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
                    setPatternsEdit(e.target.value)
                  }
                  aria-disabled={locked}
                  disabled={locked}
                  spellCheck={false}
                  autoComplete="off"
                  data-testid={`detect-patterns-${repo.full_name}`}
                  className="mt-1 h-8 font-mono text-xs"
                />
              </div>

              {detect.data.match_count > 0 ? (
                <ul
                  className="mt-2 space-y-0.5 font-mono text-[11px] text-muted-foreground"
                  data-testid={`detect-preview-${repo.full_name}`}
                >
                  {detect.data.match_preview.slice(0, previewLimit).map((p) => (
                    <li key={p} className="truncate">
                      {p}
                    </li>
                  ))}
                  {detect.data.match_count > previewLimit && (
                    <li>
                      … and {detect.data.match_count - previewLimit} more file
                      {detect.data.match_count - previewLimit === 1 ? "" : "s"}
                    </li>
                  )}
                </ul>
              ) : (
                <p
                  className="mt-2 text-xs font-medium text-amber-600 dark:text-amber-500"
                  data-testid={`detect-zero-match-${repo.full_name}`}
                >
                  No translatable files match these patterns. Adjust them before connecting, or the
                  import will bring in nothing.
                </p>
              )}

              {detect.data.truncated && (
                <p className="mt-1 text-[11px] text-muted-foreground">
                  Large repository: the scan read part of the tree, so counts are a lower bound.
                </p>
              )}
            </>
          )}
        </div>
      )}
    </div>
  );
}
