import { render, screen } from "@testing-library/react";
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
