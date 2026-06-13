import { describe, it, expect, vi } from "vite-plus/test";
import type { ReactNode } from "react";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ApiProvider } from "../context/ApiContext";
import { WorkspaceProvider } from "../context/WorkspaceContext";
import type { ApiAdapter } from "../api/adapter";
import type { Workspace } from "../types/api";
import type { GraphParams, GraphViz } from "../types/brand-graph";
import {
  relationLabel,
  changeSetStatusLabel,
  formatDate,
  formatRelative,
  TermStatusBadge,
  RelationBadge,
} from "../brand-hub/shell/atoms";
import { BrandHub } from "../brand-hub/shell/BrandHub";
import { GraphPanel } from "../brand-hub/concepts/GraphPanel";
import { ConceptGraphView } from "../brand-hub/concepts/ConceptGraphView";
import { ConceptsView } from "../brand-hub/concepts/ConceptsView";
import { ExperimentsView } from "../brand-hub/experiments/ExperimentsView";
import { sampleConcepts, sampleGraph, sampleChangesets } from "../stories/brandHubFixtures";

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
    getGraph: vi.fn().mockResolvedValue({ nodes: [], edges: [], total: 0, truncated: false }),
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

describe("GraphPanel", () => {
  it("renders a node label for every graph node", () => {
    render(<GraphPanel graph={sampleGraph} />);
    for (const node of sampleGraph.nodes) {
      expect(screen.getByText(node.label)).toBeInTheDocument();
    }
  });

  it("renders an empty SVG without crashing", () => {
    const { container } = render(
      <GraphPanel graph={{ nodes: [], edges: [], total: 0, truncated: false }} />,
    );
    expect(container.querySelector("svg")).toBeInTheDocument();
  });
});

describe("ConceptGraphView scale guard", () => {
  // A capped, truncated payload: the wide-open view must refuse to draw it.
  const truncatedGraph: GraphViz = {
    nodes: [
      { id: "c-a", label: "Alpha", domain: "commerce", status: "preferred", term_count: 2 },
      { id: "c-b", label: "Bravo", domain: "commerce", status: "forbidden", term_count: 1 },
      { id: "c-c", label: "Charlie", domain: "marketing", status: "approved", term_count: 1 },
    ],
    edges: [],
    total: 300,
    truncated: true,
  };
  // What the server returns once the query is scoped to a concept's neighbourhood:
  // a small, untruncated slice that renders normally.
  const focusedGraph: GraphViz = {
    nodes: [
      { id: "c-a", label: "Alpha", domain: "commerce", status: "preferred", term_count: 2 },
      { id: "c-b", label: "Bravo", domain: "commerce", status: "forbidden", term_count: 1 },
    ],
    edges: [{ id: "e1", source: "c-a", target: "c-b", type: "RELATED" }],
    total: 2,
    truncated: false,
  };

  function guardAdapter(): ApiAdapter {
    return mockAdapter({
      // The server scopes only when params.focus is set; selecting a concept in
      // the toolbar does not change params, so it keeps returning the hairball.
      getGraph: vi.fn(async (_ws: string, params?: GraphParams) =>
        params?.focus ? focusedGraph : truncatedGraph,
      ),
      getConcept: vi.fn().mockResolvedValue({
        id: "c-a",
        domain: "commerce",
        definition: "Alpha definition",
        terms: [],
        created_at: "",
        updated_at: "",
      }),
    });
  }

  const GUARD_HEADING = "Too many concepts to graph at once";

  it("guards the wide-open view when the server truncated the payload", async () => {
    renderWithProviders(<ConceptGraphView onOpenConcept={vi.fn()} />, guardAdapter());
    await waitFor(() => {
      expect(screen.getByText(GUARD_HEADING)).toBeInTheDocument();
    });
  });

  // Regression: selecting a concept in the toolbar only opens the side panel — it
  // does not scope the server query — so the guard must stay up. (The bug gated
  // on selectedId, which let a plain search lift the guard onto a 300-node hairball.)
  it("keeps the guard when a concept is only selected, not focused", async () => {
    const user = userEvent.setup();
    renderWithProviders(<ConceptGraphView onOpenConcept={vi.fn()} />, guardAdapter());
    await waitFor(() => {
      expect(screen.getByText(GUARD_HEADING)).toBeInTheDocument();
    });

    // The toolbar combobox is the first of the two "find a concept" triggers; its
    // onSelect sets selectedId without touching the neighbourhood scope.
    const triggers = screen.getAllByRole("button", { name: "Find and focus a concept" });
    await user.click(triggers[0]);
    await user.click(screen.getByRole("option", { name: /Alpha/ }));

    // Still guarded: the unscoped view never renders the hairball.
    expect(screen.getByText(GUARD_HEADING)).toBeInTheDocument();
  });

  // The guard's own search focuses a concept's neighbourhood, which scopes the
  // server query — so the guard lifts and the scoped canvas renders.
  it("lifts the guard once a concept's neighbourhood is focused", async () => {
    const user = userEvent.setup();
    renderWithProviders(<ConceptGraphView onOpenConcept={vi.fn()} />, guardAdapter());
    await waitFor(() => {
      expect(screen.getByText(GUARD_HEADING)).toBeInTheDocument();
    });

    // The guard's combobox (second trigger) focuses the concept's neighbourhood.
    const triggers = screen.getAllByRole("button", { name: "Find and focus a concept" });
    await user.click(triggers[1]);
    await user.click(screen.getByRole("option", { name: /Alpha/ }));

    await waitFor(() => {
      expect(screen.queryByText(GUARD_HEADING)).not.toBeInTheDocument();
    });
  });
});

describe("ConceptsView", () => {
  it("lists concepts returned by the adapter", async () => {
    const adapter = mockAdapter({
      listConcepts: vi
        .fn()
        .mockResolvedValue({ concepts: sampleConcepts, total_count: sampleConcepts.length }),
    });
    renderWithProviders(<ConceptsView onOpenConcept={vi.fn()} />, adapter);

    await waitFor(() => {
      // "Checkout" appears as both the concept's primary name and its en-US term.
      expect(screen.getAllByText("Checkout").length).toBeGreaterThan(0);
    });
    expect(screen.getByText("3 concepts")).toBeInTheDocument();
    // A unique definition confirms the row body rendered.
    expect(screen.getByText("The flow where a shopper completes a purchase.")).toBeInTheDocument();
  });

  it("shows an empty state when there are no concepts", async () => {
    renderWithProviders(<ConceptsView onOpenConcept={vi.fn()} />, mockAdapter());
    await waitFor(() => {
      expect(screen.getByText("No concepts yet")).toBeInTheDocument();
    });
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
