import { describe, it, expect, vi } from "vite-plus/test";
import type { ReactNode } from "react";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ApiProvider } from "../../context/ApiContext";
import { WorkspaceProvider } from "../../context/WorkspaceContext";
import type { ApiAdapter } from "../../api/adapter";
import type { ActivityInfo, Membership, Workspace } from "../../types/api";
import type { ChangeSet } from "../../types/brand-graph";
import { sampleConcepts } from "../../stories/voiceHubFixtures";
import { ActivityView } from "./ActivityView";

const workspace: Workspace = {
  id: "ws-1",
  name: "Demo",
  slug: "demo",
  description: "",
  logo_url: "",
  type: "personal",
  role: "owner",
};

const members: Membership[] = [
  {
    user_id: "alex",
    workspace_id: "ws-1",
    role: "admin",
    user: { id: "alex", email: "alex@acme.example", name: "Alex Romero", avatar_url: "" },
  },
  {
    user_id: "sam",
    workspace_id: "ws-1",
    role: "member",
    user: { id: "sam", email: "sam@acme.example", name: "Sam Okafor", avatar_url: "" },
  },
];

const minutesAgo = (m: number) => new Date(Date.now() - m * 60_000).toISOString();

// Newest first, as the server returns the feed: Sam comments, Alex approves a
// change and moves a term ten minutes apart, and Sam opened an experiment the
// day before.
const activities: ActivityInfo[] = [
  {
    id: "act-1",
    workspace_id: "ws-1",
    actor_id: "sam",
    actor_name: "Sam Okafor",
    type: "comment.added",
    entity_type: "concept",
    entity_id: "c-checkout",
    summary: "commented on a concept",
    created_at: minutesAgo(5),
  },
  {
    id: "act-2",
    workspace_id: "ws-1",
    actor_id: "alex",
    actor_name: "Alex Romero",
    type: "review.decided",
    entity_type: "changeset",
    entity_id: "cs-1",
    summary: "approved a change",
    created_at: minutesAgo(20),
  },
  {
    id: "act-3",
    workspace_id: "ws-1",
    actor_id: "alex",
    actor_name: "Alex Romero",
    type: "concept.term.status_changed",
    entity_type: "concept",
    entity_id: "c-basket",
    summary: "changed a term's status",
    created_at: minutesAgo(30),
  },
  {
    id: "act-4",
    workspace_id: "ws-1",
    actor_id: "sam",
    actor_name: "Sam Okafor",
    type: "changeset.created",
    entity_type: "changeset",
    entity_id: "cs-2",
    summary: "opened an experiment",
    created_at: minutesAgo(26 * 60),
  },
];

function changeset(partial: Partial<ChangeSet>): ChangeSet {
  return {
    id: "cs-1",
    workspace_id: "ws-1",
    name: "Prefer ‘Paiement’",
    status: "in_review",
    created_by: "sam",
    created_at: "2026-06-01T00:00:00Z",
    updated_at: "2026-06-13T00:00:00Z",
    ...partial,
  };
}

function mockAdapter(overrides: Partial<ApiAdapter> = {}): ApiAdapter {
  return {
    listActivities: vi.fn().mockResolvedValue({ activities, next_cursor: "" }),
    listChangesets: vi
      .fn()
      .mockResolvedValue([
        changeset({ id: "cs-1", name: "Prefer ‘Paiement’ for fr-FR" }),
        changeset({ id: "cs-2", name: "Retire ‘basket’" }),
      ]),
    listConcepts: vi
      .fn()
      .mockResolvedValue({ concepts: sampleConcepts, total_count: sampleConcepts.length }),
    listMembers: vi.fn().mockResolvedValue(members),
    getConceptStory: vi.fn(),
    getChangeset: vi.fn(),
    ...overrides,
  } as unknown as ApiAdapter;
}

function renderView(ui: ReactNode, adapter: ApiAdapter) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <ApiProvider adapter={adapter}>
        <WorkspaceProvider initialWorkspace={workspace}>{ui}</WorkspaceProvider>
      </ApiProvider>
    </QueryClientProvider>,
  );
}

describe("ActivityView", () => {
  it("asks the server for the type families the filter selects", async () => {
    const adapter = mockAdapter();
    renderView(<ActivityView />, adapter);

    await waitFor(() => expect(adapter.listActivities).toHaveBeenCalled());
    const [, query] = (adapter.listActivities as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(query.types).toEqual([
      "changeset.",
      "review.",
      "pilot.",
      "concept.",
      "observation.",
      "comment.",
    ]);
    // No per-concept story, no per-change-set detail: the feed is one read.
    expect(adapter.getConceptStory).not.toHaveBeenCalled();
    expect(adapter.getChangeset).not.toHaveBeenCalled();
  });

  it("narrows the request when a category is switched off", async () => {
    const user = userEvent.setup();
    const adapter = mockAdapter();
    renderView(<ActivityView />, adapter);

    await waitFor(() => expect(adapter.listActivities).toHaveBeenCalledTimes(1));
    await user.click(screen.getByRole("button", { name: "Changes" }));

    await waitFor(() => {
      const calls = (adapter.listActivities as ReturnType<typeof vi.fn>).mock.calls;
      expect(calls[calls.length - 1][1].types).toEqual(["concept.", "observation.", "comment."]);
    });
  });

  it("lists one row per session: who worked, and how many operations", async () => {
    renderView(<ActivityView />, mockAdapter());

    const list = await screen.findByTestId("activity-sessions");
    const rows = within(list).getAllByRole("option");
    // Sam today, Alex's two operations today, Sam yesterday.
    expect(rows).toHaveLength(3);
    expect(rows[0]).toHaveTextContent("Sam Okafor");
    expect(rows[1]).toHaveTextContent("Alex Romero");
    expect(rows[1]).toHaveTextContent("2");
    expect(within(list).getByText("Today")).toBeInTheDocument();
    expect(within(list).getByText("Yesterday")).toBeInTheDocument();
  });

  it("opens on the newest session and shows its operations", async () => {
    renderView(<ActivityView />, mockAdapter());

    const detail = await screen.findByText("commented on a concept");
    const pane = detail.closest("[data-slot='activity-session-detail']") as HTMLElement;
    expect(pane).toHaveAttribute("data-session", "act-1");
    // The concept the operation is about, named rather than shown as an id.
    expect(within(pane).getByText("Checkout")).toBeInTheDocument();
  });

  it("names the reviews a session decided in its facts", async () => {
    const user = userEvent.setup();
    renderView(<ActivityView />, mockAdapter());

    const list = await screen.findByTestId("activity-sessions");
    await user.click(within(list).getAllByRole("option")[1]);

    const decisions = await waitFor(() => {
      const el = document.querySelector("[data-slot='activity-session-decisions']");
      expect(el).not.toBeNull();
      return el as HTMLElement;
    });
    expect(within(decisions).getByText("approved a change")).toBeInTheDocument();
    expect(within(decisions).getByText("Prefer ‘Paiement’ for fr-FR")).toBeInTheDocument();
    expect(screen.getByText(/1 change · 1 concept/)).toBeInTheDocument();
  });

  it("opens what an operation is about", async () => {
    const user = userEvent.setup();
    const onOpenConcept = vi.fn();
    const onOpenExperiment = vi.fn();
    renderView(
      <ActivityView onOpenConcept={onOpenConcept} onOpenExperiment={onOpenExperiment} />,
      mockAdapter(),
    );

    const list = await screen.findByTestId("activity-sessions");
    await user.click(within(list).getAllByRole("option")[1]);
    const operations = await waitFor(() => {
      const el = document.querySelector("[data-slot='activity-operations']");
      expect(el).not.toBeNull();
      return el as HTMLElement;
    });
    await user.click(within(operations).getByText("Prefer ‘Paiement’ for fr-FR"));
    expect(onOpenExperiment).toHaveBeenCalledWith("cs-1");
    await user.click(within(operations).getByText("Cart"));
    expect(onOpenConcept).toHaveBeenCalledWith("c-basket");
  });

  it("says when there is no activity yet", async () => {
    renderView(
      <ActivityView />,
      mockAdapter({
        listActivities: vi.fn().mockResolvedValue({ activities: [], next_cursor: "" }),
      }),
    );
    expect(await screen.findByText("No activity yet")).toBeInTheDocument();
  });

  it("reads older activity a page at a time", async () => {
    const user = userEvent.setup();
    const listActivities = vi
      .fn()
      .mockResolvedValueOnce({ activities: activities.slice(0, 2), next_cursor: "c-2" })
      .mockResolvedValueOnce({ activities: activities.slice(2), next_cursor: "" });
    renderView(<ActivityView />, mockAdapter({ listActivities }));

    await user.click(await screen.findByRole("button", { name: "Show older activity" }));
    await waitFor(() => expect(listActivities).toHaveBeenCalledTimes(2));
    expect(listActivities.mock.calls[1][1].cursor).toBe("c-2");
    await waitFor(() =>
      expect(within(screen.getByTestId("activity-sessions")).getAllByRole("option")).toHaveLength(
        3,
      ),
    );
  });
});
