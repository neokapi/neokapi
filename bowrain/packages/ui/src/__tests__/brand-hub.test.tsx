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
} from "../brand-hub/shell/atoms";
import { BrandHub } from "../brand-hub/shell/BrandHub";
import { ExperimentsView } from "../brand-hub/experiments/ExperimentsView";
import { sampleChangesets } from "../stories/brandHubFixtures";

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

describe("brand-hub atoms", () => {
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
    expect(formatDate(undefined)).toBe("—");
    expect(formatDate("not-a-date")).toBe("—");
    expect(formatDate("2026-06-13T10:00:00Z")).toContain("2026");
    expect(formatRelative(undefined)).toBe("—");
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

describe("BrandHub shell", () => {
  it("renders title, description, and actions", () => {
    render(
      <BrandHub title="Concepts" description="Brand language" actions={<button>New</button>}>
        <div>body</div>
      </BrandHub>,
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
