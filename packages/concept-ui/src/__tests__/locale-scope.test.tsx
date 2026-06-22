// @vitest-environment jsdom
import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render } from "@testing-library/react";
import { ConceptList } from "../ConceptList";
import type { ConceptDataSource } from "../adapter";
import type { Concept } from "../types";

afterEach(cleanup);

const concept: Concept = {
  id: "c1",
  terms: [
    { text: "Checkout", locale: "en-US", status: "preferred" },
    { text: "Kasse", locale: "de-DE", status: "preferred" },
    { text: "Caisse", locale: "fr-FR", status: "preferred" },
  ],
};

function source(): ConceptDataSource {
  return {
    listConcepts: () => Promise.resolve({ concepts: [concept], total: 1 }),
    getConcept: () => Promise.resolve(concept),
  } as unknown as ConceptDataSource;
}

describe("ConceptList localeScope", () => {
  it("shows only the scoped locales' term chips", async () => {
    const { findByText, queryByText } = render(
      <ConceptList source={source()} onOpen={() => {}} localeScope={["en-US", "de-DE"]} />,
    );
    expect(await findByText("Kasse")).toBeTruthy(); // de-DE in scope
    expect(queryByText("Caisse")).toBeNull(); // fr-FR out of scope
  });

  it("shows every locale when unscoped", async () => {
    const { findByText } = render(<ConceptList source={source()} onOpen={() => {}} />);
    expect(await findByText("Caisse")).toBeTruthy();
  });
});
