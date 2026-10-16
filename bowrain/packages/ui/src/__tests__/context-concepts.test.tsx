import { describe, it, expect, vi } from "vite-plus/test";
import type { ReactNode } from "react";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ApiProvider } from "../context/ApiContext";
import { WorkspaceProvider } from "../context/WorkspaceContext";
import type { ApiAdapter } from "../api/adapter";
import type { ConceptInfo, Membership, Workspace } from "../types/api";
import type { ConceptStory } from "../types/brand-graph";
import { ConceptsSection } from "../context-hub/concepts/ConceptsSection";

const workspace: Workspace = {
  id: "ws-1",
  name: "Demo",
  slug: "demo",
  description: "",
  logo_url: "",
  type: "personal",
  role: "owner",
};

const cart: ConceptInfo = {
  id: "c-cart",
  domain: "commerce",
  definition: "The items a shopper has chosen and not yet paid for.",
  terms: [
    { text: "cart", locale: "en-US", status: "preferred" },
    { text: "basket", locale: "en-US", status: "deprecated" },
    { text: "Warenkorb", locale: "de-DE", status: "approved" },
  ],
  properties: { coordinates: "brand=kapimart" },
  created_at: "2026-06-01T09:00:00Z",
  updated_at: "2026-06-12T09:00:00Z",
};

const paiement: ConceptInfo = {
  id: "c-pay",
  domain: "commerce",
  definition: "Paying for an order.",
  terms: [
    { text: "payment", locale: "en-US", status: "proposed" },
    { text: "Paiement", locale: "fr-FR", status: "proposed" },
  ],
  created_at: "2026-06-02T09:00:00Z",
  updated_at: "2026-06-02T09:00:00Z",
};

const story: ConceptStory = {
  concept_id: "c-cart",
  entries: [
    { kind: "revision", at: "2026-06-01T09:00:00Z", actor: "u-alex", summary: "Created" },
    { kind: "revision", at: "2026-06-12T09:00:00Z", actor: "u-maria", summary: "Revised" },
  ],
};

const members: Membership[] = [
  {
    user_id: "u-alex",
    workspace_id: "ws-1",
    role: "admin",
    user: { id: "u-alex", email: "alex@acme.example", name: "Alex Romero", avatar_url: "" },
  },
  {
    user_id: "u-maria",
    workspace_id: "ws-1",
    role: "member",
    user: { id: "u-maria", email: "maria@acme.example", name: "Maria Lund", avatar_url: "" },
  },
];

function mockAdapter(overrides: Partial<ApiAdapter> = {}): ApiAdapter {
  const concepts = [cart, paiement];
  return {
    listConcepts: vi.fn().mockImplementation(async (_ws: string, params?: { q?: string }) => {
      const list = params?.q
        ? concepts.filter((c) => c.terms.some((t) => t.text.includes(params.q!)))
        : concepts;
      return { concepts: list, total_count: list.length };
    }),
    getConcept: vi
      .fn()
      .mockImplementation(async (_ws: string, id: string) => concepts.find((c) => c.id === id)),
    getConceptStatusCounts: vi.fn().mockResolvedValue({ total: 2, by_status: {} }),
    getConceptStory: vi.fn().mockResolvedValue(story),
    listConceptRelations: vi.fn().mockResolvedValue([]),
    listConceptComments: vi.fn().mockResolvedValue([]),
    listObservations: vi.fn().mockResolvedValue([]),
    listMarkets: vi.fn().mockResolvedValue([]),
    listMembers: vi.fn().mockResolvedValue(members),
    listChangesets: vi.fn().mockResolvedValue([]),
    listProjects: vi
      .fn()
      .mockResolvedValue([
        { id: "p-1", name: "Website", default_source_language: "en-US", target_languages: [] },
      ]),
    updateConcept: vi.fn().mockResolvedValue(undefined),
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

describe("ConceptsSection", () => {
  it("lists one row per concept with its term, status and language count", async () => {
    renderSection(<ConceptsSection />, mockAdapter());

    const rows = await screen.findAllByTestId("concept-row");
    expect(rows).toHaveLength(2);
    expect(within(rows[0]).getByText("cart")).toBeInTheDocument();
    expect(within(rows[0]).getByText("Preferred")).toBeInTheDocument();
    // English and German: two languages, whatever their terms' statuses.
    expect(within(rows[0]).getByTitle("2 languages")).toHaveTextContent("2");
    expect(screen.getByText("2 concepts")).toBeInTheDocument();
  });

  it("asks the server for a search rather than filtering a page", async () => {
    const user = userEvent.setup();
    const adapter = mockAdapter();
    renderSection(<ConceptsSection />, adapter);

    await screen.findAllByTestId("concept-row");
    await user.type(screen.getByTestId("concept-search"), "pay");

    await waitFor(() => {
      const calls = (adapter.listConcepts as ReturnType<typeof vi.fn>).mock.calls;
      expect(calls.some(([, params]) => params?.q === "pay")).toBe(true);
    });
    // A page at a time, never the whole vocabulary.
    const calls = (adapter.listConcepts as ReturnType<typeof vi.fn>).mock.calls;
    expect(calls.every(([, params]) => (params?.limit ?? 0) <= 100)).toBe(true);
    await waitFor(() => expect(screen.getAllByTestId("concept-row")).toHaveLength(1));
    expect(screen.getByText("1 match")).toBeInTheDocument();
  });

  it("selects a row and reports it, so the route can carry it", async () => {
    const user = userEvent.setup();
    const onSelect = vi.fn();
    renderSection(<ConceptsSection onSelect={onSelect} />, mockAdapter());

    const rows = await screen.findAllByTestId("concept-row");
    await user.click(rows[1]);
    expect(onSelect).toHaveBeenCalledWith("c-pay");
    expect(await screen.findByTestId("concept-view")).toHaveAttribute("data-concept", "c-pay");
  });

  it("shows the selected concept's definition, discouraged forms and languages", async () => {
    renderSection(<ConceptsSection selectedId="c-cart" />, mockAdapter());

    const detail = await screen.findByTestId("concept-view");
    expect(
      await within(detail).findByText("The items a shopper has chosen and not yet paid for."),
    ).toBeInTheDocument();
    const discouraged = within(detail).getByRole("region", { name: "Discouraged forms" });
    expect(within(discouraged).getByText("basket")).toBeInTheDocument();
    expect(within(discouraged).getByText("cart")).toBeInTheDocument();
    const languages = within(detail).getByRole("region", { name: "Terms by language" });
    expect(within(languages).getByText("Warenkorb")).toBeInTheDocument();
    // A discouraged form is not listed among the terms in use.
    expect(within(languages).queryByText("basket")).not.toBeInTheDocument();
  });

  it("names who added the concept and who changed it, from its story", async () => {
    renderSection(<ConceptsSection selectedId="c-cart" />, mockAdapter());

    const provenance = await screen.findByRole("region", { name: "Where it came from" });
    await waitFor(() => expect(within(provenance).getByText("Alex Romero")).toBeInTheDocument());
    expect(within(provenance).getByText("Maria Lund")).toBeInTheDocument();
    expect(within(provenance).getByText("Every project in the workspace")).toBeInTheDocument();
  });

  it("opens a concept the list has not read, from its link", async () => {
    const adapter = mockAdapter({
      listConcepts: vi.fn().mockResolvedValue({ concepts: [cart], total_count: 1 }),
    });
    renderSection(<ConceptsSection selectedId="c-pay" />, adapter);

    const detail = await screen.findByTestId("concept-view");
    expect(await within(detail).findByText("Paying for an order.")).toBeInTheDocument();
    expect(adapter.getConcept).toHaveBeenCalledWith("demo", "c-pay");
  });

  it("approves a proposed headline term through the concept's terms", async () => {
    const user = userEvent.setup();
    const adapter = mockAdapter();
    renderSection(<ConceptsSection selectedId="c-pay" />, adapter);

    const approve = await screen.findByRole("button", { name: /Approve/ });
    await user.click(approve);

    await waitFor(() => expect(adapter.updateConcept).toHaveBeenCalledTimes(1));
    const [, request] = (adapter.updateConcept as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(request.concept_id).toBe("c-pay");
    expect(request.terms).toContainEqual(
      expect.objectContaining({ text: "payment", locale: "en-US", status: "approved" }),
    );
    // Only the headline term moves.
    expect(request.terms).toContainEqual(
      expect.objectContaining({ text: "Paiement", status: "proposed" }),
    );
  });

  it("offers no Approve on a concept whose headline term is already decided", async () => {
    renderSection(<ConceptsSection selectedId="c-cart" />, mockAdapter());
    await screen.findByTestId("concept-view");
    expect(screen.queryByRole("button", { name: /Approve/ })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Edit/ })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Relate/ })).toBeInTheDocument();
  });

  it("raises the change-set prompt when the server refuses an edit as governed", async () => {
    const user = userEvent.setup();
    const adapter = mockAdapter({
      updateConcept: vi
        .fn()
        .mockRejectedValue(
          new Error(
            '409: {"error":"governed change requires a change-set","detail":"the term status","hint":"open a change-set"}',
          ),
        ),
    });
    const onOpenExperiments = vi.fn();
    renderSection(
      <ConceptsSection selectedId="c-pay" onOpenExperiments={onOpenExperiments} />,
      adapter,
    );

    await user.click(await screen.findByRole("button", { name: /Approve/ }));
    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByText("This change needs review")).toBeInTheDocument();
    await user.click(within(dialog).getByRole("button", { name: /Open Changes/ }));
    expect(onOpenExperiments).toHaveBeenCalled();
  });

  it("links an observation's source only for an http or https address", async () => {
    const user = userEvent.setup();
    const observation = (id: string, source: string, url: string) => ({
      id,
      workspace_id: "ws-1",
      concept_id: "c-cart",
      kind: "web" as const,
      quote: `Quote from ${source}`,
      source,
      url,
      created_by: "u-alex",
      created_at: "2026-06-10T09:00:00Z",
    });
    const adapter = mockAdapter({
      listObservations: vi
        .fn()
        .mockResolvedValue([
          observation("o-1", "Rival landing page", "https://example.com/rival"),
          observation("o-2", "Planted note", "javascript:alert(document.cookie)"),
        ]),
    });
    renderSection(<ConceptsSection selectedId="c-cart" />, adapter);

    await user.click(await screen.findByRole("tab", { name: "Evidence" }));
    const safe = await screen.findByText("Rival landing page");
    expect(safe.closest("a")).toHaveAttribute("href", "https://example.com/rival");
    expect(safe.closest("a")).toHaveAttribute("rel", "noopener noreferrer");

    // A script address is the source's name alone: there is nothing to click.
    const planted = screen.getByText("Planted note");
    expect(planted.closest("a")).toBeNull();
    expect(document.querySelector('a[href^="javascript:"]')).toBeNull();
  });

  it("says the workspace has no concepts yet", async () => {
    const adapter = mockAdapter({
      listConcepts: vi.fn().mockResolvedValue({ concepts: [], total_count: 0 }),
    });
    renderSection(<ConceptsSection />, adapter);
    expect(await screen.findByText("No concepts yet")).toBeInTheDocument();
  });

  it("says every concept is waiting on a review when a change-set holds them", async () => {
    const adapter = mockAdapter({
      listConcepts: vi.fn().mockResolvedValue({ concepts: [], total_count: 0 }),
      listChangesets: vi.fn().mockResolvedValue([
        {
          id: "cs-1",
          workspace_id: "ws-1",
          name: "Import the store's terms",
          status: "in_review",
          created_by: "u-alex",
          created_at: "2026-06-01T00:00:00Z",
          updated_at: "2026-06-13T00:00:00Z",
          submitted_at: "2026-06-13T00:00:00Z",
          ops_count: 57,
        },
      ]),
    });
    renderSection(<ConceptsSection />, adapter);
    expect(
      await screen.findByText("Every concept here is waiting on a review"),
    ).toBeInTheDocument();
    expect(screen.getByText("Import the store's terms")).toBeInTheDocument();
  });
});
