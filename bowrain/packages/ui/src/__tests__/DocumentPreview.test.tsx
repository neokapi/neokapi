/**
 * Component tests for the document preview's fetching: the item's blocks are
 * one paged query per (item, locale), cached across mounts, and no block is
 * ever fetched on its own.
 */
import { describe, it, expect, vi } from "vite-plus/test";
import { render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

import {
  DocumentPreview,
  documentIdOf,
  idTranslation,
} from "../components/editor/DocumentPreview";
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
  {
    mounts = 1,
    blocks,
    onBlockSelect = vi.fn(),
  }: { mounts?: number; blocks?: BlockInfo[]; onBlockSelect?: (id: string) => void } = {},
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
              onBlockSelect={onBlockSelect}
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


// Two id spaces meet at the iframe boundary: the store mints its own id when a
// block is ingested and keeps the format reader's as source_id, while the
// rendered document marks its blocks with the reader's. Every message crossing
// that boundary has to be spoken in the document's dialect.
//
// Getting it wrong is completely silent — postMessage has no delivery receipt
// and a querySelector that finds nothing is not an error — so it shipped: on
// every item whose format supplied its own preview (the KBF/JSX components the
// dogfood project is made of), the target-locale toggle updated nothing and the
// document went on rendering its source, which reads as the translation having
// gone missing. These assert the two spaces meet, which is the check that would
// have caught it; asserting the render "looks right" is what did not.
describe("DocumentPreview — the document and the store name blocks differently", () => {
  function readerBlock(id: string, sourceId?: string): BlockInfo {
    return {
      id,
      ...(sourceId ? { source_id: sourceId } : {}),
      source: `Source ${id}`,
      translatable: true,
      has_spans: false,
      properties: {},
      targets: { "fr-FR": { text: `Cible ${id}`, status: "translated" } },
    };
  }

  it("addresses a block by the id the document carries", () => {
    expect(documentIdOf(readerBlock("0CERYaui", "App.tsx:135:0"))).toBe("App.tsx:135:0");
  });

  it("falls back to the store's id when the document has no other name for it", () => {
    // The generic preview core/editor builds from the stored blocks themselves
    // marks them with the store's ids and carries no source_id, so there the
    // two spaces are already one.
    expect(documentIdOf(readerBlock("0CERYaui"))).toBe("0CERYaui");
  });

  it("translates both ways", () => {
    const { toDocument, toBlock } = idTranslation([
      readerBlock("b1", "App.tsx:1:0"),
      readerBlock("b2", "App.tsx:2:1"),
      readerBlock("b3"),
    ]);
    expect(toDocument.get("b1")).toBe("App.tsx:1:0");
    expect(toBlock.get("App.tsx:2:1")).toBe("b2");
    expect(toDocument.get("b3")).toBe("b3");
    expect(toBlock.get("b3")).toBe("b3");
  });

  it("selects the block the reader clicked, not a name the surface cannot resolve", async () => {
    const onBlockSelect = vi.fn();
    renderPreview(() => {}, {
      blocks: [readerBlock("0CERYaui", "App.tsx:135:0")],
      onBlockSelect,
    });
    await screen.findByTestId("preview-iframe");

    // The document reports the click under its own name for the block.
    window.postMessage({ type: "kat-block-click", blockId: "App.tsx:135:0" }, "*");

    await waitFor(() => expect(onBlockSelect).toHaveBeenCalledWith("0CERYaui"));
  });

  it("passes an unknown id through rather than swallowing the click", async () => {
    const onBlockSelect = vi.fn();
    renderPreview(() => {}, { blocks: [readerBlock("b1", "App.tsx:1:0")], onBlockSelect });
    await screen.findByTestId("preview-iframe");

    window.postMessage({ type: "kat-block-click", blockId: "App.tsx:99:9" }, "*");

    await waitFor(() => expect(onBlockSelect).toHaveBeenCalledWith("App.tsx:99:9"));
  });

  it("sends its content updates to the id the document will find", async () => {
    renderPreview(() => {}, { blocks: [readerBlock("0CERYaui", "App.tsx:135:0")] });
    const frame = (await screen.findByTestId("preview-iframe")) as HTMLIFrameElement;
    const posted = vi.spyOn(frame.contentWindow!, "postMessage");

    await waitFor(() => {
      const updates = posted.mock.calls.filter(
        (c) => (c[0] as { type?: string })?.type === "kat-update-block",
      );
      expect(updates.length).toBeGreaterThan(0);
      for (const [msg] of updates) {
        expect((msg as { blockId: string }).blockId).toBe("App.tsx:135:0");
      }
    });
  });
});
