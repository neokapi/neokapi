import { render, screen, waitFor } from "./testUtils";
import userEvent from "@testing-library/user-event";
import { describe, it, expect } from "vitest";
import type { ConceptDataSource } from "@neokapi/concept-ui";

import { ConceptsView } from "../components/ConceptsView";

// Interaction surfaces — threaded discussion, comments, review conversations —
// belong to Bowrain, the platform that actually has other people in it. Kapi
// Desktop reads and edits a LOCAL termbase, so it can neither supply nor answer
// them; wiring the slots anyway rendered a permanently-empty "Discussion" panel
// inviting a conversation the app cannot hold.
//
// The dashboard is shared with Bowrain (@neokapi/concept-ui), so this is done by
// withholding the slots, never by forking the component — Bowrain keeps the full
// set from the same code.

const concept = {
  id: "c1",
  domain: "commerce",
  definition: "The purchase completion flow.",
  terms: [
    { locale: "en-US", text: "Checkout", status: "preferred" as const },
    { locale: "de", text: "Kasse", status: "preferred" as const },
  ],
};

/** A source with the local termbase's capabilities: terms + relations only. */
function localSource(): ConceptDataSource {
  return {
    listConcepts: () => Promise.resolve({ concepts: [concept], total: 1 }),
    getConcept: () => Promise.resolve(concept),
    getRelations: () => Promise.resolve([]),
    // Deliberately absent, exactly as createLocalConceptSource leaves them:
    // getMarkets, getObservations, getComments, getTimeline, getWhereUsed.
  } as unknown as ConceptDataSource;
}

/** Click through the list into the concept dashboard. */
async function openConcept() {
  const rows = await screen.findAllByText("Checkout");
  await userEvent.click(rows[0]);
  // The dashboard is open once a relations/terms section of it has rendered.
  await waitFor(() => expect(screen.getAllByText("Checkout").length).toBeGreaterThan(0));
}

describe("ConceptsView — interaction surfaces belong to the platform", () => {
  it("does not render a discussion/comments section on a concept", async () => {
    render(<ConceptsView handle="h1" source={localSource()} />);

    await openConcept();

    for (const copy of [
      "Discussion",
      "Comments",
      "Comments on this concept.",
      "No discussion yet",
      "Could not load discussion",
    ]) {
      expect(screen.queryByText(copy), copy).not.toBeInTheDocument();
    }
  });

  it("does not render the server-sourced observations section either", async () => {
    render(<ConceptsView handle="h1" source={localSource()} />);
    await openConcept();

    for (const copy of ["Observations", "No observations yet"]) {
      expect(screen.queryByText(copy), copy).not.toBeInTheDocument();
    }
  });

  it("keeps the read/edit views the desktop CAN serve", async () => {
    render(<ConceptsView handle="h1" source={localSource()} />);
    await openConcept();

    // The terms the local termbase holds are still shown — removing the
    // interaction affordances must not thin out the substance.
    await waitFor(() => expect(screen.getAllByText("Kasse").length).toBeGreaterThan(0));
  });
});
