/**
 * Integration tests for the governed review session: the flat cross-item queue
 * builds from the dashboard stats + block reads, the keyboard model moves and
 * decides, approve advances to the next pending block, and "Approve all
 * passing" clears the queue and reflects the auto-continue to delivery.
 */
import { describe, it, expect, vi } from "vite-plus/test";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

import { ReviewSession } from "../components/review/ReviewSession";
import { ApiProvider } from "../context/ApiContext";
import { WorkspaceProvider } from "../context/WorkspaceContext";
import type { ReviewQueueFilter } from "../components/review/reviewQueue";
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

function renderSession(
  dashboardStats: TranslationDashboardStats = stats,
  prepare?: (adapter: MockAdapter) => void,
  opts?: { initialFilter?: ReviewQueueFilter; blocks?: BlockInfo[] },
): { adapter: MockAdapter } {
  const adapter = createMockAdapter(opts?.blocks ?? blocks);
  prepare?.(adapter);
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={qc}>
      <ApiProvider adapter={adapter}>
        <WorkspaceProvider initialWorkspace={mockWorkspace}>
          <ReviewSession
            project={sampleProject}
            dashboardStats={dashboardStats}
            stream="main"
            initialFilter={opts?.initialFilter}
          />
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
    expect(adapter.reviewBlockCalls[0]).toMatchObject({ reviewed: false, rung: "draft" });
  });

  // Sign off is the rung above reviewed. The platform writes it through the
  // same endpoint as an approval, with the rung named, and the unit leaves the
  // pending queue the same way.
  it("sign off sends the signed-off rung and clears the unit from the queue", async () => {
    const user = userEvent.setup();
    const { adapter } = renderSession();
    await waitForQueue();

    await user.click(screen.getByTestId("reviewer-sign-off"));

    await waitFor(() => expect(adapter.reviewBlockCalls).toHaveLength(1));
    expect(adapter.reviewBlockCalls[0]).toMatchObject({
      blockId: "b1",
      targetLocale: "fr-FR",
      reviewed: true,
      rung: "signed-off",
    });
    await waitFor(() =>
      expect(screen.getByTestId("review-pending-count").textContent).toContain("1 pending"),
    );
    expect(screen.queryByTestId(`queue-row-${e1}`)).not.toBeInTheDocument();
  });

  it("signs off via the 's' keyboard shortcut", async () => {
    const user = userEvent.setup();
    const { adapter } = renderSession();
    await waitForQueue();
    await user.keyboard("s");
    await waitFor(() => expect(adapter.reviewBlockCalls).toHaveLength(1));
    expect(adapter.reviewBlockCalls[0]).toMatchObject({ reviewed: true, rung: "signed-off" });
  });

  // Signing off is the same review permission as approving, so a translator
  // gets a disabled button that says why rather than a 403 on click.
  it("disables Approve and Sign off for a caller without review permission", async () => {
    renderSession(stats, (adapter) => {
      vi.spyOn(adapter, "getCallerPermissions").mockResolvedValue({
        permissions: ["view_content", "translate"],
        languages: [],
      });
    });
    await waitForQueue();

    await waitFor(() => expect(screen.getByTestId("reviewer-sign-off")).toBeDisabled());
    expect(screen.getByTestId("reviewer-approve")).toBeDisabled();
    expect(screen.getByTestId("reviewer-sign-off").getAttribute("title")).toContain(
      "review permission",
    );
  });

  it("runs the check pass only over the scopes the loaded queue holds", async () => {
    // A second item the dashboard flags as failing, with nothing pending in
    // the server's queue: it must cost no check request.
    const withQuietFailingItem: TranslationDashboardStats = {
      ...stats,
      item_stats: [
        ...stats.item_stats,
        {
          item_name: "empty.json",
          item_id: "itm-empty",
          format: "json",
          collection_id: "coll-default",
          block_count: 0,
          word_count: 0,
          locales: [locale({ translated_blocks: 0, failing_checks: 3 })],
        },
      ],
    };
    let qa!: ReturnType<typeof vi.spyOn<MockAdapter, "runFileCheck">>;
    renderSession(withQuietFailingItem, (adapter) => {
      qa = vi.spyOn(adapter, "runFileCheck");
    });
    await waitForQueue();

    expect(qa.mock.calls.map((c) => c[2])).not.toContain("empty.json");
  });

  // The block a save leaves behind is the server's to report. Reconstructing it
  // client-side means a second copy of the demotion rule, which can disagree —
  // so the session re-reads, and only falls back when the read cannot happen.
  it("shows the re-read block after an inline save, not a local reconstruction", async () => {
    const user = userEvent.setup();
    const serverBlock = block("b1", "Hello world", {
      text: "Bonjour le monde (server)",
      status: "draft",
    });
    let getBlock!: ReturnType<typeof vi.spyOn<MockAdapter, "getBlock">>;
    renderSession(stats, (adapter) => {
      getBlock = vi.spyOn(adapter, "getBlock").mockResolvedValue(serverBlock);
    });
    await waitForQueue();

    await user.click(screen.getByTestId("reviewer-edit"));
    await user.click(screen.getByTestId("unified-save"));

    await waitFor(() => expect(getBlock).toHaveBeenCalledTimes(1));
    expect(getBlock.mock.calls[0].slice(0, 3)).toEqual(["demo", sampleProject.id, "b1"]);
    await waitFor(() =>
      expect(screen.getByTestId("reviewer-target-cell").textContent).toContain("(server)"),
    );
  });

  it("falls back to the local reconstruction when the re-read fails", async () => {
    const user = userEvent.setup();
    renderSession(stats, (adapter) => {
      vi.spyOn(adapter, "getBlock").mockRejectedValue(new Error("offline"));
    });
    await waitForQueue();

    await user.click(screen.getByTestId("reviewer-edit"));
    await user.click(screen.getByTestId("unified-save"));

    // The save itself succeeded; a failed follow-up read must not report it as
    // an error, and the reviewer keeps showing the saved text.
    await waitFor(() => expect(screen.queryByTestId("reviewer-editor")).not.toBeInTheDocument());
    expect(screen.getByTestId("reviewer-target-cell").textContent).toContain("Bonjour le monde");
  });

  // A collection is a QUERY scope, not a view filter. It used to narrow the
  // loaded slice in the browser, so a collection with more pending work than
  // the session's 1000-entry slice showed fewer entries than its own card
  // counted — and the "N pending" it reported was the project's number.
  describe("a collection scope is asked of the server", () => {
    const twoItems: BlockInfo[] = [
      block("b1", "Hello world", "Bonjour le monde"),
      block("b2", "Goodbye now", "Au revoir"),
    ];
    const twoItemStats: TranslationDashboardStats = {
      ...stats,
      item_stats: [
        { ...stats.item_stats[0], item_name: "app.json", item_id: "itm-app" },
        { ...stats.item_stats[0], item_name: "docs.json", item_id: "itm-docs" },
      ],
    };
    // b1 lives in app.json, which is in a collection; b2 lives in docs.json,
    // which is in none — the "" scope.
    const inCollections = (a: MockAdapter) => {
      a.itemNames = { b1: "app.json", b2: "docs.json" };
      a.itemCollections = { "app.json": "coll-app" };
    };

    it("pages the collection's queue, and its total", async () => {
      const { adapter } = renderSession(twoItemStats, inCollections, {
        initialFilter: { collectionId: "coll-app" },
        blocks: twoItems,
      });
      await waitFor(() =>
        expect(screen.getByTestId("queue-row-itm-app::b1::fr-FR")).toBeInTheDocument(),
      );

      expect(adapter.pendingReviewCalls[0]?.collectionId).toBe("coll-app");
      expect(screen.queryByTestId("queue-row-itm-docs::b2::fr-FR")).not.toBeInTheDocument();
      expect(screen.getByTestId("review-pending-count").textContent).toContain("1 pending");
    });

    it("asks for the ungrouped bucket with the empty id, not with no scope", async () => {
      const { adapter } = renderSession(twoItemStats, inCollections, {
        initialFilter: { collectionId: "" },
        blocks: twoItems,
      });
      await waitFor(() =>
        expect(screen.getByTestId("queue-row-itm-docs::b2::fr-FR")).toBeInTheDocument(),
      );

      expect(adapter.pendingReviewCalls[0]?.collectionId).toBe("");
      expect(screen.queryByTestId("queue-row-itm-app::b1::fr-FR")).not.toBeInTheDocument();
    });

    it("asks for no scope at all when the session opens unfiltered", async () => {
      const { adapter } = renderSession(twoItemStats, inCollections, { blocks: twoItems });
      await waitFor(() =>
        expect(screen.getByTestId("queue-row-itm-app::b1::fr-FR")).toBeInTheDocument(),
      );

      expect(adapter.pendingReviewCalls[0]?.collectionId).toBeUndefined();
      expect(screen.getByTestId("queue-row-itm-docs::b2::fr-FR")).toBeInTheDocument();
    });

    it("reloads from the server when the reviewer clears the scope", async () => {
      const user = userEvent.setup();
      const { adapter } = renderSession(twoItemStats, inCollections, {
        initialFilter: { collectionId: "coll-app" },
        blocks: twoItems,
      });
      await waitFor(() =>
        expect(screen.getByTestId("queue-row-itm-app::b1::fr-FR")).toBeInTheDocument(),
      );

      await user.click(screen.getByTestId("filter-clear"));
      await waitFor(() =>
        expect(screen.getByTestId("queue-row-itm-docs::b2::fr-FR")).toBeInTheDocument(),
      );
      expect(adapter.pendingReviewCalls.at(-1)?.collectionId).toBeUndefined();
    });
  });

  // The queue's passing bucket is honest only if the entries carry the bars the
  // server judges on. While the payload carried check findings alone, a target
  // using a forbidden term or scoring below its profile's bar sat in "no failing
  // checks" and was counted as about to be approved — by a server that refuses
  // it. #1771 removed the guessed `off_brand` bucket rather than fake the
  // evidence; these cases pin the evidence that replaced it.
  describe("bucketing on the bars the server applies", () => {
    it("keeps a term violation and a below-bar score out of the passing count", async () => {
      renderSession(stats, (a) => {
        a.blockEvidence = {
          b1: { term_compliance: "violation" },
          b2: { term_compliance: "compliant", voice_score: 40, voice_bar: 80 },
        };
      });
      await waitForQueue();

      // Neither block carries a failing check, so a check-only bucket said two.
      expect(screen.getByTestId("filter-verdict-passing").textContent).toContain("(0)");
      expect(screen.getByTestId("filter-verdict-failing").textContent).toContain("(2)");
      expect(screen.getByTestId("approve-all-passing")).toBeDisabled();
    });

    it("counts a compliant, above-bar block as passing", async () => {
      renderSession(stats, (a) => {
        a.blockEvidence = {
          b1: { term_compliance: "compliant", voice_score: 95, voice_bar: 80 },
          b2: { term_compliance: "violation" },
        };
      });
      await waitForQueue();

      expect(screen.getByTestId("filter-verdict-passing").textContent).toContain("(1)");
      expect(screen.getByTestId("approve-all-passing")).toBeEnabled();
      expect(within(screen.getByTestId("approve-all-passing")).getByText("1")).toBeInTheDocument();
    });

    it("names the bar a block missed, and shows its score against its own bar", async () => {
      renderSession(stats, (a) => {
        a.blockEvidence = { b1: { term_compliance: "violation", voice_score: 62, voice_bar: 90 } };
      });
      await waitForQueue();

      // The bars read inside the verdict, and the score reads in the rail
      // beside the profile that set the bar it is measured against.
      const verdict = screen.getByTestId("reviewer-verdict-failing");
      expect(verdict.textContent).toContain("Misses a bar: terminology, voice");
      expect(verdict.getAttribute("title")).toContain("Terminology");
      expect(verdict.getAttribute("title")).toContain("Below the voice bar");
      await waitFor(() =>
        expect(screen.getByTestId("point-voice-score").textContent).toContain("Voice 62 of 90"),
      );
      expect(screen.getByTestId("point-voice-score")).toHaveAttribute("data-below-bar", "true");
    });

    it("counts the passing blocks the pass actually covers when a locale is filtered", async () => {
      const user = userEvent.setup();
      // The pass forwards the locale filter and nothing else, so the count
      // beside the button has to be over that locale — it used to be over the
      // whole loaded queue while the sentence beside it said "in German".
      const bilingual: BlockInfo[] = [
        { ...block("b1", "Hello world", "Bonjour le monde"), targets: { "fr-FR": "Bonjour" } },
        { ...block("b2", "Goodbye now", "Au revoir"), targets: { "de-DE": "Auf Wiedersehen" } },
      ];
      const bilingualStats: TranslationDashboardStats = {
        ...stats,
        locale_stats: [locale({}), locale({ locale: "de-DE" })],
        item_stats: [
          { ...stats.item_stats[0], locales: [locale({}), locale({ locale: "de-DE" })] },
        ],
      };
      renderSession(bilingualStats, undefined, { blocks: bilingual });
      await waitForQueue();
      expect(screen.getByTestId("approve-all-passing").textContent).toContain("2");

      await user.click(screen.getByTestId("filter-language"));
      await user.click(await screen.findByTestId("filter-language-de-DE"));
      await waitFor(() =>
        expect(screen.getByTestId("approve-all-passing").textContent).toContain("1"),
      );
    });

    it("treats an unchecked terminology verdict as nothing claimed, not as compliance", async () => {
      // No governance active for the locale: the entry passes on terminology
      // because nothing was checked, and the surface says nothing about it.
      renderSession(stats, (a) => {
        a.blockEvidence = { b1: {}, b2: {} };
      });
      await waitForQueue();

      expect(screen.getByTestId("filter-verdict-passing").textContent).toContain("(2)");
      expect(screen.queryByTestId("reviewer-blocker-terms")).not.toBeInTheDocument();
      expect(screen.queryByTestId("reviewer-voice-score")).not.toBeInTheDocument();
    });
  });

  it("names which bar the bulk pass's skipped blocks missed", async () => {
    const user = userEvent.setup();
    const { adapter } = renderSession(stats, (a) => {
      a.approvePassingResult = {
        approved: 3,
        skipped: 4,
        skipped_failing_checks: 1,
        skipped_term_violations: 2,
        skipped_below_voice_bar: 1,
        remaining_pending: 4,
        review_completed: false,
      };
    });
    await waitForQueue();

    await user.click(screen.getByTestId("approve-all-passing"));
    await user.click(await screen.findByRole("button", { name: "Approve passing" }));
    await waitFor(() => expect(adapter.approvePassingReviewCalls).toHaveLength(1));

    // "Skipped for failing checks, terminology, or the brand bar" named all
    // three every time and so told a reviewer nothing about what to fix.
    const message = await screen.findByText(/skipped/);
    expect(message.textContent).toContain("1 failing checks");
    expect(message.textContent).toContain("2 terminology");
    expect(message.textContent).toContain("1 below the voice bar");
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
