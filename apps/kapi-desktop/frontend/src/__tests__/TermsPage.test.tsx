import { describe, it, expect, vi, beforeEach } from "vite-plus/test";
import { fireEvent, render, screen, waitFor } from "./testUtils";

import { SAMPLE_CONCEPTS } from "../stories/fixtures/concepts";

const { api, call } = vi.hoisted(() => ({
  api: {
    getProjectHandles: vi.fn(),
    getProject: vi.fn(),
    getTermsStats: vi.fn(),
    listNamedTerms: vi.fn(),
    openTerms: vi.fn(),
    closeTerms: vi.fn(),
  },
  call: vi.fn(),
}));
vi.mock("../hooks/useApi", () => ({ api, call }));

import { TermsPage } from "../components/TermsPage";
import { ErrorProvider } from "../components/ErrorBanner";

beforeEach(() => {
  for (const fn of Object.values(api)) fn.mockReset();
  call.mockReset();
  api.getProjectHandles.mockResolvedValue({ termsHandle: "tb-1" });
  api.getProject.mockResolvedValue({ defaults: { source_language: "en-US" } });
  api.getTermsStats.mockResolvedValue({ count: SAMPLE_CONCEPTS.length });
  api.listNamedTerms.mockResolvedValue([
    {
      name: "my-terms",
      path: "/stores/terms/my-terms.db",
      size: 262144,
      modified: "2026-10-14T10:00:00Z",
    },
    {
      name: "voice-terms",
      path: "/stores/terms/voice-terms.db",
      size: 131072,
      modified: "2026-10-12T10:00:00Z",
    },
  ]);
  api.openTerms.mockResolvedValue("tb-2");
  call.mockImplementation((method: string) => {
    if (method === "SearchTerms")
      return Promise.resolve({ concepts: SAMPLE_CONCEPTS, total_count: SAMPLE_CONCEPTS.length });
    if (method === "GetConceptForView") return Promise.resolve(SAMPLE_CONCEPTS[0]);
    if (method === "GetRelations") return Promise.resolve([]);
    return Promise.resolve(null);
  });
});

describe("the Terms page", () => {
  it("opens the project's store as a list beside a detail pane, its count in the header", async () => {
    render(
      <ErrorProvider>
        <TermsPage tabID="t1" />
      </ErrorProvider>,
    );
    await waitFor(() => expect(screen.getByText("Project Terms")).toBeInTheDocument());
    await waitFor(() => expect(screen.getByText("4 concepts")).toBeInTheDocument());
    await waitFor(() =>
      expect(document.querySelectorAll("[data-slot='terms-concept']")).toHaveLength(4),
    );
    expect(call).toHaveBeenCalledWith("SearchTerms", "tb-1", "", "", "", 0, 100);
    // The count is the header's; the list pane does not repeat it.
    expect(screen.getAllByText("4 concepts")).toHaveLength(1);
  });

  it("lists the named stores one row each, with no card grid, and opens one", async () => {
    api.getProjectHandles.mockResolvedValue({ termsHandle: "" });
    render(
      <ErrorProvider>
        <TermsPage />
      </ErrorProvider>,
    );
    const rows = await waitFor(() => {
      const found = document.querySelectorAll("[data-slot='store-row']");
      expect(found).toHaveLength(2);
      return Array.from(found) as HTMLElement[];
    });
    expect(rows[0].textContent).toContain("my-terms");
    expect(rows[0].textContent).toContain("my-terms.db");
    expect(rows[0].textContent).not.toContain("/stores/terms/");
    expect(document.querySelector(".md\\:grid-cols-2")).toBeNull();

    fireEvent.click(rows[1]);
    await waitFor(() => expect(api.openTerms).toHaveBeenCalledWith("/stores/terms/voice-terms.db"));
    await waitFor(() => expect(screen.getByRole("heading", { name: "voice-terms" })).toBeVisible());
    await waitFor(() =>
      expect(call).toHaveBeenCalledWith("SearchTerms", "tb-2", "", "", "", 0, 100),
    );
  });
});
