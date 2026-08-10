import { describe, it, expect, vi } from "vite-plus/test";
import type { ReactNode } from "react";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ApiProvider } from "../context/ApiContext";
import { WorkspaceProvider } from "../context/WorkspaceContext";
import {
  useMemoryBrowserAdapter,
  useTermsBrowserAdapter,
} from "../hooks/useResourceBrowserAdapters";
import { ApiError, governedRefusalError, GOVERNED_REFUSAL_HINT } from "../errors/ApiError";
import type { ApiAdapter } from "../api/adapter";
import type { Workspace } from "../types/api";

/**
 * A multi-select delete is one request, not one per row. Content-memory
 * entries come back with a result per id and no transaction behind them, so a
 * partly-failed batch has to be reported as partly failed; deleting concepts
 * is refused outright, and the refusal's own words are what the browser shows.
 */

const workspace: Workspace = {
  id: "ws-1",
  name: "Demo",
  slug: "demo",
  description: "",
  logo_url: "",
  type: "personal",
  role: "owner",
};

function wrapper(adapter: ApiAdapter) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>
      <ApiProvider adapter={adapter}>
        <WorkspaceProvider initialWorkspace={workspace}>{children}</WorkspaceProvider>
      </ApiProvider>
    </QueryClientProvider>
  );
}

function mockAdapter(overrides: Partial<ApiAdapter> = {}): ApiAdapter {
  return overrides as unknown as ApiAdapter;
}

describe("content-memory bulk delete", () => {
  it("deletes a selection in one request", async () => {
    const bulkDeleteMemoryEntries = vi.fn().mockResolvedValue({
      results: [
        { id: "e1", deleted: true },
        { id: "e2", deleted: true },
      ],
      deleted: 2,
      failed: 0,
    });
    const deleteMemoryEntry = vi.fn();
    const adapter = mockAdapter({ bulkDeleteMemoryEntries, deleteMemoryEntry });

    const { result } = renderHook(() => useMemoryBrowserAdapter("en", ["fr"]), {
      wrapper: wrapper(adapter),
    });
    await waitFor(() => expect(result.current).not.toBeNull());

    await result.current!.deleteEntries(["e1", "e2"]);

    expect(bulkDeleteMemoryEntries).toHaveBeenCalledTimes(1);
    expect(bulkDeleteMemoryEntries).toHaveBeenCalledWith("demo", ["e1", "e2"]);
    expect(deleteMemoryEntry).not.toHaveBeenCalled();
  });

  it("reports the ids that did not delete", async () => {
    const adapter = mockAdapter({
      bulkDeleteMemoryEntries: vi.fn().mockResolvedValue({
        results: [
          { id: "e1", deleted: true },
          { id: "e2", deleted: false, error: "entry not found" },
        ],
        deleted: 1,
        failed: 1,
      }),
    });

    const { result } = renderHook(() => useMemoryBrowserAdapter("en", ["fr"]), {
      wrapper: wrapper(adapter),
    });
    await waitFor(() => expect(result.current).not.toBeNull());

    await expect(result.current!.deleteEntries(["e1", "e2"])).rejects.toThrow(
      "1 of 2 entries could not be deleted: entry not found",
    );
  });

  it("splits a selection past the 500-id cap", async () => {
    const bulkDeleteMemoryEntries = vi
      .fn()
      .mockImplementation(async (_ws: string, ids: string[]) => ({
        results: ids.map((id) => ({ id, deleted: true })),
        deleted: ids.length,
        failed: 0,
      }));
    const adapter = mockAdapter({ bulkDeleteMemoryEntries });

    const { result } = renderHook(() => useMemoryBrowserAdapter("en", ["fr"]), {
      wrapper: wrapper(adapter),
    });
    await waitFor(() => expect(result.current).not.toBeNull());

    await result.current!.deleteEntries(Array.from({ length: 750 }, (_, i) => `e${i}`));

    expect(bulkDeleteMemoryEntries).toHaveBeenCalledTimes(2);
    expect(bulkDeleteMemoryEntries.mock.calls[0][1]).toHaveLength(500);
    expect(bulkDeleteMemoryEntries.mock.calls[1][1]).toHaveLength(250);
  });
});

describe("concept bulk delete", () => {
  it("surfaces the governed refusal in the server's own words", async () => {
    const bulkDeleteConcepts = vi.fn().mockRejectedValue(
      new ApiError({
        message: "governed_change_required",
        status: 409,
        code: "governed_change_required",
        details: {
          detail: "Deleting concepts is a governed change.",
          hint: "Open a change-set under /context/changes.",
        },
        bodyText: "",
      }),
    );
    const deleteConcept = vi.fn();
    const adapter = mockAdapter({ bulkDeleteConcepts, deleteConcept });

    const { result } = renderHook(() => useTermsBrowserAdapter(), {
      wrapper: wrapper(adapter),
    });
    await waitFor(() => expect(result.current).not.toBeNull());

    await expect(result.current!.deleteConcepts(["c1", "c2"])).rejects.toThrow(
      "Deleting concepts is a governed change. Open a change-set under /context/changes.",
    );
    expect(bulkDeleteConcepts).toHaveBeenCalledTimes(1);
    expect(bulkDeleteConcepts).toHaveBeenCalledWith("demo", ["c1", "c2"]);
    expect(deleteConcept).not.toHaveBeenCalled();
  });

  // The desktop refuses locally — there is no server call to make — and used to
  // throw a bare Error, so `governedRefusal` returned undefined and the browser
  // rethrew the machine phrase instead of naming what was refused and what to
  // do instead. `governedRefusalError` builds the envelope the server sends.
  it("reads a locally-raised refusal exactly as it reads the server's", async () => {
    const adapter = mockAdapter({
      bulkDeleteConcepts: vi.fn().mockRejectedValue(governedRefusalError("deleting concepts")),
    });

    const { result } = renderHook(() => useTermsBrowserAdapter(), { wrapper: wrapper(adapter) });
    await waitFor(() => expect(result.current).not.toBeNull());

    await expect(result.current!.deleteConcepts(["c1"])).rejects.toThrow(
      `deleting concepts ${GOVERNED_REFUSAL_HINT}`,
    );
  });

  it("rethrows anything that is not a governed refusal", async () => {
    const adapter = mockAdapter({
      bulkDeleteConcepts: vi.fn().mockRejectedValue(new Error("network down")),
    });

    const { result } = renderHook(() => useTermsBrowserAdapter(), {
      wrapper: wrapper(adapter),
    });
    await waitFor(() => expect(result.current).not.toBeNull());

    await expect(result.current!.deleteConcepts(["c1"])).rejects.toThrow("network down");
  });
});
