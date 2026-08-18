/**
 * Component tests for the document preview's fetching: the item's blocks are
 * one paged query per (item, locale), cached across mounts, and no block is
 * ever fetched on its own.
 */
import { describe, it, expect, vi } from "vite-plus/test";
import { render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

import { DocumentPreview } from "../components/editor/DocumentPreview";
import { ApiProvider } from "../context/ApiContext";
import { WorkspaceProvider } from "../context/WorkspaceContext";
import { createMockAdapter, type MockAdapter } from "../stories/mock-adapter";
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

function renderPreview(
  prepare: (adapter: MockAdapter) => void,
  { mounts = 1, blocks }: { mounts?: number; blocks?: BlockInfo[] } = {},
) {
  const adapter = createMockAdapter(blocks);
  prepare(adapter);
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={qc}>
      <ApiProvider adapter={adapter}>
        <WorkspaceProvider initialWorkspace={mockWorkspace}>
          {Array.from({ length: mounts }, (_, i) => (
            <DocumentPreview
              key={i}
              projectId="proj-1"
              itemName="messages.json"
              targetLocale="fr-FR"
              previewContentMode="target"
              onBlockSelect={vi.fn()}
            />
          ))}
        </WorkspaceProvider>
      </ApiProvider>
    </QueryClientProvider>,
  );
}

describe("DocumentPreview — the document is one query, never one per block", () => {
  it("pages the item's blocks for the locale and asks for no block HTML", async () => {
    let list!: ReturnType<typeof vi.spyOn<MockAdapter, "getFileBlocks">>;
    let blockHTML!: ReturnType<typeof vi.spyOn<MockAdapter, "renderBlockHTML">>;
    renderPreview((adapter) => {
      list = vi.spyOn(adapter, "getFileBlocks");
      blockHTML = vi.spyOn(adapter, "renderBlockHTML");
    });

    await waitFor(() => expect(screen.getByTestId("preview-iframe")).toBeInTheDocument());
    await waitFor(() => expect(list).toHaveBeenCalledTimes(1));
    expect(list.mock.calls[0][1]).toBe("proj-1");
    expect(list.mock.calls[0][2]).toBe("messages.json");
    expect(list.mock.calls[0][4]).toMatchObject({ locale: "fr-FR", limit: 500, offset: 0 });
    // The target text travels in the blocks payload, so the per-block render
    // route is not called at all.
    expect(blockHTML).not.toHaveBeenCalled();
  });

  it("serves a second mount of the same item and locale from the cache", async () => {
    let list!: ReturnType<typeof vi.spyOn<MockAdapter, "getFileBlocks">>;
    renderPreview(
      (adapter) => {
        list = vi.spyOn(adapter, "getFileBlocks");
      },
      { mounts: 2 },
    );

    await waitFor(() => expect(screen.getAllByTestId("preview-iframe")).toHaveLength(2));
    await waitFor(() => expect(list).toHaveBeenCalledTimes(1));
  });
});

// Reading a locale a document has not been translated into shows the source —
// a document is still a document with its translation outstanding. Showing it
// *silently*, under the locale's own name, reads as a translation that happens
// to match the source, or as a toggle that did nothing.
describe("DocumentPreview — what the target side is actually showing", () => {
  function block(id: string, target?: string): BlockInfo {
    return {
      id,
      source: `Source ${id}`,
      translatable: true,
      has_spans: false,
      properties: {},
      targets: target ? { "fr-FR": { text: target, status: "translated" } } : {},
    };
  }

  it("says so when nothing is translated into the locale", async () => {
    renderPreview(() => {}, { blocks: [block("b1"), block("b2")] });

    const notice = await screen.findByTestId("preview-coverage");
    expect(notice).toHaveTextContent(/Nothing is translated into/);
  });

  it("counts the blocks that are, when only some are", async () => {
    renderPreview(() => {}, { blocks: [block("b1", "Bonjour"), block("b2"), block("b3")] });

    const notice = await screen.findByTestId("preview-coverage");
    expect(notice).toHaveTextContent(/1 of 3 blocks are translated/);
    expect(notice).toHaveTextContent(/the rest read as the source/);
  });

  it("says nothing when the document is fully translated", async () => {
    renderPreview(() => {}, { blocks: [block("b1", "Bonjour"), block("b2", "Salut")] });

    await waitFor(() => expect(screen.getByTestId("preview-iframe")).toBeInTheDocument());
    expect(screen.queryByTestId("preview-coverage")).toBeNull();
  });

  it("counts only the blocks there is something to translate in", async () => {
    const untranslatable: BlockInfo = { ...block("b3"), translatable: false };
    renderPreview(() => {}, { blocks: [block("b1", "Bonjour"), untranslatable] });

    await waitFor(() => expect(screen.getByTestId("preview-iframe")).toBeInTheDocument());
    expect(screen.queryByTestId("preview-coverage")).toBeNull();
  });
});
