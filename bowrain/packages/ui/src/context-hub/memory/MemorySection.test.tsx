import { describe, it, expect, vi } from "vite-plus/test";
import type { ReactNode } from "react";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ApiProvider } from "../../context/ApiContext";
import { WorkspaceProvider } from "../../context/WorkspaceContext";
import type { ApiAdapter } from "../../api/adapter";
import type { MemoryEntryInfo, ProjectInfo, Workspace } from "../../types/api";
import { MemorySection } from "./MemorySection";

const workspace: Workspace = {
  id: "ws-1",
  name: "Demo",
  slug: "demo",
  description: "",
  logo_url: "",
  type: "personal",
  role: "owner",
};

const projects: ProjectInfo[] = [
  {
    id: "p-web",
    name: "Marketing Website",
    default_source_language: "en-US",
    target_languages: ["de-DE", "fr-FR"],
  },
  {
    id: "p-app",
    name: "Mobile App",
    default_source_language: "en-US",
    target_languages: ["de-DE"],
  },
];

const entries: MemoryEntryInfo[] = [
  {
    id: "e-1",
    source: "Proceed to checkout",
    target: "Weiter zur Kasse",
    source_language: "en-US",
    target_language: "de-DE",
    project_id: "p-web",
    updated_at: "2026-06-13T10:00:00Z",
  },
  {
    id: "e-2",
    source: "Your basket is empty",
    target: "Ihr Warenkorb ist leer",
    source_language: "en-US",
    target_language: "de-DE",
    updated_at: "2026-06-12T10:00:00Z",
  },
];

function mockAdapter(overrides: Partial<ApiAdapter> = {}): ApiAdapter {
  return {
    listProjects: vi.fn().mockResolvedValue(projects),
    getMemoryCount: vi.fn().mockResolvedValue(entries.length),
    getMemoryEntries: vi.fn().mockImplementation(async (_ws: string, q: string) => {
      const list = q ? entries.filter((e) => e.source.includes(q)) : entries;
      return { entries: list, total_count: list.length };
    }),
    addMemoryEntry: vi.fn().mockResolvedValue(entries[0]),
    updateMemoryEntry: vi.fn().mockResolvedValue(undefined),
    deleteMemoryEntry: vi.fn().mockResolvedValue(undefined),
    bulkDeleteMemoryEntries: vi.fn().mockImplementation(async (_ws: string, ids: string[]) => ({
      results: ids.map((id) => ({ id, deleted: true })),
      deleted: ids.length,
      failed: 0,
    })),
    ...overrides,
  } as unknown as ApiAdapter;
}

function renderSection(ui: ReactNode, adapter: ApiAdapter) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <ApiProvider adapter={adapter}>
        <WorkspaceProvider initialWorkspace={workspace}>{ui}</WorkspaceProvider>
      </ApiProvider>
    </QueryClientProvider>,
  );
}

describe("MemorySection", () => {
  it("reads pairs into the language most of the workspace translates into", async () => {
    const adapter = mockAdapter();
    renderSection(<MemorySection />, adapter);

    expect(await screen.findByTestId("tm-entry-e-1-de-DE")).toHaveTextContent(
      "Proceed to checkout",
    );
    expect(screen.getByTestId("tm-entry-e-2-de-DE")).toBeInTheDocument();
    await waitFor(() => {
      const calls = (adapter.getMemoryEntries as ReturnType<typeof vi.fn>).mock.calls;
      expect(calls[calls.length - 1].slice(2, 4)).toEqual(["en-US", "de-DE"]);
    });
    expect(screen.getByText("2 entries")).toBeInTheDocument();
    // Two target languages: the picker is offered.
    expect(screen.getByRole("combobox", { name: "Target language" })).toBeInTheDocument();
  });

  it("asks the server for a search, a page at a time", async () => {
    const user = userEvent.setup();
    const adapter = mockAdapter();
    renderSection(<MemorySection />, adapter);

    await screen.findByTestId("tm-entry-e-1-de-DE");
    await user.type(screen.getByTestId("tm-search"), "basket");
    await waitFor(() => expect(screen.queryByTestId("tm-entry-e-1-de-DE")).not.toBeInTheDocument());
    const calls = (adapter.getMemoryEntries as ReturnType<typeof vi.fn>).mock.calls;
    expect(calls.some((c) => c[1] === "basket")).toBe(true);
    expect(calls.every((c) => c[5] <= 100)).toBe(true);
    expect(screen.getByText("1 entry matches")).toBeInTheDocument();
  });

  it("shows the selected pair in full with where it came from", async () => {
    const user = userEvent.setup();
    renderSection(<MemorySection />, mockAdapter());

    await user.click(await screen.findByTestId("tm-entry-e-1-de-DE"));
    const detail = document.querySelector("[data-slot='memory-pair-detail']") as HTMLElement;
    expect(detail).toHaveAttribute("data-entry", "e-1");
    expect(within(detail).getByText("Proceed to checkout")).toBeInTheDocument();
    expect(within(detail).getByText("Weiter zur Kasse")).toBeInTheDocument();
    expect(within(detail).getByText("Marketing Website")).toBeInTheDocument();
  });

  it("holds an entry with no project for the whole workspace", async () => {
    const user = userEvent.setup();
    renderSection(<MemorySection />, mockAdapter());

    await user.click(await screen.findByTestId("tm-entry-e-2-de-DE"));
    expect(screen.getByText("Every project in the workspace")).toBeInTheDocument();
  });

  it("saves both sides of an edited pair", async () => {
    const user = userEvent.setup();
    const adapter = mockAdapter();
    renderSection(<MemorySection />, adapter);

    await user.click(await screen.findByTestId("tm-entry-e-1-de-DE"));
    await user.click(screen.getByRole("button", { name: /Edit/ }));
    const target = screen.getByRole("textbox", { name: "Target" });
    await user.clear(target);
    await user.type(target, "Zur Kasse");
    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(adapter.updateMemoryEntry).toHaveBeenCalledTimes(1));
    expect(adapter.updateMemoryEntry).toHaveBeenCalledWith("demo", {
      project_id: "p-web",
      entry_id: "e-1",
      source: "Proceed to checkout",
      target: "Zur Kasse",
      source_locale: "en-US",
      target_locale: "de-DE",
    });
  });

  it("deletes a pair after the second click", async () => {
    const user = userEvent.setup();
    const adapter = mockAdapter();
    renderSection(<MemorySection />, adapter);

    await user.click(await screen.findByTestId("tm-entry-e-2-de-DE"));
    await user.click(screen.getByRole("button", { name: /Delete/ }));
    expect(adapter.deleteMemoryEntry).not.toHaveBeenCalled();
    await user.click(screen.getByRole("button", { name: "Confirm" }));
    await waitFor(() => expect(adapter.deleteMemoryEntry).toHaveBeenCalledWith("demo", "e-2"));
  });

  it("deletes every marked entry in one request, and says how many failed", async () => {
    const user = userEvent.setup();
    const adapter = mockAdapter({
      bulkDeleteMemoryEntries: vi.fn().mockResolvedValue({
        results: [
          { id: "e-1", deleted: true },
          { id: "e-2", deleted: false, error: "not found" },
        ],
        deleted: 1,
        failed: 1,
      }),
    });
    renderSection(<MemorySection />, adapter);

    await screen.findByTestId("tm-entry-e-1-de-DE");
    await user.click(screen.getByRole("button", { name: "Mark all shown" }));
    const bulk = document.querySelector("[data-slot='memory-bulk-detail']") as HTMLElement;
    expect(within(bulk).getByText("2 entries marked")).toBeInTheDocument();

    await user.click(within(bulk).getByRole("button", { name: /Delete/ }));
    await user.click(within(bulk).getByRole("button", { name: "Confirm" }));
    await waitFor(() =>
      expect(adapter.bulkDeleteMemoryEntries).toHaveBeenCalledWith("demo", ["e-1", "e-2"]),
    );
    expect(
      await screen.findByText(/1 of 2 entries could not be deleted: not found/),
    ).toBeInTheDocument();
  });

  it("adds a pair by hand", async () => {
    const user = userEvent.setup();
    const adapter = mockAdapter();
    renderSection(<MemorySection />, adapter);

    await screen.findByTestId("tm-entry-e-1-de-DE");
    await user.click(screen.getByRole("button", { name: /Add entry/ }));
    const dialog = await screen.findByRole("dialog");
    await user.type(within(dialog).getByLabelText("Source"), "Sign in");
    await user.type(within(dialog).getByLabelText("Target"), "Anmelden");
    await user.click(within(dialog).getByRole("button", { name: "Add entry" }));

    await waitFor(() =>
      expect(adapter.addMemoryEntry).toHaveBeenCalledWith(
        "demo",
        "Sign in",
        "Anmelden",
        "en-US",
        "de-DE",
      ),
    );
  });

  it("says when nothing is held into the language yet", async () => {
    renderSection(
      <MemorySection />,
      mockAdapter({ getMemoryEntries: vi.fn().mockResolvedValue({ entries: [], total_count: 0 }) }),
    );
    expect(await screen.findByText("No approved wording into German yet")).toBeInTheDocument();
  });
});
