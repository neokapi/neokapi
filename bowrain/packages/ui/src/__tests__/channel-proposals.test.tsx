import { describe, it, expect, vi } from "vite-plus/test";
import type { ReactNode } from "react";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ApiProvider } from "../context/ApiContext";
import { WorkspaceProvider } from "../context/WorkspaceContext";
import type { ApiAdapter } from "../api/adapter";
import type { Workspace } from "../types/api";
import { ChannelProposalsPanel } from "../brand-hub/proposals";
import { fragmentedChannels } from "../stories/contextProfileFixtures";

function workspaceAs(role: string): Workspace {
  return {
    id: "ws-1",
    name: "Demo",
    slug: "demo",
    description: "",
    logo_url: "",
    type: "personal",
    role,
  };
}

function renderPanel(adapter: ApiAdapter, role = "owner") {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <ApiProvider adapter={adapter}>
        <WorkspaceProvider initialWorkspace={workspaceAs(role)}>
          <ChannelProposalsPanel />
        </WorkspaceProvider>
      </ApiProvider>
    </QueryClientProvider>,
  );
}

function adapterFor(proposals: unknown, judge = vi.fn()): ApiAdapter {
  return {
    listChannelProposals: vi.fn().mockResolvedValue(proposals),
    judgeChannelProposal: judge,
  } as unknown as ApiAdapter;
}

describe("ChannelProposalsPanel", () => {
  it("states the claim, its evidence, and the two things a reviewer can say", async () => {
    renderPanel(adapterFor(fragmentedChannels));

    await waitFor(() => expect(screen.getByText("help")).toBeInTheDocument());
    expect(screen.getByText("help-centre")).toBeInTheDocument();
    expect(screen.getByText(/one slug is a prefix of the other/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /one channel/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /two channels/i })).toBeInTheDocument();
  });

  it("keeps a judged pair visible with its judgement, not hidden", async () => {
    renderPanel(adapterFor(fragmentedChannels));

    await waitFor(() => expect(screen.getByText("marketing-site")).toBeInTheDocument());
    // The dismissed pair carries its verdict as a badge, with no buttons.
    expect(screen.getByText("Two channels", { selector: "span" })).toBeInTheDocument();
    expect(screen.getByText(/Judged /)).toBeInTheDocument();
  });

  it("sends the proposal's own key with the verdict", async () => {
    const judge = vi.fn().mockResolvedValue({ status: "accepted" });
    renderPanel(adapterFor(fragmentedChannels, judge));

    await waitFor(() => expect(screen.getByText("help")).toBeInTheDocument());
    await userEvent.click(screen.getByRole("button", { name: /one channel/i }));

    await waitFor(() =>
      expect(judge).toHaveBeenCalledWith("demo", {
        profile: "bowrain",
        proposed_channel: "help",
        existing_channel: "help-centre",
        status: "accepted",
      }),
    );
  });

  it("shows a member who may not govern what is waiting, without the verdicts", async () => {
    renderPanel(adapterFor(fragmentedChannels), "member");

    await waitFor(() => expect(screen.getByText("help")).toBeInTheDocument());
    expect(screen.getByText("Waiting on a workspace admin")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /one channel/i })).not.toBeInTheDocument();
  });

  it("renders nothing at all when the workspace has raised no pair", async () => {
    const { container } = renderPanel(adapterFor({ proposals: [] }));

    await waitFor(() => expect(container).toBeEmptyDOMElement());
  });
});
