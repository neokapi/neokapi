import { render, screen } from "./testUtils";
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
