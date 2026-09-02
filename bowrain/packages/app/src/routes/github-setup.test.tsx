import { describe, it, expect, vi, afterEach } from "vite-plus/test";
import { act, render, screen, fireEvent, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  AnalyticsProvider,
  ApiProvider,
  AuthProvider,
  apiErrorFromResponse,
  type ApiAdapter,
  type User,
  type Workspace,
} from "@neokapi/ui";
import { GithubSetupRoute } from "./github-setup";

// The route reads its search params from the router and navigates on import
// completion; neither needs a real router instance. `search.value` is
// mutable so individual tests can drop the id or add setup_action.
const search = vi.hoisted(() => ({
  value: { installation_id: 147350515 } as Record<string, unknown>,
}));
const router = vi.hoisted(() => ({ navigate: vi.fn() }));
vi.mock("@tanstack/react-router", () => ({
  useSearch: () => search.value,
  useNavigate: () => router.navigate,
  Link: ({ children }: { children?: React.ReactNode }) => <a>{children}</a>,
}));

afterEach(() => {
  search.value = { installation_id: 147350515 };
  router.navigate.mockClear();
});

const user: User = {
  id: "user-1",
  email: "ada@example.com",
  name: "Ada Lovelace",
  avatar_url: "",
  onboarded_at: "2026-01-01T00:00:00Z",
};

function ws(slug: string, name: string): Workspace {
  return {
    id: `ws-${slug}`,
    name,
    slug,
    description: "",
    logo_url: "",
    type: "personal",
    role: "owner",
  };
}

const repo = {
  full_name: "acme/website",
  default_branch: "main",
  private: false,
};

/** What the detect endpoint reports for the default single-repo fixture. */
const detection = {
  monorepo_markers: [] as string[],
  workspaces: [] as string[],
  signals: [{ id: "locale-dir", label: "locale catalogs", dir: "src/locales", files: 2 }],
  proposed_patterns: "src/locales/**/*.json",
  match_count: 2,
  match_preview: ["src/locales/en.json", "src/locales/fr.json"],
  truncated: false,
};

/** The exact error the founder drill hit: the initial clone failed server-side. */
const CLONE_FAILURE =
  "git clone: fatal: could not read Username for 'https://github.com': terminal prompts disabled\n: exit status 128";

function setup({
  workspaces = [ws("ada", "Ada Lovelace")],
  projects = [] as { id: string; name: string }[],
  overrides = {} as Partial<Record<keyof ApiAdapter, unknown>>,
  capture = undefined as ((event: string, props?: Record<string, unknown>) => void) | undefined,
} = {}) {
  // Mutable backing list so a createWorkspace + refetch round-trip keeps the
  // new workspace, as the server would.
  const workspaceList = [...workspaces];
  const api = {
    listWorkspaces: vi.fn().mockImplementation(() => Promise.resolve([...workspaceList])),
    listProjects: vi.fn().mockImplementation(() => Promise.resolve(projects)),
    githubSetupState: vi.fn().mockResolvedValue({ state: "signed-state", expires_in: 1800 }),
    claimInstallation: vi.fn().mockResolvedValue({ installation_id: 147350515, account: "ada" }),
    listInstallationRepos: vi.fn().mockResolvedValue([repo]),
    detectInstallationRepo: vi.fn().mockResolvedValue(detection),
    createWorkspace: vi.fn().mockImplementation((name: string, slug: string) => {
      const created = ws(slug, name);
      workspaceList.push(created);
      return Promise.resolve(created);
    }),
    createProject: vi.fn().mockResolvedValue({ id: "p-9", name: "website" }),
    bindInstallationRepo: vi.fn().mockResolvedValue({ connector_id: "c-1", project_id: "p-9" }),
    getConnectorStatus: vi.fn().mockResolvedValue({ lastSync: "", errors: [] }),
    fetchConnector: vi.fn().mockResolvedValue({ items_fetched: 0 }),
    ...overrides,
  } as unknown as ApiAdapter;
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={queryClient}>
      <ApiProvider adapter={api}>
        <AuthProvider initialUser={user}>
          <AnalyticsProvider capture={capture}>
            <GithubSetupRoute />
          </AnalyticsProvider>
        </AuthProvider>
      </ApiProvider>
    </QueryClientProvider>,
  );
  return { api, queryClient };
}

describe("GithubSetupRoute missing installation id", () => {
  it("shows the exact recovery path when GitHub redirected without the id", async () => {
    search.value = { setup_action: "install" };
    setup();

    const steps = await screen.findByTestId("installation-id-recovery");
    expect(steps).toHaveTextContent("github.com/settings/installations");
    expect(steps).toHaveTextContent("Configure");
    expect(steps).toHaveTextContent("Save");
    expect(screen.getByRole("link", { name: "github.com/settings/installations" })).toHaveAttribute(
      "href",
      "https://github.com/settings/installations",
    );
    // Came from GitHub — the copy says the redirect lost the id.
    expect(screen.getByText(/GitHub sent you here without the installation id/)).toBeVisible();
  });

  it("shows the same recovery steps for an out-of-context visit, plus the install link", async () => {
    search.value = {};
    setup();

    await screen.findByTestId("installation-id-recovery");
    expect(screen.getByText(/This page is where GitHub sends you/)).toBeVisible();
    // The install link carries the signed state naming this workspace, so the
    // installation comes back attributable rather than anonymous.
    await waitFor(() =>
      expect(screen.getByRole("link", { name: "the Bowrain app" })).toHaveAttribute(
        "href",
        "https://github.com/apps/bowrain-cloud/installations/new?state=signed-state",
      ),
    );
  });

  it("emits github_setup_installation_missing when setup_action arrives without an id", async () => {
    search.value = { setup_action: "update" };
    const capture = vi.fn();
    setup({ capture });

    await screen.findByTestId("installation-id-recovery");
    expect(capture).toHaveBeenCalledWith("github_setup_installation_missing", {
      setup_action: "update",
    });
  });

  it("does not emit the missing-id event on a normal redirect or a plain visit", async () => {
    const capture = vi.fn();
    search.value = { installation_id: 147350515, setup_action: "install" };
    setup({ capture });
    await screen.findByTestId("workspace-card-ada");

    search.value = {};
    setup({ capture });
    await waitFor(() => expect(screen.getByTestId("installation-id-recovery")).toBeVisible());

    expect(capture).not.toHaveBeenCalledWith(
      "github_setup_installation_missing",
      expect.anything(),
    );
  });
});

describe("GithubSetupRoute setup_action=update", () => {
  it("behaves exactly like install: repos load for the id and bind normally", async () => {
    search.value = { installation_id: 147350515, setup_action: "update" };
    const { api } = setup({ projects: [{ id: "p-1", name: "Website" }] });

    await waitFor(() => expect(api.listInstallationRepos).toHaveBeenCalledWith("ada", "147350515"));
    const select = await screen.findByTestId("project-select-acme/website");
    fireEvent.change(select, { target: { value: "p-1" } });
    fireEvent.click(screen.getByRole("button", { name: "Connect" }));
    await waitFor(() =>
      expect(api.bindInstallationRepo).toHaveBeenCalledWith(
        "ada",
        "147350515",
        expect.objectContaining({
          repository: "acme/website",
          project_id: "p-1",
        }),
      ),
    );
  });
});

describe("GithubSetupRoute installation ownership", () => {
  it("redeems the signed state before reading the installation", async () => {
    search.value = { installation_id: 147350515, setup_action: "install", state: "echoed-state" };
    const { api } = setup();

    await waitFor(() =>
      expect(api.claimInstallation).toHaveBeenCalledWith("ada", "147350515", "echoed-state"),
    );
    // Only once the installation belongs to the workspace is it read.
    await waitFor(() => expect(api.listInstallationRepos).toHaveBeenCalledWith("ada", "147350515"));
  });

  it("does not claim when GitHub sent no state", async () => {
    search.value = { installation_id: 147350515, setup_action: "install" };
    const { api } = setup();

    await waitFor(() => expect(api.listInstallationRepos).toHaveBeenCalled());
    expect(api.claimInstallation).not.toHaveBeenCalled();
  });

  it("offers a reconnect path when the installation is not this workspace's", async () => {
    search.value = { installation_id: 147350515 };
    setup({
      overrides: {
        listInstallationRepos: vi
          .fn()
          .mockRejectedValue(
            apiErrorFromResponse(404, JSON.stringify({ error: "installation not found" }), null),
          ),
      },
    });

    const notice = await screen.findByTestId("installation-not-connected");
    expect(notice).toHaveTextContent("not connected to");
    // Reconnecting carries the signed state, which is what attributes the
    // installation to this workspace on the way back.
    await waitFor(() =>
      expect(screen.getByRole("link", { name: "Connect this installation" })).toHaveAttribute(
        "href",
        "https://github.com/apps/bowrain-cloud/installations/new?state=signed-state",
      ),
    );
  });
});

describe("GithubSetupRoute workspace chooser", () => {
  it("renders a card per workspace and switches the selection on click", async () => {
    const { api } = setup({
      workspaces: [ws("ada", "Ada Lovelace"), ws("acme", "Acme Corp")],
    });

    const first = await screen.findByTestId("workspace-card-ada");
    const second = screen.getByTestId("workspace-card-acme");
    expect(first).toHaveAttribute("aria-pressed", "true");
    expect(second).toHaveAttribute("aria-pressed", "false");

    fireEvent.click(second);

    await waitFor(() =>
      expect(screen.getByTestId("workspace-card-acme")).toHaveAttribute("aria-pressed", "true"),
    );
    expect(screen.getByTestId("workspace-card-ada")).toHaveAttribute("aria-pressed", "false");
    await waitFor(() => expect(api.listProjects).toHaveBeenCalledWith("acme"));
    await waitFor(() =>
      expect(api.listInstallationRepos).toHaveBeenCalledWith("acme", "147350515"),
    );
  });

  it("shows a single workspace as the selected card", async () => {
    setup({ workspaces: [ws("ada", "Ada Lovelace")] });

    const card = await screen.findByTestId("workspace-card-ada");
    expect(card).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByText("ada")).toBeInTheDocument();
  });

  it("creates a workspace inline, selects it, and collapses the form", async () => {
    const { api } = setup();

    fireEvent.click(await screen.findByTestId("workspace-card-new"));
    fireEvent.change(screen.getByTestId("new-workspace-name"), {
      target: { value: "Team Rocket" },
    });
    // The handle is suggested from the name.
    expect(screen.getByTestId("new-workspace-slug")).toHaveValue("team-rocket");

    fireEvent.click(screen.getByTestId("new-workspace-create"));

    await waitFor(() =>
      expect(api.createWorkspace).toHaveBeenCalledWith("Team Rocket", "team-rocket"),
    );
    const created = await screen.findByTestId("workspace-card-team-rocket");
    expect(created).toHaveAttribute("aria-pressed", "true");
    expect(screen.queryByTestId("new-workspace-name")).not.toBeInTheDocument();
  });

  it("surfaces a server error inline and keeps the form open", async () => {
    const { api } = setup({
      overrides: {
        createWorkspace: vi.fn().mockRejectedValue(new Error("slug already taken")),
      },
    });

    fireEvent.click(await screen.findByTestId("workspace-card-new"));
    fireEvent.change(screen.getByTestId("new-workspace-name"), {
      target: { value: "Team Rocket" },
    });
    fireEvent.click(screen.getByTestId("new-workspace-create"));

    const notice = await screen.findByTestId("new-workspace-error");
    expect(notice).toHaveTextContent(/slug already taken/i);
    expect(api.createWorkspace).toHaveBeenCalled();
    expect(screen.getByTestId("new-workspace-name")).toBeInTheDocument();
  });
});

describe("GithubSetupRoute project path", () => {
  it("creates and binds a project inline with the repo's short name prefilled", async () => {
    const { api } = setup();

    const nameInput = await screen.findByTestId("new-project-name-acme/website");
    expect(nameInput).toHaveValue("website");

    fireEvent.click(screen.getByTestId("create-and-connect-acme/website"));

    await waitFor(() =>
      expect(api.createProject).toHaveBeenCalledWith("ada", "website", "en", ["fr"]),
    );
    await waitFor(() =>
      expect(api.bindInstallationRepo).toHaveBeenCalledWith(
        "ada",
        "147350515",
        expect.objectContaining({
          repository: "acme/website",
          project_id: "p-9",
        }),
      ),
    );
    // The row hands over to the import tracker.
    await screen.findByTestId("importing-acme/website");
  });

  it("binds an existing project chosen from the native select", async () => {
    const { api } = setup({ projects: [{ id: "p-1", name: "Website" }] });

    const select = await screen.findByTestId("project-select-acme/website");
    // "+ New project…" is the default: the inline create form is showing.
    expect(select).toHaveValue("__new__");
    expect(screen.getByTestId("new-project-name-acme/website")).toBeInTheDocument();

    fireEvent.change(select, { target: { value: "p-1" } });
    expect(screen.queryByTestId("new-project-name-acme/website")).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Connect" }));

    await waitFor(() =>
      expect(api.bindInstallationRepo).toHaveBeenCalledWith(
        "ada",
        "147350515",
        expect.objectContaining({
          repository: "acme/website",
          project_id: "p-1",
        }),
      ),
    );
    expect(api.createProject).not.toHaveBeenCalled();
    await screen.findByTestId("importing-acme/website");
  });

  it("shows the project-limit copy and reference when Create & connect hits the plan limit", async () => {
    setup({
      overrides: {
        createProject: vi.fn().mockRejectedValue(
          apiErrorFromResponse(
            403,
            JSON.stringify({
              current: 1,
              error: "project_limit_reached",
              limit: 1,
              message: "This workspace's plan allows 1 project(s) and it already has 1.",
              reference: "req-limit-1",
            }),
          ),
        ),
      },
    });

    fireEvent.click(await screen.findByTestId("create-and-connect-acme/website"));

    const notice = await screen.findByTestId("connect-error-acme/website");
    expect(notice).toHaveTextContent("This workspace's plan allows 1 project and already has 1");
    expect(notice).toHaveTextContent("Create a new workspace, choose another, or get in touch.");
    expect(screen.getByTestId("connect-error-acme/website-reference")).toHaveTextContent(
      "req-limit-1",
    );
    // Never the raw JSON the incident showed.
    expect(notice.textContent).not.toContain('{"current"');
  });
});

/**
 * This route is covered here rather than in `bowrain/apps/web/e2e` because the
 * e2e stack cannot reach GitHub: it configures no GitHub App, and every
 * installation endpoint mints a token against api.github.com. Issue #1972 names
 * the two pieces that would make the flow drivable end-to-end.
 */
describe("GithubSetupRoute starter prompt", () => {
  /** What the connector reports once the initial import has landed. */
  const importedStatus = { lastSync: "2026-08-11T09:00:00Z", errors: [] as string[] };

  it("offers the assistant prompt with the repository folded in, the moment it binds", async () => {
    setup();

    fireEvent.click(await screen.findByTestId("create-and-connect-acme/website"));

    const panel = await screen.findByTestId("github-setup-connected");
    expect(panel).toHaveTextContent("Build your brand starter pack with your AI");
    const prompt = screen.getByTestId("starter-prompt").textContent ?? "";
    expect(prompt).toContain("Install the kapi skill");
    expect(prompt).toContain("read https://github.com/acme/website");
    expect(prompt).toContain("the Ada Lovelace workspace");
    // The import is still running, and the prompt does not wait for it.
    expect(screen.getByTestId("importing-acme/website")).toBeInTheDocument();
  });

  it("keeps the user on the hand-over screen and offers the project explicitly", async () => {
    setup({ overrides: { getConnectorStatus: vi.fn().mockResolvedValue(importedStatus) } });

    fireEvent.click(await screen.findByTestId("create-and-connect-acme/website"));

    const open = await screen.findByTestId("github-setup-open-project");
    expect(screen.getByTestId("imported-acme/website")).toHaveTextContent("imported");
    // Nothing navigated on its own — the prompt would have gone unread.
    expect(router.navigate).not.toHaveBeenCalled();

    fireEvent.click(open);
    expect(router.navigate).toHaveBeenCalledWith({
      to: "/$workspace/p/$projectId/s/$stream",
      params: { workspace: "ada", projectId: "p-9", stream: "main" },
    });
  });

  it("offers nothing before a repository is connected", async () => {
    setup();

    await screen.findByTestId("create-and-connect-acme/website");
    expect(screen.queryByTestId("github-setup-connected")).not.toBeInTheDocument();
  });

  it("copies the prompt to the clipboard", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", {
      value: { writeText },
      configurable: true,
    });
    setup();

    fireEvent.click(await screen.findByTestId("create-and-connect-acme/website"));
    fireEvent.click(await screen.findByTestId("copy-starter-prompt"));

    await waitFor(() => expect(writeText).toHaveBeenCalledTimes(1));
    expect(writeText.mock.calls[0][0]).toContain("read https://github.com/acme/website");
  });
});

describe("GithubSetupRoute content detection", () => {
  it("auto-expands the single repo, shows the conclusion, proposal, and preview", async () => {
    const { api } = setup();

    const conclusion = await screen.findByTestId("detect-conclusion-acme/website");
    expect(conclusion).toHaveTextContent(
      "Detected: locale catalogs in src/locales · 2 files match",
    );
    expect(api.detectInstallationRepo).toHaveBeenCalledWith("ada", "147350515", "acme/website", {
      scope: undefined,
      patterns: undefined,
    });

    // The proposal pre-fills the editable patterns input.
    expect(screen.getByTestId("detect-patterns-acme/website")).toHaveValue("src/locales/**/*.json");
    const preview = screen.getByTestId("detect-preview-acme/website");
    expect(preview).toHaveTextContent("src/locales/en.json");
    expect(preview).toHaveTextContent("src/locales/fr.json");
    expect(screen.queryByTestId("detect-zero-match-acme/website")).not.toBeInTheDocument();
  });

  it("sends the detected proposal as the bind's patterns", async () => {
    const { api } = setup();

    await screen.findByTestId("detect-conclusion-acme/website");
    fireEvent.click(screen.getByTestId("create-and-connect-acme/website"));

    await waitFor(() =>
      expect(api.bindInstallationRepo).toHaveBeenCalledWith("ada", "147350515", {
        repository: "acme/website",
        project_id: "p-9",
        patterns: "src/locales/**/*.json",
      }),
    );
  });

  it("lets the user edit the patterns and binds with the edited value", async () => {
    const { api } = setup();

    await screen.findByTestId("detect-conclusion-acme/website");
    fireEvent.change(screen.getByTestId("detect-patterns-acme/website"), {
      target: { value: "docs/**/*.md,docs/**/*.mdx" },
    });
    fireEvent.click(screen.getByTestId("create-and-connect-acme/website"));

    await waitFor(() =>
      expect(api.bindInstallationRepo).toHaveBeenCalledWith("ada", "147350515", {
        repository: "acme/website",
        project_id: "p-9",
        patterns: "docs/**/*.md,docs/**/*.mdx",
      }),
    );
  });

  it("shows a plain warning when nothing matches instead of a silent empty import", async () => {
    setup({
      overrides: {
        detectInstallationRepo: vi.fn().mockResolvedValue({
          ...detection,
          signals: [],
          proposed_patterns: "**/*.json",
          match_count: 0,
          match_preview: [],
        }),
      },
    });

    const warning = await screen.findByTestId("detect-zero-match-acme/website");
    expect(warning).toHaveTextContent(/No translatable files match/);
    expect(screen.getByTestId("detect-conclusion-acme/website")).toHaveTextContent(
      "No known catalogs detected · 0 files match",
    );
  });

  it("offers a subdirectory scope for a monorepo and re-detects on selection", async () => {
    const scoped = {
      ...detection,
      monorepo_markers: ["pnpm-workspace.yaml"],
      workspaces: ["apps/api", "apps/web"],
      signals: [
        {
          id: "react-i18next",
          label: "i18next catalogs",
          dir: "apps/web/public/locales",
          files: 3,
        },
      ],
      proposed_patterns: "apps/web/public/locales/**/*.json",
      match_count: 3,
      match_preview: ["apps/web/public/locales/en/common.json"],
    };
    const { api } = setup({
      overrides: { detectInstallationRepo: vi.fn().mockResolvedValue(scoped) },
    });

    const conclusion = await screen.findByTestId("detect-conclusion-acme/website");
    expect(conclusion).toHaveTextContent(
      "Detected: pnpm monorepo · i18next catalogs in apps/web/public/locales · 3 files match",
    );

    fireEvent.change(screen.getByTestId("detect-scope-acme/website"), {
      target: { value: "apps/web" },
    });
    await waitFor(() =>
      expect(api.detectInstallationRepo).toHaveBeenCalledWith("ada", "147350515", "acme/website", {
        scope: "apps/web",
        patterns: undefined,
      }),
    );
  });
});

describe("GithubSetupRoute import lockout", () => {
  const docsRepo = { full_name: "acme/docs", default_branch: "main", private: false };

  it("locks the rest of the wizard while an import is in flight", async () => {
    setup({
      overrides: {
        listInstallationRepos: vi.fn().mockResolvedValue([repo, docsRepo]),
      },
    });

    fireEvent.click(await screen.findByTestId("create-and-connect-acme/website"));
    await screen.findByTestId("importing-acme/website");

    // The workspace cards lock…
    expect(screen.getByTestId("workspace-card-ada")).toBeDisabled();
    expect(screen.getByTestId("workspace-card-ada")).toHaveAttribute("aria-disabled", "true");
    expect(screen.getByTestId("workspace-card-new")).toBeDisabled();
    // …and so do the other row's controls.
    expect(screen.getByTestId("create-and-connect-acme/docs")).toBeDisabled();
    expect(screen.getByTestId("create-and-connect-acme/docs")).toHaveAttribute(
      "aria-disabled",
      "true",
    );
    expect(screen.getByTestId("new-project-name-acme/docs")).toBeDisabled();
    expect(screen.getByTestId("detect-toggle-acme/docs")).toBeDisabled();
  });

  it("keeps the lockout on failure but the importing row's Retry stays active", async () => {
    setup({
      overrides: {
        listInstallationRepos: vi.fn().mockResolvedValue([repo, docsRepo]),
        getConnectorStatus: vi.fn().mockResolvedValue({
          lastSync: "0001-01-01T00:00:00Z",
          errors: [CLONE_FAILURE],
        }),
      },
    });

    fireEvent.click(await screen.findByTestId("create-and-connect-acme/website"));

    const retry = await screen.findByTestId("retry-import-acme/website");
    expect(retry).toBeEnabled();
    expect(screen.getByTestId("create-and-connect-acme/docs")).toBeDisabled();
    expect(screen.getByTestId("workspace-card-ada")).toBeDisabled();
  });

  it("releases the lock when the connect itself fails", async () => {
    setup({
      overrides: {
        listInstallationRepos: vi.fn().mockResolvedValue([repo, docsRepo]),
        bindInstallationRepo: vi.fn().mockRejectedValue(new Error("boom")),
        createProject: vi.fn().mockRejectedValue(new Error("boom")),
      },
    });

    fireEvent.click(await screen.findByTestId("create-and-connect-acme/website"));

    await screen.findByTestId("connect-error-acme/website");
    expect(screen.getByTestId("create-and-connect-acme/docs")).toBeEnabled();
    expect(screen.getByTestId("workspace-card-ada")).toBeEnabled();
  });
});

describe("GithubSetupRoute import failure and progress", () => {
  it("surfaces the recorded server-side failure (production clone-error shape)", async () => {
    setup({
      overrides: {
        getConnectorStatus: vi.fn().mockResolvedValue({
          lastSync: "0001-01-01T00:00:00Z",
          errors: [CLONE_FAILURE],
        }),
      },
    });

    fireEvent.click(await screen.findByTestId("create-and-connect-acme/website"));

    // Never an endless "Importing…": the recorded error shows, with Retry and
    // the connectors-panel escape hatch.
    const failed = await screen.findByTestId("import-failed-acme/website");
    expect(screen.getByTestId("import-error-acme/website")).toHaveTextContent("exit status 128");
    expect(failed).toHaveTextContent("connectors panel");
    expect(screen.getByTestId("retry-import-acme/website")).toBeEnabled();
    expect(screen.queryByTestId("importing-acme/website")).not.toBeInTheDocument();
  });

  it("retries through the connectors fetch and returns to the pending state", async () => {
    const status = vi.fn().mockResolvedValue({
      lastSync: "0001-01-01T00:00:00Z",
      errors: [CLONE_FAILURE],
    });
    const { api } = setup({
      overrides: {
        getConnectorStatus: status,
        fetchConnector: vi.fn(
          () =>
            new Promise(() => {
              // Keeps the retry pending, pinning the row back on "Importing…".
            }),
        ),
      },
    });

    fireEvent.click(await screen.findByTestId("create-and-connect-acme/website"));
    fireEvent.click(await screen.findByTestId("retry-import-acme/website"));

    await screen.findByTestId("importing-acme/website");
    expect(api.fetchConnector).toHaveBeenCalledWith("ada", "c-1", "p-9");
  });

  it("shows elapsed seconds and admits slowness after 45s (fake timers)", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    try {
      setup();

      fireEvent.click(await screen.findByTestId("create-and-connect-acme/website"));
      await screen.findByTestId("importing-acme/website");
      expect(screen.queryByTestId("import-slow-acme/website")).not.toBeInTheDocument();

      await act(async () => {
        await vi.advanceTimersByTimeAsync(46_000);
      });

      const elapsed = screen.getByTestId("import-elapsed-acme/website").textContent ?? "";
      expect(Number.parseInt(elapsed, 10)).toBeGreaterThanOrEqual(45);
      expect(screen.getByTestId("import-slow-acme/website")).toHaveTextContent(
        /Taking longer than expected/,
      );
    } finally {
      vi.useRealTimers();
    }
  });

  it("stops pretending when the status poll itself keeps failing", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    try {
      setup({
        overrides: {
          getConnectorStatus: vi.fn().mockRejectedValue(new Error("connector not found")),
        },
      });

      fireEvent.click(await screen.findByTestId("create-and-connect-acme/website"));
      await screen.findByTestId("importing-acme/website");

      // Three failed polls at the 2s interval flip the row to failed.
      await act(async () => {
        await vi.advanceTimersByTimeAsync(10_000);
      });

      await screen.findByTestId("import-failed-acme/website");
      expect(screen.getByTestId("retry-import-acme/website")).toBeEnabled();
    } finally {
      vi.useRealTimers();
    }
  });
});
