import { describe, it, expect, vi } from "vite-plus/test";
import { act, fireEvent, render, screen, waitFor, within } from "./testUtils";

import { ConceptsView, CONCEPT_PAGE } from "../components/ConceptsView";
import { createLocalConceptSource } from "../lib/localConceptSource";
import { SAMPLE_CONCEPTS, makeConceptBackend, manyConcepts } from "../stories/fixtures/concepts";
import type { ConceptDTO } from "../lib/localConceptSource";

function renderTerms(
  seed: ConceptDTO[] = SAMPLE_CONCEPTS,
  props: Partial<React.ComponentProps<typeof ConceptsView>> = {},
) {
  const backend = makeConceptBackend(seed, seed === SAMPLE_CONCEPTS ? undefined : []);
  const spies = {
    searchTerms: vi.spyOn(backend, "searchTerms"),
    addRelation: vi.spyOn(backend, "addRelation"),
    removeRelation: vi.spyOn(backend, "removeRelation"),
  };
  const source = createLocalConceptSource("h1", backend);
  const view = render(<ConceptsView handle="h1" source={source} sourceLocale="en-US" {...props} />);
  return { view, backend, spies };
}

const row = (id: string) =>
  document.querySelector(`[data-slot='terms-concept'][data-list-detail-id='${id}']`) as HTMLElement;
const detail = () => document.querySelector("[data-slot='terms-concept-detail']") as HTMLElement;

async function rows() {
  await waitFor(() =>
    expect(document.querySelectorAll("[data-slot='terms-concept']").length).toBeGreaterThan(0),
  );
  return Array.from(document.querySelectorAll<HTMLElement>("[data-slot='terms-concept']"));
}

describe("Terms as a list of concepts beside a detail pane", () => {
  it("lists one row per concept: its term, that term's status, and its languages", async () => {
    renderTerms();
    const list = await rows();
    expect(list.map((r) => r.getAttribute("data-list-detail-id"))).toEqual([
      "c1",
      "c2",
      "c3",
      "c4",
    ]);
    const cart = row("c1");
    expect(cart.querySelector(".font-medium")?.textContent).toBe("shopping cart");
    expect(cart.querySelector("[data-slot='term-status']")?.textContent).toBe("Preferred");
    expect(cart.textContent).toMatch(/4$/);
    // The forms a concept discourages stay out of its row.
    expect(cart.textContent).not.toContain("basket");
    expect(cart.textContent).not.toContain("trolley");
  });

  it("opens with nothing selected and says what the pane shows", async () => {
    renderTerms();
    await rows();
    expect(screen.getByText("Choose a concept to see it here")).toBeInTheDocument();
    expect(detail()).toBeNull();
  });

  it("shows the selected concept: definition, what to write instead, terms by language, provenance", async () => {
    renderTerms();
    await rows();
    fireEvent.click(row("c1"));
    await waitFor(() => expect(detail()).not.toBeNull());
    const pane = detail();
    expect(within(pane).getByRole("heading", { level: 2 }).textContent).toContain("shopping cart");
    expect(pane.textContent).toContain("The list of items a shopper has chosen");

    const discouraged = pane.querySelector("[data-slot='terms-discouraged']") as HTMLElement;
    expect(discouraged.textContent).toContain("basket");
    expect(discouraged.textContent).toContain("use shopping cart");
    expect(discouraged.textContent).toContain("trolley");
    expect(discouraged.textContent).toContain("UK usage; the store copy says cart.");

    const languages = pane.querySelector("[data-slot='terms-translations']") as HTMLElement;
    expect(languages.textContent).toContain("Warenkorb");
    expect(languages.textContent).toContain("panier");
    expect(languages.textContent).toContain("handlekurv");
    expect(languages.textContent).toContain("also shopping carts");
    expect(languages.textContent).not.toContain("basket");

    // The list's copy stands in at once; the full record brings the rest.
    const provenance = () => pane.querySelector("[data-slot='terms-provenance']") as HTMLElement;
    expect(provenance().textContent).toContain("Terminology");
    expect(provenance().textContent).toContain("Commerce");
    await waitFor(() => expect(provenance().textContent).toContain("kapimart-store"));
    expect(provenance().querySelector("time")).not.toBeNull();
    expect(provenance().querySelectorAll("[data-slot='coordinate-chip']").length).toBe(2);
  });

  it("moves the selection with the list's keys, and down from the search goes into the list", async () => {
    renderTerms();
    await rows();
    const search = screen.getByRole("searchbox", { name: "Search concepts" });
    fireEvent.keyDown(search, { key: "ArrowDown" });
    expect(row("c1").getAttribute("aria-selected")).toBe("true");
    const list = screen.getByRole("listbox", { name: "Concepts" });
    expect(document.activeElement).toBe(list);
    fireEvent.keyDown(list, { key: "ArrowDown" });
    expect(row("c2").getAttribute("aria-selected")).toBe("true");
    fireEvent.keyDown(list, { key: "End" });
    expect(row("c4").getAttribute("aria-selected")).toBe("true");
    await waitFor(() => expect(detail()?.getAttribute("data-concept")).toBe("c4"));
  });

  it("filters as you type, through the store's own search", async () => {
    const { spies } = renderTerms();
    await rows();
    fireEvent.change(screen.getByRole("searchbox", { name: "Search concepts" }), {
      target: { value: "kasse" },
    });
    await waitFor(() =>
      expect(
        Array.from(document.querySelectorAll("[data-slot='terms-concept']")).map((r) =>
          r.getAttribute("data-list-detail-id"),
        ),
      ).toEqual(["c3"]),
    );
    expect(spies.searchTerms).toHaveBeenLastCalledWith("h1", "kasse", "", "", 0, CONCEPT_PAGE);
    expect(screen.getByText("1 match")).toBeInTheDocument();
  });

  it("says when nothing matches, and keeps the search", async () => {
    renderTerms();
    await rows();
    fireEvent.change(screen.getByRole("searchbox", { name: "Search concepts" }), {
      target: { value: "zzz" },
    });
    await waitFor(() => expect(screen.getByText("No concept matches this search.")).toBeVisible());
    expect(screen.getByRole("searchbox", { name: "Search concepts" })).toHaveValue("zzz");
  });

  it("reads a large store a page at a time", async () => {
    const { spies } = renderTerms(manyConcepts(250));
    await rows();
    expect(document.querySelectorAll("[data-slot='terms-concept']")).toHaveLength(CONCEPT_PAGE);
    fireEvent.click(screen.getByRole("button", { name: `Show ${CONCEPT_PAGE} more` }));
    await waitFor(() =>
      expect(document.querySelectorAll("[data-slot='terms-concept']")).toHaveLength(
        2 * CONCEPT_PAGE,
      ),
    );
    expect(spies.searchTerms).toHaveBeenLastCalledWith(
      "h1",
      "",
      "",
      "",
      CONCEPT_PAGE,
      CONCEPT_PAGE,
    );
    expect(screen.getByRole("button", { name: "Show 50 more" })).toBeInTheDocument();
  });

  it("narrows a concept's languages to the Active Filter's, and says how many it hides", async () => {
    renderTerms(SAMPLE_CONCEPTS, { localeScope: ["de-DE"], defaultSelected: "c1" });
    await waitFor(() => expect(detail()).not.toBeNull());
    const languages = detail().querySelector("[data-slot='terms-translations']") as HTMLElement;
    expect(languages.textContent).toContain("Warenkorb");
    expect(languages.textContent).toContain("shopping cart");
    expect(languages.textContent).not.toContain("panier");
    expect(languages.textContent).toContain("2 more languages are outside the filter");
  });

  it("lists a concept's relations, and opens the related concept", async () => {
    renderTerms(SAMPLE_CONCEPTS, { defaultSelected: "c1" });
    const relation = await waitFor(() => {
      const el = detail()?.querySelector("[data-slot='terms-relation']");
      expect(el).toBeTruthy();
      return el as HTMLElement;
    });
    expect(relation.textContent).toContain("related to");
    await waitFor(() => expect(relation.textContent).toContain("checkout"));
    fireEvent.click(within(relation).getByRole("button", { name: "checkout" }));
    await waitFor(() => expect(detail().getAttribute("data-concept")).toBe("c3"));
    expect(row("c3").getAttribute("aria-selected")).toBe("true");
  });

  it("removes a relation through the store", async () => {
    const { spies } = renderTerms(SAMPLE_CONCEPTS, { defaultSelected: "c1" });
    const remove = await screen.findByRole("button", { name: "Remove relation to checkout" });
    fireEvent.click(remove);
    await waitFor(() => expect(spies.removeRelation).toHaveBeenCalledWith("h1", "r1"));
    await waitFor(() => expect(detail().querySelector("[data-slot='terms-relation']")).toBeNull());
  });

  it("relates the concept to another from the toolbar", async () => {
    const { spies } = renderTerms(SAMPLE_CONCEPTS, { defaultSelected: "c4" });
    await waitFor(() => expect(detail()).not.toBeNull());
    fireEvent.click(detail().querySelector("[data-slot='terms-relate']") as HTMLElement);
    const dialog = await screen.findByRole("dialog");
    const option = await within(dialog).findByRole("option", { name: "KapiMart" });
    fireEvent.click(option);
    await act(async () => {
      fireEvent.click(within(dialog).getByRole("button", { name: /Add relation/ }));
    });
    await waitFor(() =>
      expect(spies.addRelation).toHaveBeenCalledWith(
        "h1",
        expect.objectContaining({ source_id: "c4", target_id: "c2", type: "RELATED" }),
      ),
    );
    await waitFor(() =>
      expect(detail().querySelector("[data-slot='terms-relation']")?.textContent).toContain(
        "KapiMart",
      ),
    );
  });

  it("says a store with nothing in it has no terms yet", async () => {
    renderTerms([]);
    await waitFor(() => expect(screen.getByText("No terms yet")).toBeInTheDocument());
    expect(screen.queryByRole("listbox")).toBeNull();
  });
});

// Interaction surfaces (threaded discussion, comments, observations) belong to
// the platform, which has other people in it. The desktop reads and edits a
// local terms store, which can neither supply nor answer them.
describe("Terms carries no interaction surfaces", () => {
  it("renders no discussion, comments or observations on a concept", async () => {
    renderTerms(SAMPLE_CONCEPTS, { defaultSelected: "c3" });
    await waitFor(() => expect(detail()).not.toBeNull());
    for (const copy of ["Discussion", "Comments", "No discussion yet", "Observations"]) {
      expect(screen.queryByText(copy), copy).not.toBeInTheDocument();
    }
  });
});
