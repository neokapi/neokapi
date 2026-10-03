import { render, screen, waitFor, within } from "./testUtils";
import userEvent from "@testing-library/user-event";
import { MemoryChanges } from "../stories/memoryChanges";
import { describe, it, expect, vi } from "vitest";
import { FilePreview } from "../components/FilePreview";
import type { ContentTree } from "@neokapi/ui-primitives/preview";

const tree: ContentTree = {
  format: "json",
  root: [
    {
      kind: "block",
      id: "greeting",
      name: "greeting",
      type: "text",
      translatable: true,
      sourceLocale: "en",
      source: [{ text: "Please utilize the dashboard" }],
      targets: { fr: [{ text: "Veuillez utiliser le tableau de bord" }] },
      overlays: [
        {
          type: "term",
          side: "source",
          spans: [
            {
              range: { startRun: 0, startOffset: 19, endRun: 1, endOffset: 28 },
              text: "dashboard",
              props: { term: "dashboard", target: "tableau de bord" },
            },
          ],
        },
      ],
    },
  ],
  stats: { layers: 0, groups: 0, blocks: 1, data: 0, media: 0, runs: 1 },
};

describe("FilePreview", () => {
  it("renders the DocumentViewer from a preset tree without a backend", () => {
    render(
      <FilePreview
        tabID="tab-1"
        filePath="/abs/locales/en.json"
        filename="locales/en.json"
        onClose={vi.fn()}
        tree={tree}
      />,
    );
    // Header shows the filename and the DocumentViewer tabs render.
    expect(screen.getAllByText("locales/en.json").length).toBeGreaterThan(0);
    expect(screen.getByRole("tab", { name: /preview/i })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: /blocks/i })).toBeInTheDocument();
    // The source text is rendered.
    expect(screen.getByText(/Please/)).toBeInTheDocument();
  });

  it("is closed (renders nothing visible) when filePath is null", () => {
    render(<FilePreview tabID="tab-1" filePath={null} filename="" onClose={vi.fn()} tree={tree} />);
    expect(screen.queryByRole("tab", { name: /preview/i })).not.toBeInTheDocument();
  });
});

describe("FilePreview at a block named by id", () => {
  const named: ContentTree = {
    format: "json",
    root: [
      {
        kind: "block",
        id: "b1",
        name: "app.greeting",
        type: "text",
        translatable: true,
        sourceLocale: "en",
        source: [{ text: "Please utilize the dashboard" }],
      },
    ],
    stats: { layers: 0, groups: 0, blocks: 1, data: 0, media: 0, runs: 1 },
  };

  it("addresses the block by its unit key and marks the highlighted span", () => {
    render(
      <FilePreview
        tabID="tab-1"
        filePath="/abs/locales/en.json"
        filename="locales/en.json"
        onClose={vi.fn()}
        tree={named}
        focusBlockID="b1"
        focusNote={<span>Major</span>}
        backLabel="Back to checks"
        highlights={{
          b1: [
            {
              side: "source",
              anchor: { kind: "range", start: { run: 0, offset: 7 }, end: { run: 0, offset: 14 } },
              tone: "destructive",
              label: 'Forbidden term "utilize" found',
              emphasis: "focus",
            },
          ],
        }}
      />,
    );
    const row = document.querySelector('[data-slot="file-preview-focus"]') as HTMLElement;
    expect(row).toHaveTextContent("app.greeting");
    expect(row).toHaveTextContent("Major");
    expect(row).toHaveTextContent("Back to checks");
    expect(document.querySelector('[data-review-focus="true"]')).toBeTruthy();
    const mark = document.querySelector('mark[data-overlay-type="finding"]');
    expect(mark).toHaveTextContent("utilize");
    expect(mark?.getAttribute("data-emphasis")).toBe("focus");
  });

  it("keeps the id when the tree does not hold the block, and says so", () => {
    render(
      <FilePreview
        tabID="tab-1"
        filePath="/abs/locales/en.json"
        filename="locales/en.json"
        onClose={vi.fn()}
        tree={named}
        focusBlockID="missing"
      />,
    );
    const row = document.querySelector('[data-slot="file-preview-focus"]') as HTMLElement;
    expect(row).toHaveTextContent("missing");
    expect(screen.getByText("This unit is not in the rendered document.")).toBeInTheDocument();
  });
});

// The document view draws; the edit beside it commits, as a change set sent
// with the revision it read.
describe("FilePreview editing a unit", () => {
  function renderEditable(changes: MemoryChanges) {
    render(
      <FilePreview
        tabID="tab-1"
        filePath="/abs/locales/en.json"
        filename="locales/en.json"
        onClose={vi.fn()}
        tree={tree}
        focusKey="greeting"
        changes={changes}
      />,
    );
  }
  const source = (): MemoryChanges =>
    new MemoryChanges([
      {
        doc: "/abs/locales/en.json",
        block: "greeting",
        ref: { doc: "locales/en.json", block: "greeting" },
        text: "Please utilize the dashboard",
        editions: { fr: { text: "Veuillez utiliser le tableau de bord" } },
      },
    ]);

  it("edits the focused unit's source and sends it with the revision it read", async () => {
    const changes = source();
    const [before] = (await changes.read({ doc: "/abs/locales/en.json" })).blocks;
    renderEditable(changes);
    await userEvent.click(screen.getByRole("button", { name: "Edit" }));
    const editor = await waitFor(() => {
      const el = document.querySelector<HTMLElement>(
        "[data-slot='unit-edit-editor'] [contenteditable='true']",
      );
      expect(el).not.toBeNull();
      return el!;
    });
    await userEvent.type(editor, "Now ");
    const typed = editor.textContent;
    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(changes.sets).toHaveLength(1));
    expect(changes.sets[0].ops[0]).toEqual({
      op: "set_content",
      at: { doc: "locales/en.json", block: "greeting" },
      if_match: before.rev,
      text: typed,
    });
  });

  it("offers no edit, and draws no focus row, for a file opened whole", () => {
    render(
      <FilePreview
        tabID="tab-1"
        filePath="/abs/locales/en.json"
        filename="locales/en.json"
        onClose={vi.fn()}
        tree={tree}
        changes={source()}
      />,
    );
    expect(document.querySelector('[data-slot="file-preview-focus"]')).toBeNull();
    expect(screen.queryByRole("button", { name: "Edit" })).not.toBeInTheDocument();
  });

  // A plural translation is edited a form at a time from the document view
  // too: the read lists the translation's own plural, and the edit names the
  // form by its path.
  it("edits one form of a plural translation by its path", async () => {
    const n = { kind: "placeholder", type: "jsx:var", equiv: "count" };
    const changes = new MemoryChanges([
      {
        doc: "/abs/locales/en.json",
        block: "greeting",
        ref: { doc: "locales/en.json", block: "greeting" },
        text: 'You have <x id="n/"/> items',
        codes: { "n/": n },
        editions: {
          fr: {
            text: 'Vous avez <x id="n/"/> articles',
            structures: [
              {
                path: [0],
                kind: "plural",
                pivot: "count",
                branches: {
                  one: 'Un seul <x id="n/"/> article',
                  other: 'Vous avez <x id="n/"/> articles',
                },
              },
            ],
          },
        },
      },
    ]);
    const [before] = (await changes.read({ doc: "/abs/locales/en.json" })).blocks;
    render(
      <FilePreview
        tabID="tab-1"
        filePath="/abs/locales/en.json"
        filename="locales/en.json"
        onClose={vi.fn()}
        tree={tree}
        focusKey="greeting"
        side="fr"
        changes={changes}
      />,
    );
    await userEvent.click(screen.getByRole("button", { name: "Edit" }));
    const one = await waitFor(() => {
      const el = document.querySelector<HTMLElement>(
        "[data-slot='edition-editor-plural-form'][data-form='one'] [contenteditable='true']",
      );
      expect(el).not.toBeNull();
      return el!;
    });
    await userEvent.type(one, "Plus ");
    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(changes.sets).toHaveLength(1));
    const op = changes.sets[0].ops[0];
    expect(op).toMatchObject({
      op: "set_content",
      at: { doc: "locales/en.json", block: "greeting", edition: "fr" },
      if_match: before.editions?.fr.rev,
      path: [0, { plural: "one" }],
    });
    expect(op.op === "set_content" && op.text).toContain('<x id="n/"/>');
    await waitFor(() => expect(document.querySelector("[data-slot='edition-refused']")).toBeNull());
  });

  it("switches to the translation the document carries", async () => {
    renderEditable(source());
    await userEvent.click(screen.getByRole("button", { name: "Edit" }));
    const sides = await waitFor(() => {
      const el = document.querySelector("[data-slot='unit-edit-sides']");
      expect(el).not.toBeNull();
      return el!;
    });
    const buttons = within(sides as HTMLElement).getAllByRole("button");
    await userEvent.click(buttons[buttons.length - 1]);
    await waitFor(() =>
      expect(
        document.querySelector("[data-slot='unit-edit-editor'] [contenteditable='true']")
          ?.textContent,
      ).toBe("Veuillez utiliser le tableau de bord"),
    );
  });
});
