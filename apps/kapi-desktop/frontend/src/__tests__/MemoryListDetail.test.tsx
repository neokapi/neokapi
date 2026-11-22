import { describe, it, expect, vi } from "vite-plus/test";
import { act, fireEvent, render, screen, waitFor, within } from "./testUtils";

import { MemoryListDetail, MEMORY_PAGE, rowsOf } from "../components/MemoryListDetail";
import {
  SAMPLE_ENTRIES,
  SIX_LANGUAGES,
  makeMemoryAdapter,
  manyEntries,
} from "../stories/fixtures/memory";
import type { MemoryEntryDTO } from "@neokapi/ui-primitives";

function renderMemory(
  entries: MemoryEntryDTO[] = SAMPLE_ENTRIES,
  props: Partial<React.ComponentProps<typeof MemoryListDetail>> = {},
) {
  const adapter = makeMemoryAdapter(entries);
  const spies = {
    search: vi.spyOn(adapter, "search"),
    searchFiltered: vi.spyOn(adapter, "searchFiltered" as never) as unknown as ReturnType<
      typeof vi.fn
    >,
    deleteEntries: vi.spyOn(adapter, "deleteEntries"),
    lookup: vi.spyOn(adapter, "lookup" as never) as unknown as ReturnType<typeof vi.fn>,
    updateEntry: vi.spyOn(adapter, "updateEntry"),
    deleteEntry: vi.spyOn(adapter, "deleteEntry"),
    addEntry: vi.spyOn(adapter, "addEntry"),
    openUnit: vi.spyOn(adapter, "openUnit" as never) as unknown as ReturnType<typeof vi.fn>,
  };
  const view = render(<MemoryListDetail adapter={adapter} handle="mem-1" {...props} />);
  return { view, adapter, spies };
}

const pairRows = () =>
  Array.from(document.querySelectorAll<HTMLElement>("[data-slot='memory-pair']"));
const pair = (id: string) =>
  document.querySelector(`[data-slot='memory-pair'][data-list-detail-id='${id}']`) as HTMLElement;
const detail = () => document.querySelector("[data-slot='memory-pair-detail']") as HTMLElement;

async function loaded() {
  await waitFor(() => expect(pairRows().length).toBeGreaterThan(0));
}

const ids = () => pairRows().map((r) => r.getAttribute("data-list-detail-id"));

/** Type into the filter bar and press Enter, the way a search is sent. */
function searchFor(text: string) {
  const input = screen.getByTestId("filterbar-search");
  fireEvent.change(input, { target: { value: text } });
  fireEvent.keyDown(input, { key: "Enter" });
}

describe("rows of a content memory", () => {
  it("gives one row per source wording, with every other language it is held in", () => {
    const rows = rowsOf(SAMPLE_ENTRIES);
    expect(rows.map((r) => r.id)).toEqual(["1", "2", "3"]);
    expect(rows[0].targets).toEqual(["fr-FR", "de-DE"]);
    expect(rowsOf(SAMPLE_ENTRIES, new Set(["de-DE"])).map((r) => [r.id, r.targets])).toEqual([
      ["1", ["de-DE"]],
    ]);
  });

  it("puts entries that share their source wording on one row", () => {
    const twin: MemoryEntryDTO = {
      ...SAMPLE_ENTRIES[0],
      id: "1b",
      variants: {
        "en-US": SAMPLE_ENTRIES[0].variants["en-US"],
        "ja-JP": { locale: "ja-JP", text: "レジに進む", runs: [{ text: "レジに進む" }] },
      },
    };
    const rows = rowsOf([SAMPLE_ENTRIES[0], twin, SAMPLE_ENTRIES[1]]);
    expect(rows.map((r) => r.id)).toEqual(["1", "2"]);
    expect(rows[0].entries.map((e) => e.id)).toEqual(["1", "1b"]);
    expect(rows[0].targets).toEqual(["fr-FR", "de-DE", "ja-JP"]);
  });
});

describe("Content memory as a list of source wordings beside a detail pane", () => {
  it("lists one row per source wording: the source on one line, and its languages", async () => {
    renderMemory();
    await loaded();
    expect(pairRows().map((r) => r.getAttribute("data-list-detail-id"))).toEqual(["1", "2", "3"]);
    const row = pair("2");
    expect(row.getAttribute("data-testid")).toBe("tm-entry-2");
    expect(row.textContent).toContain("Click");
    expect(row.textContent).toContain("to finish your checkout");
    expect(row.textContent).not.toContain("Cliquez");
    expect(row.querySelector("[data-slot='locale-label']")?.textContent).toContain("fr-FR");
    const pills = Array.from(pair("1").querySelectorAll("[data-slot='locale-label']"));
    expect(pills.map((p) => p.textContent)).toEqual(["fr-FR", "de-DE"]);
  });

  it("shows three languages on a row and counts the rest", async () => {
    renderMemory([SIX_LANGUAGES]);
    await loaded();
    const row = pair("six");
    expect(row.querySelectorAll("[data-slot='locale-label']")).toHaveLength(3);
    expect(row.querySelector("[data-slot='memory-more-languages']")?.textContent).toBe("+2");
    fireEvent.click(row);
    const variants = Array.from(detail().querySelectorAll("[data-slot='memory-variant']"));
    expect(variants.map((v) => v.getAttribute("data-locale"))).toEqual([
      "en",
      "ar",
      "de",
      "fr",
      "ja",
      "nb",
    ]);
  });

  it("opens with nothing selected and says what the pane shows", async () => {
    renderMemory();
    await loaded();
    expect(screen.getByText("Choose an entry to see it here")).toBeInTheDocument();
    expect(detail()).toBeNull();
  });

  it("shows both sides in full, with provenance and the unit it was approved for", async () => {
    renderMemory();
    await loaded();
    fireEvent.click(pair("2"));
    const pane = detail();
    const variants = Array.from(pane.querySelectorAll("[data-slot='memory-variant']"));
    expect(variants.map((v) => v.getAttribute("data-locale"))).toEqual(["en-US", "fr-FR"]);
    expect(variants[1].textContent).toContain("Cliquez");
    expect(variants[1].textContent).toContain("pour terminer votre commande");
    expect(pane.textContent).toContain("Bold marks the link text.");

    const provenance = pane.querySelector("[data-slot='memory-provenance']") as HTMLElement;
    expect(provenance.textContent).toContain("docs/checkout.md#p3");
    expect(provenance.textContent).toContain("asgeir");
    expect(provenance.querySelector("[data-slot='actor-mark']")).not.toBeNull();
    expect(provenance.querySelector("time")).not.toBeNull();

    const used = pane.querySelector("[data-slot='memory-where-used']") as HTMLElement;
    expect(within(used).getByTestId("entry-unit").textContent).toBe("p3");
    expect(used.querySelectorAll("[data-slot='coordinate-chip']")).toHaveLength(2);
  });

  it("resolves where an entry's unit sits when the entry records no point", async () => {
    renderMemory(SAMPLE_ENTRIES, { defaultSelected: "1" });
    await waitFor(() =>
      expect(
        detail()?.querySelectorAll("[data-slot='memory-where-used'] [data-slot='coordinate-chip']"),
      ).toHaveLength(2),
    );
  });

  it("opens the unit an entry was approved for", async () => {
    const { spies } = renderMemory(SAMPLE_ENTRIES, { defaultSelected: "1" });
    await waitFor(() => expect(detail()).not.toBeNull());
    fireEvent.click(within(detail()).getByTestId("entry-unit"));
    expect(spies.openUnit).toHaveBeenCalledWith(expect.objectContaining({ id: "1" }));
  });

  it("shows the wording in every language the row holds", async () => {
    renderMemory(SAMPLE_ENTRIES, { defaultSelected: "1" });
    await waitFor(() => expect(detail()).not.toBeNull());
    const variants = Array.from(detail().querySelectorAll("[data-slot='memory-variant']"));
    expect(variants.map((v) => v.getAttribute("data-locale"))).toEqual(["en-US", "fr-FR", "de-DE"]);
    expect(detail().textContent).toContain("Passer à la caisse");
    expect(detail().textContent).toContain("Zur Kasse gehen");
  });

  it("shows only the Active Filter's languages in the detail", async () => {
    renderMemory(SAMPLE_ENTRIES, { targetLocales: ["de-DE"], defaultSelected: "1" });
    await waitFor(() => expect(detail()).not.toBeNull());
    const variants = Array.from(detail().querySelectorAll("[data-slot='memory-variant']"));
    expect(variants.map((v) => v.getAttribute("data-locale"))).toEqual(["en-US", "de-DE"]);
  });

  it("moves the selection with the list's keys", async () => {
    renderMemory();
    await loaded();
    fireEvent.click(pair("1"));
    const list = screen.getByRole("listbox", { name: "Entries" });
    fireEvent.keyDown(list, { key: "j" });
    expect(pair("2").getAttribute("aria-selected")).toBe("true");
    fireEvent.keyDown(list, { key: "End" });
    expect(pair("3").getAttribute("aria-selected")).toBe("true");
  });

  it("searches the store when Enter sends the search", async () => {
    const { spies } = renderMemory();
    await loaded();
    searchFor("cart");
    await waitFor(() => expect(ids()).toEqual(["3"]));
    expect(spies.search).toHaveBeenLastCalledWith("cart", "", "", 0, MEMORY_PAGE);
    expect(screen.getByText("1 entry matches")).toBeInTheDocument();
  });

  it("narrows the rows to the Active Filter's language, asking the store for that language", async () => {
    const { spies } = renderMemory(SAMPLE_ENTRIES, { targetLocales: ["de-DE"] });
    await loaded();
    expect(pairRows().map((r) => r.getAttribute("data-list-detail-id"))).toEqual(["1"]);
    expect(spies.search).toHaveBeenCalledWith("", "", "de-DE", 0, MEMORY_PAGE);
  });

  it("reads a large store a page at a time", async () => {
    const { spies } = renderMemory(manyEntries(150));
    await loaded();
    fireEvent.click(screen.getByRole("button", { name: "Show 50 more entries" }));
    await waitFor(() =>
      expect(spies.search).toHaveBeenLastCalledWith("", "", "", MEMORY_PAGE, MEMORY_PAGE),
    );
    await waitFor(() =>
      expect(document.querySelector("[data-list-detail-id='m149']")).not.toBeNull(),
    );
    expect(screen.queryByRole("button", { name: /more entries/ })).toBeNull();
  });

  it("opens the editor on one language from its own Edit button", async () => {
    renderMemory(SAMPLE_ENTRIES, { defaultSelected: "1" });
    await waitFor(() => expect(detail()).not.toBeNull());
    fireEvent.click(within(detail()).getByRole("button", { name: "Edit the fr-FR wording" }));
    expect(
      detail().querySelector(
        "[data-slot='memory-variant'][data-locale='de-DE'] [contenteditable='true']",
      ),
    ).toBeNull();
    await waitFor(() =>
      expect(
        detail().querySelector(
          "[data-slot='memory-variant'][data-locale='fr-FR'] [contenteditable='true']",
        ),
      ).not.toBeNull(),
    );
  });

  it("deletes the entry after a confirmation, and clears the selection", async () => {
    const { spies } = renderMemory(SAMPLE_ENTRIES, { defaultSelected: "3" });
    await waitFor(() => expect(detail()).not.toBeNull());
    fireEvent.click(within(detail()).getByRole("button", { name: /Delete/ }));
    fireEvent.click(within(detail()).getByRole("button", { name: "Confirm" }));
    await waitFor(() => expect(spies.deleteEntry).toHaveBeenCalledWith("3"));
    await waitFor(() => expect(pair("3")).toBeNull());
    expect(screen.getByText("Choose an entry to see it here")).toBeInTheDocument();
  });

  it("adds an entry by hand from the page's dialog, and keeps the selection through the refetch", async () => {
    const onAddingChange = vi.fn();
    const { spies } = renderMemory(SAMPLE_ENTRIES, {
      adding: true,
      onAddingChange,
      defaultSelected: "2",
      sourceLocale: "en-US",
    });
    await loaded();
    const dialog = await screen.findByRole("dialog");
    fireEvent.change(within(dialog).getByLabelText("Source"), { target: { value: "Pay now" } });
    fireEvent.change(within(dialog).getByLabelText("Target"), { target: { value: "Payer" } });
    fireEvent.change(within(dialog).getByLabelText("Target language"), {
      target: { value: "fr-FR" },
    });
    await act(async () => {
      fireEvent.click(within(dialog).getByRole("button", { name: "Add entry" }));
    });
    await waitFor(() =>
      expect(spies.addEntry).toHaveBeenCalledWith({
        variants: { "en-US": { text: "Pay now" }, "fr-FR": { text: "Payer" } },
        hint_src_lang: "en-US",
      }),
    );
    await waitFor(() => expect(onAddingChange).toHaveBeenCalledWith(false));
    await waitFor(() => expect(pair("new-1")).not.toBeNull());
    expect(pair("2").getAttribute("aria-selected")).toBe("true");
    expect(detail().getAttribute("data-entry")).toBe("2");
  });

  it("says a store with nothing in it holds no approved wording yet", async () => {
    renderMemory([]);
    await waitFor(() => expect(screen.getByText("No approved wording yet")).toBeInTheDocument());
    expect(screen.queryByRole("listbox")).toBeNull();
  });
});

describe("the filter bar: the store's facets and filter tokens", () => {
  it("offers a field for each facet the store counts", async () => {
    renderMemory();
    await loaded();
    fireEvent.click(screen.getByRole("button", { name: /Filters/ }));
    for (const key of ["language:", "target:", "project:", "entity:", "import:", "codes:"]) {
      expect(screen.getByText(key)).toBeInTheDocument();
    }
  });

  it("narrows through the store's filtered search when a token is typed", async () => {
    const { spies } = renderMemory();
    await loaded();
    searchFor("codes:with");
    await waitFor(() => expect(ids()).toEqual(["2", "3"]));
    expect(spies.searchFiltered).toHaveBeenLastCalledWith(
      "",
      "",
      "",
      { has_codes: true },
      0,
      MEMORY_PAGE,
    );
  });

  it("adds a token from the Filters menu, by facet and value", async () => {
    const { spies } = renderMemory();
    await loaded();
    fireEvent.click(screen.getByRole("button", { name: /Filters/ }));
    fireEvent.click(screen.getByText("entity:"));
    fireEvent.click(screen.getByText("entity:person · 1"));
    await waitFor(() => expect(ids()).toEqual(["3"]));
    expect(spies.searchFiltered).toHaveBeenLastCalledWith(
      "",
      "",
      "",
      { entity_types: ["entity:person"] },
      0,
      MEMORY_PAGE,
    );
  });

  it("narrows by import session and by project", async () => {
    const { spies } = renderMemory();
    await loaded();
    searchFor("import:imp-1");
    await waitFor(() => expect(ids()).toEqual(["3"]));
    expect(spies.searchFiltered).toHaveBeenLastCalledWith(
      "",
      "",
      "",
      { session_ids: ["imp-1"] },
      0,
      MEMORY_PAGE,
    );
    fireEvent.click(screen.getByText("imp-1").parentElement!.querySelector("button")!);
    searchFor("project:kapimart");
    await waitFor(() => expect(ids()).toEqual(["1", "2"]));
  });

  it("keeps the rows held in one target language with target:", async () => {
    const { spies } = renderMemory();
    await loaded();
    searchFor("target:de-DE");
    await waitFor(() => expect(ids()).toEqual(["1"]));
    expect(spies.search).toHaveBeenLastCalledWith("", "", "de-DE", 0, MEMORY_PAGE);
  });

  it("searches the wording in one language with language:", async () => {
    const { spies } = renderMemory();
    await loaded();
    searchFor("language:nb-NO handlekurven");
    await waitFor(() => expect(ids()).toEqual(["3"]));
    expect(spies.search).toHaveBeenLastCalledWith("handlekurven", "nb-NO", "", 0, MEMORY_PAGE);
  });
});

describe("marking entries and acting on all of them", () => {
  const mark = (id: string) =>
    fireEvent.click(pair(id).querySelector("[data-slot='memory-mark']") as HTMLElement);
  const bulk = () => document.querySelector("[data-slot='memory-bulk-detail']") as HTMLElement;

  it("marks a row's entry from its box without moving the selection", async () => {
    renderMemory(SAMPLE_ENTRIES, { defaultSelected: "3" });
    await loaded();
    mark("1");
    expect(pair("1").hasAttribute("data-marked")).toBe(true);
    expect(pair("2").hasAttribute("data-marked")).toBe(false);
    expect(pair("3").getAttribute("aria-selected")).toBe("true");
    expect(within(bulk()).getByRole("heading").textContent).toBe("1 entry marked");
  });

  it("marks the selected row with x, and every entry in view with Mark all shown", async () => {
    renderMemory();
    await loaded();
    fireEvent.click(pair("2"));
    fireEvent.keyDown(screen.getByRole("listbox", { name: "Entries" }), { key: "x" });
    expect(pair("2").hasAttribute("data-marked")).toBe(true);
    fireEvent.click(screen.getByRole("button", { name: "Mark all shown" }));
    expect(within(bulk()).getByRole("heading").textContent).toBe("3 entries marked");
    fireEvent.click(screen.getByRole("button", { name: "Unmark all shown" }));
    expect(bulk()).toBeNull();
  });

  it("deletes every marked entry after a confirmation, from the toolbar", async () => {
    const { spies } = renderMemory();
    await loaded();
    mark("1");
    mark("2");
    fireEvent.click(within(bulk()).getByRole("button", { name: /Delete/ }));
    expect(spies.deleteEntries).not.toHaveBeenCalled();
    fireEvent.click(within(bulk()).getByRole("button", { name: "Confirm" }));
    await waitFor(() => expect(spies.deleteEntries).toHaveBeenCalledWith(["1", "2"]));
    await waitFor(() => expect(ids()).toEqual(["3"]));
    expect(bulk()).toBeNull();
  });

  it("marks the names in every marked entry, and clears the marks", async () => {
    renderMemory();
    await loaded();
    mark("1");
    mark("3");
    fireEvent.click(bulk().querySelector("[data-slot='memory-bulk-mark-entities']") as HTMLElement);
    expect(await screen.findByText(/across 2 selected entries/)).toBeInTheDocument();
    fireEvent.click(bulk().querySelector("[data-slot='memory-bulk-clear']") as HTMLElement);
    expect(bulk()).toBeNull();
  });
});

describe("Find similar: the fuzzy lookup as a mode of the search", () => {
  it("looks up wording like a sentence and lists the matches with their scores", async () => {
    const { spies } = renderMemory(SAMPLE_ENTRIES, { sourceLocale: "en-US" });
    await loaded();
    fireEvent.click(screen.getByRole("radio", { name: "Find similar" }));
    const input = screen.getByTestId("tm-lookup");
    fireEvent.change(input, { target: { value: "Proceed to checkout" } });
    fireEvent.keyDown(input, { key: "Enter" });
    await waitFor(() =>
      expect(spies.lookup).toHaveBeenCalledWith(
        expect.objectContaining({
          text: "Proceed to checkout",
          source_locale: "en-US",
          target_locale: "fr-FR",
          entities: [],
        }),
      ),
    );
    await waitFor(() => expect(ids()[0]).toBe("1"));
    expect(pair("1").querySelector("[data-slot='memory-match-score']")?.textContent).toBe("100%");
    fireEvent.click(pair("1"));
    expect(detail().querySelector("[data-slot='memory-match']")).not.toBeNull();
  });

  it("matches the names marked in the sentence as placeholders", async () => {
    const { spies } = renderMemory(SAMPLE_ENTRIES, { sourceLocale: "en-US" });
    await loaded();
    fireEvent.click(screen.getByRole("radio", { name: "Find similar" }));
    const input = screen.getByTestId("tm-lookup") as HTMLInputElement;
    fireEvent.change(input, { target: { value: "Alice added 3 items to the cart" } });
    input.setSelectionRange(0, 5);
    fireEvent.mouseUp(input);
    fireEvent.click(screen.getByRole("button", { name: "Person" }));
    fireEvent.keyDown(input, { key: "Enter" });
    await waitFor(() =>
      expect(spies.lookup).toHaveBeenCalledWith(
        expect.objectContaining({
          entities: [{ text: "Alice", type: "entity:person", start: 0, end: 5 }],
        }),
      ),
    );
    await waitFor(() => expect(ids()).toContain("3"));
    fireEvent.click(pair("3"));
    expect(detail().querySelector("[data-slot='memory-match']")?.textContent).toContain(
      "Alice takes the place of Bob",
    );
  });

  it("goes back to the search, and its filters, in one click", async () => {
    renderMemory();
    await loaded();
    fireEvent.click(screen.getByRole("radio", { name: "Find similar" }));
    expect(screen.queryByTestId("filterbar-search")).toBeNull();
    fireEvent.click(screen.getByRole("radio", { name: "Search" }));
    expect(screen.getByTestId("filterbar-search")).toBeInTheDocument();
    await loaded();
  });
});

describe("entries that share their source wording", () => {
  const twin: MemoryEntryDTO = {
    ...SAMPLE_ENTRIES[0],
    id: "1b",
    variants: {
      "en-US": SAMPLE_ENTRIES[0].variants["en-US"],
      "ja-JP": { locale: "ja-JP", text: "レジに進む", runs: [{ text: "レジに進む" }] },
    },
  };
  const entries = [SAMPLE_ENTRIES[0], twin, SAMPLE_ENTRIES[1]];

  it("shows each entry's languages in the one row's detail", async () => {
    renderMemory(entries, { defaultSelected: "1" });
    await waitFor(() => expect(detail()).not.toBeNull());
    expect(ids()).toEqual(["1", "2"]);
    const variants = Array.from(detail().querySelectorAll("[data-slot='memory-variant']"));
    expect(variants.map((v) => v.getAttribute("data-locale"))).toEqual([
      "en-US",
      "fr-FR",
      "de-DE",
      "ja-JP",
    ]);
    expect(detail().querySelectorAll("[data-slot='memory-provenance']")).toHaveLength(2);
  });

  it("marks and deletes every entry in the row", async () => {
    const { spies } = renderMemory(entries, { defaultSelected: "1" });
    await waitFor(() => expect(detail()).not.toBeNull());
    fireEvent.click(pair("1").querySelector("[data-slot='memory-mark']") as HTMLElement);
    const bulk = document.querySelector("[data-slot='memory-bulk-detail']") as HTMLElement;
    expect(within(bulk).getByRole("heading").textContent).toBe("2 entries marked");
    fireEvent.click(within(bulk).getByRole("button", { name: "Clear" }));
    fireEvent.click(within(detail()).getByRole("button", { name: /Delete/ }));
    fireEvent.click(within(detail()).getByRole("button", { name: "Confirm" }));
    await waitFor(() => expect(spies.deleteEntries).toHaveBeenCalledWith(["1", "1b"]));
  });
});
