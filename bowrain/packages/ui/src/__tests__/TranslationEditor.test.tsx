/**
 * Component tests for the Translate editor's shared search filter (Visual +
 * Table views), which is a debounced server query; the progress bar, which is
 * the server's histogram; and the term-insert behaviour (insert at the open
 * editor's Lexical cursor vs. append-and-persist fallback).
 */
import { describe, it, expect, vi } from "vite-plus/test";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import { TranslationEditor } from "../components/TranslationEditor";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ApiProvider } from "../context/ApiContext";
import { WorkspaceProvider } from "../context/WorkspaceContext";
import { BreadcrumbProvider } from "../context/BreadcrumbContext";
import { createMockAdapter, type MockAdapter } from "../stories/mock-adapter";
import { sampleProject } from "../stories/fixtures";
import type { BlockInfo, Workspace } from "../types/api";

const mockWorkspace: Workspace = {
  id: "ws-1",
  name: "Demo Workspace",
  slug: "demo",
  description: "",
  logo_url: "",
  type: "personal",
  role: "owner",
};

function makeBlock(id: string, source: string, frTarget: string): BlockInfo {
  return {
    id,
    source,
    source_coded: source,
    source_spans: [],
    targets: { "fr-FR": frTarget },
    targets_coded: { "fr-FR": frTarget },
    translatable: true,
    has_spans: false,
    properties: {},
  };
}

const testBlocks: BlockInfo[] = [
  makeBlock("b1", "Hello world", "Bonjour le monde"),
  makeBlock("b2", "Goodbye now", "Au revoir"),
  makeBlock("b3", "Open settings", "Ouvrir les réglages"),
];

function renderEditor(
  opts: {
    view?: "visual" | "table";
    blocks?: BlockInfo[];
    /** Runs against the adapter before the first render, for spies. */
    prepare?: (adapter: MockAdapter) => void;
  } = {},
): {
  adapter: MockAdapter;
} {
  const adapter = createMockAdapter(opts.blocks ?? testBlocks);
  opts.prepare?.(adapter);
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={qc}>
      <ApiProvider adapter={adapter}>
        <WorkspaceProvider initialWorkspace={mockWorkspace}>
          <BreadcrumbProvider>
            <TranslationEditor
              project={sampleProject}
              fileName="messages.json"
              onBack={vi.fn()}
              defaultView={opts.view}
            />
          </BreadcrumbProvider>
        </WorkspaceProvider>
      </ApiProvider>
    </QueryClientProvider>,
  );
  return { adapter };
}

async function waitForBlocks(count: number) {
  await waitFor(() =>
    expect(screen.getByTestId("status-bar").textContent).toContain(`of ${count}`),
  );
}

describe("TranslationEditor — search filter in the Visual view", () => {
  it("renders the search box in the Visual view and filters the card list", async () => {
    const user = userEvent.setup();
    renderEditor({ view: "visual" });
    await waitForBlocks(3);

    // Same box/state the Table view uses, now present in Visual too.
    const search = screen.getByTestId("search-input");
    await user.type(search, "Goodbye");

    await waitForBlocks(1);
    // The visual layout stays mounted and the card shows the matching block.
    expect(screen.getByTestId("visual-editor-layout")).toBeInTheDocument();
    expect(screen.getByTestId("status-bar").textContent).toContain("Block 1 of 1");
  });

  it("matches against the target text of the active locale too", async () => {
    const user = userEvent.setup();
    renderEditor({ view: "visual" });
    await waitForBlocks(3);

    await user.type(screen.getByTestId("search-input"), "réglages");
    await waitForBlocks(1);
  });

  it("clamps the selection when the filter narrows past the selected index", async () => {
    const user = userEvent.setup();
    renderEditor({ view: "table" });
    await waitForBlocks(3);

    // Select the last row, then filter down to a single earlier block.
    await user.click(screen.getByTestId("block-row-2"));
    expect(screen.getByTestId("status-bar").textContent).toContain("Block 3 of 3");

    await user.type(screen.getByTestId("search-input"), "Hello");
    await waitFor(() =>
      expect(screen.getByTestId("status-bar").textContent).toContain("Block 1 of 1"),
    );
  });

  it("keeps the Visual view rendered (not blanked) after narrowing with a later block selected", async () => {
    const user = userEvent.setup();
    renderEditor({ view: "visual" });
    await waitForBlocks(3);

    // Navigate to the last block via the preview-card's Next affordance is
    // covered elsewhere; here we drive selection through the Table view's
    // click surface and switch back, proving the shared selection clamps.
    await user.click(screen.getByTestId("view-table"));
    await user.click(screen.getByTestId("block-row-2"));
    await user.click(screen.getByTestId("view-visual"));

    await user.type(screen.getByTestId("search-input"), "Hello");
    await waitFor(() =>
      expect(screen.getByTestId("status-bar").textContent).toContain("Block 1 of 1"),
    );
    expect(screen.getByTestId("visual-editor-layout")).toBeInTheDocument();
    expect(screen.queryByText("No blocks to display")).not.toBeInTheDocument();
  });
});

describe("TranslationEditor — the search and the counts are server queries", () => {
  it("sends the query and the locale once the keystrokes settle", async () => {
    const user = userEvent.setup();
    let list!: ReturnType<typeof vi.spyOn<MockAdapter, "getFileBlocks">>;
    renderEditor({
      view: "table",
      prepare: (adapter) => {
        list = vi.spyOn(adapter, "getFileBlocks");
      },
    });
    await waitForBlocks(3);

    await user.type(screen.getByTestId("search-input"), "Goodbye");

    await waitFor(() =>
      expect(list.mock.calls.at(-1)?.[4]).toMatchObject({ locale: "fr-FR", q: "Goodbye" }),
    );
    // The box debounces, so the settled query runs — not one per keystroke.
    expect(list.mock.calls.filter((c) => c[4]?.q).length).toBeLessThanOrEqual(2);
    await waitForBlocks(1);
  });

  it("draws the progress bar from the counted histogram, not the loaded page", async () => {
    renderEditor({
      view: "table",
      prepare: (adapter) => {
        vi.spyOn(adapter, "getBlockCounts").mockResolvedValue({
          total: 40,
          translatable: 36,
          locale: "fr-FR",
          status: { "not-started": 20, draft: 5, translated: 4, reviewed: 7 },
        });
      },
    });
    await waitForBlocks(3);

    await waitFor(() =>
      expect(screen.getByTestId("progress-text").textContent).toContain("(16/36 translated)"),
    );
    const text = screen.getByTestId("progress-text").textContent ?? "";
    expect(text).toContain("44%");
    expect(text).toContain("7 reviewed");
    expect(text).toContain("20 pending");
  });
});

describe("TranslationEditor — review actions persist via api.reviewBlock", () => {
  it("Approve in the Visual card's Review mode calls reviewBlock with the active locale", async () => {
    const user = userEvent.setup();
    const { adapter } = renderEditor({ view: "visual" });
    await waitForBlocks(3);

    await user.click(screen.getByRole("tab", { name: "Review" }));
    await user.click(screen.getByTestId("approve-btn"));

    await waitFor(() => expect(adapter.reviewBlockCalls).toHaveLength(1));
    expect(adapter.reviewBlockCalls[0]).toMatchObject({
      workspaceSlug: "demo",
      projectId: sampleProject.id,
      itemName: "messages.json",
      blockId: "b1",
      targetLocale: "fr-FR",
      reviewed: true,
    });
    // Approve advances to the next block once the call resolves; step back to
    // see b1's chip, driven by the optimistic per-locale Target.Status write.
    await waitFor(() =>
      expect(screen.getByTestId("status-bar").textContent).toContain("Block 2 of 3"),
    );
    await user.click(screen.getByTestId("prev-block-btn"));
    expect(screen.getByText("Reviewed")).toBeInTheDocument();
  });

  it("Reject calls reviewBlock with reviewed=false + draft (re-enters the work queue)", async () => {
    const user = userEvent.setup();
    const { adapter } = renderEditor({ view: "visual" });
    await waitForBlocks(3);

    await user.click(screen.getByRole("tab", { name: "Review" }));
    await user.click(screen.getByTestId("reject-btn"));

    await waitFor(() => expect(adapter.reviewBlockCalls).toHaveLength(1));
    // A rejection demotes to draft (host's rejected → draft mapping), not to
    // translated — a rejected text must not keep passing coverage gates.
    expect(adapter.reviewBlockCalls[0]).toMatchObject({
      blockId: "b1",
      targetLocale: "fr-FR",
      reviewed: false,
      rung: "draft",
    });
  });

  it("Sign off calls reviewBlock with the signed-off rung and advances", async () => {
    const user = userEvent.setup();
    const { adapter } = renderEditor({ view: "visual" });
    await waitForBlocks(3);

    await user.click(screen.getByRole("tab", { name: "Review" }));
    await user.click(screen.getByTestId("sign-off-btn"));

    await waitFor(() => expect(adapter.reviewBlockCalls).toHaveLength(1));
    expect(adapter.reviewBlockCalls[0]).toMatchObject({
      blockId: "b1",
      targetLocale: "fr-FR",
      reviewed: true,
      rung: "signed-off",
    });
    await waitFor(() =>
      expect(screen.getByTestId("status-bar").textContent).toContain("Block 2 of 3"),
    );
  });

  it("rolls back the optimistic status, stays on the block, and surfaces an error when the call fails", async () => {
    const user = userEvent.setup();
    const { adapter } = renderEditor({ view: "visual" });
    adapter.failReviewBlock = true;
    await waitForBlocks(3);

    await user.click(screen.getByRole("tab", { name: "Review" }));
    expect(screen.getByText("Translated")).toBeInTheDocument();
    await user.click(screen.getByTestId("approve-btn"));

    await waitFor(() =>
      expect(screen.getByText("Couldn't mark the block as reviewed")).toBeInTheDocument(),
    );
    expect(adapter.reviewBlockCalls).toHaveLength(1);
    // Approve only advances after a SUCCESSFUL persist — on failure the
    // reviewer stays on the failed block, with its chip rolled back, so the
    // error refers to the block on screen.
    expect(screen.getByTestId("status-bar").textContent).toContain("Block 1 of 3");
    expect(screen.getByText("Translated")).toBeInTheDocument();
    expect(screen.queryByText("Reviewed")).not.toBeInTheDocument();
  });

  it("disables Approve for an untranslated block (the server would 422 it)", async () => {
    const user = userEvent.setup();
    const untranslated = [makeBlock("b1", "Hello world", "")];
    const { adapter } = renderEditor({ view: "visual", blocks: untranslated });
    await waitForBlocks(1);

    await user.click(screen.getByRole("tab", { name: "Review" }));
    expect(screen.getByTestId("approve-btn")).toBeDisabled();
    expect(adapter.reviewBlockCalls).toHaveLength(0);

    // Rejecting an untranslated block is a client-side no-op: the server would
    // 200 it without doing anything, and an optimistic write here would
    // fabricate a phantom {text: "", status} entry that is never rolled back.
    await user.click(screen.getByTestId("reject-btn"));
    expect(adapter.reviewBlockCalls).toHaveLength(0);
    expect(screen.getByText("Not started")).toBeInTheDocument();
  });
});

describe("TranslationEditor — saving an edit preserves the per-locale review status", () => {
  it("keeps the Reviewed chip after a save (optimistic entry keeps the {text, status} shape)", async () => {
    const user = userEvent.setup();
    const reviewed: BlockInfo = {
      ...makeBlock("b1", "Hello world", "Bonjour le monde"),
      targets: { "fr-FR": { text: "Bonjour le monde", status: "reviewed" } },
    };
    const { adapter } = renderEditor({ view: "visual", blocks: [reviewed] });
    const updateCodedSpy = vi.spyOn(adapter, "updateBlockTargetCoded");
    await waitForBlocks(1);

    expect(screen.getByText("Reviewed")).toBeInTheDocument();

    await user.click(screen.getByTestId("target-display"));
    await screen.findByTestId("unified-target-editor");
    await user.click(screen.getByTestId("unified-save"));

    await waitFor(() => expect(updateCodedSpy).toHaveBeenCalledTimes(1));
    // Re-saving IDENTICAL content is not an edit: the review decision still
    // applies (statusAfterEdit), so the optimistic {text, status} entry keeps
    // Reviewed — matching the server, which only demotes a reviewed target
    // when the content actually changes.
    await waitFor(() => expect(screen.getByText("Reviewed")).toBeInTheDocument());
    expect(screen.queryByText("Translated")).not.toBeInTheDocument();
  });
});

describe("TranslationEditor — term insert", () => {
  it("appends to the stored target and persists when no editor is open", async () => {
    const user = userEvent.setup();
    const { adapter } = renderEditor({ view: "visual" });
    const updateSpy = vi.spyOn(adapter, "updateBlockTarget");
    await waitForBlocks(3);

    // Term sidebar renders the mock adapter's term match for the selected block.
    const insertBtn = await screen.findByTestId("term-insert-0-0");
    await user.click(insertBtn);

    await waitFor(() => expect(updateSpy).toHaveBeenCalledTimes(1));
    const req = updateSpy.mock.calls[0][1];
    expect(req.block_id).toBe("b1");
    expect(req.target_locale).toBe("fr-FR");
    expect(req.text).toBe("Bonjour le monde localisation");
  });

  it("inserts at the open editor's cursor without persisting when a target editor is open", async () => {
    const user = userEvent.setup();
    const { adapter } = renderEditor({ view: "visual" });
    const updateSpy = vi.spyOn(adapter, "updateBlockTarget");
    const updateCodedSpy = vi.spyOn(adapter, "updateBlockTargetCoded");
    await waitForBlocks(3);

    // Open the Lexical target editor for the selected block.
    await user.click(screen.getByTestId("target-display"));
    await screen.findByTestId("unified-target-editor");

    await user.click(await screen.findByTestId("term-insert-0-0"));

    // The term lands inside the in-progress edit — no immediate persist.
    await waitFor(() =>
      expect(screen.getByTestId("unified-target-editor").textContent).toContain("localisation"),
    );
    expect(updateSpy).not.toHaveBeenCalled();
    expect(updateCodedSpy).not.toHaveBeenCalled();

    // The user's explicit save carries the inserted term.
    await user.click(screen.getByTestId("unified-save"));
    await waitFor(() => expect(updateCodedSpy).toHaveBeenCalledTimes(1));
    const req = updateCodedSpy.mock.calls[0][1];
    expect(req.block_id).toBe("b1");
    expect(req.coded_text).toContain("localisation");
    expect(req.coded_text).toContain("Bonjour le monde");
  });

  it("falls back to append-and-persist in the Table view when no cell editor is open", async () => {
    const user = userEvent.setup();
    const { adapter } = renderEditor({ view: "table" });
    const updateSpy = vi.spyOn(adapter, "updateBlockTarget");
    await waitForBlocks(3);

    await user.click(screen.getByTestId("block-row-1"));
    // Term matches load for the newly selected block; the sidebar is a
    // Visual-view affordance, so drive the handler through the Visual view.
    await user.click(screen.getByTestId("view-visual"));
    const insertBtn = await screen.findByTestId("term-insert-0-0");
    await user.click(insertBtn);

    await waitFor(() => expect(updateSpy).toHaveBeenCalledTimes(1));
    const req = updateSpy.mock.calls[0][1];
    expect(req.block_id).toBe("b2");
    expect(req.text).toBe("Au revoir localisation");
  });
});
