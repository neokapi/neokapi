import { describe, it, expect, vi, beforeEach } from "vitest";
import { fireEvent, render, screen, waitFor } from "./testUtils";

import { SAMPLE_ENTRIES } from "../stories/fixtures/memory";

const { api, call } = vi.hoisted(() => ({
  api: {
    getProjectHandles: vi.fn(),
    getProject: vi.fn(),
    getMemoryStats: vi.fn(),
    getMemoryFacets: vi.fn(),
    searchMemoryEntries: vi.fn(),
    listNamedMemories: vi.fn(),
    openMemory: vi.fn(),
    closeMemory: vi.fn(),
    getKnownLocales: vi.fn(),
    listFilters: vi.fn(),
  },
  call: vi.fn(),
}));
vi.mock("../hooks/useApi", () => ({ api, call }));

import { MemoriesPage } from "../components/MemoriesPage";
import { ErrorProvider } from "../components/ErrorBanner";

beforeEach(() => {
  for (const fn of Object.values(api)) fn.mockReset();
  call.mockReset();
  api.getProjectHandles.mockResolvedValue({ memoryHandle: "mem-1" });
  api.getProject.mockResolvedValue({ defaults: { source_language: "en-US" } });
  api.getMemoryStats.mockResolvedValue({ count: SAMPLE_ENTRIES.length });
  api.getMemoryFacets.mockResolvedValue(null);
  api.searchMemoryEntries.mockResolvedValue({
    entries: SAMPLE_ENTRIES,
    total_count: SAMPLE_ENTRIES.length,
  });
  api.listNamedMemories.mockResolvedValue([
    {
      name: "my-project",
      path: "/stores/memory/my-project.db",
      size: 524288,
      modified: "2026-10-14T10:00:00Z",
    },
  ]);
  api.getKnownLocales.mockResolvedValue([]);
  api.listFilters.mockResolvedValue([]);
  call.mockResolvedValue(null);
});

function renderPage(ui: React.ReactElement) {
  return render(<ErrorProvider>{ui}</ErrorProvider>);
}

describe("the Content memory page", () => {
  it("opens the project's store as pairs beside a detail pane, its count in the header", async () => {
    renderPage(<MemoriesPage tabID="t1" />);
    await waitFor(() => expect(screen.getByText("Project Content Memory")).toBeInTheDocument());
    await waitFor(() => expect(screen.getByText("3 entries")).toBeInTheDocument());
    await waitFor(() =>
      expect(document.querySelectorAll("[data-slot='memory-pair']")).toHaveLength(4),
    );
    expect(api.searchMemoryEntries).toHaveBeenCalledWith("mem-1", "", "", "", 0, 100);
    expect(screen.getByTestId("tm-browser")).toBeInTheDocument();
  });

  it("adds an entry from the page header", async () => {
    renderPage(<MemoriesPage tabID="t1" />);
    const add = await waitFor(() => {
      const el = document.querySelector("[data-slot='memory-add']");
      expect(el).toBeTruthy();
      return el as HTMLElement;
    });
    fireEvent.click(add);
    await waitFor(() => expect(screen.getByRole("dialog", { name: "Add an entry" })).toBeVisible());
  });

  it("lists the named stores one row each, and opens one", async () => {
    api.getProjectHandles.mockResolvedValue({ memoryHandle: "" });
    api.openMemory.mockResolvedValue("mem-2");
    renderPage(<MemoriesPage />);
    const row = await waitFor(() => {
      const el = document.querySelector("[data-slot='store-row']");
      expect(el).toBeTruthy();
      return el as HTMLElement;
    });
    expect(row.textContent).toContain("my-project.db");
    expect(document.querySelector(".md\\:grid-cols-2")).toBeNull();
    fireEvent.click(row);
    await waitFor(() =>
      expect(api.openMemory).toHaveBeenCalledWith("/stores/memory/my-project.db"),
    );
    await waitFor(() =>
      expect(api.searchMemoryEntries).toHaveBeenCalledWith("mem-2", "", "", "", 0, 100),
    );
  });
});
