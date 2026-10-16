import { describe, it, expect, vi } from "vitest";
import { act, fireEvent, render, screen, waitFor, within } from "./testUtils";

import { MemoryListDetail, MEMORY_PAGE, pairsOf } from "../components/MemoryListDetail";
import { SAMPLE_ENTRIES, makeMemoryAdapter, manyEntries } from "../stories/fixtures/memory";
import type { MemoryEntryDTO } from "@neokapi/ui-primitives";

function renderMemory(
  entries: MemoryEntryDTO[] = SAMPLE_ENTRIES,
  props: Partial<React.ComponentProps<typeof MemoryListDetail>> = {},
) {
  const adapter = makeMemoryAdapter(entries);
  const spies = {
    search: vi.spyOn(adapter, "search"),
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

describe("pairs of a content memory entry", () => {
  it("gives one pair per language other than the source", () => {
    expect(pairsOf(SAMPLE_ENTRIES[0]).map((p) => p.id)).toEqual(["1::fr-FR", "1::de-DE"]);
    expect(pairsOf(SAMPLE_ENTRIES[0], new Set(["de-DE"])).map((p) => p.id)).toEqual(["1::de-DE"]);
  });
});

describe("Content memory as a list of pairs beside a detail pane", () => {
  it("lists one row per pair: the source on one line, and the target's language", async () => {
    renderMemory();
    await loaded();
    expect(pairRows().map((r) => r.getAttribute("data-list-detail-id"))).toEqual([
      "1::fr-FR",
      "1::de-DE",
      "2::fr-FR",
      "3::nb-NO",
    ]);
    const row = pair("2::fr-FR");
    expect(row.getAttribute("data-testid")).toBe("tm-entry-2-fr-FR");
    expect(row.textContent).toContain("Click");
    expect(row.textContent).toContain("to finish your checkout");
    expect(row.textContent).not.toContain("Cliquez");
    expect(row.querySelector("[data-slot='locale-label']")?.textContent).toContain("fr-FR");
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
    fireEvent.click(pair("2::fr-FR"));
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
    renderMemory(SAMPLE_ENTRIES, { defaultSelected: "1::fr-FR" });
    await waitFor(() =>
      expect(
        detail()?.querySelectorAll("[data-slot='memory-where-used'] [data-slot='coordinate-chip']"),
      ).toHaveLength(2),
    );
  });

  it("opens the unit an entry was approved for", async () => {
    const { spies } = renderMemory(SAMPLE_ENTRIES, { defaultSelected: "1::fr-FR" });
    await waitFor(() => expect(detail()).not.toBeNull());
    fireEvent.click(within(detail()).getByTestId("entry-unit"));
    expect(spies.openUnit).toHaveBeenCalledWith(expect.objectContaining({ id: "1" }));
  });

  it("names the entry's other languages and opens one", async () => {
    renderMemory(SAMPLE_ENTRIES, { defaultSelected: "1::fr-FR" });
    const others = await waitFor(() => {
      const el = detail()?.querySelector("[data-slot='memory-other-languages']");
      expect(el).toBeTruthy();
      return el as HTMLElement;
    });
    fireEvent.click(within(others).getByRole("button"));
    expect(pair("1::de-DE").getAttribute("aria-selected")).toBe("true");
    expect(detail().textContent).toContain("Zur Kasse gehen");
  });

  it("moves the selection with the list's keys, and down from the search goes into the list", async () => {
    renderMemory();
    await loaded();
    const search = screen.getByTestId("tm-search");
    fireEvent.keyDown(search, { key: "ArrowDown" });
    expect(pair("1::fr-FR").getAttribute("aria-selected")).toBe("true");
    const list = screen.getByRole("listbox", { name: "Entries" });
    expect(document.activeElement).toBe(list);
    fireEvent.keyDown(list, { key: "j" });
    expect(pair("1::de-DE").getAttribute("aria-selected")).toBe("true");
  });

  it("filters as you type, through the store's own search", async () => {
    const { spies } = renderMemory();
    await loaded();
    fireEvent.change(screen.getByTestId("tm-search"), { target: { value: "cart" } });
    await waitFor(() =>
      expect(pairRows().map((r) => r.getAttribute("data-list-detail-id"))).toEqual(["3::nb-NO"]),
    );
    expect(spies.search).toHaveBeenLastCalledWith("cart", "", "", 0, MEMORY_PAGE);
    expect(screen.getByText("1 entry matches")).toBeInTheDocument();
  });

  it("narrows the rows to the Active Filter's language, asking the store for that language", async () => {
    const { spies } = renderMemory(SAMPLE_ENTRIES, { targetLocales: ["de-DE"] });
    await loaded();
    expect(pairRows().map((r) => r.getAttribute("data-list-detail-id"))).toEqual(["1::de-DE"]);
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
      expect(document.querySelector("[data-list-detail-id='m149::fr-FR']")).not.toBeNull(),
    );
    expect(screen.queryByRole("button", { name: /more entries/ })).toBeNull();
  });

  it("opens the editor on the target from the toolbar", async () => {
    renderMemory(SAMPLE_ENTRIES, { defaultSelected: "1::fr-FR" });
    await waitFor(() => expect(detail()).not.toBeNull());
    fireEvent.click(detail().querySelector("[data-slot='memory-edit']") as HTMLElement);
    await waitFor(() =>
      expect(
        detail().querySelector(
          "[data-slot='memory-variant'][data-locale='fr-FR'] [contenteditable='true']",
        ),
      ).not.toBeNull(),
    );
  });

  it("deletes the entry after a confirmation, and clears the selection", async () => {
    const { spies } = renderMemory(SAMPLE_ENTRIES, { defaultSelected: "3::nb-NO" });
    await waitFor(() => expect(detail()).not.toBeNull());
    fireEvent.click(within(detail()).getByRole("button", { name: /Delete/ }));
    fireEvent.click(within(detail()).getByRole("button", { name: "Confirm" }));
    await waitFor(() => expect(spies.deleteEntry).toHaveBeenCalledWith("3"));
    await waitFor(() => expect(pair("3::nb-NO")).toBeNull());
    expect(screen.getByText("Choose an entry to see it here")).toBeInTheDocument();
  });

  it("adds an entry by hand from the page's dialog, and keeps the selection through the refetch", async () => {
    const onAddingChange = vi.fn();
    const { spies } = renderMemory(SAMPLE_ENTRIES, {
      adding: true,
      onAddingChange,
      defaultSelected: "2::fr-FR",
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
    await waitFor(() => expect(pair("new-1::fr-FR")).not.toBeNull());
    expect(pair("2::fr-FR").getAttribute("aria-selected")).toBe("true");
    expect(detail().getAttribute("data-entry")).toBe("2");
  });

  it("says a store with nothing in it holds no approved wording yet", async () => {
    renderMemory([]);
    await waitFor(() => expect(screen.getByText("No approved wording yet")).toBeInTheDocument());
    expect(screen.queryByRole("listbox")).toBeNull();
  });
});
