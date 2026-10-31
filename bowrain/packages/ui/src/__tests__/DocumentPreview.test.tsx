/**
 * Component tests for the document preview's fetching: the item's blocks are
 * one paged query per (item, locale), cached across mounts, and no block is
 * ever fetched on its own.
 */
import { describe, it, expect, vi } from "vite-plus/test";
import { render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

import { DocumentPreview, blockKeyOf, idTranslation } from "../components/editor/DocumentPreview";
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
    selectedBlockId,
  }: {
    mounts?: number;
    blocks?: BlockInfo[];
    onBlockSelect?: (id: string) => void;
    selectedBlockId?: string;
  } = {},
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
              selectedBlockId={selectedBlockId}
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

// Two id spaces meet at the iframe boundary. The store gives each block a row
// id and keeps its durable key as source_id; the server renders the document
// with each <kat-block> marked by that key (core/editor.BuildPreview marks with
// convergence.BlockKey, which is also what the store keys source_id on). Every
// message crossing the boundary names a block by that key.
//
// A mismatch is silent: postMessage has no delivery receipt, and a
// querySelector that finds nothing raises no error. The target-locale toggle
// then updates nothing and the document keeps its source. These assert the two
// spaces meet, for the two shapes the key takes: the key a push or an upload
// resolved, and the name a reader gave the block.
const KEYED = [
  { shape: "a pushed file", rowId: "0CERYaui", key: "u-645b61b044a769e4" },
  { shape: "a named reader", rowId: "fqJRlT74", key: "errors.network.timeout" },
] as const;

function storedBlock(id: string, sourceId?: string): BlockInfo {
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

function postedOfType(frame: HTMLIFrameElement, type: string) {
  const posted = vi.spyOn(frame.contentWindow!, "postMessage");
  return () =>
    posted.mock.calls
      .map(([msg]) => msg as { type?: string; blockId?: string })
      .filter((msg) => msg?.type === type);
}

describe.each(KEYED)("DocumentPreview — $shape", ({ rowId, key }) => {
  it("names the block by its stored key", () => {
    expect(blockKeyOf(storedBlock(rowId, key))).toBe(key);
  });

  it("translates both ways", () => {
    const { toDocument, toBlock } = idTranslation([storedBlock(rowId, key), storedBlock("b2")]);
    expect(toDocument.get(rowId)).toBe(key);
    expect(toBlock.get(key)).toBe(rowId);
  });

  it("selects the row of the block the reader clicked", async () => {
    const onBlockSelect = vi.fn();
    renderPreview(() => {}, { blocks: [storedBlock(rowId, key)], onBlockSelect });
    await screen.findByTestId("preview-iframe");

    // The document reports the click under the key it marks the block with.
    window.postMessage({ type: "kat-block-click", blockId: key }, "*");

    await waitFor(() => expect(onBlockSelect).toHaveBeenCalledWith(rowId));
  });

  it("marks the selected block by its key", async () => {
    renderPreview(() => {}, { blocks: [storedBlock(rowId, key)], selectedBlockId: rowId });
    const frame = (await screen.findByTestId("preview-iframe")) as HTMLIFrameElement;
    const selections = postedOfType(frame, "kat-select-block");

    await waitFor(() => {
      expect(selections().length).toBeGreaterThan(0);
      for (const msg of selections()) expect(msg.blockId).toBe(key);
    });
  });

  it("sends its content updates to the key the document marks", async () => {
    renderPreview(() => {}, { blocks: [storedBlock(rowId, key)] });
    const frame = (await screen.findByTestId("preview-iframe")) as HTMLIFrameElement;
    const updates = postedOfType(frame, "kat-update-block");

    await waitFor(() => {
      expect(updates().length).toBeGreaterThan(0);
      for (const msg of updates()) expect(msg.blockId).toBe(key);
    });
  });
});

describe("DocumentPreview — a block the document does not mark", () => {
  it("falls back to the row id for a block stored without an item", () => {
    expect(blockKeyOf(storedBlock("0CERYaui"))).toBe("0CERYaui");
  });

  it("passes an unknown id through rather than swallowing the click", async () => {
    const onBlockSelect = vi.fn();
    renderPreview(() => {}, {
      blocks: [storedBlock("b1", "u-645b61b044a769e4")],
      onBlockSelect,
    });
    await screen.findByTestId("preview-iframe");

    window.postMessage({ type: "kat-block-click", blockId: "u-ffffffffffffffff" }, "*");

    await waitFor(() => expect(onBlockSelect).toHaveBeenCalledWith("u-ffffffffffffffff"));
  });
});
