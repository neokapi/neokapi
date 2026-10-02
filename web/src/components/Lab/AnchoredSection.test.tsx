import React from "react";
import { renderToString } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

const collectAnchor = vi.fn();
vi.mock("@docusaurus/useBrokenLinks", () => ({
  default: () => ({ collectAnchor, collectLink: vi.fn() }),
}));

const { AnchoredSection } = await import("./AnchoredSection");

describe("AnchoredSection", () => {
  it("registers the id it renders", () => {
    const html = renderToString(
      <AnchoredSection id="electives" className="stage">
        <h2>Electives</h2>
      </AnchoredSection>,
    );
    expect(html).toContain('id="electives"');
    expect(html).toContain('class="stage"');
    expect(collectAnchor).toHaveBeenCalledWith("electives");
  });
});
