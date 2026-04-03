/* eslint-disable @typescript-eslint/unbound-method -- vitest mock assertions reference methods */
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { TMBrowser } from "@neokapi/ui-primitives";
import type { TMAdapter, TMEntryDTO } from "@neokapi/ui-primitives";

function makeTMEntry(overrides: Partial<TMEntryDTO> = {}): TMEntryDTO {
  return {
    id: "tm-1",
    source_text: "Hello",
    target_text: "Bonjour",
    source_coded: "",
    target_coded: "",
    source_spans: [],
    target_spans: [],
    source_locale: "en-US",
    target_locale: "fr-FR",
    project_id: "",
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
      makeTMEntry({ id: "1", source_text: "Hello", target_text: "Bonjour" }),
      makeTMEntry({ id: "2", source_text: "Goodbye", target_text: "Au revoir" }),
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
        expect(adapter.search).toHaveBeenCalledWith("", "en-US", "fr-FR", 0, 50);
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
    it("triggers adapter.search with debounced query", async () => {
      adapter = createMockAdapter([]);
      render(<TMBrowser adapter={adapter} sourceLocale="en-US" targetLocales={["fr-FR"]} />);

      const input = screen.getByPlaceholderText("Search translation memory...");
      await userEvent.type(input, "test query");

      // Wait for debounce (200ms) + re-render
      await waitFor(
        () => {
          expect(adapter.search).toHaveBeenCalledWith("test query", "en-US", "fr-FR", 0, 50);
        },
        { timeout: 1000 },
      );
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
      // First render with no results but with a search query active
      const emptyAdapter: TMAdapter = {
        ...createMockAdapter([]),
        search: vi
          .fn()
          .mockResolvedValueOnce({ entries: [], total_count: 0 }) // initial load
          .mockResolvedValue({ entries: [], total_count: 0 }), // after search
      };
      render(<TMBrowser adapter={emptyAdapter} sourceLocale="en-US" targetLocales={["fr-FR"]} />);

      await waitFor(() => {
        expect(screen.getByText("No entries yet.")).toBeInTheDocument();
      });

      const input = screen.getByPlaceholderText("Search translation memory...");
      await userEvent.type(input, "nonexistent");

      await waitFor(
        () => {
          expect(screen.getByText("No entries match your search.")).toBeInTheDocument();
        },
        { timeout: 1000 },
      );
    });
  });

  describe("pagination", () => {
    it("shows pagination when total exceeds page size", async () => {
      // 60 total entries with page size 50 = 2 pages
      const entries = Array.from({ length: 50 }, (_, i) =>
        makeTMEntry({ id: `e-${i}`, source_text: `Source ${i}`, target_text: `Target ${i}` }),
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
        makeTMEntry({ id: `e-${i}`, source_text: `Source ${i}` }),
      );
      adapter = createMockAdapter(entries, 60);
      render(<TMBrowser adapter={adapter} sourceLocale="en-US" targetLocales={["fr-FR"]} />);

      await waitFor(() => {
        expect(screen.getByText("Next")).toBeInTheDocument();
      });

      await userEvent.click(screen.getByText("Next"));

      await waitFor(() => {
        // Should call search with offset 50
        expect(adapter.search).toHaveBeenCalledWith("", "en-US", "fr-FR", 50, 50);
      });
    });
  });

  describe("edit flow", () => {
    it("enters edit mode, changes text, and saves", async () => {
      const entry = makeTMEntry({ id: "e1", source_text: "Hello", target_text: "Bonjour" });
      adapter = createMockAdapter([entry]);

      render(<TMBrowser adapter={adapter} sourceLocale="en-US" targetLocales={["fr-FR"]} />);

      await waitFor(() => {
        expect(screen.getByText("Hello")).toBeInTheDocument();
      });

      // Click Edit
      await userEvent.click(screen.getByText("Edit"));

      // Should show an input with the target text
      const editInput = screen.getByDisplayValue("Bonjour");
      expect(editInput).toBeInTheDocument();

      // Clear and type new text
      await userEvent.clear(editInput);
      await userEvent.type(editInput, "Salut");

      // Click Save
      await userEvent.click(screen.getByText("Save"));

      await waitFor(() => {
        expect(adapter.updateEntry).toHaveBeenCalledWith({
          entry_id: "e1",
          source: "Hello",
          target: "Salut",
          source_locale: "en-US",
          target_locale: "fr-FR",
          project_id: "",
        });
      });
    });

    it("cancels edit mode", async () => {
      const entry = makeTMEntry({ id: "e1", target_text: "Bonjour" });
      adapter = createMockAdapter([entry]);

      render(<TMBrowser adapter={adapter} sourceLocale="en-US" targetLocales={["fr-FR"]} />);

      await waitFor(() => {
        expect(screen.getByText("Edit")).toBeInTheDocument();
      });

      await userEvent.click(screen.getByText("Edit"));
      expect(screen.getByDisplayValue("Bonjour")).toBeInTheDocument();

      await userEvent.click(screen.getByText("Cancel"));

      // Edit input should be gone
      expect(screen.queryByDisplayValue("Bonjour")).not.toBeInTheDocument();
      expect(adapter.updateEntry).not.toHaveBeenCalled();
    });
  });

  describe("delete", () => {
    it("calls adapter.deleteEntry when clicking delete on an entry", async () => {
      const entry = makeTMEntry({ id: "e1", source_text: "Hello" });
      adapter = createMockAdapter([entry]);

      render(<TMBrowser adapter={adapter} sourceLocale="en-US" targetLocales={["fr-FR"]} />);

      await waitFor(() => {
        expect(screen.getByText("Hello")).toBeInTheDocument();
      });

      await userEvent.click(screen.getByText("Delete"));

      await waitFor(() => {
        expect(adapter.deleteEntry).toHaveBeenCalledWith("e1");
      });
    });
  });

  describe("bulk select + delete", () => {
    it("shows bulk action bar when entries are selected", async () => {
      const entries = [
        makeTMEntry({ id: "e1", source_text: "Hello" }),
        makeTMEntry({ id: "e2", source_text: "World" }),
      ];
      adapter = createMockAdapter(entries);

      render(<TMBrowser adapter={adapter} sourceLocale="en-US" targetLocales={["fr-FR"]} />);

      await waitFor(() => {
        expect(screen.getByText("Hello")).toBeInTheDocument();
      });

      // Select first entry
      const checkbox = screen.getByLabelText("Select entry Hello");
      await userEvent.click(checkbox);

      // Bulk action bar should appear
      expect(screen.getByText("1 selected")).toBeInTheDocument();
    });

    it("bulk delete requires confirmation then calls adapter.deleteEntries", async () => {
      const entries = [
        makeTMEntry({ id: "e1", source_text: "Hello" }),
        makeTMEntry({ id: "e2", source_text: "World" }),
      ];
      adapter = createMockAdapter(entries);

      render(<TMBrowser adapter={adapter} sourceLocale="en-US" targetLocales={["fr-FR"]} />);

      await waitFor(() => {
        expect(screen.getByText("Hello")).toBeInTheDocument();
      });

      // Select both entries
      await userEvent.click(screen.getByLabelText("Select entry Hello"));
      await userEvent.click(screen.getByLabelText("Select entry World"));

      expect(screen.getByText("2 selected")).toBeInTheDocument();

      // First click on Delete in bulk bar — triggers confirmation
      const bulkBar = screen.getByText("2 selected").closest("div")!;
      const deleteBtn = within(bulkBar).getByText("Delete");
      await userEvent.click(deleteBtn);

      // Confirmation button should appear
      const confirmBtn = screen.getByText(/Confirm delete 2/);
      expect(confirmBtn).toBeInTheDocument();

      // Click confirm
      await userEvent.click(confirmBtn);

      await waitFor(() => {
        expect(adapter.deleteEntries).toHaveBeenCalledWith(expect.arrayContaining(["e1", "e2"]));
      });
    });
  });

  describe("lookup panel", () => {
    it("shows lookup panel when showLookup=true and adapter has lookup", async () => {
      const lookupAdapter: TMAdapter = {
        ...createMockAdapter([makeTMEntry()]),
        lookup: vi.fn().mockResolvedValue([]),
      };

      render(
        <TMBrowser
          adapter={lookupAdapter}
          sourceLocale="en-US"
          targetLocales={["fr-FR"]}
          showLookup={true}
        />,
      );

      await waitFor(() => {
        // TMLookupPanel renders both an h3 "Lookup" heading and a "Lookup" button
        const lookupElements = screen.getAllByText("Lookup");
        expect(lookupElements.length).toBeGreaterThanOrEqual(1);
      });
    });

    it("does not show lookup panel when showLookup=false", async () => {
      const lookupAdapter: TMAdapter = {
        ...createMockAdapter([makeTMEntry()]),
        lookup: vi.fn().mockResolvedValue([]),
      };

      render(
        <TMBrowser
          adapter={lookupAdapter}
          sourceLocale="en-US"
          targetLocales={["fr-FR"]}
          showLookup={false}
        />,
      );

      await waitFor(() => {
        expect(screen.getByTestId("tm-browser")).toBeInTheDocument();
      });

      // Lookup heading should not be present
      expect(screen.queryAllByText("Lookup")).toHaveLength(0);
    });
  });

  describe("add entry", () => {
    it("opens add form and calls adapter.addEntry", async () => {
      adapter = createMockAdapter([]);
      render(<TMBrowser adapter={adapter} sourceLocale="en-US" targetLocales={["fr-FR"]} />);

      await waitFor(() => {
        expect(screen.getByText("Add Entry")).toBeInTheDocument();
      });

      await userEvent.click(screen.getByText("Add Entry"));

      // Dialog should open
      expect(screen.getByText("Add TM Entry")).toBeInTheDocument();

      // Fill in source and target
      const sourceInput = screen.getByPlaceholderText("Source text");
      const targetInput = screen.getByPlaceholderText("Target text");
      await userEvent.type(sourceInput, "New source");
      await userEvent.type(targetInput, "New target");

      // Click Add button in dialog
      await userEvent.click(screen.getByRole("button", { name: "Add" }));

      await waitFor(() => {
        expect(adapter.addEntry).toHaveBeenCalledWith({
          source: "New source",
          target: "New target",
          source_locale: "en-US",
          target_locale: "fr-FR",
        });
      });
    });
  });
});
