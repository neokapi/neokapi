import { describe, it, expect, vi } from "vite-plus/test";
import type { ReactNode } from "react";
import { render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ApiProvider } from "../context/ApiContext";
import { WorkspaceProvider } from "../context/WorkspaceContext";
import type { ApiAdapter } from "../api/adapter";
import type { Workspace } from "../types/api";
import {
  relationLabel,
  changeSetStatusLabel,
  formatDate,
  formatRelative,
  TermStatusBadge,
  RelationBadge,
} from "../context-hub/shell/atoms";
import { ContextHub } from "../context-hub/shell/ContextHub";
import { ExperimentsView } from "../context-hub/experiments/ExperimentsView";
import { RollupMatrix } from "../context-hub/dashboard/RollupMatrix";
import { sampleChangesets } from "../stories/voiceHubFixtures";

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
    listChangesets: vi.fn().mockResolvedValue([]),
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

describe("context-hub atoms", () => {
  it("labels relations in plain words", () => {
    expect(relationLabel("REPLACED_BY")).toBe("replaced by");
    expect(relationLabel("COMPETITOR")).toBe("competitor");
    expect(relationLabel("HAS_PART")).toBe("has part");
  });

  it("labels change-set statuses", () => {
    expect(changeSetStatusLabel("in_review")).toBe("In review");
    expect(changeSetStatusLabel("merged")).toBe("Merged");
  });

  it("formats dates and falls back on empty/invalid input", () => {
    expect(formatDate(undefined)).toBe("·");
    expect(formatDate("not-a-date")).toBe("·");
    expect(formatDate("2026-06-13T10:00:00Z")).toContain("2026");
    expect(formatRelative(undefined)).toBe("·");
  });

  it("renders a term-status badge", () => {
    render(<TermStatusBadge status="forbidden" />);
    expect(screen.getByText("forbidden")).toBeInTheDocument();
  });

  it("renders a relation badge", () => {
    render(<RelationBadge type="REPLACED_BY" />);
    expect(screen.getByText("replaced by")).toBeInTheDocument();
  });
});

describe("ContextHub shell", () => {
  it("renders title, description, and actions", () => {
    render(
      <ContextHub title="Concepts" description="Brand language" actions={<button>New</button>}>
        <div>body</div>
      </ContextHub>,
    );
    expect(screen.getByRole("heading", { name: "Concepts" })).toBeInTheDocument();
    expect(screen.getByText("Brand language")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "New" })).toBeInTheDocument();
    expect(screen.getByText("body")).toBeInTheDocument();
  });
});

describe("ExperimentsView", () => {
  it("groups change-sets under their status", async () => {
    const adapter = mockAdapter({
      listChangesets: vi.fn().mockResolvedValue(sampleChangesets),
    });
    renderWithProviders(<ExperimentsView onOpenExperiment={vi.fn()} />, adapter);

    await waitFor(() => {
      expect(screen.getByText("Prefer ‘Paiement’ for fr-FR")).toBeInTheDocument();
    });
    // Status group badges from the fixtures (draft, in_review, merged).
    expect(screen.getByText("In review")).toBeInTheDocument();
    expect(screen.getByText("Merged")).toBeInTheDocument();
  });
});

describe("RollupMatrix judges each project against its own profile bar", () => {
  const rollup = {
    projects: [
      {
        project_id: "strict",
        project_name: "Strict",
        profile_id: "p-strict",
        profile_name: "Strict voice",
        overall: 85,
        trend: "flat" as const,
        scored_blocks: 10,
        last_scored_at: "2026-01-01T00:00:00Z",
      },
      {
        project_id: "default",
        project_name: "Default",
        profile_id: "p-default",
        profile_name: "Default voice",
        overall: 85,
        trend: "flat" as const,
        scored_blocks: 10,
        last_scored_at: "2026-01-01T00:00:00Z",
      },
    ],
    total: 2,
    limit: 50,
    offset: 0,
  };

  it("shows an 85 below the bar under min_score 90 and above it under the default", async () => {
    const adapter = mockAdapter({
      getVoiceRollup: vi.fn().mockResolvedValue(rollup),
      listVoiceProfiles: vi
        .fn()
        .mockResolvedValue([{ id: "p-strict", min_score: 90 }, { id: "p-default" }]),
    });
    renderWithProviders(<RollupMatrix />, adapter);

    await waitFor(() => expect(screen.getAllByTestId("rollup-score")).toHaveLength(2));
    const [strict, dflt] = screen.getAllByTestId("rollup-score");
    expect(strict.dataset.belowBar).toBe("true");
    expect(strict.className).toContain("text-warning");
    expect(dflt.dataset.belowBar).toBe("false");
    expect(dflt.className).toContain("text-success");
  });
});
