/* eslint-disable @typescript-eslint/unbound-method -- vitest mock assertions reference methods */
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { TMBrowser } from "@neokapi/ui-primitives";
import type { TMAdapter, TMEntryDTO, VariantDTO } from "@neokapi/ui-primitives";

function v(locale: string, text: string): VariantDTO {
  return { locale, text, runs: [{ text }] };
}

function makeTMEntry(overrides: Partial<TMEntryDTO> = {}): TMEntryDTO {
  return {
    id: "tm-1",
    project_id: "",
    hint_src_lang: "en-US",
    variants: {
      "en-US": v("en-US", "Hello"),
      "fr-FR": v("fr-FR", "Bonjour"),
    },
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
    ...overrides,
  };
}

function createMockAdapter(entries: TMEntryDTO[] = [], totalCount?: number): TMAdapter {
  return {
    search: vi.fn().mockResolvedValue({
      entries,
      total_count: totalCount ?? entries.length,
    }),
    getEntry: vi.fn().mockResolvedValue(null),
    addEntry: vi.fn().mockResolvedValue(undefined),
    updateEntry: vi.fn().mockResolvedValue(undefined),
    deleteEntry: vi.fn().mockResolvedValue(undefined),
    deleteEntries: vi.fn().mockResolvedValue(undefined),
  };
}

describe("TMBrowser", () => {
  let adapter: TMAdapter;

  describe("rendering entries", () => {
    const entries = [
      makeTMEntry({
        id: "1",
        variants: { "en-US": v("en-US", "Hello"), "fr-FR": v("fr-FR", "Bonjour") },
      }),
      makeTMEntry({
        id: "2",
        variants: { "en-US": v("en-US", "Goodbye"), "fr-FR": v("fr-FR", "Au revoir") },
      }),
    ];

    beforeEach(() => {
      adapter = createMockAdapter(entries);
    });

    it("renders entries from adapter", async () => {
      render(<TMBrowser adapter={adapter} sourceLocale="en-US" targetLocales={["fr-FR"]} />);

      await waitFor(() => {
        expect(screen.getByText("Hello")).toBeInTheDocument();
        expect(screen.getByText("Bonjour")).toBeInTheDocument();
        expect(screen.getByText("Goodbye")).toBeInTheDocument();
        expect(screen.getByText("Au revoir")).toBeInTheDocument();
      });
    });

    it("calls adapter.search on mount", async () => {
      render(<TMBrowser adapter={adapter} sourceLocale="en-US" targetLocales={["fr-FR"]} />);

      await waitFor(() => {
        expect(adapter.search).toHaveBeenCalledWith("", "", "", 0, 50);
      });
    });

    it("displays entry count", async () => {
      render(<TMBrowser adapter={adapter} sourceLocale="en-US" targetLocales={["fr-FR"]} />);

      await waitFor(() => {
        expect(screen.getByText("2 entries")).toBeInTheDocument();
      });
    });

    it("displays singular entry count", async () => {
      const singleAdapter = createMockAdapter([entries[0]], 1);
      render(<TMBrowser adapter={singleAdapter} sourceLocale="en-US" targetLocales={["fr-FR"]} />);

      await waitFor(() => {
        expect(screen.getByText("1 entry")).toBeInTheDocument();
      });
    });
  });

  describe("search", () => {
    it("triggers adapter.search with submitted query", async () => {
      adapter = createMockAdapter([]);
      render(<TMBrowser adapter={adapter} sourceLocale="en-US" targetLocales={["fr-FR"]} />);

      const input = screen.getByPlaceholderText("Search translation memory...");
      await userEvent.type(input, "test query{Enter}");

      await waitFor(() => {
        expect(adapter.search).toHaveBeenCalledWith("test query", "", "", 0, 50);
      });
    });
  });

  describe("empty state", () => {
    it("shows empty state when no entries", async () => {
      adapter = createMockAdapter([]);
      render(<TMBrowser adapter={adapter} sourceLocale="en-US" targetLocales={["fr-FR"]} />);

      await waitFor(() => {
        expect(screen.getByText("No entries yet.")).toBeInTheDocument();
      });
    });

    it("shows search empty state with clear button", async () => {
      const emptyAdapter: TMAdapter = {
        ...createMockAdapter([]),
        search: vi
          .fn()
          .mockResolvedValueOnce({ entries: [], total_count: 0 })
          .mockResolvedValue({ entries: [], total_count: 0 }),
      };
      render(<TMBrowser adapter={emptyAdapter} sourceLocale="en-US" targetLocales={["fr-FR"]} />);

      await waitFor(() => {
        expect(screen.getByText("No entries yet.")).toBeInTheDocument();
      });

      const input = screen.getByPlaceholderText("Search translation memory...");
      await userEvent.type(input, "nonexistent{Enter}");

      await waitFor(() => {
        expect(screen.getByText("No entries match your search.")).toBeInTheDocument();
      });
    });
  });

  describe("pagination", () => {
    it("shows pagination when total exceeds page size", async () => {
      const entries = Array.from({ length: 50 }, (_, i) =>
        makeTMEntry({
          id: `e-${i}`,
          variants: {
            "en-US": v("en-US", `Source ${i}`),
            "fr-FR": v("fr-FR", `Target ${i}`),
          },
        }),
      );
      adapter = createMockAdapter(entries, 60);
      render(<TMBrowser adapter={adapter} sourceLocale="en-US" targetLocales={["fr-FR"]} />);

      await waitFor(() => {
        expect(screen.getByText("Next")).toBeInTheDocument();
        expect(screen.getByText("Previous")).toBeInTheDocument();
        expect(screen.getByText("1 / 2")).toBeInTheDocument();
      });
    });

    it("navigates to next page", async () => {
      const entries = Array.from({ length: 50 }, (_, i) =>
        makeTMEntry({
          id: `e-${i}`,
          variants: { "en-US": v("en-US", `Source ${i}`), "fr-FR": v("fr-FR", `T${i}`) },
        }),
      );
      adapter = createMockAdapter(entries, 60);
      render(<TMBrowser adapter={adapter} sourceLocale="en-US" targetLocales={["fr-FR"]} />);

      await waitFor(() => {
        expect(screen.getByText("Next")).toBeInTheDocument();
      });

      await userEvent.click(screen.getByText("Next"));

      await waitFor(() => {
        expect(adapter.search).toHaveBeenCalledWith("", "", "", 50, 50);
      });
    });
  });

  describe("edit flow", () => {
    it("enters edit mode and shows inline code editor", async () => {
      const entry = makeTMEntry({
        id: "e1",
        variants: { "en-US": v("en-US", "Hello"), "fr-FR": v("fr-FR", "Bonjour") },
      });
      adapter = createMockAdapter([entry]);

      render(<TMBrowser adapter={adapter} sourceLocale="en-US" targetLocales={["fr-FR"]} />);

      await waitFor(() => {
        expect(screen.getByText("Hello")).toBeInTheDocument();
      });

      await userEvent.click(screen.getByText("Edit"));

      await waitFor(() => {
        expect(document.querySelector('[contenteditable="true"]')).not.toBeNull();
      });
    });
  });

  describe("delete", () => {
    it("calls adapter.deleteEntry when clicking delete on an entry", async () => {
      const entry = makeTMEntry({
        id: "e1",
        variants: { "en-US": v("en-US", "Hello"), "fr-FR": v("fr-FR", "Bonjour") },
      });
      adapter = createMockAdapter([entry]);

      render(<TMBrowser adapter={adapter} sourceLocale="en-US" targetLocales={["fr-FR"]} />);

      await waitFor(() => {
        expect(screen.getByText("Hello")).toBeInTheDocument();
      });

      await userEvent.click(screen.getByText("Delete"));
      await userEvent.click(screen.getByText("Confirm"));

      await waitFor(() => {
        expect(adapter.deleteEntry).toHaveBeenCalledWith("e1");
      });
    });
  });

  describe("bulk select + delete", () => {
    it("shows bulk action bar when entries are selected", async () => {
      const entries = [
        makeTMEntry({
          id: "e1",
          variants: { "en-US": v("en-US", "Hello"), "fr-FR": v("fr-FR", "Bonjour") },
        }),
        makeTMEntry({
          id: "e2",
          variants: { "en-US": v("en-US", "World"), "fr-FR": v("fr-FR", "Monde") },
        }),
      ];
      adapter = createMockAdapter(entries);

      render(<TMBrowser adapter={adapter} sourceLocale="en-US" targetLocales={["fr-FR"]} />);

      await waitFor(() => {
        expect(screen.getByText("Hello")).toBeInTheDocument();
      });

      const checkbox = screen.getByLabelText("Select entry Hello");
      await userEvent.click(checkbox);

      expect(screen.getByText("1 selected")).toBeInTheDocument();
    });

    it("bulk delete requires confirmation then calls adapter.deleteEntries", async () => {
      const entries = [
        makeTMEntry({
          id: "e1",
          variants: { "en-US": v("en-US", "Hello"), "fr-FR": v("fr-FR", "Bonjour") },
        }),
        makeTMEntry({
          id: "e2",
          variants: { "en-US": v("en-US", "World"), "fr-FR": v("fr-FR", "Monde") },
        }),
      ];
      adapter = createMockAdapter(entries);

      render(<TMBrowser adapter={adapter} sourceLocale="en-US" targetLocales={["fr-FR"]} />);

      await waitFor(() => {
        expect(screen.getByText("Hello")).toBeInTheDocument();
      });

      await userEvent.click(screen.getByLabelText("Select entry Hello"));
      await userEvent.click(screen.getByLabelText("Select entry World"));

      expect(screen.getByText("2 selected")).toBeInTheDocument();

      const bulkBar = screen.getByText("2 selected").closest("div")!;
      const deleteBtn = within(bulkBar).getByText("Delete");
      await userEvent.click(deleteBtn);

      const confirmBtn = screen.getByText(/Confirm delete 2/);
      expect(confirmBtn).toBeInTheDocument();

      await userEvent.click(confirmBtn);

      await waitFor(() => {
        expect(adapter.deleteEntries).toHaveBeenCalledWith(expect.arrayContaining(["e1", "e2"]));
      });
    });
  });

  describe("facet sidebar", () => {
    it("shows facet sidebar when adapter has getFacets", async () => {
      const facetAdapter: TMAdapter = {
        ...createMockAdapter([makeTMEntry()]),
        getFacets: vi.fn().mockResolvedValue({
          locales: [
            { locale: "en-US", count: 1 },
            { locale: "fr-FR", count: 1 },
          ],
          projects: [],
          entity_types: [],
          import_sessions: [],
          has_codes: 0,
          no_codes: 1,
        }),
      };

      render(<TMBrowser adapter={facetAdapter} sourceLocale="en-US" targetLocales={["fr-FR"]} />);

      await waitFor(() => {
        expect(screen.getByText("Filters")).toBeInTheDocument();
      });
    });
  });

  describe("add entry", () => {
    it("opens add form and calls adapter.addEntry with variants map", async () => {
      adapter = createMockAdapter([]);
      render(<TMBrowser adapter={adapter} sourceLocale="en-US" targetLocales={["fr-FR"]} />);

      await waitFor(() => {
        expect(screen.getByText("Add Entry")).toBeInTheDocument();
      });

      await userEvent.click(screen.getByText("Add Entry"));

      expect(screen.getByText("Add TM Entry")).toBeInTheDocument();

      const sourceInput = screen.getByPlaceholderText("Source text");
      const targetInput = screen.getByPlaceholderText("Target text");
      await userEvent.type(sourceInput, "New source");
      await userEvent.type(targetInput, "New target");

      await userEvent.click(screen.getByRole("button", { name: "Add" }));

      await waitFor(() => {
        expect(adapter.addEntry).toHaveBeenCalledWith({
          variants: {
            "en-US": { text: "New source" },
            "fr-FR": { text: "New target" },
          },
          hint_src_lang: "en-US",
        });
      });
    });
  });
});
