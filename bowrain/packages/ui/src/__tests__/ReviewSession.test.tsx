/**
 * Integration tests for the governed review session: the flat cross-item queue
 * builds from the dashboard stats + block reads, the keyboard model moves and
 * decides, approve advances to the next pending block, and "Approve all
 * passing" clears the queue and reflects the auto-continue to delivery.
 */
import { describe, it, expect } from "vite-plus/test";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

import { ReviewSession } from "../components/review/ReviewSession";
import { ApiProvider } from "../context/ApiContext";
import { WorkspaceProvider } from "../context/WorkspaceContext";
import { createMockAdapter, type MockAdapter } from "../stories/mock-adapter";
import { sampleProject } from "../stories/fixtures";
import type { BlockInfo, TargetEntry, TranslationDashboardStats, Workspace } from "../types/api";

const mockWorkspace: Workspace = {
  id: "ws-1",
  name: "Demo Workspace",
  slug: "demo",
  description: "",
  logo_url: "",
  type: "personal",
  role: "owner",
};

function block(id: string, source: string, target: TargetEntry): BlockInfo {
  return {
    id,
    source,
    source_coded: source,
    source_spans: [],
    targets: { "fr-FR": target },
    translatable: true,
    has_spans: false,
    properties: {},
  };
}

const blocks: BlockInfo[] = [
  block("b1", "Hello world", "Bonjour le monde"),
  block("b2", "Goodbye now", "Au revoir"),
  block("b3", "Settings", { text: "Réglages", status: "reviewed" }),
];

function locale(over: Record<string, unknown>) {
  return {
    locale: "fr-FR",
    translated_blocks: 2,
    total_blocks: 3,
    translated_words: 0,
    total_words: 0,
    percentage: 66,
    approved_blocks: 0,
    failing_checks: 0,
    ...over,
  };
}

const stats: TranslationDashboardStats = {
  locale_stats: [locale({})],
  item_stats: [
    {
      item_name: "messages.json",
      item_id: "itm-msg1",
      format: "json",
      collection_id: "coll-default",
      block_count: 3,
      word_count: 0,
      locales: [locale({})],
    },
  ],
  collection_stats: [],
  total_blocks: 3,
  translatable_blocks: 3,
  total_source_words: 0,
};

function renderSession(): { adapter: MockAdapter } {
  const adapter = createMockAdapter(blocks);
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={qc}>
      <ApiProvider adapter={adapter}>
        <WorkspaceProvider initialWorkspace={mockWorkspace}>
          <ReviewSession project={sampleProject} dashboardStats={stats} stream="main" />
        </WorkspaceProvider>
      </ApiProvider>
    </QueryClientProvider>,
  );
  return { adapter };
}

const e1 = "itm-msg1::b1::fr-FR";
const e2 = "itm-msg1::b2::fr-FR";

async function waitForQueue() {
  await waitFor(() => expect(screen.getByTestId(`queue-row-${e1}`)).toBeInTheDocument());
}

describe("ReviewSession", () => {
  it("builds the flat pending queue from stats + blocks (reviewed block excluded)", async () => {
    renderSession();
    await waitForQueue();
    // b1, b2 pending; b3 is reviewed → not in the queue.
    expect(screen.getByTestId(`queue-row-${e2}`)).toBeInTheDocument();
    expect(screen.queryByTestId("queue-row-itm-msg1::b3::fr-FR")).not.toBeInTheDocument();
    expect(screen.getByTestId("review-pending-count").textContent).toContain("2 pending");
    expect(screen.getByTestId("reviewer-position").textContent).toContain("1 of 2");
  });

  it("moves between blocks with the j/k keyboard model", async () => {
    const user = userEvent.setup();
    renderSession();
    await waitForQueue();
    expect(screen.getByTestId("reviewer-position").textContent).toContain("1 of 2");
    await user.keyboard("j");
    await waitFor(() =>
      expect(screen.getByTestId("reviewer-position").textContent).toContain("2 of 2"),
    );
    await user.keyboard("k");
    await waitFor(() =>
      expect(screen.getByTestId("reviewer-position").textContent).toContain("1 of 2"),
    );
  });

  it("approve persists via reviewBlock and advances to the next pending block", async () => {
    const user = userEvent.setup();
    const { adapter } = renderSession();
    await waitForQueue();

    await user.click(screen.getByTestId("reviewer-approve"));

    await waitFor(() => expect(adapter.reviewBlockCalls).toHaveLength(1));
    expect(adapter.reviewBlockCalls[0]).toMatchObject({
      blockId: "b1",
      targetLocale: "fr-FR",
      reviewed: true,
    });
    // The queue shrinks and the next pending block slides into place.
    await waitFor(() =>
      expect(screen.getByTestId("review-pending-count").textContent).toContain("1 pending"),
    );
    expect(screen.queryByTestId(`queue-row-${e1}`)).not.toBeInTheDocument();
    expect(screen.getByTestId("reviewer-position").textContent).toContain("1 of 1");
  });

  it("approves via the 'a' keyboard shortcut", async () => {
    const user = userEvent.setup();
    const { adapter } = renderSession();
    await waitForQueue();
    await user.keyboard("a");
    await waitFor(() => expect(adapter.reviewBlockCalls).toHaveLength(1));
    expect(adapter.reviewBlockCalls[0].reviewed).toBe(true);
  });

  it("reject demotes to draft via reviewBlock", async () => {
    const user = userEvent.setup();
    const { adapter } = renderSession();
    await waitForQueue();
    await user.click(screen.getByTestId("reviewer-reject"));
    await waitFor(() => expect(adapter.reviewBlockCalls).toHaveLength(1));
    expect(adapter.reviewBlockCalls[0]).toMatchObject({ reviewed: false, demoteTo: "draft" });
  });

  it("'Approve all passing' clears the queue and shows the delivering state", async () => {
    const user = userEvent.setup();
    const { adapter } = renderSession();
    await waitForQueue();

    await user.click(screen.getByTestId("approve-all-passing"));
    // Confirm in the dialog.
    const confirm = await screen.findByRole("button", { name: "Approve passing" });
    await user.click(confirm);

    await waitFor(() => expect(adapter.approvePassingReviewCalls).toHaveLength(1));
    await waitFor(() => expect(screen.getByTestId("review-delivering")).toBeInTheDocument());
    expect(
      within(screen.getByTestId("review-delivering")).getByText(/delivering/i),
    ).toBeInTheDocument();
  });
});
