// @vitest-environment jsdom
import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import ConversionExplorer, { GENERATIVE_TARGETS } from "./ConversionExplorer";

afterEach(cleanup);

describe("GENERATIVE_TARGETS", () => {
  it("offers generative writers and excludes skeleton-driven formats", () => {
    const ids = GENERATIVE_TARGETS.map((t) => t.id);
    // Generative document/interchange writers a foreign model can produce.
    for (const id of ["doclang", "markdown", "html", "xliff", "po", "json"]) {
      expect(ids).toContain(id);
    }
    // Skeleton-driven / binary writers need the original file and must NOT be
    // offered as conversion targets (they would emit empty/broken output).
    for (const id of ["openxml", "odf", "idml", "epub", "csv", "image"]) {
      expect(ids).not.toContain(id);
    }
  });

  it("gives every target an output extension", () => {
    for (const t of GENERATIVE_TARGETS) {
      expect(t.ext.length).toBeGreaterThan(0);
      expect(t.label.length).toBeGreaterThan(0);
    }
  });
});

describe("ConversionExplorer", () => {
  it("renders the output-format selector with the generative targets", () => {
    // assets=null defers WASM booting, so this is a pure render smoke test.
    render(<ConversionExplorer assets={null} />);
    expect(screen.getByText("Convert to")).toBeTruthy();
    expect(screen.getByRole("option", { name: "DocLang" })).toBeTruthy();
    expect(screen.getByRole("option", { name: "Markdown" })).toBeTruthy();
  });
});
