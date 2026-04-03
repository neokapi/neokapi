/* eslint-disable @typescript-eslint/unbound-method -- vitest mock assertions reference methods */
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { TermbaseBrowser } from "@neokapi/ui-primitives";
import type { TermbaseAdapter, ConceptDTO } from "@neokapi/ui-primitives";

function makeConcept(overrides: Partial<ConceptDTO> = {}): ConceptDTO {
  return {
    id: "c-1",
    project_id: "",
    domain: "Legal",
    definition: "A binding agreement between parties",
    source: "",
    terms: [
      { text: "contract", locale: "en-US", status: "preferred" },
      { text: "contrat", locale: "fr-FR", status: "approved" },
    ],
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
    ...overrides,
  };
}

function createMockAdapter(concepts: ConceptDTO[] = [], totalCount?: number): TermbaseAdapter {
  return {
    search: vi.fn().mockResolvedValue({
      concepts,
      total_count: totalCount ?? concepts.length,
    }),
    getConcept: vi.fn().mockResolvedValue(null),
    addConcept: vi.fn().mockResolvedValue(undefined),
    updateConcept: vi.fn().mockResolvedValue(undefined),
    deleteConcept: vi.fn().mockResolvedValue(undefined),
    deleteConcepts: vi.fn().mockResolvedValue(undefined),
  };
}

describe("TermbaseBrowser", () => {
  let adapter: TermbaseAdapter;

  describe("rendering concepts", () => {
    const concepts = [
      makeConcept({
        id: "c1",
        domain: "Legal",
        terms: [
          { text: "contract", locale: "en-US", status: "preferred" },
          { text: "contrat", locale: "fr-FR", status: "approved" },
        ],
      }),
      makeConcept({
        id: "c2",
        domain: "Medical",
        definition: "The identification of a disease or condition",
        terms: [
          { text: "diagnosis", locale: "en-US", status: "preferred" },
          { text: "diagnostic", locale: "fr-FR", status: "preferred" },
        ],
      }),
    ];

    beforeEach(() => {
      adapter = createMockAdapter(concepts);
    });

    it("renders concepts from adapter", async () => {
      render(<TermbaseBrowser adapter={adapter} sourceLocale="en-US" targetLocales={["fr-FR"]} />);

      await waitFor(() => {
        expect(screen.getByText("contract")).toBeInTheDocument();
        expect(screen.getByText("contrat")).toBeInTheDocument();
        expect(screen.getByText("diagnosis")).toBeInTheDocument();
        expect(screen.getByText("diagnostic")).toBeInTheDocument();
      });
    });

    it("renders domain labels", async () => {
      render(<TermbaseBrowser adapter={adapter} sourceLocale="en-US" targetLocales={["fr-FR"]} />);

      await waitFor(() => {
        expect(screen.getByText("Legal")).toBeInTheDocument();
        expect(screen.getByText("Medical")).toBeInTheDocument();
      });
    });

    it("renders concept count", async () => {
      render(<TermbaseBrowser adapter={adapter} sourceLocale="en-US" targetLocales={["fr-FR"]} />);

      await waitFor(() => {
        expect(screen.getByText("2 concepts")).toBeInTheDocument();
      });
    });

    it("renders singular concept count", async () => {
      const singleAdapter = createMockAdapter([concepts[0]], 1);
      render(
        <TermbaseBrowser adapter={singleAdapter} sourceLocale="en-US" targetLocales={["fr-FR"]} />,
      );

      await waitFor(() => {
        expect(screen.getByText("1 concept")).toBeInTheDocument();
      });
    });

    it("renders term status badges", async () => {
      render(<TermbaseBrowser adapter={adapter} sourceLocale="en-US" targetLocales={["fr-FR"]} />);

      await waitFor(() => {
        // All "preferred" badges
        const preferredBadges = screen.getAllByText("preferred");
        expect(preferredBadges.length).toBeGreaterThanOrEqual(2);

        // "approved" badge
        expect(screen.getByText("approved")).toBeInTheDocument();
      });
    });

    it("renders definition text", async () => {
      render(<TermbaseBrowser adapter={adapter} sourceLocale="en-US" targetLocales={["fr-FR"]} />);

      await waitFor(() => {
        expect(screen.getByText("A binding agreement between parties")).toBeInTheDocument();
      });
    });
  });

  describe("search", () => {
    it("triggers adapter.search with debounced query", async () => {
      adapter = createMockAdapter([]);
      render(<TermbaseBrowser adapter={adapter} sourceLocale="en-US" targetLocales={["fr-FR"]} />);

      const input = screen.getByPlaceholderText("Search terminology...");
      await userEvent.type(input, "contract");

      await waitFor(
        () => {
          expect(adapter.search).toHaveBeenCalledWith("contract", "en-US", "fr-FR", 0, 50);
        },
        { timeout: 1000 },
      );
    });
  });

  describe("empty state", () => {
    it("shows empty state when no concepts", async () => {
      adapter = createMockAdapter([]);
      render(<TermbaseBrowser adapter={adapter} sourceLocale="en-US" targetLocales={["fr-FR"]} />);

      await waitFor(() => {
        expect(screen.getByText("No concepts yet.")).toBeInTheDocument();
      });
    });

    it("shows 'Add your first concept' link in empty state", async () => {
      adapter = createMockAdapter([]);
      render(<TermbaseBrowser adapter={adapter} sourceLocale="en-US" targetLocales={["fr-FR"]} />);

      await waitFor(() => {
        expect(screen.getByText("Add your first concept")).toBeInTheDocument();
      });
    });

    it("shows search empty state when search has no results", async () => {
      const emptyAdapter: TermbaseAdapter = {
        ...createMockAdapter([]),
        search: vi
          .fn()
          .mockResolvedValueOnce({ concepts: [], total_count: 0 })
          .mockResolvedValue({ concepts: [], total_count: 0 }),
      };

      render(
        <TermbaseBrowser adapter={emptyAdapter} sourceLocale="en-US" targetLocales={["fr-FR"]} />,
      );

      await waitFor(() => {
        expect(screen.getByText("No concepts yet.")).toBeInTheDocument();
      });

      const input = screen.getByPlaceholderText("Search terminology...");
      await userEvent.type(input, "nonexistent");

      await waitFor(
        () => {
          expect(screen.getByText("No concepts match your search.")).toBeInTheDocument();
        },
        { timeout: 1000 },
      );
    });
  });

  describe("add concept flow", () => {
    it("opens add form and calls adapter.addConcept", async () => {
      adapter = createMockAdapter([]);
      render(<TermbaseBrowser adapter={adapter} sourceLocale="en-US" targetLocales={["fr-FR"]} />);

      await waitFor(() => {
        expect(screen.getByText("Add Concept")).toBeInTheDocument();
      });

      await userEvent.click(screen.getByText("Add Concept"));

      // Dialog should open
      expect(screen.getByText("New Concept")).toBeInTheDocument();

      // Fill in terms (the first term input)
      const termInputs = screen.getAllByPlaceholderText("Term");
      await userEvent.type(termInputs[0], "agreement");
      await userEvent.type(termInputs[1], "accord");

      // Fill optional fields
      const domainInput = screen.getByPlaceholderText("e.g. Legal, Medical");
      await userEvent.type(domainInput, "Legal");

      // Click Save
      await userEvent.click(screen.getByRole("button", { name: "Save" }));

      await waitFor(() => {
        expect(adapter.addConcept).toHaveBeenCalledWith(
          expect.objectContaining({
            domain: "Legal",
            terms: expect.arrayContaining([
              expect.objectContaining({ text: "agreement", locale: "en-US" }),
              expect.objectContaining({ text: "accord", locale: "fr-FR" }),
            ]),
          }),
        );
      });
    });

    it("cancels add form without calling adapter", async () => {
      adapter = createMockAdapter([]);
      render(<TermbaseBrowser adapter={adapter} sourceLocale="en-US" targetLocales={["fr-FR"]} />);

      await waitFor(() => {
        expect(screen.getByText("Add Concept")).toBeInTheDocument();
      });

      await userEvent.click(screen.getByText("Add Concept"));
      expect(screen.getByText("New Concept")).toBeInTheDocument();

      await userEvent.click(screen.getByRole("button", { name: "Cancel" }));

      // Dialog should close
      expect(screen.queryByText("New Concept")).not.toBeInTheDocument();
      expect(adapter.addConcept).not.toHaveBeenCalled();
    });
  });

  describe("delete with confirmation", () => {
    it("requires confirmation before deleting a concept", async () => {
      const concept = makeConcept({ id: "c1", domain: "Legal" });
      adapter = createMockAdapter([concept]);

      render(<TermbaseBrowser adapter={adapter} sourceLocale="en-US" targetLocales={["fr-FR"]} />);

      await waitFor(() => {
        expect(screen.getByText("contract")).toBeInTheDocument();
      });

      // First click on Delete shows Confirm/Cancel
      const conceptCard = screen.getByTestId("concept-c1");
      const deleteBtn = within(conceptCard).getByText("Delete");
      await userEvent.click(deleteBtn);

      // Confirm and Cancel should appear
      const confirmBtn = within(conceptCard).getByText("Confirm");
      const cancelBtn = within(conceptCard).getAllByText("Cancel")[0];
      expect(confirmBtn).toBeInTheDocument();
      expect(cancelBtn).toBeInTheDocument();

      // Clicking Confirm actually deletes
      await userEvent.click(confirmBtn);

      await waitFor(() => {
        expect(adapter.deleteConcept).toHaveBeenCalledWith("c1");
      });
    });

    it("cancels delete confirmation", async () => {
      const concept = makeConcept({ id: "c1" });
      adapter = createMockAdapter([concept]);

      render(<TermbaseBrowser adapter={adapter} sourceLocale="en-US" targetLocales={["fr-FR"]} />);

      await waitFor(() => {
        expect(screen.getByText("contract")).toBeInTheDocument();
      });

      const conceptCard = screen.getByTestId("concept-c1");
      await userEvent.click(within(conceptCard).getByText("Delete"));

      // Click Cancel to dismiss confirmation
      await userEvent.click(within(conceptCard).getAllByText("Cancel")[0]);

      // Should be back to normal Delete button
      expect(within(conceptCard).getByText("Delete")).toBeInTheDocument();
      expect(adapter.deleteConcept).not.toHaveBeenCalled();
    });
  });

  describe("bulk actions", () => {
    it("shows bulk action bar when concepts are selected", async () => {
      const concepts = [
        makeConcept({ id: "c1", terms: [{ text: "word1", locale: "en-US", status: "preferred" }] }),
        makeConcept({ id: "c2", terms: [{ text: "word2", locale: "en-US", status: "preferred" }] }),
      ];
      adapter = createMockAdapter(concepts);

      render(<TermbaseBrowser adapter={adapter} sourceLocale="en-US" targetLocales={["fr-FR"]} />);

      await waitFor(() => {
        expect(screen.getByText("word1")).toBeInTheDocument();
      });

      // Select first concept via checkbox
      const conceptCard = screen.getByTestId("concept-c1");
      const checkbox = within(conceptCard).getByRole("checkbox");
      await userEvent.click(checkbox);

      expect(screen.getByText("1 selected")).toBeInTheDocument();
    });
  });

  describe("pagination", () => {
    it("shows pagination when total exceeds page size", async () => {
      const concepts = Array.from({ length: 50 }, (_, i) =>
        makeConcept({
          id: `c-${i}`,
          terms: [{ text: `term-${i}`, locale: "en-US", status: "preferred" }],
        }),
      );
      adapter = createMockAdapter(concepts, 60);
      render(<TermbaseBrowser adapter={adapter} sourceLocale="en-US" targetLocales={["fr-FR"]} />);

      await waitFor(() => {
        expect(screen.getByText("Next")).toBeInTheDocument();
        expect(screen.getByText("1 / 2")).toBeInTheDocument();
      });
    });
  });
});
