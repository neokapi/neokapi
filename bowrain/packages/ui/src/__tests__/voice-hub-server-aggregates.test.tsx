// Verify that voice-hub surfaces use server aggregate endpoints and do not
// recompute totals from paginated concept lists.
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
import { VocabularyByStatus, LocaleCoveragePanel } from "../context-hub/dashboard/CoveragePanel";
import { WhatsNew } from "../context-hub/dashboard/WhatsNew";
import { whatsNewRows } from "../context-hub/dashboard/newsRows";
import { VoiceDashboardView } from "../context-hub/dashboard/VoiceDashboardView";
import { PendingConceptChanges } from "../context-hub/concepts/PendingConceptChanges";
import { OpBuilder } from "../context-hub/experiments/OpBuilder";
import { sampleConcepts } from "../stories/voiceHubFixtures";

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
    listVoiceProfiles: vi.fn().mockResolvedValue([]),
    getVoiceRollup: vi.fn().mockResolvedValue({ projects: [], total: 0, limit: 50, offset: 0 }),
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

describe("WhatsNew", () => {
  const waiting = (cs: ChangeSet) => whatsNewRows([cs], []);

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
    renderWithProviders(<WhatsNew rows={waiting(cs)} keyboard={false} />, adapter);

    await waitFor(() => expect(screen.getByText("34 blocks")).toBeInTheDocument());
    expect(screen.getByText("+12 new")).toBeInTheDocument();
    expect(screen.getByText("−7 resolved")).toBeInTheDocument();
    // The live walk is the slowest read in the platform; the dashboard never runs it.
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
    renderWithProviders(<WhatsNew rows={waiting(cs)} keyboard={false} />, adapter);

    await waitFor(() => expect(screen.getByText("≥34 blocks")).toBeInTheDocument());
  });

  it("says a proposal has no measurement rather than claiming nothing is affected", async () => {
    const adapter = mockAdapter();
    renderWithProviders(<WhatsNew rows={waiting(changeset({}))} keyboard={false} />, adapter);
    await waitFor(() => expect(screen.getByText("not measured")).toBeInTheDocument());
  });

  it("leads a change-set's row with who proposed it, and opens it from one button", async () => {
    const adapter = mockAdapter({
      listMembers: vi
        .fn()
        .mockResolvedValue([
          { user_id: "sam", user: { name: "Sam Okafor", email: "sam@example.com" } },
        ]),
    });
    const onOpenChangeset = vi.fn();
    renderWithProviders(
      <WhatsNew
        rows={waiting(changeset({}))}
        nameOf={(id) => (id === "sam" ? "Sam Okafor" : (id ?? ""))}
        onOpenChangeset={onOpenChangeset}
        keyboard={false}
      />,
      adapter,
    );
    expect(screen.getByRole("img", { name: "Sam Okafor" })).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Review" }));
    expect(onOpenChangeset).toHaveBeenCalledWith("cs-1");
  });

  it("opens a voice profile's review from its row, as Decide while a disagreement waits", async () => {
    const adapter = mockAdapter();
    const onOpenReview = vi.fn();
    const rows = whatsNewRows(
      [],
      [
        {
          id: "vp-1",
          name: "Core voice",
          candidates: [
            {
              term: "log in",
              replacement: "sign in",
              correction_count: 4,
              dimension: "vocabulary",
              status: "pending",
            },
            {
              term: "log in",
              replacement: "log on",
              correction_count: 3,
              dimension: "vocabulary",
              status: "pending",
            },
          ],
        },
      ],
    );
    renderWithProviders(
      <WhatsNew rows={rows} onOpenReview={onOpenReview} keyboard={false} />,
      adapter,
    );
    expect(screen.getByText("1 needs you")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Decide" }));
    expect(onOpenReview).toHaveBeenCalledWith("vp-1");
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

describe("VoiceDashboardView", () => {
  it("counts each suggestion once over every profile, and a disagreement as needing you", async () => {
    const shared = [
      {
        term: "utilize",
        replacement: "use",
        correction_count: 5,
        dimension: "vocabulary",
        status: "pending",
      },
      {
        term: "log in",
        replacement: "sign in",
        correction_count: 4,
        dimension: "vocabulary",
        status: "pending",
      },
      {
        term: "log in",
        replacement: "log on",
        correction_count: 3,
        dimension: "vocabulary",
        status: "pending",
      },
    ];
    const adapter = mockAdapter({
      listVoiceProfiles: vi.fn().mockResolvedValue([
        { id: "vp-1", name: "Core voice" },
        { id: "vp-2", name: "Support voice" },
      ]),
      listVoiceCandidates: vi.fn().mockResolvedValue(shared),
      getChangesetCounts: vi.fn().mockResolvedValue({ total: 2, by_status: { in_review: 2 } }),
    });
    renderWithProviders(<VoiceDashboardView keyboard={false} />, adapter);

    await waitFor(() => expect(screen.getByText("3 suggestions")).toBeInTheDocument());
    expect(screen.getByText("3 need you")).toBeInTheDocument();
    expect(adapter.listVoiceCandidates).toHaveBeenCalledTimes(2);
    expect(screen.getAllByText("Core voice").length).toBeGreaterThan(0);
  });

  it("takes its metric counts from the count endpoints, not from limit=1 lists", async () => {
    const adapter = mockAdapter({
      getConceptStatusCounts: vi.fn().mockResolvedValue({ total: 412, by_status: {} }),
      getChangesetCounts: vi.fn().mockResolvedValue({
        total: 20,
        by_status: { draft: 3, in_review: 5, approved: 2, merged: 9, abandoned: 1 },
      }),
      listChangesets: vi.fn().mockResolvedValue([]),
    });
    renderWithProviders(<VoiceDashboardView />, adapter);

    await waitFor(() => expect(screen.getByText("412")).toBeInTheDocument());
    // Pending = in_review + approved; active = everything not terminal.
    expect(screen.getByText("7")).toBeInTheDocument();
    expect(screen.getByText("10")).toBeInTheDocument();
    expect(adapter.getChangesetCounts).toHaveBeenCalledTimes(1);
  });

  it("orders recently changed concepts server-side", async () => {
    const adapter = mockAdapter();
    renderWithProviders(<VoiceDashboardView />, adapter);

    await waitFor(() => expect(adapter.listConcepts).toHaveBeenCalled());
    const calls = (adapter.listConcepts as ReturnType<typeof vi.fn>).mock.calls;
    expect(calls.some(([, params]) => params?.sort === "updated_at")).toBe(true);
    // No call asks for the whole vocabulary in order to take six of it.
    expect(calls.every(([, params]) => (params?.limit ?? 0) <= 100)).toBe(true);
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
