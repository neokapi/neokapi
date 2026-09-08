// @vitest-environment jsdom
import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render } from "@testing-library/react";
import { ConceptList } from "../ConceptList";
import { ConceptView } from "../ConceptView";
import type { ConceptDataSource } from "../adapter";
import type { Concept } from "../types";

afterEach(cleanup);

// Terms listed Arabic first, each preferred in its own locale — the KapiMart
// shape that headed an English-source card with the Arabic term.
const concept: Concept = {
  id: "c1",
  terms: [
    { text: "مستودع", locale: "ar", status: "preferred" },
    { text: "Lager", locale: "de-DE", status: "preferred" },
    { text: "Warehouse", locale: "en-US", status: "preferred" },
    { text: "Entrepôt", locale: "fr-FR", status: "preferred" },
  ],
};

function source(): ConceptDataSource {
  return {
    listConcepts: () => Promise.resolve({ concepts: [concept], total: 1 }),
    getConcept: () => Promise.resolve(concept),
  } as unknown as ConceptDataSource;
}

async function headingOf(element: HTMLElement | null): Promise<string> {
  return element?.textContent?.trim() ?? "";
}

describe("ConceptList naming", () => {
  it("heads a row with the source locale's term", async () => {
    const { findByTestId } = render(
      <ConceptList source={source()} onOpen={() => {}} naming={{ sourceLocale: "en-US" }} />,
    );
    const row = await findByTestId("concept-row");
    expect(await headingOf(row.querySelector("span.font-medium"))).toBe("Warehouse");
  });

  it("heads a row with the viewer's locale when the source locale has no term", async () => {
    const { findByTestId } = render(
      <ConceptList
        source={source()}
        onOpen={() => {}}
        naming={{ sourceLocale: "nb-NO", uiLocale: "fr-FR" }}
      />,
    );
    const row = await findByTestId("concept-row");
    expect(await headingOf(row.querySelector("span.font-medium"))).toBe("Entrepôt");
  });

  it("heads a row in English with no locale hints", async () => {
    const { findByTestId } = render(<ConceptList source={source()} onOpen={() => {}} />);
    const row = await findByTestId("concept-row");
    expect(await headingOf(row.querySelector("span.font-medium"))).toBe("Warehouse");
  });
});

describe("ConceptView naming", () => {
  it("heads the concept with the source locale's term", async () => {
    const { findByTestId } = render(
      <ConceptView
        conceptId="c1"
        source={source()}
        onNavigate={() => {}}
        naming={{ sourceLocale: "de-DE" }}
      />,
    );
    const header = await findByTestId("concept-header");
    expect(header.textContent).toContain("Lager");
  });
});
