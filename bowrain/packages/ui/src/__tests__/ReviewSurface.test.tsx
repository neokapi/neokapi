/**
 * Component tests for the Review surface's persistence wiring: approve /
 * reject / bulk-mark-reviewed call `api.reviewBlock` with the active target
 * locale, apply the per-locale optimistic update, and roll it back when the
 * server call fails.
 */
import { describe, it, expect, vi } from "vite-plus/test";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import { ReviewSurface } from "../components/ReviewSurface";
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

function renderSurface(blocks: BlockInfo[] = testBlocks): { adapter: MockAdapter } {
  const adapter = createMockAdapter(blocks);
  render(
    <ApiProvider adapter={adapter}>
      <WorkspaceProvider initialWorkspace={mockWorkspace}>
        <BreadcrumbProvider>
          <ReviewSurface project={sampleProject} fileName="messages.json" onBack={vi.fn()} />
        </BreadcrumbProvider>
      </WorkspaceProvider>
    </ApiProvider>,
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

describe("ReviewSurface — bulk mark reviewed", () => {
  it("loops reviewBlock over the selected blocks and reports the count", async () => {
    const user = userEvent.setup();
    const { adapter } = renderSurface();
    await waitForRows();

    await user.click(screen.getByTestId("review-select-b1"));
    await user.click(screen.getByTestId("review-select-b2"));
    await user.click(screen.getByTestId("bulk-mark-reviewed"));

    await waitFor(() => expect(adapter.reviewBlockCalls).toHaveLength(2));
    expect(adapter.reviewBlockCalls.map((c) => c.blockId).sort()).toEqual(["b1", "b2"]);
    for (const call of adapter.reviewBlockCalls) {
      expect(call).toMatchObject({ targetLocale: "fr-FR", reviewed: true });
    }
    await waitFor(() =>
      expect(screen.getByText("Marked 2 block(s) as reviewed")).toBeInTheDocument(),
    );
    expect(screen.getByTestId("review-status-b1").textContent).toBe("Reviewed");
    expect(screen.getByTestId("review-status-b2").textContent).toBe("Reviewed");
  });

  it("disables bulk and per-row review buttons while the bulk loop runs", async () => {
    const user = userEvent.setup();
    const { adapter } = renderSurface();
    await waitForRows();

    // Make reviewBlock hang until released so the loop stays in flight.
    let release!: () => void;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    const original = adapter.reviewBlock.bind(adapter);
    vi.spyOn(adapter, "reviewBlock").mockImplementation(async (...args) => {
      await gate;
      return original(...args);
    });

    await user.click(screen.getByTestId("review-select-b1"));
    await user.click(screen.getByTestId("review-select-b2"));
    await user.click(screen.getByTestId("bulk-mark-reviewed"));

    // While the loop awaits the server, a single approve/reject or a second
    // bulk click would race its optimistic writes and rollbacks — disabled.
    await waitFor(() => expect(screen.getByTestId("bulk-mark-reviewed")).toBeDisabled());
    expect(screen.getByTestId("approve-b2")).toBeDisabled();
    expect(screen.getByTestId("reject-b1")).toBeDisabled();

    release();
    await waitFor(() =>
      expect(screen.getByText("Marked 2 block(s) as reviewed")).toBeInTheDocument(),
    );
    // Buttons re-enable once the loop finishes.
    expect(screen.getByTestId("reject-b1")).toBeEnabled();
  });

  it("skips untranslated blocks in the bulk set instead of looping guaranteed 422s", async () => {
    const user = userEvent.setup();
    const { adapter } = renderSurface([
      makeBlock("b1", "Hello world", "Bonjour le monde"),
      makeBlock("b2", "Goodbye now", ""),
    ]);
    await waitForRows();

    // Select-all on filter=all includes the untranslated block.
    await user.click(screen.getByTestId("select-all"));
    await user.click(screen.getByTestId("bulk-mark-reviewed"));

    await waitFor(() =>
      expect(screen.getByText("Marked 1 block(s) as reviewed")).toBeInTheDocument(),
    );
    expect(adapter.reviewBlockCalls).toHaveLength(1);
    expect(adapter.reviewBlockCalls[0]).toMatchObject({ blockId: "b1", reviewed: true });
    expect(screen.getByTestId("review-status-b2").textContent).toBe("Not Started");
  });

  it("continues past per-block failures, rolls back only the failed block, and reports both counts", async () => {
    const user = userEvent.setup();
    const { adapter } = renderSurface();
    await waitForRows();

    // Fail only b1; b2 succeeds — the loop must keep going.
    const original = adapter.reviewBlock.bind(adapter);
    vi.spyOn(adapter, "reviewBlock").mockImplementation(async (...args) => {
      if (args[3] === "b1") throw new Error("boom");
      return original(...args);
    });

    await user.click(screen.getByTestId("review-select-b1"));
    await user.click(screen.getByTestId("review-select-b2"));
    await user.click(screen.getByTestId("bulk-mark-reviewed"));

    await waitFor(() =>
      expect(screen.getByText("Marked 1 block(s) as reviewed")).toBeInTheDocument(),
    );
    expect(screen.getByText("Couldn't mark 1 block(s) as reviewed")).toBeInTheDocument();
    // The failed block rolled back; the successful one stuck.
    expect(screen.getByTestId("review-status-b1").textContent).toBe("Translated");
    expect(screen.getByTestId("review-status-b2").textContent).toBe("Reviewed");
  });
});
