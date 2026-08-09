/**
 * Component tests for the Review surface's server wiring: approve / reject
 * call `api.reviewBlock` with the active target locale, apply the per-locale
 * optimistic update and roll it back on failure; the bulk actions are one
 * request each with per-block outcomes; and the status filter and its
 * histogram are block queries rather than passes over a full download.
 */
import { describe, it, expect, vi } from "vite-plus/test";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import { ReviewSurface } from "../components/ReviewSurface";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ApiProvider } from "../context/ApiContext";
import { WorkspaceProvider } from "../context/WorkspaceContext";
import { BreadcrumbProvider } from "../context/BreadcrumbContext";
import { createMockAdapter, type MockAdapter } from "../stories/mock-adapter";
import { sampleProject } from "../stories/fixtures";
import type { BlockInfo, TargetEntry, Workspace } from "../types/api";

const mockWorkspace: Workspace = {
  id: "ws-1",
  name: "Demo Workspace",
  slug: "demo",
  description: "",
  logo_url: "",
  type: "personal",
  role: "owner",
};

function makeBlock(id: string, source: string, frTarget: TargetEntry): BlockInfo {
  return {
    id,
    source,
    source_coded: source,
    source_spans: [],
    targets: { "fr-FR": frTarget },
    translatable: true,
    has_spans: false,
    properties: {},
  };
}

const testBlocks: BlockInfo[] = [
  makeBlock("b1", "Hello world", "Bonjour le monde"),
  makeBlock("b2", "Goodbye now", "Au revoir"),
  makeBlock("b3", "Open settings", { text: "Ouvrir les réglages", status: "reviewed" }),
];

function renderSurface(
  blocks: BlockInfo[] = testBlocks,
  prepare?: (adapter: MockAdapter) => void,
): { adapter: MockAdapter } {
  const adapter = createMockAdapter(blocks);
  prepare?.(adapter);
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={qc}>
      <ApiProvider adapter={adapter}>
        <WorkspaceProvider initialWorkspace={mockWorkspace}>
          <BreadcrumbProvider>
            <ReviewSurface project={sampleProject} fileName="messages.json" onBack={vi.fn()} />
          </BreadcrumbProvider>
        </WorkspaceProvider>
      </ApiProvider>
    </QueryClientProvider>,
  );
  return { adapter };
}

async function waitForRows() {
  await waitFor(() => expect(screen.getByTestId("review-row-b1")).toBeInTheDocument());
}

describe("ReviewSurface — approve/reject persist via api.reviewBlock", () => {
  it("approve calls reviewBlock with the active locale and flips the chip", async () => {
    const user = userEvent.setup();
    const { adapter } = renderSurface();
    await waitForRows();

    expect(screen.getByTestId("review-status-b1").textContent).toBe("Translated");
    await user.click(screen.getByTestId("approve-b1"));

    await waitFor(() => expect(adapter.reviewBlockCalls).toHaveLength(1));
    expect(adapter.reviewBlockCalls[0]).toMatchObject({
      workspaceSlug: "demo",
      projectId: sampleProject.id,
      itemName: "messages.json",
      blockId: "b1",
      targetLocale: "fr-FR",
      reviewed: true,
    });
    expect(screen.getByTestId("review-status-b1").textContent).toBe("Reviewed");
  });

  it("reject calls reviewBlock with reviewed=false + draft and demotes the block", async () => {
    const user = userEvent.setup();
    const { adapter } = renderSurface();
    await waitForRows();

    // b3 starts reviewed (per-locale Target.Status in the payload).
    expect(screen.getByTestId("review-status-b3").textContent).toBe("Reviewed");
    await user.click(screen.getByTestId("reject-b3"));

    await waitFor(() => expect(adapter.reviewBlockCalls).toHaveLength(1));
    // A rejection demotes to draft (the unit re-enters the work queue,
    // matching the host review service's rejected → draft mapping) — not to
    // translated, which would leave the rejected text passing coverage gates.
    expect(adapter.reviewBlockCalls[0]).toMatchObject({
      blockId: "b3",
      targetLocale: "fr-FR",
      reviewed: false,
      demoteTo: "draft",
    });
    expect(screen.getByTestId("review-status-b3").textContent).toBe("Draft");
  });

  it("disables Reject for an untranslated block (clearing nothing is a no-op)", async () => {
    const { adapter } = renderSurface([
      makeBlock("b1", "Hello world", "Bonjour le monde"),
      makeBlock("b2", "Goodbye now", ""),
    ]);
    await waitForRows();

    expect(screen.getByTestId("review-status-b2").textContent).toBe("Not Started");
    expect(screen.getByTestId("reject-b2")).toBeDisabled();
    expect(screen.getByTestId("reject-b1")).toBeEnabled();
    expect(adapter.reviewBlockCalls).toHaveLength(0);
  });

  it("disables Approve for an untranslated block (the server would 422 it)", async () => {
    const { adapter } = renderSurface([
      makeBlock("b1", "Hello world", "Bonjour le monde"),
      makeBlock("b2", "Goodbye now", ""),
    ]);
    await waitForRows();

    expect(screen.getByTestId("review-status-b2").textContent).toBe("Not Started");
    expect(screen.getByTestId("approve-b2")).toBeDisabled();
    expect(screen.getByTestId("approve-b1")).toBeEnabled();
    expect(adapter.reviewBlockCalls).toHaveLength(0);
  });

  it("rolls back the optimistic update and surfaces an error when the call fails", async () => {
    const user = userEvent.setup();
    const { adapter } = renderSurface();
    adapter.failReviewBlock = true;
    await waitForRows();

    await user.click(screen.getByTestId("approve-b1"));

    // The call was attempted, the failure surfaced, and the chip reverted.
    await waitFor(() =>
      expect(screen.getByText("Couldn't mark the block as reviewed")).toBeInTheDocument(),
    );
    expect(adapter.reviewBlockCalls).toHaveLength(1);
    expect(screen.getByTestId("review-status-b1").textContent).toBe("Translated");
  });
});

describe("ReviewSurface — bulk actions are one request", () => {
  it("sends the whole selection in one bulk-review request and reports the count", async () => {
    const user = userEvent.setup();
    const { adapter } = renderSurface();
    const bulk = vi.spyOn(adapter, "bulkReviewBlocks");
    await waitForRows();

    await user.click(screen.getByTestId("review-select-b1"));
    await user.click(screen.getByTestId("review-select-b2"));
    await user.click(screen.getByTestId("bulk-mark-reviewed"));

    await waitFor(() => expect(bulk).toHaveBeenCalledTimes(1));
    const req = bulk.mock.calls[0][1];
    expect([...req.block_ids].sort()).toEqual(["b1", "b2"]);
    expect(req).toMatchObject({
      project_id: sampleProject.id,
      item_name: "messages.json",
      target_locale: "fr-FR",
      approve: true,
    });
    // The batch replaces the per-block loop entirely.
    expect(adapter.reviewBlockCalls).toHaveLength(0);
    await waitFor(() =>
      expect(screen.getByText("Marked 2 block(s) as reviewed")).toBeInTheDocument(),
    );
    expect(screen.getByTestId("review-status-b1").textContent).toBe("Reviewed");
    expect(screen.getByTestId("review-status-b2").textContent).toBe("Reviewed");
  });

  it("disables bulk and per-row review buttons while the request is in flight", async () => {
    const user = userEvent.setup();
    const { adapter } = renderSurface();
    await waitForRows();

    // Hold the batch open so the surface stays mid-request.
    let release!: () => void;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    const original = adapter.bulkReviewBlocks.bind(adapter);
    vi.spyOn(adapter, "bulkReviewBlocks").mockImplementation(async (...args) => {
      await gate;
      return original(...args);
    });

    await user.click(screen.getByTestId("review-select-b1"));
    await user.click(screen.getByTestId("review-select-b2"));
    await user.click(screen.getByTestId("bulk-mark-reviewed"));

    // While the batch awaits the server, a single approve/reject or a second
    // bulk click would race its writes — disabled.
    await waitFor(() => expect(screen.getByTestId("bulk-mark-reviewed")).toBeDisabled());
    expect(screen.getByTestId("approve-b2")).toBeDisabled();
    expect(screen.getByTestId("reject-b1")).toBeDisabled();

    release();
    await waitFor(() =>
      expect(screen.getByText("Marked 2 block(s) as reviewed")).toBeInTheDocument(),
    );
    // Buttons re-enable once the batch finishes.
    expect(screen.getByTestId("reject-b1")).toBeEnabled();
  });

  it("leaves untranslated blocks out of the request instead of sending certain refusals", async () => {
    const user = userEvent.setup();
    const { adapter } = renderSurface([
      makeBlock("b1", "Hello world", "Bonjour le monde"),
      makeBlock("b2", "Goodbye now", ""),
    ]);
    const bulk = vi.spyOn(adapter, "bulkReviewBlocks");
    await waitForRows();

    // Select-all on filter=all includes the untranslated block.
    await user.click(screen.getByTestId("select-all"));
    await user.click(screen.getByTestId("bulk-mark-reviewed"));

    await waitFor(() => expect(bulk).toHaveBeenCalledTimes(1));
    expect(bulk.mock.calls[0][1].block_ids).toEqual(["b1"]);
    await waitFor(() =>
      expect(screen.getByText("Marked 1 block(s) as reviewed")).toBeInTheDocument(),
    );
    expect(screen.getByTestId("review-status-b2").textContent).toBe("Not Started");
  });

  it("surfaces the blocks that refused, and moves only the ones that took the decision", async () => {
    const user = userEvent.setup();
    const { adapter } = renderSurface();
    await waitForRows();

    // One protected block refuses inside an otherwise successful batch.
    vi.spyOn(adapter, "bulkReviewBlocks").mockResolvedValue({
      results: [
        { block_id: "b1", ok: false, error: "block is protected" },
        { block_id: "b2", ok: true, status: "reviewed" },
      ],
      succeeded: 1,
      failed: 1,
      review_completed: false,
    });

    await user.click(screen.getByTestId("review-select-b1"));
    await user.click(screen.getByTestId("review-select-b2"));
    await user.click(screen.getByTestId("bulk-mark-reviewed"));

    await waitFor(() =>
      expect(screen.getByText("Marked 1 block(s) as reviewed")).toBeInTheDocument(),
    );
    expect(screen.getByText("Couldn't mark 1 block(s) as reviewed")).toBeInTheDocument();
    // The refused block kept its status; the approved one moved.
    expect(screen.getByTestId("review-status-b1").textContent).toBe("Translated");
    expect(screen.getByTestId("review-status-b2").textContent).toBe("Reviewed");
  });

  it("applies content memory across the selection in one request", async () => {
    const user = userEvent.setup();
    const { adapter } = renderSurface();
    const bulk = vi.spyOn(adapter, "bulkApplyMemory");
    const lookup = vi.spyOn(adapter, "lookupMemoryForBlock");
    await waitForRows();

    await user.click(screen.getByTestId("review-select-b1"));
    await user.click(screen.getByTestId("review-select-b2"));
    await user.click(screen.getByTestId("bulk-apply-tm"));

    await waitFor(() => expect(bulk).toHaveBeenCalledTimes(1));
    const req = bulk.mock.calls[0][1];
    expect([...req.block_ids].sort()).toEqual(["b1", "b2"]);
    expect(req).toMatchObject({ project_id: sampleProject.id, target_locale: "fr-FR" });
    // No per-block lookup + write pair anymore.
    expect(lookup).not.toHaveBeenCalled();
    // The mock holds no content memory, so nothing clears the threshold.
    await waitFor(() =>
      expect(screen.getByText("Applied 0 exact content-memory match(es)")).toBeInTheDocument(),
    );
  });
});

describe("ReviewSurface — the filter and the histogram are server queries", () => {
  it("asks for the selected bucket rather than filtering a full page", async () => {
    const user = userEvent.setup();
    const { adapter } = renderSurface();
    const list = vi.spyOn(adapter, "getFileBlocks");
    await waitForRows();

    await user.click(screen.getByTestId("filter-reviewed"));

    await waitFor(() =>
      expect(list.mock.calls.at(-1)?.[4]).toMatchObject({
        locale: "fr-FR",
        translatable: true,
        status: "reviewed",
      }),
    );
    // b3 is the only reviewed block, so it is the only row the server returns.
    await waitFor(() => expect(screen.getByTestId("review-row-b3")).toBeInTheDocument());
    expect(screen.queryByTestId("review-row-b1")).not.toBeInTheDocument();
  });

  it("labels each filter with the counted histogram, not the loaded rows", async () => {
    // A histogram over the whole file — larger than the three rows loaded.
    let counts!: ReturnType<typeof vi.spyOn<MockAdapter, "getBlockCounts">>;
    renderSurface(testBlocks, (adapter) => {
      counts = vi.spyOn(adapter, "getBlockCounts").mockResolvedValue({
        total: 40,
        translatable: 36,
        locale: "fr-FR",
        status: { "not-started": 20, draft: 5, translated: 4, reviewed: 7 },
      });
    });
    await waitForRows();

    await waitFor(() => expect(counts).toHaveBeenCalled());
    expect(counts.mock.calls[0].slice(1, 4)).toEqual([sampleProject.id, "messages.json", "fr-FR"]);
    expect(screen.getByTestId("filter-all").textContent).toContain("(36)");
    expect(screen.getByTestId("filter-reviewed").textContent).toContain("(7)");
    expect(screen.getByTestId("filter-translated").textContent).toContain("(4)");
    expect(screen.getByTestId("filter-not-started").textContent).toContain("(20)");
  });
});
