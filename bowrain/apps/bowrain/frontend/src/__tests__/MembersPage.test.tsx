/* eslint-disable @typescript-eslint/unbound-method -- asserting on vi.fn() mock references is intentional */
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, fireEvent, within } from "@testing-library/react";
import {
  ApiProvider,
  WorkspaceProvider,
  TooltipProvider,
  type ApiAdapter,
  type Membership,
  type Workspace,
} from "@neokapi/ui";
import { MembersPage } from "../components/MembersPage";

const teamWorkspace: Workspace = {
  id: "ws1",
  name: "Acme",
  slug: "acme",
  description: "",
  logo_url: "",
  type: "team",
  role: "owner",
};

const members: Membership[] = [
  {
    user_id: "u-owner",
    workspace_id: "ws1",
    role: "owner",
    user: { id: "u-owner", email: "owner@acme.test", name: "Olive Owner", avatar_url: "" },
  },
  {
    user_id: "u-member",
    workspace_id: "ws1",
    role: "member",
    user: { id: "u-member", email: "mel@acme.test", name: "Mel Member", avatar_url: "" },
  },
];

function makeAdapter(overrides: Partial<ApiAdapter> = {}): ApiAdapter {
  return {
    listMembers: vi.fn().mockResolvedValue(members),
    updateMemberRole: vi.fn().mockResolvedValue(undefined),
    removeMember: vi.fn().mockResolvedValue(undefined),
    listInvites: vi.fn().mockResolvedValue([]),
    createInvite: vi.fn(),
    deleteInvite: vi.fn(),
    ...overrides,
  } as unknown as ApiAdapter;
}

function renderPage(adapter: ApiAdapter, ws: Workspace = teamWorkspace) {
  return render(
    <TooltipProvider>
      <ApiProvider adapter={adapter}>
        <WorkspaceProvider initialWorkspace={ws}>
          <MembersPage />
        </WorkspaceProvider>
      </ApiProvider>
    </TooltipProvider>,
  );
}

describe("MembersPage", () => {
  beforeEach(() => vi.clearAllMocks());

  it("fetches and renders the member roster via the adapter", async () => {
    const adapter = makeAdapter();
    renderPage(adapter);

    expect(adapter.listMembers).toHaveBeenCalledWith("acme");
    await waitFor(() => expect(screen.getByText("Olive Owner")).toBeInTheDocument());
    expect(screen.getByText("Mel Member")).toBeInTheDocument();

    const list = screen.getByTestId("members-list");
    // Owner row shows a static badge; member row shows an editable role control.
    expect(within(list).getByTestId("member-role-u-member")).toBeInTheDocument();
    expect(within(list).queryByTestId("member-role-u-owner")).not.toBeInTheDocument();
  });

  it("removes a member through the adapter and drops the row", async () => {
    const adapter = makeAdapter();
    renderPage(adapter);

    await waitFor(() => expect(screen.getByText("Mel Member")).toBeInTheDocument());

    fireEvent.click(screen.getByTestId("member-remove-u-member"));

    await waitFor(() => expect(adapter.removeMember).toHaveBeenCalledWith("acme", "u-member"));
    await waitFor(() => expect(screen.queryByText("Mel Member")).not.toBeInTheDocument());
  });

  it("shows an error when loading members fails", async () => {
    const adapter = makeAdapter({
      listMembers: vi.fn().mockRejectedValue(new Error("boom")),
    });
    renderPage(adapter);

    await waitFor(() => expect(screen.getByText("boom")).toBeInTheDocument());
  });

  it("renders a connect prompt for a personal workspace", () => {
    const adapter = makeAdapter();
    const personal: Workspace = { ...teamWorkspace, type: "personal" };
    renderPage(adapter, personal);

    expect(screen.getByTestId("members-empty")).toBeInTheDocument();
    expect(adapter.listMembers).not.toHaveBeenCalled();
  });
});
