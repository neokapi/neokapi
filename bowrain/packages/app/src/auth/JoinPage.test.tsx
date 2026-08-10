import { describe, it, expect, vi } from "vite-plus/test";
import { render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  ApiProvider,
  AuthProvider,
  WorkspaceProvider,
  useWorkspace,
  type AcceptInviteResponse,
  type ApiAdapter,
  type User,
  type Workspace,
} from "@neokapi/ui";
import { JoinPage } from "./JoinPage";

/** Reports which workspace the page left selected. */
function ActiveWorkspaceProbe() {
  const { activeWorkspace } = useWorkspace();
  return <div data-testid="active-workspace">{activeWorkspace?.slug ?? "none"}</div>;
}

/**
 * The join confirmation names the workspace and the role from the accept
 * response. The server used to answer `{"status":"joined"}` alone, which read
 * as "You are now a undefined of undefined" and — because the same response
 * carries the slug — also left the joined workspace unselected.
 */

const user: User = {
  id: "user-1",
  email: "ada@example.com",
  name: "Ada Lovelace",
  avatar_url: "",
  onboarded_at: "2026-01-01T00:00:00Z",
};

const acme: Workspace = {
  id: "ws-1",
  name: "Acme Translations",
  slug: "acme",
  description: "",
  logo_url: "",
  type: "team",
  role: "admin",
};

function setup(accepted: Partial<AcceptInviteResponse>, workspaces: Workspace[] = [acme]) {
  const api = {
    getCurrentUser: vi.fn().mockResolvedValue(user),
    acceptInvite: vi.fn().mockResolvedValue(accepted as AcceptInviteResponse),
    listWorkspaces: vi.fn().mockResolvedValue(workspaces),
  } as unknown as ApiAdapter;

  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const onJoined = vi.fn();
  render(
    <QueryClientProvider client={queryClient}>
      <ApiProvider adapter={api}>
        <AuthProvider initialUser={user}>
          <WorkspaceProvider initialWorkspaces={[]}>
            <JoinPage code="abc123" onJoined={onJoined} />
            <ActiveWorkspaceProbe />
          </WorkspaceProvider>
        </AuthProvider>
      </ApiProvider>
    </QueryClientProvider>,
  );
  return { api, onJoined };
}

describe("JoinPage", () => {
  it("names the role and workspace the invite granted", async () => {
    setup({
      status: "joined",
      workspace_id: "ws-1",
      workspace_slug: "acme",
      workspace_name: "Acme Translations",
      role: "member",
    });

    await screen.findByText("Joined!");
    expect(screen.getByText(/You are now a member of/)).toBeInTheDocument();
    expect(screen.getByText("Acme Translations")).toBeInTheDocument();
    expect(screen.queryByText(/undefined/)).not.toBeInTheDocument();
  });

  it("uses the right article for a vowel-initial role", async () => {
    setup({
      status: "joined",
      workspace_id: "ws-1",
      workspace_slug: "acme",
      workspace_name: "Acme Translations",
      role: "admin",
    });

    await screen.findByText("Joined!");
    expect(screen.getByText(/You are now an admin of/)).toBeInTheDocument();
  });

  it("switches to the workspace the invite named", async () => {
    const { api } = setup({
      status: "joined",
      workspace_id: "ws-1",
      workspace_slug: "acme",
      workspace_name: "Acme Translations",
      role: "member",
    });

    await screen.findByText("Joined!");
    // Selecting the joined workspace keys on the slug from the same response,
    // so an acceptance that omitted it left the user on their previous one.
    await waitFor(() => expect(api.listWorkspaces).toHaveBeenCalled());
    await waitFor(() => expect(screen.getByTestId("active-workspace")).toHaveTextContent("acme"));
  });

  it("still confirms a join whose workspace the server could not name", async () => {
    setup({
      status: "joined",
      workspace_id: "ws-1",
      workspace_slug: "",
      workspace_name: "",
      role: "",
    });

    await screen.findByText("Joined!");
    expect(screen.getByText("You have joined the workspace")).toBeInTheDocument();
    expect(screen.queryByText(/undefined/)).not.toBeInTheDocument();
  });

  it("names the workspace even when the role is missing", async () => {
    setup({
      status: "joined",
      workspace_id: "ws-1",
      workspace_slug: "acme",
      workspace_name: "Acme Translations",
      role: "",
    });

    await screen.findByText("Joined!");
    expect(screen.getByText(/You have joined/)).toBeInTheDocument();
    expect(screen.getByText("Acme Translations")).toBeInTheDocument();
  });
});
