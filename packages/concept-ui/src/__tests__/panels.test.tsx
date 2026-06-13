// @vitest-environment jsdom
import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render } from "@testing-library/react";
import { ConceptTimeline } from "../ConceptTimeline";
import { ConstraintsPanel } from "../ConstraintsPanel";
import { resolveCapabilities } from "../adapter";
import type { ConceptDataSource } from "../adapter";
import type { ConceptSectionProps } from "../ConceptView";
import type { Concept } from "../types";
import { makePanelSource } from "../stories/panel-fixtures";

afterEach(cleanup);

function ctx(source: ConceptDataSource, id: string): ConceptSectionProps {
  const concept = source.getConcept(id) as Concept; // memory source resolves sync
  return { concept, source, capabilities: resolveCapabilities(source), onNavigate: () => {} };
}

/** The rich source, but with getRelations rejecting — a failed core read. */
function withFailingRelations(): ConceptDataSource {
  return { ...makePanelSource(), getRelations: () => Promise.reject(new Error("network down")) };
}

describe("ConceptTimeline", () => {
  it("renders the platform revision log grouped with kinds", async () => {
    const props = ctx(makePanelSource(), "checkout");
    const { findByText, queryByText } = render(<ConceptTimeline {...props} />);
    expect(await findByText("Created concept")).toBeTruthy();
    expect(await findByText("Shipped voice refresh v4")).toBeTruthy();
    // Direction toggle shows when there is more than one event.
    expect(await findByText("Newest")).toBeTruthy();
    // Rich source → no degradation caption.
    expect(queryByText(/Derived from the local termbase/)).toBeNull();
  });

  it("degrades to a synthesised core timeline with a caption", async () => {
    const props = ctx(makePanelSource({ rich: false }), "checkout");
    const { findByText } = render(<ConceptTimeline {...props} />);
    expect(await findByText("Concept created")).toBeTruthy();
    expect(await findByText(/Derived from the local termbase/)).toBeTruthy();
  });

  it("shows an empty state for an undated concept", async () => {
    const props = ctx(makePanelSource({ rich: false }), "wishlist");
    const { findByText } = render(<ConceptTimeline {...props} />);
    expect(await findByText("No history yet")).toBeTruthy();
  });

  it("surfaces a failed fetch instead of an empty history", async () => {
    const props = ctx(withFailingRelations(), "checkout");
    const { findByText, queryByText } = render(<ConceptTimeline {...props} />);
    expect(await findByText("Could not load timeline")).toBeTruthy();
    expect(queryByText("No history yet")).toBeNull();
  });
});

describe("ConstraintsPanel", () => {
  it("renders the time chart plus the banned/preferred summary", async () => {
    const props = ctx(makePanelSource(), "checkout");
    const { findByText, findAllByText } = render(<ConstraintsPanel {...props} />);
    expect(await findByText("Banned")).toBeTruthy();
    // "Preferred" appears as the column header and on status chips.
    expect((await findAllByText("Preferred")).length).toBeGreaterThan(0);
    expect(await findByText("Voucher")).toBeTruthy(); // a banned term
    expect((await findAllByText(/Today,/)).length).toBeGreaterThan(0); // now marker caption
  });

  it("falls back to the summary alone when nothing is dated", async () => {
    const props = ctx(makePanelSource({ rich: false }), "wishlist");
    const { findByText, queryByText } = render(<ConstraintsPanel {...props} />);
    expect(await findByText("Saved items")).toBeTruthy(); // forbidden → banned
    expect(await findByText("Banned")).toBeTruthy();
    // No dated windows → no "today" marker chart.
    expect(queryByText(/Today,/)).toBeNull();
  });

  it("surfaces a failed fetch instead of reading as no constraints", async () => {
    const props = ctx(withFailingRelations(), "checkout");
    const { findByText, queryByText } = render(<ConstraintsPanel {...props} />);
    expect(await findByText("Could not load constraints")).toBeTruthy();
    expect(queryByText("No constraints")).toBeNull();
  });
});
