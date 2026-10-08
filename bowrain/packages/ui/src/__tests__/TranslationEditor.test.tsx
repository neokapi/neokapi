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
import { mockRevision } from "../stories/mockContentChanges";
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
          status: { "not-started": 20, draft: 5, translated: 4, established: 7 },
        });
      },
    });
    await waitForBlocks(3);

    await waitFor(() =>
      expect(screen.getByTestId("progress-text").textContent).toContain("(16/36 translated)"),
    );
    const text = screen.getByTestId("progress-text").textContent ?? "";
    expect(text).toContain("44%");
    expect(text).toContain("7 approved");
    expect(text).toContain("20 pending");
  });
});

describe("TranslationEditor — review actions persist as decide operations", () => {
  it("Approve in the Visual card's Review mode decides on the revision it showed", async () => {
    const user = userEvent.setup();
    const { adapter } = renderEditor({ view: "visual" });
    await waitForBlocks(3);

    await user.click(screen.getByRole("tab", { name: "Review" }));
    await user.click(screen.getByTestId("approve-btn"));

    await waitFor(() => expect(adapter.opsOf("decide")).toHaveLength(1));
    expect(adapter.opsOf("decide")[0]).toMatchObject({
      workspaceSlug: "demo",
      projectId: sampleProject.id,
      at: { doc: "messages.json", block: "b1", edition: "fr-FR" },
      if_match: mockRevision(testBlocks[0], "fr-FR"),
      outcome: "establish",
    });
    // Approve advances to the next block once the call resolves; step back to
    // see b1's chip, driven by the optimistic per-locale Edition.Status write.
    await waitFor(() =>
      expect(screen.getByTestId("status-bar").textContent).toContain("Block 2 of 3"),
    );
    await user.click(screen.getByTestId("prev-block-btn"));
    expect(screen.getByText("Approved")).toBeInTheDocument();
  });

  it("Reject sends a reject decision (re-enters the work queue)", async () => {
    const user = userEvent.setup();
    const { adapter } = renderEditor({ view: "visual" });
    await waitForBlocks(3);

    await user.click(screen.getByRole("tab", { name: "Review" }));
    await user.click(screen.getByTestId("reject-btn"));

    await waitFor(() => expect(adapter.opsOf("decide")).toHaveLength(1));
    // A rejection demotes to draft (host's rejected → draft mapping), not to
    // translated — a rejected text must not keep passing coverage gates.
    expect(adapter.opsOf("decide")[0]).toMatchObject({
      at: { block: "b1", edition: "fr-FR" },
      outcome: "reject",
    });
  });

  it("rolls back the optimistic status, stays on the block, and surfaces an error when the call fails", async () => {
    const user = userEvent.setup();
    const { adapter } = renderEditor({ view: "visual" });
    adapter.failApplyChanges = true;
    await waitForBlocks(3);

    await user.click(screen.getByRole("tab", { name: "Review" }));
    expect(screen.getByText("Translated")).toBeInTheDocument();
    await user.click(screen.getByTestId("approve-btn"));

    await waitFor(() =>
      expect(screen.getByText("Couldn't mark the block as approved")).toBeInTheDocument(),
    );
    expect(adapter.opsOf("decide")).toHaveLength(1);
    // Approve only advances after a SUCCESSFUL persist — on failure the
    // reviewer stays on the failed block, with its chip rolled back, so the
    // error refers to the block on screen.
    expect(screen.getByTestId("status-bar").textContent).toContain("Block 1 of 3");
    expect(screen.getByText("Translated")).toBeInTheDocument();
    expect(screen.queryByText("Approved")).not.toBeInTheDocument();
  });

  it("disables Approve for an untranslated block (the server would 422 it)", async () => {
    const user = userEvent.setup();
    const untranslated = [makeBlock("b1", "Hello world", "")];
    const { adapter } = renderEditor({ view: "visual", blocks: untranslated });
    await waitForBlocks(1);

    await user.click(screen.getByRole("tab", { name: "Review" }));
    expect(screen.getByTestId("approve-btn")).toBeDisabled();
    expect(adapter.opsOf("decide")).toHaveLength(0);

    // Rejecting an untranslated block is a client-side no-op: the server would
    // 200 it without doing anything, and an optimistic write here would
    // fabricate a phantom {text: "", status} entry that is never rolled back.
    await user.click(screen.getByTestId("reject-btn"));
    expect(adapter.opsOf("decide")).toHaveLength(0);
    expect(screen.getByText("Not started")).toBeInTheDocument();
  });
});

describe("TranslationEditor — saving an edit preserves the per-locale review status", () => {
  it("keeps the Reviewed chip after a save (optimistic entry keeps the {text, status} shape)", async () => {
    const user = userEvent.setup();
    const reviewed: BlockInfo = {
      ...makeBlock("b1", "Hello world", "Bonjour le monde"),
      targets: { "fr-FR": { text: "Bonjour le monde", status: "established" } },
    };
    const { adapter } = renderEditor({ view: "visual", blocks: [reviewed] });
    await waitForBlocks(1);

    expect(screen.getByText("Approved")).toBeInTheDocument();

    await user.click(screen.getByTestId("target-display"));
    await screen.findByTestId("unified-target-editor");
    await user.click(screen.getByTestId("unified-save"));

    await waitFor(() => expect(adapter.opsOf("set_content")).toHaveLength(1));
    // Re-saving IDENTICAL content is not an edit: the review decision still
    // applies (statusAfterEdit), so the optimistic {text, status} entry keeps
    // Established, matching the change service, which leaves an unchanged
    // translation as it stands.
    await waitFor(() => expect(screen.getByText("Approved")).toBeInTheDocument());
    expect(screen.queryByText("Translated")).not.toBeInTheDocument();
  });
});

describe("TranslationEditor — a save names the revision the editor showed", () => {
  it("saves the runs the editor produced on the revision it read", async () => {
    const user = userEvent.setup();
    const { adapter } = renderEditor({ view: "visual" });
    await waitForBlocks(3);

    await user.click(screen.getByTestId("target-display"));
    await screen.findByTestId("unified-target-editor");
    await user.click(screen.getByTestId("unified-save"));

    await waitFor(() => expect(adapter.opsOf("set_content")).toHaveLength(1));
    expect(adapter.opsOf("set_content")[0]).toMatchObject({
      at: { doc: "messages.json", block: "b1", edition: "fr-FR" },
      if_match: mockRevision(testBlocks[0], "fr-FR"),
      runs: [{ text: "Bonjour le monde" }],
    });
  });

  it("shows a translation someone saved meanwhile, and saves over it only when asked", async () => {
    const user = userEvent.setup();
    const { adapter } = renderEditor({ view: "visual" });
    await waitForBlocks(3);

    await user.click(screen.getByTestId("target-display"));
    await screen.findByTestId("unified-target-editor");
    adapter.editElsewhere("b1", "fr-FR", "Salut le monde");
    await user.click(screen.getByTestId("unified-save"));

    const dialog = await screen.findByTestId("stale-change-dialog");
    expect(screen.getByTestId("stale-current").textContent).toBe("Salut le monde");
    expect(screen.getByTestId("stale-mine").textContent).toBe("Bonjour le monde");
    expect(adapter.opsOf("set_content")).toHaveLength(1);

    await user.click(screen.getByTestId("stale-reapply"));
    await waitFor(() => expect(adapter.opsOf("set_content")).toHaveLength(2));
    const [first, second] = adapter.opsOf("set_content");
    expect(second.if_match).not.toBe(first.if_match);
    expect(dialog).not.toBeInTheDocument();
    // The edit landed: the next block is up, and the block holds the editor's text.
    await waitFor(() =>
      expect(screen.getByTestId("status-bar").textContent).toContain("Block 2 of 3"),
    );
    const saved = await adapter.getBlock("demo", sampleProject.id, "b1");
    expect(saved.targets["fr-FR"]).toMatchObject({ text: "Bonjour le monde" });
  });

  it("keeps the other translation and closes the editor when asked to", async () => {
    const user = userEvent.setup();
    const { adapter } = renderEditor({ view: "visual" });
    await waitForBlocks(3);

    await user.click(screen.getByTestId("target-display"));
    await screen.findByTestId("unified-target-editor");
    adapter.editElsewhere("b1", "fr-FR", "Salut le monde");
    await user.click(screen.getByTestId("unified-save"));
    await user.click(await screen.findByTestId("stale-keep"));

    await waitFor(() =>
      expect(screen.queryByTestId("unified-target-editor")).not.toBeInTheDocument(),
    );
    expect(adapter.opsOf("set_content")).toHaveLength(1);
    await waitFor(() =>
      expect(screen.getByTestId("target-display").textContent).toContain("Salut le monde"),
    );
  });
});

describe("TranslationEditor — term insert", () => {
  it("appends to the stored target and persists when no editor is open", async () => {
    const user = userEvent.setup();
    const { adapter } = renderEditor({ view: "visual" });
    await waitForBlocks(3);

    // Term sidebar renders the mock adapter's term match for the selected block.
    const insertBtn = await screen.findByTestId("term-insert-0-0");
    await user.click(insertBtn);

    await waitFor(() => expect(adapter.opsOf("set_content")).toHaveLength(1));
    expect(adapter.opsOf("set_content")[0]).toMatchObject({
      at: { block: "b1", edition: "fr-FR" },
      if_match: mockRevision(testBlocks[0], "fr-FR"),
      runs: [{ text: "Bonjour le monde localisation" }],
    });
  });

  it("keeps the translation's inline codes when it appends the term", async () => {
    const user = userEvent.setup();
    const bold: BlockInfo = {
      id: "b1",
      source: "Save now",
      source_runs: [
        { text: "Save " },
        { pcOpen: { id: "1", type: "fmt:bold", data: "<b>", equiv: "b" } },
        { text: "now" },
        { pcClose: { id: "1", type: "fmt:bold", data: "</b>", equiv: "b" } },
      ],
      targets: { "fr-FR": { text: "Enregistrer maintenant", status: "translated" } },
      targets_runs: {
        "fr-FR": [
          { text: "Enregistrer " },
          { pcOpen: { id: "1", type: "fmt:bold", data: "<b>", equiv: "b" } },
          { text: "maintenant" },
          { pcClose: { id: "1", type: "fmt:bold", data: "</b>", equiv: "b" } },
        ],
      },
      translatable: true,
      has_spans: true,
      properties: {},
    };
    const { adapter } = renderEditor({ view: "visual", blocks: [bold] });
    await waitForBlocks(1);

    await user.click(await screen.findByTestId("term-insert-0-0"));

    await waitFor(() => expect(adapter.opsOf("set_content")).toHaveLength(1));
    const op = adapter.opsOf("set_content")[0];
    expect(op.text).toBeUndefined();
    expect(op.runs).toEqual([
      { text: "Enregistrer " },
      { pcOpen: { id: "1", type: "fmt:bold", equiv: "b" } },
      { text: "maintenant" },
      { pcClose: { id: "1", type: "fmt:bold", equiv: "b" } },
      { text: " localisation" },
    ]);
  });

  it("inserts at the open editor's cursor without persisting when a target editor is open", async () => {
    const user = userEvent.setup();
    const { adapter } = renderEditor({ view: "visual" });
    await waitForBlocks(3);

    // Open the Lexical target editor for the selected block.
    await user.click(screen.getByTestId("target-display"));
    await screen.findByTestId("unified-target-editor");

    await user.click(await screen.findByTestId("term-insert-0-0"));

    // The term lands inside the in-progress edit — no immediate persist.
    await waitFor(() =>
      expect(screen.getByTestId("unified-target-editor").textContent).toContain("localisation"),
    );
    expect(adapter.changeSetCalls).toHaveLength(0);

    // The user's explicit save carries the inserted term.
    await user.click(screen.getByTestId("unified-save"));
    await waitFor(() => expect(adapter.opsOf("set_content")).toHaveLength(1));
    const op = adapter.opsOf("set_content")[0];
    expect(op.at.block).toBe("b1");
    const saved = JSON.stringify(op.runs);
    expect(saved).toContain("localisation");
    expect(saved).toContain("Bonjour le monde");
  });

  it("falls back to append-and-persist in the Table view when no cell editor is open", async () => {
    const user = userEvent.setup();
    const { adapter } = renderEditor({ view: "table" });
    await waitForBlocks(3);

    await user.click(screen.getByTestId("block-row-1"));
    // Term matches load for the newly selected block; the sidebar is a
    // Visual-view affordance, so drive the handler through the Visual view.
    await user.click(screen.getByTestId("view-visual"));
    const insertBtn = await screen.findByTestId("term-insert-0-0");
    await user.click(insertBtn);

    await waitFor(() => expect(adapter.opsOf("set_content")).toHaveLength(1));
    expect(adapter.opsOf("set_content")[0]).toMatchObject({
      at: { block: "b2" },
      runs: [{ text: "Au revoir localisation" }],
    });
  });
});

describe("TranslationEditor — a content-memory match", () => {
  it("is saved as its runs, so the codes its text leaves out stay", async () => {
    const user = userEvent.setup();
    const name = { ph: { id: "name", type: "code:variable", data: "{name}", equiv: "{name}" } };
    const greeting: BlockInfo = {
      id: "b1",
      source: "Hello ",
      source_runs: [{ text: "Hello " }, name],
      targets: {},
      translatable: true,
      has_spans: true,
      properties: {},
    };
    const { adapter } = renderEditor({
      view: "visual",
      blocks: [greeting],
      prepare: (a) => {
        a.lookupMemoryForBlock = async () => [
          {
            source: "Hello {name}",
            target: "Bonjour {name}",
            target_runs: [{ text: "Bonjour " }, name],
            score: 1,
            match_type: "exact",
          },
        ];
      },
    });
    await waitForBlocks(1);

    await user.click(await screen.findByTestId("tm-apply-0"));

    await waitFor(() => expect(adapter.opsOf("set_content")).toHaveLength(1));
    const op = adapter.opsOf("set_content")[0];
    expect(op).toMatchObject({ if_match: "absent" });
    expect(op.text).toBeUndefined();
    expect(op.runs).toEqual([
      { text: "Bonjour " },
      { ph: { id: "name", type: "code:variable", equiv: "{name}" } },
    ]);
  });
});

describe("TranslationEditor — a save the project's checks refuse", () => {
  const finding = {
    rule: "terms.vocabulary",
    message: 'Use "réglages", not "paramètres"',
    fails: true,
  };

  it("shows the findings, and saves anyway only when asked", async () => {
    const user = userEvent.setup();
    const { adapter } = renderEditor({
      view: "visual",
      prepare: (a) => {
        a.failingCheck = [finding];
      },
    });
    await waitForBlocks(3);

    await user.click(screen.getByTestId("target-display"));
    await screen.findByTestId("unified-target-editor");
    await user.click(screen.getByTestId("unified-save"));

    const dialog = await screen.findByTestId("check-findings-dialog");
    expect(screen.getByTestId("check-finding").textContent).toContain(finding.message);
    expect(screen.getByTestId("check-findings-mine").textContent).toBe("Bonjour le monde");
    expect(adapter.changeSetCalls).toHaveLength(1);
    expect(adapter.changeSetCalls[0].set.gate).toBeUndefined();

    await user.click(screen.getByTestId("findings-override"));
    await waitFor(() => expect(adapter.changeSetCalls).toHaveLength(2));
    expect(adapter.changeSetCalls[1].set.gate).toBe("report");
    expect(adapter.changeSetCalls[1].set.ops[0]).toEqual(adapter.changeSetCalls[0].set.ops[0]);
    expect(dialog).not.toBeInTheDocument();
    await waitFor(() =>
      expect(screen.getByTestId("status-bar").textContent).toContain("Block 2 of 3"),
    );
  });

  it("keeps the editor open on the person's wording when they do not save", async () => {
    const user = userEvent.setup();
    const { adapter } = renderEditor({
      view: "visual",
      prepare: (a) => {
        a.failingCheck = [finding];
      },
    });
    await waitForBlocks(3);

    await user.click(screen.getByTestId("target-display"));
    await screen.findByTestId("unified-target-editor");
    await user.click(screen.getByTestId("unified-save"));
    await user.click(await screen.findByTestId("findings-revise"));

    await waitFor(() =>
      expect(screen.queryByTestId("check-findings-dialog")).not.toBeInTheDocument(),
    );
    expect(screen.getByTestId("unified-target-editor")).toBeInTheDocument();
    expect(adapter.changeSetCalls).toHaveLength(1);
    expect(screen.queryByText("Couldn't save the translation")).not.toBeInTheDocument();
  });
});

describe("TranslationEditor — a plural translation", () => {
  const n = { ph: { id: "n", type: "code:variable", data: "#", equiv: "#" } };
  const pluralBlock: BlockInfo = {
    id: "b1",
    source: " items",
    source_runs: [
      {
        plural: {
          pivot: "count",
          forms: { one: [n, { text: " item" }], other: [n, { text: " items" }] },
        },
      },
    ],
    targets: { "fr-FR": { text: " articles", status: "translated" } },
    targets_runs: {
      "fr-FR": [
        {
          plural: {
            pivot: "count",
            forms: { one: [n, { text: " article" }], other: [n, { text: " articles" }] },
          },
        },
      ],
    },
    translatable: true,
    has_spans: true,
    properties: {},
  };

  it("opens on its forms and saves the whole plural as runs", async () => {
    const user = userEvent.setup();
    const { adapter } = renderEditor({ view: "visual", blocks: [pluralBlock] });
    await waitForBlocks(1);

    await user.click(screen.getByTestId("target-display"));
    const editor = await screen.findByTestId("unified-target-editor");
    expect(editor.getAttribute("data-mode")).toBe("plural");
    await user.click(screen.getByTestId("unified-save"));

    await waitFor(() => expect(adapter.opsOf("set_content")).toHaveLength(1));
    const op = adapter.opsOf("set_content")[0];
    expect(op.text).toBeUndefined();
    expect(op.runs).toEqual([
      {
        plural: {
          pivot: "count",
          forms: {
            one: [{ ph: { id: "n", type: "code:variable", equiv: "#" } }, { text: " article" }],
            other: [{ ph: { id: "n", type: "code:variable", equiv: "#" } }, { text: " articles" }],
          },
        },
      },
    ]);
  });
});

describe("TranslationEditor — a note", () => {
  it("is shown once the change lands, even when the list cannot be read back", async () => {
    const user = userEvent.setup();
    const { adapter } = renderEditor({
      view: "visual",
      prepare: (a) => {
        // The desktop queues a note while the server is out of reach; the list
        // is the server's, and cannot be read until it returns.
        let reads = 0;
        const list = a.listBlockNotes.bind(a);
        a.listBlockNotes = async (...args) => {
          reads += 1;
          if (reads > 1) throw new Error("the server is out of reach");
          return list(...args);
        };
      },
    });
    await waitForBlocks(3);

    await user.click(screen.getByRole("tab", { name: "Enrich" }));
    await user.type(screen.getByTestId("note-input"), "Check the tone");
    await user.click(screen.getByTestId("submit-note-btn"));

    await waitFor(() => expect(adapter.opsOf("annotate")).toHaveLength(1));
    expect(await screen.findByText("Check the tone")).toBeInTheDocument();
    expect(screen.queryByText("Couldn't add the note")).not.toBeInTheDocument();
  });
});

describe("TranslationEditor: the problems panel", () => {
  it("says the checks did not run when the check request fails, never that nothing was found", async () => {
    const user = userEvent.setup();
    renderEditor({
      prepare: (adapter) => {
        vi.spyOn(adapter, "runFileCheck").mockRejectedValue(new Error("check service unavailable"));
      },
    });
    await waitForBlocks(3);

    await user.click(screen.getByTestId("problems-toggle"));
    expect(await screen.findByTestId("problems-check-failed")).toBeInTheDocument();
    expect(screen.queryByText(/No issues found/)).not.toBeInTheDocument();
  });
});
