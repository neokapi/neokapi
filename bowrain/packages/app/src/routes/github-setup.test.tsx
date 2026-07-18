import { describe, it, expect, vi, afterEach } from "vite-plus/test";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
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
vi.mock("@tanstack/react-router", () => ({
  useSearch: () => search.value,
  useNavigate: () => vi.fn(),
  Link: ({ children }: { children?: React.ReactNode }) => <a>{children}</a>,
}));

afterEach(() => {
  search.value = { installation_id: 147350515 };
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
    listInstallationRepos: vi.fn().mockResolvedValue([repo]),
    createWorkspace: vi.fn().mockImplementation((name: string, slug: string) => {
      const created = ws(slug, name);
      workspaceList.push(created);
      return Promise.resolve(created);
    }),
    createProject: vi.fn().mockResolvedValue({ id: "p-9", name: "website" }),
    bindInstallationRepo: vi.fn().mockResolvedValue({ connector_id: "c-1", project_id: "p-9" }),
    getConnectorStatus: vi.fn().mockResolvedValue({ lastSync: "", errors: [] }),
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
    expect(screen.getByRole("link", { name: "the Bowrain app" })).toHaveAttribute(
      "href",
      "https://github.com/apps/bowrain-cloud/installations/new",
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
      expect(api.bindInstallationRepo).toHaveBeenCalledWith("ada", "147350515", {
        repository: "acme/website",
        project_id: "p-1",
      }),
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
      expect(api.bindInstallationRepo).toHaveBeenCalledWith("ada", "147350515", {
        repository: "acme/website",
        project_id: "p-9",
      }),
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
      expect(api.bindInstallationRepo).toHaveBeenCalledWith("ada", "147350515", {
        repository: "acme/website",
        project_id: "p-1",
      }),
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
    expect(notice).toHaveTextContent(
      "Create a new workspace, choose another, or upgrade the plan.",
    );
    expect(screen.getByTestId("connect-error-acme/website-reference")).toHaveTextContent(
      "req-limit-1",
    );
    // Never the raw JSON the incident showed.
    expect(notice.textContent).not.toContain('{"current"');
  });
});
