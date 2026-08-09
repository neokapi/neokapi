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
import type { Workspace } from "../types/api";

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
  { mounts = 1 }: { mounts?: number } = {},
) {
  const adapter = createMockAdapter();
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
