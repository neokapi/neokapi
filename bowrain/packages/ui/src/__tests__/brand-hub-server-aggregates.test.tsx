// The brand hub asks the server the questions it used to answer itself. These
// tests pin the direction of that: each surface reads the aggregate endpoint,
// and the fan-out it replaced is asserted absent — a panel that quietly folds a
// page of concepts again would still render, and only these calls notice.
import { describe, it, expect, vi } from "vite-plus/test";
import type { ReactNode } from "react";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ApiProvider } from "../context/ApiContext";
import { WorkspaceProvider } from "../context/WorkspaceContext";
import type { ApiAdapter } from "../api/adapter";
import type { Workspace } from "../types/api";
import type { ChangeSet } from "../types/brand-graph";
import { VocabularyByStatus, LocaleCoveragePanel } from "../brand-hub/dashboard/CoveragePanel";
import { PendingDecisions } from "../brand-hub/dashboard/PendingDecisions";
import { BrandDashboardView } from "../brand-hub/dashboard/BrandDashboardView";
import { PendingConceptChanges } from "../brand-hub/concepts/PendingConceptChanges";
import { ActivityView } from "../brand-hub/activity/ActivityView";
import { OpBuilder } from "../brand-hub/experiments/OpBuilder";
import { sampleActivities, sampleConcepts } from "../stories/brandHubFixtures";

const workspace: Workspace = {
  id: "ws-1",
  name: "Demo",
  slug: "demo",
  description: "",
  logo_url: "",
  type: "personal",
  role: "owner",
};

function mockAdapter(overrides: Partial<ApiAdapter> = {}): ApiAdapter {
  return {
    listConcepts: vi.fn().mockResolvedValue({ concepts: [], total_count: 0 }),
    listMarkets: vi.fn().mockResolvedValue([]),
    listMembers: vi.fn().mockResolvedValue([]),
    listChangesets: vi.fn().mockResolvedValue([]),
    listBrandProfiles: vi.fn().mockResolvedValue([]),
    getBrandRollup: vi.fn().mockResolvedValue({ projects: [], total: 0, limit: 50, offset: 0 }),
    getConceptStatusCounts: vi.fn().mockResolvedValue({ total: 0, by_status: {} }),
    getConceptLocaleCoverage: vi.fn().mockResolvedValue({ total: 0, locales: [] }),
    getChangesetCounts: vi.fn().mockResolvedValue({ total: 0, by_status: {} }),
    getChangesetBlastRadius: vi.fn(),
    getChangeset: vi.fn(),
    getConceptStory: vi.fn(),
    listActivities: vi.fn().mockResolvedValue({ activities: [], next_cursor: "" }),
    listProjects: vi.fn().mockResolvedValue([]),
    ...overrides,
  } as unknown as ApiAdapter;
}

function renderWithProviders(ui: ReactNode, adapter: ApiAdapter) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <ApiProvider adapter={adapter}>
        <WorkspaceProvider initialWorkspace={workspace}>{ui}</WorkspaceProvider>
      </ApiProvider>
    </QueryClientProvider>,
  );
}

function changeset(partial: Partial<ChangeSet>): ChangeSet {
  return {
    id: "cs-1",
    workspace_id: "ws-1",
    name: "Prefer ‘Paiement’",
    status: "in_review",
    created_by: "sam",
    created_at: "2026-06-01T00:00:00Z",
    updated_at: "2026-06-13T00:00:00Z",
    submitted_at: "2026-06-13T00:00:00Z",
    ...partial,
  };
}

describe("VocabularyByStatus", () => {
  it("reads one status-counts endpoint instead of a list call per status", async () => {
    const adapter = mockAdapter({
      getConceptStatusCounts: vi.fn().mockResolvedValue({
        total: 40,
        by_status: { preferred: 31, approved: 12, admitted: 4, proposed: 7, forbidden: 2 },
      }),
    });
    renderWithProviders(<VocabularyByStatus />, adapter);

    await waitFor(() => expect(screen.getByText("31")).toBeInTheDocument());
    expect(screen.getByText("12")).toBeInTheDocument();
    expect(screen.getByText("2")).toBeInTheDocument();
    expect(adapter.getConceptStatusCounts).toHaveBeenCalledTimes(1);
    expect(adapter.listConcepts).not.toHaveBeenCalled();
  });

  it("shows an empty vocabulary rather than a bar chart of zeroes", async () => {
    const adapter = mockAdapter();
    renderWithProviders(<VocabularyByStatus />, adapter);
    await waitFor(() => expect(screen.getByText("No concepts yet")).toBeInTheDocument());
  });
});

describe("LocaleCoveragePanel", () => {
  it("reads coverage over the workspace, not over a page of concepts", async () => {
    const adapter = mockAdapter({
      getConceptLocaleCoverage: vi.fn().mockResolvedValue({
        total: 400,
        locales: [
          { locale: "en-US", present: 400, total: 400, pct: 100 },
          { locale: "de-DE", present: 120, total: 400, pct: 30 },
        ],
      }),
    });
    renderWithProviders(<LocaleCoveragePanel />, adapter);

    await waitFor(() => expect(screen.getByText("en-US")).toBeInTheDocument());
    // The totals are the workspace's, so they can exceed any page size.
    expect(screen.getByText("400/400 · 100%")).toBeInTheDocument();
    expect(screen.getByText("120/400 · 30%")).toBeInTheDocument();
    expect(adapter.listConcepts).not.toHaveBeenCalled();
  });
});

describe("PendingDecisions", () => {
  it("shows the blast radius stored on the change-set without walking content", async () => {
    const adapter = mockAdapter();
    const cs = changeset({
      impact_summary: {
        total_blocks: 1280,
        affected_blocks: 34,
        new_violations: 12,
        resolved: 7,
        words: 210,
        projects: 2,
        computed_at: "2026-06-13T00:00:00Z",
      },
    });
    renderWithProviders(<PendingDecisions changesets={[cs]} />, adapter);

    await waitFor(() => expect(screen.getByText("34")).toBeInTheDocument());
    expect(screen.getByText("210")).toBeInTheDocument();
    expect(screen.getByText("+12 new")).toBeInTheDocument();
    expect(screen.getByText("−7 resolved")).toBeInTheDocument();
    // The live walk is the slowest read in the platform; the inbox never runs it.
    expect(adapter.getChangesetBlastRadius).not.toHaveBeenCalled();
  });

  it("reports a partial walk as a floor rather than a total", async () => {
    const adapter = mockAdapter();
    const cs = changeset({
      impact_summary: {
        total_blocks: 1280,
        affected_blocks: 34,
        new_violations: 0,
        resolved: 0,
        words: 210,
        projects: 2,
        partial: true,
        partial_reason: "budget exhausted",
        computed_at: "2026-06-13T00:00:00Z",
      },
    });
    renderWithProviders(<PendingDecisions changesets={[cs]} />, adapter);

    await waitFor(() => expect(screen.getByText("≥34")).toBeInTheDocument());
    expect(screen.getByText("partial")).toBeInTheDocument();
  });

  it("says a proposal has no measurement rather than claiming nothing is affected", async () => {
    const adapter = mockAdapter();
    renderWithProviders(<PendingDecisions changesets={[changeset({})]} />, adapter);

    await waitFor(() =>
      expect(
        screen.getByText("Blast radius is measured when a proposal is submitted."),
      ).toBeInTheDocument(),
    );
  });
});

describe("PendingConceptChanges", () => {
  it("counts ops from the list row instead of reading each change-set", async () => {
    const adapter = mockAdapter({
      listChangesets: vi.fn().mockResolvedValue([changeset({ ops_count: 57 })]),
    });
    renderWithProviders(<PendingConceptChanges />, adapter);

    await waitFor(() => expect(screen.getByText("57")).toBeInTheDocument());
    expect(screen.getByText(/changes ·/)).toBeInTheDocument();
    expect(adapter.getChangeset).not.toHaveBeenCalled();
  });
});

describe("BrandDashboardView", () => {
  it("takes its metric counts from the count endpoints, not from limit=1 lists", async () => {
    const adapter = mockAdapter({
      getConceptStatusCounts: vi.fn().mockResolvedValue({ total: 412, by_status: {} }),
      getChangesetCounts: vi.fn().mockResolvedValue({
        total: 20,
        by_status: { draft: 3, in_review: 5, approved: 2, merged: 9, abandoned: 1 },
      }),
      listChangesets: vi.fn().mockResolvedValue([]),
    });
    renderWithProviders(<BrandDashboardView />, adapter);

    await waitFor(() => expect(screen.getByText("412")).toBeInTheDocument());
    // Pending = in_review + approved; active = everything not terminal.
    expect(screen.getByText("7")).toBeInTheDocument();
    expect(screen.getByText("10")).toBeInTheDocument();
    expect(adapter.getChangesetCounts).toHaveBeenCalledTimes(1);
  });

  it("orders recently changed concepts server-side", async () => {
    const adapter = mockAdapter();
    renderWithProviders(<BrandDashboardView />, adapter);

    await waitFor(() => expect(adapter.listConcepts).toHaveBeenCalled());
    const calls = (adapter.listConcepts as ReturnType<typeof vi.fn>).mock.calls;
    expect(calls.some(([, params]) => params?.sort === "updated_at")).toBe(true);
    // No call asks for the whole vocabulary in order to take six of it.
    expect(calls.every(([, params]) => (params?.limit ?? 0) <= 100)).toBe(true);
  });
});

describe("ActivityView", () => {
  it("asks the server for the type families the filter selects", async () => {
    const adapter = mockAdapter({
      listActivities: vi.fn().mockResolvedValue({ activities: sampleActivities, next_cursor: "" }),
      listChangesets: vi.fn().mockResolvedValue([changeset({ id: "cs-1", name: "Prefer term" })]),
      listConcepts: vi.fn().mockResolvedValue({
        concepts: sampleConcepts,
        total_count: sampleConcepts.length,
      }),
    });
    renderWithProviders(<ActivityView />, adapter);

    await waitFor(() => expect(adapter.listActivities).toHaveBeenCalled());
    const [, query] = (adapter.listActivities as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(query.types).toEqual([
      "changeset.",
      "review.",
      "pilot.",
      "concept.",
      "observation.",
      "comment.",
    ]);
    // The 27-way fan-out is gone: no per-concept story, no per-change-set detail.
    expect(adapter.getConceptStory).not.toHaveBeenCalled();
    expect(adapter.getChangeset).not.toHaveBeenCalled();
  });

  it("names the change-set a row is about, keeping the server sentence as detail", async () => {
    const adapter = mockAdapter({
      listActivities: vi.fn().mockResolvedValue({ activities: sampleActivities, next_cursor: "" }),
      listChangesets: vi
        .fn()
        .mockResolvedValue([changeset({ id: "cs-1", name: "Prefer ‘Paiement’ for fr-FR" })]),
      listConcepts: vi.fn().mockResolvedValue({
        concepts: sampleConcepts,
        total_count: sampleConcepts.length,
      }),
    });
    renderWithProviders(<ActivityView />, adapter);

    await waitFor(() => expect(screen.getAllByText("Prefer ‘Paiement’ for fr-FR").length).toBe(2));
    expect(screen.getByText("approved a change")).toBeInTheDocument();
    // The concept rows resolve their names the same way.
    expect(screen.getByText("Checkout")).toBeInTheDocument();
  });

  it("narrows the request when a category is switched off", async () => {
    const user = userEvent.setup();
    const adapter = mockAdapter({
      listActivities: vi.fn().mockResolvedValue({ activities: sampleActivities, next_cursor: "" }),
    });
    renderWithProviders(<ActivityView />, adapter);

    await waitFor(() => expect(adapter.listActivities).toHaveBeenCalledTimes(1));
    await user.click(screen.getByRole("button", { name: /Experiments/ }));

    await waitFor(() => {
      const calls = (adapter.listActivities as ReturnType<typeof vi.fn>).mock.calls;
      expect(calls[calls.length - 1][1].types).toEqual(["concept.", "observation.", "comment."]);
    });
  });
});

describe("OpBuilder concept picker", () => {
  it("searches the vocabulary server-side rather than filtering a fetched page", async () => {
    const user = userEvent.setup();
    const adapter = mockAdapter({
      listConcepts: vi.fn().mockResolvedValue({ concepts: sampleConcepts, total_count: 340 }),
    });
    renderWithProviders(<OpBuilder changesetId="cs-1" />, adapter);

    await user.click(screen.getByRole("button", { name: /Ban a term/ }));
    await user.type(screen.getByTestId("op-term-concept"), "check");

    await waitFor(() => {
      const calls = (adapter.listConcepts as ReturnType<typeof vi.fn>).mock.calls;
      expect(calls.some(([, params]) => params?.q === "check")).toBe(true);
    });
    // Never the whole vocabulary: the search is what finds a concept past page one.
    const calls = (adapter.listConcepts as ReturnType<typeof vi.fn>).mock.calls;
    expect(calls.every(([, params]) => (params?.limit ?? 0) <= 20)).toBe(true);
  });

  it("says how much of the vocabulary the list shows", async () => {
    const user = userEvent.setup();
    const adapter = mockAdapter({
      listConcepts: vi.fn().mockResolvedValue({ concepts: sampleConcepts, total_count: 340 }),
    });
    renderWithProviders(<OpBuilder changesetId="cs-1" />, adapter);

    await user.click(screen.getByRole("button", { name: /Ban a term/ }));
    await user.click(screen.getByTestId("op-term-concept"));

    await waitFor(() => expect(screen.getByTestId("list-cap-row")).toBeInTheDocument());
    expect(screen.getByTestId("list-cap-row")).toHaveTextContent("of 340 concepts");
  });
});
