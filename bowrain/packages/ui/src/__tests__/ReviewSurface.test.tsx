/**
 * Component tests for the Review surface: the reading pane is the document
 * itself (the shared preview kit over the projected content tree), a block is
 * opened by reading and decided in the slide-in inspector, and the server
 * wiring behind that is unchanged — approve / reject call `api.reviewBlock`
 * with the active target locale, apply the per-locale optimistic update and
 * roll it back on failure; the bulk actions are one request each with per-block
 * outcomes; and the status filter and its histogram are block queries rather
 * than passes over a full download.
 */
import { describe, it, expect, vi } from "vite-plus/test";
import { render, screen, waitFor, within } from "@testing-library/react";
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

/** The document renders one element per block, carrying its id and status. */
async function waitForDocument() {
  await waitFor(() => expect(screen.getByTestId("review-block-b1")).toBeInTheDocument());
}

/** Open a block's inspector by activating it in the document. */
async function openBlock(user: ReturnType<typeof userEvent.setup>, id: string) {
  await user.click(screen.getByTestId(`review-block-${id}`));
  await screen.findByTestId("review-inspector");
}

describe("ReviewSurface — the reading pane is the document", () => {
  it("renders each block as content, marked with its review status", async () => {
    renderSurface();
    await waitForDocument();

    // The pane reads the target locale — review is reading the translation.
    expect(screen.getByTestId("review-document").textContent).toContain("Bonjour le monde");
    expect(screen.getByTestId("review-block-b1")).toHaveAttribute("data-status", "translated");
    expect(screen.getByTestId("review-block-b3")).toHaveAttribute("data-status", "reviewed");
    // No source/target grid: the source is a reading of the same document.
    expect(screen.queryByTestId("review-row-b1")).not.toBeInTheDocument();
  });

  it("reads the source when the source language is chosen, without reloading", async () => {
    // One selector holds every language, the source among them: choosing it is
    // how the source is read, and it is a view change rather than a new query.
    const user = userEvent.setup();
    const { adapter } = renderSurface();
    const list = vi.spyOn(adapter, "getFileBlocks");
    await waitForDocument();

    await user.click(screen.getByTestId("language-scope"));
    await user.click(await screen.findByTestId("language-scope-en-US"));

    await waitFor(() =>
      expect(screen.getByTestId("review-document").textContent).toContain("Hello world"),
    );
    expect(list).not.toHaveBeenCalled();
  });

  it("names every language of the project in one list, source marked", async () => {
    const user = userEvent.setup();
    renderSurface();
    await waitForDocument();

    await user.click(screen.getByTestId("language-scope"));

    const source = await screen.findByTestId("language-scope-en-US");
    expect(source.textContent).toContain("American English");
    expect(source.textContent).toContain("source");
    expect((await screen.findByTestId("language-scope-fr-FR")).textContent).toContain(
      "French (France)",
    );
    // The lane toggle is gone: the language answers which side is read.
    expect(screen.queryByTestId("read-source")).not.toBeInTheDocument();
    expect(screen.queryByTestId("read-target")).not.toBeInTheDocument();
  });

  it("says how much of the bucket the pane holds", async () => {
    renderSurface(testBlocks, (adapter) => {
      vi.spyOn(adapter, "getBlockCounts").mockResolvedValue({
        total: 40,
        translatable: 36,
        locale: "fr-FR",
        status: { "not-started": 20, draft: 5, translated: 4, reviewed: 7 },
      });
    });
    await waitForDocument();

    await waitFor(() =>
      expect(screen.getByTestId("page-summary").textContent).toBe("Showing 3 of 36"),
    );
  });
});

describe("ReviewSurface — approve/reject persist via api.reviewBlock", () => {
  it("approve calls reviewBlock with the active locale and flips the chip", async () => {
    const user = userEvent.setup();
    const { adapter } = renderSurface();
    await waitForDocument();
    await openBlock(user, "b1");

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
    // The document's margin states the same thing.
    expect(screen.getByTestId("review-block-b1")).toHaveAttribute("data-status", "reviewed");
  });

  it("reject calls reviewBlock with reviewed=false + draft and demotes the block", async () => {
    const user = userEvent.setup();
    const { adapter } = renderSurface();
    await waitForDocument();
    // b3 starts reviewed (per-locale Target.Status in the payload).
    await openBlock(user, "b3");

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
      rung: "draft",
    });
    expect(screen.getByTestId("review-status-b3").textContent).toBe("Draft");
  });

  it("sign off calls reviewBlock with the signed-off rung and shows the rung", async () => {
    const user = userEvent.setup();
    const { adapter } = renderSurface();
    await waitForDocument();
    await openBlock(user, "b1");

    await user.click(screen.getByTestId("sign-off-b1"));

    await waitFor(() => expect(adapter.reviewBlockCalls).toHaveLength(1));
    expect(adapter.reviewBlockCalls[0]).toMatchObject({
      blockId: "b1",
      targetLocale: "fr-FR",
      reviewed: true,
      rung: "signed-off",
    });
    // The chip reads the ladder rung; the document margin reads the coarser
    // bucket, which files signed-off under reviewed.
    expect(screen.getByTestId("review-status-b1").textContent).toBe("Signed off");
    expect(screen.getByTestId("review-block-b1")).toHaveAttribute("data-status", "reviewed");
  });

  it("disables both decisions for an untranslated block", async () => {
    const user = userEvent.setup();
    const { adapter } = renderSurface([
      makeBlock("b1", "Hello world", "Bonjour le monde"),
      makeBlock("b2", "Goodbye now", ""),
    ]);
    await waitForDocument();
    await openBlock(user, "b2");

    expect(screen.getByTestId("review-status-b2").textContent).toBe("Not started");
    // Nothing to reject (clearing it is a server-side no-op) and nothing to
    // approve (the server would 422 it).
    expect(screen.getByTestId("reject-b2")).toBeDisabled();
    expect(screen.getByTestId("approve-b2")).toBeDisabled();
    expect(adapter.reviewBlockCalls).toHaveLength(0);
  });

  it("decides the block being read from the keyboard", async () => {
    const user = userEvent.setup();
    const { adapter } = renderSurface();
    await waitForDocument();

    // J moves to the first block, A approves it.
    await user.keyboard("j");
    await user.keyboard("a");

    await waitFor(() => expect(adapter.reviewBlockCalls).toHaveLength(1));
    expect(adapter.reviewBlockCalls[0]).toMatchObject({ blockId: "b1", reviewed: true });
    expect(screen.getByTestId("review-block-b1")).toHaveAttribute("data-status", "reviewed");

    // S signs the same block off, one rung up.
    await user.keyboard("s");
    await waitFor(() => expect(adapter.reviewBlockCalls).toHaveLength(2));
    expect(adapter.reviewBlockCalls[1]).toMatchObject({
      blockId: "b1",
      reviewed: true,
      rung: "signed-off",
    });
  });

  it("rolls back the optimistic update and surfaces an error when the call fails", async () => {
    const user = userEvent.setup();
    const { adapter } = renderSurface();
    adapter.failReviewBlock = true;
    await waitForDocument();
    await openBlock(user, "b1");

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
  it("sends the whole batch in one bulk-review request and reports the count", async () => {
    const user = userEvent.setup();
    const { adapter } = renderSurface();
    const bulk = vi.spyOn(adapter, "bulkReviewBlocks");
    await waitForDocument();

    // The batch is marked up while reading: open a block, add it, move on.
    await openBlock(user, "b1");
    await user.click(screen.getByTestId("review-mark-b1"));
    await openBlock(user, "b2");
    await user.click(screen.getByTestId("review-mark-b2"));
    await user.keyboard("{Escape}");
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
    expect(screen.getByTestId("review-block-b1")).toHaveAttribute("data-status", "reviewed");
    expect(screen.getByTestId("review-block-b2")).toHaveAttribute("data-status", "reviewed");
  });

  it("holds the single decisions while the batch is in flight", async () => {
    const user = userEvent.setup();
    const { adapter } = renderSurface();
    await waitForDocument();

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

    await user.click(screen.getByTestId("mark-all-shown"));
    await user.click(screen.getByTestId("bulk-mark-reviewed"));

    // While the batch awaits the server, a single approve/reject or a second
    // bulk click would race its writes — disabled.
    await waitFor(() => expect(screen.getByTestId("bulk-mark-reviewed")).toBeDisabled());
    await openBlock(user, "b1");
    expect(screen.getByTestId("approve-b1")).toBeDisabled();
    expect(screen.getByTestId("reject-b1")).toBeDisabled();

    release();
    await waitFor(() =>
      expect(screen.getByText("Marked 3 block(s) as reviewed")).toBeInTheDocument(),
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
    await waitForDocument();

    // Marking everything shown on filter=all includes the untranslated block.
    await user.click(screen.getByTestId("mark-all-shown"));
    await user.click(screen.getByTestId("bulk-mark-reviewed"));

    await waitFor(() => expect(bulk).toHaveBeenCalledTimes(1));
    expect(bulk.mock.calls[0][1].block_ids).toEqual(["b1"]);
    await waitFor(() =>
      expect(screen.getByText("Marked 1 block(s) as reviewed")).toBeInTheDocument(),
    );
    expect(screen.getByTestId("review-block-b2")).toHaveAttribute("data-status", "not-started");
  });

  it("surfaces the blocks that refused, and moves only the ones that took the decision", async () => {
    const user = userEvent.setup();
    const { adapter } = renderSurface();
    await waitForDocument();

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

    await user.click(screen.getByTestId("mark-all-shown"));
    await user.click(screen.getByTestId("bulk-mark-reviewed"));

    await waitFor(() =>
      expect(screen.getByText("Marked 1 block(s) as reviewed")).toBeInTheDocument(),
    );
    expect(screen.getByText("Couldn't mark 1 block(s) as reviewed")).toBeInTheDocument();
    // The refused block kept its status; the approved one moved.
    expect(screen.getByTestId("review-block-b1")).toHaveAttribute("data-status", "translated");
    expect(screen.getByTestId("review-block-b2")).toHaveAttribute("data-status", "reviewed");
  });

  it("asks the pass what it would write before writing it", async () => {
    const user = userEvent.setup();
    const { adapter } = renderSurface();
    const bulk = vi.spyOn(adapter, "bulkApplyMemory").mockResolvedValue({
      applied: [{ block_id: "b1", text: "Bonjour le monde", score: 1 }],
      skipped: [{ block_id: "b2", reason: "no match above threshold" }],
    });
    const lookup = vi.spyOn(adapter, "lookupMemoryForBlock");
    await waitForDocument();

    await user.click(screen.getByTestId("mark-all-shown"));
    await user.click(screen.getByTestId("bulk-apply-tm"));

    // The first request is the preview: nothing is written yet.
    await waitFor(() => expect(bulk).toHaveBeenCalledTimes(1));
    const previewReq = bulk.mock.calls[0][1];
    expect([...previewReq.block_ids].sort()).toEqual(["b1", "b2", "b3"]);
    expect(previewReq).toMatchObject({
      project_id: sampleProject.id,
      target_locale: "fr-FR",
      preview: true,
    });

    // The reviewer reads the wording that is about to land, then commits.
    const dialog = await screen.findByTestId("apply-memory-dialog");
    expect(dialog.textContent).toContain("Bonjour le monde");
    await user.click(screen.getByTestId("apply-memory-confirm"));

    await waitFor(() => expect(bulk).toHaveBeenCalledTimes(2));
    expect(bulk.mock.calls[1][1].preview).toBeUndefined();
    // No per-block lookup + write pair anymore.
    expect(lookup).not.toHaveBeenCalled();
    await waitFor(() =>
      expect(screen.getByText("Applied 1 exact content-memory match(es)")).toBeInTheDocument(),
    );
  });

  it("says so when the batch matches nothing, and applies nothing", async () => {
    const user = userEvent.setup();
    const { adapter } = renderSurface();
    const bulk = vi.spyOn(adapter, "bulkApplyMemory");
    await waitForDocument();

    await user.click(screen.getByTestId("mark-all-shown"));
    await user.click(screen.getByTestId("bulk-apply-tm"));

    const dialog = await screen.findByTestId("apply-memory-dialog");
    expect(dialog.textContent).toContain("Nothing to apply");
    expect(screen.getByTestId("apply-memory-confirm")).toBeDisabled();
    expect(bulk).toHaveBeenCalledTimes(1);
  });
});

describe("ReviewSurface — the inspector carries the block's evidence", () => {
  it("asks for the block's terms when it is opened, once", async () => {
    const user = userEvent.setup();
    const { adapter } = renderSurface();
    const terms = vi.spyOn(adapter, "lookupTermsForBlock");
    await waitForDocument();

    // Nothing is asked for while the document is only being read.
    expect(terms).not.toHaveBeenCalled();

    await openBlock(user, "b1");
    await waitFor(() => expect(terms).toHaveBeenCalledTimes(1));
    expect(terms.mock.calls[0].slice(1, 5)).toEqual([
      sampleProject.id,
      "messages.json",
      "b1",
      "fr-FR",
    ]);

    // Re-opening the same block reads the cached lookup.
    await user.keyboard("{Escape}");
    await openBlock(user, "b1");
    expect(terms).toHaveBeenCalledTimes(1);
  });

  it("asks for the block's context once, and once more for another block", async () => {
    const user = userEvent.setup();
    const { adapter } = renderSurface();
    const ctx = vi.spyOn(adapter, "getReviewContext");
    await waitForDocument();

    await openBlock(user, "b1");
    await waitFor(() => expect(ctx).toHaveBeenCalledTimes(1));
    expect(ctx.mock.calls[0].slice(1, 5)).toEqual([
      sampleProject.id,
      "messages.json",
      "b1",
      "fr-FR",
    ]);

    // The answer lands in state, and the render that shows it must not ask
    // again — an effect that lists its own cache as a dependency does.
    await user.keyboard("{Escape}");
    await openBlock(user, "b1");
    expect(ctx).toHaveBeenCalledTimes(1);

    // A different block is a different point, so it is asked for.
    await user.keyboard("{Escape}");
    await openBlock(user, "b2");
    await waitFor(() => expect(ctx).toHaveBeenCalledTimes(2));
  });

  it("draws the same point rail the queue draws", async () => {
    const user = userEvent.setup();
    renderSurface();
    await waitForDocument();
    await openBlock(user, "b1");

    // One review model, two surfaces: the document's inspector names what
    // governs the block as the queue's reviewer does.
    const point = await screen.findByTestId("inspector-point");
    await waitFor(() => expect(point.textContent).toContain("In force here"));
  });

  it("lists the findings the payload gives no position for", async () => {
    const user = userEvent.setup();
    renderSurface(testBlocks, (adapter) => {
      vi.spyOn(adapter, "runFileCheck").mockResolvedValue([
        {
          blockId: "b1",
          issues: [{ type: "spacing", severity: "warning", message: "Trailing double space" }],
        },
      ]);
    });
    await waitForDocument();

    await user.click(screen.getByTestId("run-check-btn"));
    // The block is flagged in the document…
    await waitFor(() =>
      expect(screen.getByTestId("review-block-b1")).toHaveAttribute("data-flagged", "true"),
    );
    // …and the finding itself is named in the inspector, not guessed onto a span.
    await openBlock(user, "b1");
    expect(
      within(screen.getByTestId("inspector-check")).getByText(/Trailing double space/),
    ).toBeInTheDocument();
  });
});

describe("ReviewSurface — the filter and the histogram are server queries", () => {
  it("asks for the selected bucket rather than filtering a full page", async () => {
    const user = userEvent.setup();
    const { adapter } = renderSurface();
    const list = vi.spyOn(adapter, "getFileBlocks");
    await waitForDocument();

    await user.click(screen.getByTestId("filter-reviewed"));

    await waitFor(() =>
      expect(list.mock.calls.at(-1)?.[4]).toMatchObject({
        locale: "fr-FR",
        translatable: true,
        status: "reviewed",
      }),
    );
    // b3 is the only reviewed block, so it is the only one the server returns.
    await waitFor(() => expect(screen.getByTestId("review-block-b3")).toBeInTheDocument());
    expect(screen.queryByTestId("review-block-b1")).not.toBeInTheDocument();
  });

  it("labels each filter with the counted histogram, not the loaded blocks", async () => {
    // A histogram over the whole file — larger than the three blocks loaded.
    let counts!: ReturnType<typeof vi.spyOn<MockAdapter, "getBlockCounts">>;
    renderSurface(testBlocks, (adapter) => {
      counts = vi.spyOn(adapter, "getBlockCounts").mockResolvedValue({
        total: 40,
        translatable: 36,
        locale: "fr-FR",
        status: { "not-started": 20, draft: 5, translated: 4, reviewed: 7 },
      });
    });
    await waitForDocument();

    await waitFor(() => expect(counts).toHaveBeenCalled());
    expect(counts.mock.calls[0].slice(1, 4)).toEqual([sampleProject.id, "messages.json", "fr-FR"]);
    expect(screen.getByTestId("filter-all").textContent).toContain("(36)");
    expect(screen.getByTestId("filter-reviewed").textContent).toContain("(7)");
    expect(screen.getByTestId("filter-translated").textContent).toContain("(4)");
    expect(screen.getByTestId("filter-not-started").textContent).toContain("(20)");
  });
});
