// @vitest-environment jsdom
import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import ConversionExplorer, { DOCUMENT_FAMILIES, DOCUMENT_TARGETS } from "./ConversionExplorer";

afterEach(cleanup);

describe("DOCUMENT_TARGETS", () => {
  it("offers the document writers the engine generates", () => {
    const ids = DOCUMENT_TARGETS.map((t) => t.id);
    for (const id of ["doclang", "markdown", "html", "asciidoc", "plaintext"]) {
      expect(ids).toContain(id);
    }
  });

  it("offers no catalog, skeleton-bound, media or interchange format", () => {
    const ids = DOCUMENT_TARGETS.map((t) => t.id);
    // A Word document is not a string catalog.
    for (const id of ["json", "yaml", "properties", "androidxml", "applestrings", "resx"]) {
      expect(ids).not.toContain(id);
    }
    // Skeleton-driven / binary writers need the original file.
    for (const id of ["openxml", "odf", "idml", "epub", "csv", "image", "audio", "video"]) {
      expect(ids).not.toContain(id);
    }
    // Bilingual interchange belongs to the extract/merge loop, not convert.
    for (const id of ["xliff", "xliff2", "po", "tmx", "kbf"]) {
      expect(ids).not.toContain(id);
    }
  });

  it("names the document families the engine declares", () => {
    expect([...DOCUMENT_FAMILIES].sort()).toEqual(["plain-text", "rich-markup"]);
  });

  it("gives every target an output extension", () => {
    for (const t of DOCUMENT_TARGETS) {
      expect(t.ext.length).toBeGreaterThan(0);
      expect(t.label.length).toBeGreaterThan(0);
    }
  });
});

describe("ConversionExplorer", () => {
  it("gates the lab behind an explicit Run action", () => {
    render(<ConversionExplorer assets={null} />);
    // The body lays out behind the zero-shift gate overlay; the labeled
    // primary action ("Run in your browser") is the gate.
    expect(screen.getByRole("button", { name: /run in your browser/i })).toBeTruthy();
  });

  it("renders the input picker behind the gate, without booting WASM", () => {
    // assets=null → the engine never boots, so there is no parsed input yet and
    // the DocumentViewer (with its output-format pills) only appears post-Run.
    // The input picker still lays out behind the zero-shift gate overlay.
    render(<ConversionExplorer assets={null} />);
    expect(screen.getByText("Input")).toBeTruthy();
  });

  it("selects a served sample by name once it loads", async () => {
    const bytes = new TextEncoder().encode("# Served\n");
    const originalFetch = globalThis.fetch;
    globalThis.fetch = (async () =>
      new Response(bytes, { status: 200 })) as unknown as typeof globalThis.fetch;
    try {
      render(
        <ConversionExplorer
          assets={null}
          samples={[{ url: "/samples/handbook.md", name: "handbook.md" }]}
          defaultSampleId="handbook.md"
        />,
      );
      // The selector shows the requested file once the fetch has landed.
      expect(await screen.findByText("handbook.md")).toBeTruthy();
    } finally {
      globalThis.fetch = originalFetch;
    }
  });
});
